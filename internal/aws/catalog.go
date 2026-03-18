package aws

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/arcoloom/arco-provider-aws/internal/provider"
)

const (
	instanceMetadataURL = "https://raw.githubusercontent.com/arcoloom/arco-catalog/refs/heads/main/data/aws/instance_metadata.json"
	instanceRegionsURL  = "https://raw.githubusercontent.com/arcoloom/arco-catalog/refs/heads/main/data/aws/instance_regions.json"
	seriesModelsURL     = "https://raw.githubusercontent.com/arcoloom/arco-catalog/refs/heads/main/data/aws/series_models.json"

	defaultCatalogCacheTTL   = 7 * 24 * time.Hour
	defaultOperatingSystem   = "Linux"
	defaultTenancy           = "Shared"
	defaultPreinstalledSoft  = "NA"
	defaultLicenseModel      = "No License required"
	defaultCurrency          = "USD"
	defaultMemoryUnitDivisor = 1.0
)

type remoteFetcher interface {
	Fetch(context.Context, string) ([]byte, error)
}

type httpRemoteFetcher struct {
	client *http.Client
}

func (f httpRemoteFetcher) Fetch(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request for %s: %w", url, err)
	}

	resp, err := f.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch %s: unexpected status %d", url, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", url, err)
	}

	return body, nil
}

type catalogRepository struct {
	baseDir string
	fetcher remoteFetcher
	now     func() time.Time

	mu          sync.Mutex
	cached      *catalogSnapshot
	cachedAt    time.Time
	cacheWindow time.Duration
}

type catalogSnapshot struct {
	metadataByType map[string]catalogInstanceMetadataRecord
	regionsByType  map[string][]provider.Region
	pricesByKey    map[string]catalogOnDemandPriceRecord
	seriesToTypes  map[string][]string
}

type catalogInstanceMetadataRecord struct {
	Series                string          `json:"series"`
	InstanceType          string          `json:"instance_type"`
	Family                string          `json:"family"`
	PrettyName            string          `json:"pretty_name"`
	Generation            string          `json:"generation"`
	VCPU                  int32           `json:"vCPU"`
	Memory                float64         `json:"memory"`
	ClockSpeedGHz         string          `json:"clock_speed_ghz"`
	PhysicalProcessor     string          `json:"physical_processor"`
	Arch                  []string        `json:"arch"`
	NetworkPerformance    string          `json:"network_performance"`
	EnhancedNetworking    bool            `json:"enhanced_networking"`
	VPCOnly               bool            `json:"vpc_only"`
	IPv6Support           bool            `json:"ipv6_support"`
	PlacementGroupSupport bool            `json:"placement_group_support"`
	EBSOptimized          bool            `json:"ebs_optimized"`
	SupportOS             []string        `json:"support_os"`
	GPU                   float64         `json:"GPU"`
	GPUModel              string          `json:"GPU_model"`
	GPUMemory             float64         `json:"GPU_memory"`
	FPGA                  int32           `json:"FPGA"`
	Storage               json.RawMessage `json:"storage"`
	Raw                   map[string]any  `json:"-"`
}

func (r *catalogInstanceMetadataRecord) UnmarshalJSON(data []byte) error {
	type alias catalogInstanceMetadataRecord
	var decoded alias
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	*r = catalogInstanceMetadataRecord(decoded)
	r.Raw = raw
	return nil
}

type catalogRegionRecord struct {
	Series        string `json:"series"`
	InstanceType  string `json:"instance_type"`
	RegionCode    string `json:"region_code"`
	RegionName    string `json:"region_name"`
	OnDemandPrice string `json:"on_demand_price"`
}

type catalogOnDemandPriceRecord struct {
	InstanceType string
	Region       provider.Region
	Price        string
}

type catalogSeriesModelsRecord struct {
	Series        string   `json:"series"`
	InstanceCount int32    `json:"instance_count"`
	InstanceTypes []string `json:"instance_types"`
}

func newCatalogRepository() *catalogRepository {
	baseDir := filepath.Join(os.TempDir(), "arcoloom", "instances", "aws")
	if homeDir, err := os.UserHomeDir(); err == nil && strings.TrimSpace(homeDir) != "" {
		baseDir = filepath.Join(homeDir, ".arcoloom", "instances", "aws")
	}

	return &catalogRepository{
		baseDir: baseDir,
		fetcher: httpRemoteFetcher{
			client: &http.Client{Timeout: 30 * time.Second},
		},
		now:         time.Now,
		cacheWindow: 5 * time.Minute,
	}
}

func (r *catalogRepository) loadCatalog(ctx context.Context) (catalogSnapshot, error) {
	r.mu.Lock()
	if r.cached != nil && r.now().Sub(r.cachedAt) < r.cacheWindow {
		snapshot := *r.cached
		r.mu.Unlock()
		return snapshot, nil
	}
	r.mu.Unlock()

	var metadata []catalogInstanceMetadataRecord
	if err := r.loadCachedJSON(ctx, "catalog/instance_metadata.json", instanceMetadataURL, defaultCatalogCacheTTL, &metadata); err != nil {
		return catalogSnapshot{}, err
	}

	var regions []catalogRegionRecord
	if err := r.loadCachedJSON(ctx, "catalog/instance_regions.json", instanceRegionsURL, defaultCatalogCacheTTL, &regions); err != nil {
		return catalogSnapshot{}, err
	}

	var seriesModels []catalogSeriesModelsRecord
	if err := r.loadCachedJSON(ctx, "catalog/series_models.json", seriesModelsURL, defaultCatalogCacheTTL, &seriesModels); err != nil {
		return catalogSnapshot{}, err
	}

	snapshot := buildCatalogSnapshot(metadata, regions, seriesModels)

	r.mu.Lock()
	r.cached = &snapshot
	r.cachedAt = r.now()
	r.mu.Unlock()

	return snapshot, nil
}

func buildCatalogSnapshot(
	metadata []catalogInstanceMetadataRecord,
	regions []catalogRegionRecord,
	seriesModels []catalogSeriesModelsRecord,
) catalogSnapshot {
	metadataByType := make(map[string]catalogInstanceMetadataRecord, len(metadata))
	for _, item := range metadata {
		instanceType := strings.TrimSpace(item.InstanceType)
		if instanceType == "" {
			continue
		}
		item.InstanceType = instanceType
		item.Series = strings.TrimSpace(item.Series)
		metadataByType[instanceType] = item
	}

	regionsByType := make(map[string][]provider.Region)
	pricesByKey := make(map[string]catalogOnDemandPriceRecord)
	regionSeen := make(map[string]map[string]struct{})
	for _, item := range regions {
		instanceType := strings.TrimSpace(item.InstanceType)
		regionCode := strings.TrimSpace(item.RegionCode)
		if instanceType == "" || regionCode == "" {
			continue
		}
		regionName := strings.TrimSpace(item.RegionName)
		if _, ok := regionSeen[instanceType]; !ok {
			regionSeen[instanceType] = make(map[string]struct{})
		}
		if _, ok := regionSeen[instanceType][regionCode]; ok {
			if price := strings.TrimSpace(item.OnDemandPrice); price != "" {
				pricesByKey[priceKey(instanceType, regionCode)] = catalogOnDemandPriceRecord{
					InstanceType: instanceType,
					Region: provider.Region{
						Code: regionCode,
						Name: regionName,
					},
					Price: price,
				}
			}
			continue
		}
		regionSeen[instanceType][regionCode] = struct{}{}
		regionsByType[instanceType] = append(regionsByType[instanceType], provider.Region{
			Code: regionCode,
			Name: regionName,
		})
		if price := strings.TrimSpace(item.OnDemandPrice); price != "" {
			pricesByKey[priceKey(instanceType, regionCode)] = catalogOnDemandPriceRecord{
				InstanceType: instanceType,
				Region: provider.Region{
					Code: regionCode,
					Name: regionName,
				},
				Price: price,
			}
		}
	}
	for instanceType := range regionsByType {
		slices.SortFunc(regionsByType[instanceType], func(a, b provider.Region) int {
			return strings.Compare(a.Code, b.Code)
		})
	}

	seriesToTypes := make(map[string][]string)
	for _, item := range seriesModels {
		series := strings.TrimSpace(item.Series)
		if series == "" {
			continue
		}
		types := append([]string(nil), item.InstanceTypes...)
		slices.Sort(types)
		seriesToTypes[series] = types
	}
	for instanceType, item := range metadataByType {
		if item.Series == "" {
			continue
		}
		if _, ok := seriesToTypes[item.Series]; !ok {
			seriesToTypes[item.Series] = []string{instanceType}
			continue
		}
		if !slices.Contains(seriesToTypes[item.Series], instanceType) {
			seriesToTypes[item.Series] = append(seriesToTypes[item.Series], instanceType)
			slices.Sort(seriesToTypes[item.Series])
		}
	}

	return catalogSnapshot{
		metadataByType: metadataByType,
		regionsByType:  regionsByType,
		pricesByKey:    pricesByKey,
		seriesToTypes:  seriesToTypes,
	}
}

func (r *catalogRepository) loadCachedJSON(ctx context.Context, relativePath string, sourceURL string, ttl time.Duration, target any) error {
	body, err := r.readThroughCache(ctx, relativePath, sourceURL, ttl)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(body, target); err != nil {
		return fmt.Errorf("decode %s: %w", relativePath, err)
	}

	return nil
}

func (r *catalogRepository) readThroughCache(ctx context.Context, relativePath string, sourceURL string, ttl time.Duration) ([]byte, error) {
	cachePath := filepath.Join(r.baseDir, relativePath)
	if info, err := os.Stat(cachePath); err == nil {
		if r.now().Sub(info.ModTime()) <= ttl {
			return os.ReadFile(cachePath)
		}
	}

	body, fetchErr := r.fetcher.Fetch(ctx, sourceURL)
	if fetchErr == nil {
		if err := writeCacheFile(cachePath, body); err != nil {
			return nil, err
		}
		return body, nil
	}

	if _, err := os.Stat(cachePath); err == nil {
		return os.ReadFile(cachePath)
	}

	return nil, fetchErr
}

func writeCacheFile(path string, body []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create cache dir for %s: %w", path, err)
	}

	tempFile, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return fmt.Errorf("create temp cache file for %s: %w", path, err)
	}
	tempName := tempFile.Name()
	defer os.Remove(tempName)

	if _, err := tempFile.Write(body); err != nil {
		_ = tempFile.Close()
		return fmt.Errorf("write temp cache file for %s: %w", path, err)
	}
	if err := tempFile.Close(); err != nil {
		return fmt.Errorf("close temp cache file for %s: %w", path, err)
	}
	if err := os.Rename(tempName, path); err != nil {
		return fmt.Errorf("move cache file into place for %s: %w", path, err)
	}

	return nil
}

func (s *Service) ListInstanceTypes(ctx context.Context, req provider.ListInstanceTypesRequest) (provider.ListInstanceTypesResult, error) {
	snapshot, err := s.catalog.loadCatalog(ctx)
	if err != nil {
		return provider.ListInstanceTypesResult{}, err
	}

	regionFilter := effectiveCatalogRegion(req.Region)
	generationFilter := normalizeToken(req.Generation)
	seriesFilter := buildNormalizedSet(req.Series)
	typeFilter := buildNormalizedSet(req.InstanceTypes)
	archFilter := buildNormalizedSet(req.Architectures)

	items := make([]provider.InstanceTypeSummary, 0)
	for _, instanceType := range sortedInstanceTypes(snapshot.metadataByType) {
		record := snapshot.metadataByType[instanceType]
		regions := snapshot.regionsByType[instanceType]
		if !matchesInstanceFilters(record, regions, regionFilter, generationFilter, seriesFilter, typeFilter, archFilter) {
			continue
		}
		items = append(items, buildInstanceTypeSummary(record, regions))
	}

	return provider.ListInstanceTypesResult{
		Items:    items,
		Warnings: buildMissingTypeWarnings(req.InstanceTypes, snapshot.metadataByType),
	}, nil
}

func (s *Service) GetInstanceTypeInfo(ctx context.Context, req provider.GetInstanceTypeInfoRequest) (provider.GetInstanceTypeInfoResult, error) {
	snapshot, err := s.catalog.loadCatalog(ctx)
	if err != nil {
		return provider.GetInstanceTypeInfoResult{}, err
	}

	regionFilter := effectiveCatalogRegion(req.Region)
	seriesFilter := buildNormalizedSet(req.Series)
	typeFilter := buildNormalizedSet(req.InstanceTypes)

	items := make([]provider.InstanceTypeInfo, 0)
	for _, instanceType := range sortedInstanceTypes(snapshot.metadataByType) {
		record := snapshot.metadataByType[instanceType]
		regions := snapshot.regionsByType[instanceType]
		if len(seriesFilter) > 0 {
			if _, ok := seriesFilter[normalizeToken(record.Series)]; !ok {
				continue
			}
		}
		if len(typeFilter) > 0 {
			if _, ok := typeFilter[normalizeToken(instanceType)]; !ok {
				continue
			}
		}
		if regionFilter != "" && !supportsRegion(regions, regionFilter) {
			continue
		}
		items = append(items, buildInstanceTypeInfo(record, regions))
	}

	return provider.GetInstanceTypeInfoResult{
		Items:    items,
		Warnings: buildMissingTypeWarnings(req.InstanceTypes, snapshot.metadataByType),
	}, nil
}

func (s *Service) GetInstancePrices(ctx context.Context, req provider.GetInstancePricesRequest) (provider.GetInstancePricesResult, error) {
	region := effectiveCatalogRegion(req.Region)
	if region == "" {
		return provider.GetInstancePricesResult{}, errors.New("region is required for price queries")
	}

	purchaseOption := req.PurchaseOption
	if purchaseOption == "" {
		purchaseOption = provider.PurchaseOptionOnDemand
	}
	if purchaseOption != provider.PurchaseOptionOnDemand {
		return provider.GetInstancePricesResult{}, fmt.Errorf("purchase option %q is not supported yet", purchaseOption)
	}

	operatingSystem := normalizePriceOperatingSystem(req.OperatingSystem)
	tenancy := normalizePriceTenancy(req.Tenancy)
	preinstalledSoftware := normalizePricePreinstalledSoftware(req.PreinstalledSoftware)
	licenseModel := normalizePriceLicenseModel(req.LicenseModel)
	currency := normalizePriceCurrency(req.Currency)
	if currency != defaultCurrency {
		return provider.GetInstancePricesResult{
			Warnings: buildMissingPriceWarnings(req.InstanceTypes, nil),
		}, nil
	}

	snapshot, err := s.catalog.loadCatalog(ctx)
	if err != nil {
		return provider.GetInstancePricesResult{}, err
	}

	items := make([]provider.InstancePrice, 0)
	requestedTypes := buildNormalizedSet(req.InstanceTypes)
	if !supportsCatalogOnDemandPriceFilters(operatingSystem, tenancy, preinstalledSoftware, licenseModel) {
		return provider.GetInstancePricesResult{
			Items:    items,
			Warnings: buildMissingPriceWarnings(req.InstanceTypes, items),
		}, nil
	}

	for _, instanceType := range sortedInstanceTypes(snapshot.metadataByType) {
		if len(requestedTypes) > 0 {
			if _, ok := requestedTypes[normalizeToken(instanceType)]; !ok {
				continue
			}
		}
		price, ok := snapshot.pricesByKey[priceKey(instanceType, region)]
		if !ok {
			continue
		}
		if !supportsRegion(snapshot.regionsByType[instanceType], region) {
			continue
		}
		items = append(items, provider.InstancePrice{
			InstanceType:         instanceType,
			Region:               price.Region,
			PurchaseOption:       provider.PurchaseOptionOnDemand,
			OperatingSystem:      operatingSystem,
			Tenancy:              tenancy,
			PreinstalledSoftware: preinstalledSoftware,
			LicenseModel:         licenseModel,
			BillingUnit:          "Hrs",
			Currency:             currency,
			Price:                price.Price,
			Description:          fmt.Sprintf("%s on-demand hourly price from instance_regions.json", instanceType),
		})
	}

	slices.SortFunc(items, func(a, b provider.InstancePrice) int {
		if cmp := strings.Compare(a.InstanceType, b.InstanceType); cmp != 0 {
			return cmp
		}
		if cmp := strings.Compare(a.Price, b.Price); cmp != 0 {
			return cmp
		}
		return strings.Compare(a.SKU, b.SKU)
	})

	return provider.GetInstancePricesResult{
		Items:    items,
		Warnings: buildMissingPriceWarnings(req.InstanceTypes, items),
	}, nil
}

func sortedInstanceTypes(items map[string]catalogInstanceMetadataRecord) []string {
	keys := make([]string, 0, len(items))
	for key := range items {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}

func matchesInstanceFilters(
	record catalogInstanceMetadataRecord,
	regions []provider.Region,
	regionFilter string,
	generationFilter string,
	seriesFilter map[string]struct{},
	typeFilter map[string]struct{},
	archFilter map[string]struct{},
) bool {
	if len(typeFilter) > 0 {
		if _, ok := typeFilter[normalizeToken(record.InstanceType)]; !ok {
			return false
		}
	}
	if len(seriesFilter) > 0 {
		if _, ok := seriesFilter[normalizeToken(record.Series)]; !ok {
			return false
		}
	}
	if generationFilter != "" && normalizeToken(record.Generation) != generationFilter {
		return false
	}
	if regionFilter != "" && !supportsRegion(regions, regionFilter) {
		return false
	}
	if len(archFilter) > 0 && !matchesArchitecture(record.Arch, archFilter) {
		return false
	}

	return true
}

func buildInstanceTypeSummary(record catalogInstanceMetadataRecord, regions []provider.Region) provider.InstanceTypeSummary {
	return provider.InstanceTypeSummary{
		InstanceType:         record.InstanceType,
		Series:               record.Series,
		Family:               record.Family,
		Category:             normalizeFamilyCategory(record.Family),
		DisplayName:          record.PrettyName,
		Generation:           record.Generation,
		VCPU:                 record.VCPU,
		MemoryGiB:            record.Memory / defaultMemoryUnitDivisor,
		Architectures:        append([]string(nil), record.Arch...),
		SupportedRegionCount: int32(len(regions)),
	}
}

func buildInstanceTypeInfo(record catalogInstanceMetadataRecord, regions []provider.Region) provider.InstanceTypeInfo {
	info := provider.InstanceTypeInfo{
		InstanceType:              record.InstanceType,
		Series:                    record.Series,
		Family:                    record.Family,
		Category:                  normalizeFamilyCategory(record.Family),
		DisplayName:               record.PrettyName,
		Generation:                record.Generation,
		VCPU:                      record.VCPU,
		MemoryGiB:                 record.Memory / defaultMemoryUnitDivisor,
		Architectures:             append([]string(nil), record.Arch...),
		CPUManufacturer:           deriveCPUManufacturer(record.PhysicalProcessor),
		CPUModel:                  record.PhysicalProcessor,
		CPUClockSpeedGHz:          record.ClockSpeedGHz,
		NetworkPerformance:        record.NetworkPerformance,
		EnhancedNetworking:        record.EnhancedNetworking,
		IPv6Supported:             record.IPv6Support,
		PlacementGroupSupported:   record.PlacementGroupSupport,
		VPCOnly:                   record.VPCOnly,
		EBSOptimized:              record.EBSOptimized,
		SupportedRegions:          append([]provider.Region(nil), regions...),
		SupportedOperatingSystems: append([]string(nil), record.SupportOS...),
		Attributes:                buildProviderAttributes(record.Raw),
	}

	if accelerator := buildGPUAccelerator(record); accelerator != nil {
		info.Accelerators = append(info.Accelerators, *accelerator)
	}
	if accelerator := buildFPGAAccelerator(record); accelerator != nil {
		info.Accelerators = append(info.Accelerators, *accelerator)
	}
	if storage := buildLocalStorage(record.Raw["storage"]); storage != nil {
		info.LocalStorage = storage
	}

	return info
}

func buildGPUAccelerator(record catalogInstanceMetadataRecord) *provider.Accelerator {
	if record.GPU <= 0 {
		return nil
	}

	return &provider.Accelerator{
		Kind:      provider.AcceleratorKindGPU,
		Model:     record.GPUModel,
		Count:     record.GPU,
		MemoryGiB: record.GPUMemory,
	}
}

func buildFPGAAccelerator(record catalogInstanceMetadataRecord) *provider.Accelerator {
	if record.FPGA <= 0 {
		return nil
	}

	return &provider.Accelerator{
		Kind:  provider.AcceleratorKindFPGA,
		Count: float64(record.FPGA),
	}
}

func buildLocalStorage(raw any) *provider.LocalStorage {
	if raw == nil {
		return nil
	}

	storage := &provider.LocalStorage{HasLocalStorage: true}
	object, ok := raw.(map[string]any)
	if !ok {
		return storage
	}

	if medium, ok := stringFromAny(object["medium"]); ok {
		storage.Medium = medium
	} else if medium, ok := stringFromAny(object["type"]); ok {
		storage.Medium = medium
	}
	if count, ok := floatFromAny(object["devices"]); ok {
		storage.DiskCount = int32(count)
	} else if count, ok := floatFromAny(object["count"]); ok {
		storage.DiskCount = int32(count)
	}
	if size, ok := floatFromAny(object["total_size"]); ok {
		storage.TotalSizeGiB = size
	} else if size, ok := floatFromAny(object["size"]); ok {
		storage.TotalSizeGiB = size
	} else if size, ok := floatFromAny(object["size_gb"]); ok {
		storage.TotalSizeGiB = size
	}

	return storage
}

func buildProviderAttributes(raw map[string]any) map[string]string {
	if len(raw) == 0 {
		return nil
	}

	ignored := map[string]struct{}{
		"series":                  {},
		"instance_type":           {},
		"family":                  {},
		"pretty_name":             {},
		"generation":              {},
		"vCPU":                    {},
		"memory":                  {},
		"clock_speed_ghz":         {},
		"physical_processor":      {},
		"arch":                    {},
		"network_performance":     {},
		"enhanced_networking":     {},
		"vpc_only":                {},
		"ipv6_support":            {},
		"placement_group_support": {},
		"ebs_optimized":           {},
		"support_os":              {},
		"GPU":                     {},
		"GPU_model":               {},
		"GPU_memory":              {},
		"FPGA":                    {},
		"storage":                 {},
	}

	attributes := make(map[string]string)
	for key, value := range raw {
		if _, ok := ignored[key]; ok {
			continue
		}
		attributes[key] = stringifyAttribute(value)
	}

	if len(attributes) == 0 {
		return nil
	}

	return attributes
}

func stringifyAttribute(value any) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return v
	case bool:
		if v {
			return "true"
		}
		return "false"
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	default:
		encoded, err := json.Marshal(v)
		if err != nil {
			return fmt.Sprint(v)
		}
		return string(encoded)
	}
}

func supportsCatalogOnDemandPriceFilters(operatingSystem, tenancy, preinstalledSoftware, licenseModel string) bool {
	return operatingSystem == defaultOperatingSystem &&
		tenancy == defaultTenancy &&
		preinstalledSoftware == defaultPreinstalledSoft &&
		licenseModel == defaultLicenseModel
}

func priceKey(instanceType string, region string) string {
	return normalizeToken(instanceType) + "@" + normalizeToken(region)
}

func buildMissingTypeWarnings(requested []string, metadataByType map[string]catalogInstanceMetadataRecord) []provider.Warning {
	if len(requested) == 0 {
		return nil
	}

	warnings := make([]provider.Warning, 0)
	for _, instanceType := range requested {
		normalized := normalizeToken(instanceType)
		if normalized == "" {
			continue
		}
		found := false
		for key := range metadataByType {
			if normalizeToken(key) == normalized {
				found = true
				break
			}
		}
		if found {
			continue
		}
		warnings = append(warnings, provider.Warning{
			Code:    "INSTANCE_TYPE_NOT_FOUND",
			Message: fmt.Sprintf("instance type %s was not found in catalog", instanceType),
		})
	}

	return warnings
}

func buildMissingPriceWarnings(requested []string, items []provider.InstancePrice) []provider.Warning {
	if len(requested) == 0 {
		return nil
	}

	found := make(map[string]struct{}, len(items))
	for _, item := range items {
		found[normalizeToken(item.InstanceType)] = struct{}{}
	}

	warnings := make([]provider.Warning, 0)
	for _, instanceType := range requested {
		if _, ok := found[normalizeToken(instanceType)]; ok {
			continue
		}
		warnings = append(warnings, provider.Warning{
			Code:    "PRICE_NOT_FOUND",
			Message: fmt.Sprintf("on-demand price for instance type %s was not found", instanceType),
		})
	}

	return warnings
}

func effectiveCatalogRegion(requestedRegion string) string {
	if region := strings.TrimSpace(requestedRegion); region != "" && !isAllSelector(region) {
		return region
	}
	return ""
}

func buildNormalizedSet(values []string) map[string]struct{} {
	if len(values) == 0 {
		return nil
	}

	result := make(map[string]struct{}, len(values))
	for _, value := range values {
		if normalized := normalizeToken(value); normalized != "" {
			result[normalized] = struct{}{}
		}
	}

	if len(result) == 0 {
		return nil
	}

	return result
}

func normalizeToken(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func matchesArchitecture(architectures []string, filter map[string]struct{}) bool {
	for _, architecture := range architectures {
		if _, ok := filter[normalizeToken(architecture)]; ok {
			return true
		}
	}
	return false
}

func supportsRegion(regions []provider.Region, region string) bool {
	for _, item := range regions {
		if item.Code == region {
			return true
		}
	}
	return false
}

func findRegionName(regions []provider.Region, region string) string {
	for _, item := range regions {
		if item.Code == region {
			return item.Name
		}
	}
	return ""
}

func normalizeFamilyCategory(family string) string {
	family = strings.ToLower(strings.TrimSpace(family))
	if family == "" {
		return ""
	}

	var builder strings.Builder
	lastDash := false
	for _, r := range family {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			builder.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			builder.WriteByte('-')
			lastDash = true
		}
	}

	return strings.Trim(builder.String(), "-")
}

func deriveCPUManufacturer(processor string) string {
	lower := strings.ToLower(processor)
	switch {
	case strings.Contains(lower, "amd"):
		return "AMD"
	case strings.Contains(lower, "intel"):
		return "Intel"
	case strings.Contains(lower, "graviton"), strings.Contains(lower, "aws"):
		return "AWS"
	default:
		return ""
	}
}

func stringFromAny(value any) (string, bool) {
	text, ok := value.(string)
	return text, ok
}

func floatFromAny(value any) (float64, bool) {
	switch v := value.(type) {
	case float64:
		return v, true
	case float32:
		return float64(v), true
	case int:
		return float64(v), true
	case int32:
		return float64(v), true
	case int64:
		return float64(v), true
	default:
		return 0, false
	}
}

func normalizePriceOperatingSystem(value string) string {
	if strings.TrimSpace(value) == "" {
		return defaultOperatingSystem
	}

	switch normalizeToken(value) {
	case "linux", "linux/unix":
		return "Linux"
	case "windows":
		return "Windows"
	case "rhel", "redhat":
		return "RHEL"
	case "suse":
		return "SUSE"
	default:
		return strings.TrimSpace(value)
	}
}

func normalizePriceTenancy(value string) string {
	if strings.TrimSpace(value) == "" {
		return defaultTenancy
	}

	switch normalizeToken(value) {
	case "shared", "default":
		return "Shared"
	case "dedicated":
		return "Dedicated"
	case "host":
		return "Host"
	default:
		return strings.TrimSpace(value)
	}
}

func normalizePricePreinstalledSoftware(value string) string {
	if strings.TrimSpace(value) == "" {
		return defaultPreinstalledSoft
	}

	switch normalizeToken(value) {
	case "na", "none":
		return "NA"
	default:
		return strings.TrimSpace(value)
	}
}

func normalizePriceLicenseModel(value string) string {
	if strings.TrimSpace(value) == "" {
		return defaultLicenseModel
	}

	switch normalizeToken(value) {
	case "no-license-required", "nolicenserequired", "none":
		return "No License required"
	case "bring-your-own-license", "byol":
		return "Bring your own license"
	default:
		return strings.TrimSpace(value)
	}
}

func normalizePriceCurrency(value string) string {
	if strings.TrimSpace(value) == "" {
		return defaultCurrency
	}
	return strings.ToUpper(strings.TrimSpace(value))
}
