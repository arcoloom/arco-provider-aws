package aws

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/arcoloom/arco-provider-aws/internal/provider"
)

const (
	defaultMarketFeedRefreshInterval   = 15 * time.Minute
	defaultMarketFeedHeartbeatInterval = 30 * time.Second
	marketFeedChunkSize                = 250
	spotMarketBatchSize                = 200
	spotMarketBatchDelay               = 200 * time.Millisecond
)

func (s *Service) WatchMarketFeed(ctx context.Context, req provider.WatchMarketFeedRequest, emit func(provider.WatchMarketFeedEvent) error) error {
	if req.Credentials.AWS == nil && len(req.Credentials.AWSAccounts) == 0 {
		return fmt.Errorf("aws iam credentials are required")
	}

	refreshInterval := defaultMarketFeedRefreshInterval
	heartbeatInterval := defaultMarketFeedHeartbeatInterval

	for {
		if ctx.Err() != nil {
			return nil
		}

		snapshotToken := strconv.FormatInt(time.Now().UTC().UnixNano(), 10)
		offerings, warnings, err := s.buildMarketSnapshot(ctx, req)
		if err != nil {
			return err
		}

		if err := emit(provider.WatchMarketFeedEvent{
			Type:          provider.WatchMarketFeedEventTypeBegin,
			SnapshotToken: snapshotToken,
		}); err != nil {
			return err
		}
		for offset := 0; offset < len(offerings); offset += marketFeedChunkSize {
			limit := offset + marketFeedChunkSize
			if limit > len(offerings) {
				limit = len(offerings)
			}
			if err := emit(provider.WatchMarketFeedEvent{
				Type:      provider.WatchMarketFeedEventTypeChunk,
				Offerings: append([]provider.MarketOffering(nil), offerings[offset:limit]...),
			}); err != nil {
				return err
			}
		}
		resumeToken := snapshotToken
		if err := emit(provider.WatchMarketFeedEvent{
			Type:          provider.WatchMarketFeedEventTypeCommit,
			SnapshotToken: snapshotToken,
			ResumeToken:   resumeToken,
		}); err != nil {
			return err
		}

		for _, warning := range warnings {
			if err := emit(provider.WatchMarketFeedEvent{
				Type:     provider.WatchMarketFeedEventTypeWarning,
				Warnings: []provider.Warning{warning},
			}); err != nil {
				return err
			}
		}

		refreshTimer := time.NewTimer(refreshInterval)
		heartbeatTicker := time.NewTicker(heartbeatInterval)
		waiting := true
		for waiting {
			select {
			case <-ctx.Done():
				heartbeatTicker.Stop()
				refreshTimer.Stop()
				return nil
			case <-refreshTimer.C:
				waiting = false
			case <-heartbeatTicker.C:
				if err := emit(provider.WatchMarketFeedEvent{
					Type:        provider.WatchMarketFeedEventTypeHeartbeat,
					ResumeToken: resumeToken,
				}); err != nil {
					heartbeatTicker.Stop()
					refreshTimer.Stop()
					return err
				}
			}
		}
		heartbeatTicker.Stop()
		refreshTimer.Stop()
	}
}

func (s *Service) buildMarketSnapshot(ctx context.Context, req provider.WatchMarketFeedRequest) ([]provider.MarketOffering, []provider.Warning, error) {
	snapshot, err := s.catalog.loadCatalog(ctx)
	if err != nil {
		return nil, nil, err
	}

	accounts := routeAWSAccounts(req.Credentials, req.Scope)
	if len(accounts) == 0 {
		return nil, nil, fmt.Errorf("aws iam credentials are required")
	}

	offerings := make([]provider.MarketOffering, 0)
	warnings := make([]provider.Warning, 0)
	for _, account := range accounts {
		accountRegions, err := s.listMarketRegionsForAccount(ctx, req.Scope, account)
		if err != nil {
			return nil, nil, err
		}
		onDemandOfferings, onDemandWarnings, err := s.buildOnDemandMarketOfferings(ctx, req.Scope, snapshot, account, accountRegions)
		if err != nil {
			return nil, nil, err
		}
		offerings = append(offerings, onDemandOfferings...)
		warnings = append(warnings, onDemandWarnings...)

		spotOfferings, spotWarnings, err := s.buildSpotMarketOfferings(ctx, req.Scope, snapshot, account, accountRegions)
		if err != nil {
			return nil, nil, err
		}
		offerings = append(offerings, spotOfferings...)
		warnings = append(warnings, spotWarnings...)
	}
	slices.SortFunc(offerings, func(left, right provider.MarketOffering) int {
		switch {
		case left.ScopeID != right.ScopeID:
			return strings.Compare(left.ScopeID, right.ScopeID)
		case left.Region != right.Region:
			return strings.Compare(left.Region, right.Region)
		case left.AvailabilityZone != right.AvailabilityZone:
			return strings.Compare(left.AvailabilityZone, right.AvailabilityZone)
		case left.InstanceType != right.InstanceType:
			return strings.Compare(left.InstanceType, right.InstanceType)
		default:
			return strings.Compare(string(left.PurchaseOption), string(right.PurchaseOption))
		}
	})

	return offerings, warnings, nil
}

func (s *Service) buildOnDemandMarketOfferings(ctx context.Context, scope provider.ConnectionScope, snapshot catalogSnapshot, account routedAWSAccount, allowedRegions []string) ([]provider.MarketOffering, []provider.Warning, error) {
	items := make([]provider.MarketOffering, 0)
	warnings := make([]provider.Warning, 0)
	allowed := buildStringSet(allowedRegions)
	for _, region := range allowedRegions {
		if len(allowed) != 0 {
			if _, ok := allowed[normalizeToken(region)]; !ok {
				continue
			}
		}
		prices, err := s.getOnDemandPrices(
			ctx,
			provider.Credentials{AWS: &account.Credentials},
			scope,
			snapshot,
			region,
			nil,
			defaultOperatingSystem,
			defaultTenancy,
			defaultPreinstalledSoft,
			defaultLicenseModel,
			defaultCurrency,
		)
		if err != nil {
			if shouldSkipMarketRegionError(err) {
				warnings = append(warnings, marketRegionWarning("on-demand", region, err))
				continue
			}
			return nil, nil, err
		}
		for _, price := range prices {
			record, ok := snapshot.metadataByType[price.InstanceType]
			if !ok {
				continue
			}
			priceValue, err := strconv.ParseFloat(strings.TrimSpace(price.Price), 64)
			if err != nil {
				continue
			}
			attributes := buildMarketOfferingAttributes(record, "pricing_api", price.Price, price.EffectiveAt, provider.SpotInventory{})
			if priceValue <= 0 {
				attributes["health_state"] = "blocked"
				attributes["price_confidence"] = "invalid"
				attributes["effective_hourly_price_usd"] = "0"
				warnings = append(warnings, invalidPriceWarning("on-demand", region, price.InstanceType, priceValue))
			}
			items = append(items, provider.MarketOffering{
				ScopeID:          account.ScopeID,
				Region:           price.Region.Code,
				AvailabilityZone: "",
				ZoneID:           "",
				InstanceType:     price.InstanceType,
				PurchaseOption:   provider.PurchaseOptionOnDemand,
				CPUMilli:         record.VCPU * 1000,
				MemoryMiB:        int32(record.Memory * 1024),
				GPUCount:         int32(record.GPU),
				HourlyPriceUSD:   priceValue,
				Attributes:       attributes,
			})
		}
	}
	return items, warnings, nil
}

func (s *Service) buildSpotMarketOfferings(ctx context.Context, scope provider.ConnectionScope, snapshot catalogSnapshot, account routedAWSAccount, regions []string) ([]provider.MarketOffering, []provider.Warning, error) {
	items := make([]provider.MarketOffering, 0)
	warnings := make([]provider.Warning, 0)
	for _, region := range regions {
		regionTypes := marketInstanceTypesForRegion(snapshot, region)
		if len(regionTypes) == 0 {
			continue
		}

		regionCfg, err := s.clientFactory.NewConfig(ctx, account.Credentials, effectiveEndpointRegion(scope, region), scope.Endpoint)
		if err != nil {
			return nil, nil, fmt.Errorf("build ec2 config for region %s: %w", region, err)
		}
		regionClient := s.clientFactory.NewEC2(ec2ClientOptions{
			Config:   regionCfg,
			Endpoint: scope.Endpoint,
		})

		availabilityZones, zoneIDToName, zoneWarnings, err := resolveAvailabilityZones(ctx, regionClient, region, nil)
		warnings = append(warnings, zoneWarnings...)
		if err != nil {
			if shouldSkipMarketRegionError(err) {
				warnings = append(warnings, marketRegionWarning("spot", region, err))
				continue
			}
			return nil, nil, err
		}
		batches := chunkStrings(regionTypes, spotMarketBatchSize)
		for index, batch := range batches {
			if ctx.Err() != nil {
				return nil, nil, ctx.Err()
			}
			offerings, err := fetchOfferings(ctx, regionClient, batch)
			if err != nil {
				if shouldSkipMarketBatchError(err) {
					warnings = append(warnings, marketBatchWarning(region, batch, "describe_instance_type_offerings", err))
					continue
				}
				return nil, nil, err
			}
			prices, err := fetchLatestSpotPrices(ctx, regionClient, batch)
			if err != nil {
				if shouldSkipMarketBatchError(err) {
					warnings = append(warnings, marketBatchWarning(region, batch, "describe_spot_price_history", err))
					continue
				}
				return nil, nil, err
			}
			for _, spotItem := range buildSpotItems(region, batch, availabilityZones, offerings, prices, nil) {
				record, ok := snapshot.metadataByType[spotItem.InstanceType]
				if !ok {
					continue
				}
				priceValue, err := strconv.ParseFloat(strings.TrimSpace(spotItem.Price), 64)
				if err != nil || !spotItem.HasPrice {
					continue
				}
				attributes := buildMarketOfferingAttributes(record, "spot", spotItem.Price, spotItem.Timestamp, spotItem.Inventory)
				items = append(items, provider.MarketOffering{
					ScopeID:          account.ScopeID,
					Region:           spotItem.Region,
					AvailabilityZone: spotItem.AvailabilityZone,
					ZoneID:           availabilityZoneID(zoneIDToName, spotItem.AvailabilityZone),
					InstanceType:     spotItem.InstanceType,
					PurchaseOption:   provider.PurchaseOptionSpot,
					CPUMilli:         record.VCPU * 1000,
					MemoryMiB:        int32(record.Memory * 1024),
					GPUCount:         int32(record.GPU),
					HourlyPriceUSD:   priceValue,
					Attributes:       attributes,
				})
			}
			if index == len(batches)-1 {
				continue
			}
			if !sleepWithContextAWS(ctx, spotMarketBatchDelay) {
				return nil, nil, ctx.Err()
			}
		}
	}
	return items, warnings, nil
}

func marketInstanceTypesForRegion(snapshot catalogSnapshot, region string) []string {
	items := make([]string, 0)
	for instanceType, supportedRegions := range snapshot.regionsByType {
		if supportsRegion(supportedRegions, region) {
			items = append(items, instanceType)
		}
	}
	slices.Sort(items)
	return items
}

func (s *Service) listMarketRegionsForAccount(ctx context.Context, scope provider.ConnectionScope, account routedAWSAccount) ([]string, error) {
	baseRegion := effectiveDiscoveryBaseRegion("")
	cfg, err := s.clientFactory.NewConfig(ctx, account.Credentials, effectiveEndpointRegion(scope, baseRegion), scope.Endpoint)
	if err != nil {
		return nil, err
	}
	return listAccountRegions(ctx, s.clientFactory.NewEC2(ec2ClientOptions{
		Config:   cfg,
		Endpoint: scope.Endpoint,
	}))
}

func chunkStrings(items []string, chunkSize int) [][]string {
	if chunkSize <= 0 || len(items) == 0 {
		return [][]string{append([]string(nil), items...)}
	}
	chunks := make([][]string, 0, (len(items)+chunkSize-1)/chunkSize)
	for start := 0; start < len(items); start += chunkSize {
		end := start + chunkSize
		if end > len(items) {
			end = len(items)
		}
		chunks = append(chunks, append([]string(nil), items[start:end]...))
	}
	return chunks
}

func buildStringSet(items []string) map[string]struct{} {
	if len(items) == 0 {
		return nil
	}
	result := make(map[string]struct{}, len(items))
	for _, item := range items {
		if trimmed := normalizeToken(item); trimmed != "" {
			result[trimmed] = struct{}{}
		}
	}
	return result
}

func buildMarketOfferingAttributes(record catalogInstanceMetadataRecord, source string, price string, effectiveAt time.Time, inventory provider.SpotInventory) map[string]string {
	attributes := buildProviderAttributes(record.Raw)
	if len(attributes) == 0 {
		attributes = make(map[string]string)
	}
	attributes["health_state"] = "healthy"
	attributes["price_confidence"] = "observed"
	if architectures := normalizeMarketArchitectures(record.Arch); len(architectures) > 0 {
		attributes["architectures"] = strings.Join(architectures, ",")
		if len(architectures) == 1 {
			attributes["architecture"] = architectures[0]
		}
	}
	attributes["source"] = source
	attributes["ipv6_supported"] = strconvBool(record.IPv6Support)
	if burstMinutes := normalizeBurstMinutes(record.Raw["burst_minutes"]); burstMinutes != "" {
		attributes["burstable"] = "true"
		attributes["cpu_credits_supported"] = "true"
		attributes["cpu_credit_mode"] = "burstable"
		attributes["cpu_credit_burst_minutes"] = burstMinutes
	} else {
		attributes["burstable"] = "false"
		attributes["cpu_credits_supported"] = "false"
		attributes["cpu_credit_mode"] = "standard"
	}
	if strings.TrimSpace(record.GPUModel) != "" {
		attributes["gpu_model"] = strings.TrimSpace(record.GPUModel)
	}
	if familyClass := plannerFamilyClass(record.Family); familyClass != "" {
		attributes["family_class"] = familyClass
	}
	if generationRank := plannerGenerationRank(record.InstanceType, record.Generation); generationRank != "" {
		attributes["generation_rank"] = generationRank
	}
	if memoryPerVCPU := plannerMemoryPerVCPU(float64(record.VCPU), record.Memory); memoryPerVCPU != "" {
		attributes["memory_per_vcpu"] = memoryPerVCPU
	}
	if value := strings.TrimSpace(record.NetworkPerformance); value != "" {
		attributes["network_performance"] = value
	}
	if value := strings.TrimSpace(record.ClockSpeedGHz); value != "" {
		attributes["clock_speed_ghz"] = value
	}
	if value := strings.TrimSpace(record.PhysicalProcessor); value != "" {
		attributes["physical_processor"] = value
	}
	copyRawAttribute(attributes, record.Raw, "memory_speed")
	copyRawAttribute(attributes, record.Raw, "coremark_iterations_second")
	copyRawAttribute(attributes, record.Raw, "base_performance")
	if strings.TrimSpace(price) != "" {
		attributes["spot_price"] = strings.TrimSpace(price)
	}
	if !effectiveAt.IsZero() {
		attributes["price_effective_at"] = effectiveAt.UTC().Format(time.RFC3339)
	}
	if normalizedPrice := strings.TrimSpace(price); normalizedPrice != "" {
		attributes["effective_hourly_price_usd"] = normalizedPrice
	}
	if inventory.Status != "" {
		attributes["inventory_status"] = inventory.Status
	}
	if inventory.HasCapacityScore {
		attributes["capacity_score"] = strconv.FormatInt(int64(inventory.CapacityScore), 10)
	}
	return attributes
}

func copyRawAttribute(attributes map[string]string, raw map[string]any, key string) {
	if len(attributes) == 0 || len(raw) == 0 {
		return
	}
	value, ok := raw[key]
	if !ok {
		return
	}
	if normalized := strings.TrimSpace(stringifyAttribute(value)); normalized != "" {
		attributes[key] = normalized
	}
}

func normalizeBurstMinutes(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case float32:
		return strconv.FormatFloat(float64(typed), 'f', -1, 64)
	case int:
		return strconv.Itoa(typed)
	case int32:
		return strconv.FormatInt(int64(typed), 10)
	case int64:
		return strconv.FormatInt(typed, 10)
	case json.Number:
		return typed.String()
	case string:
		return strings.TrimSpace(typed)
	default:
		return strings.TrimSpace(fmt.Sprint(typed))
	}
}

func normalizeMarketArchitectures(values []string) []string {
	if len(values) == 0 {
		return nil
	}

	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		switch normalizeMarketArchitecture(value) {
		case "x86", "arm64":
			if _, ok := seen[normalizeMarketArchitecture(value)]; ok {
				continue
			}
			canonical := normalizeMarketArchitecture(value)
			seen[canonical] = struct{}{}
			result = append(result, canonical)
		}
	}

	if len(result) == 0 {
		return nil
	}

	return result
}

func normalizeMarketArchitecture(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "x86", "x86_64", "x86-64", "amd64":
		return "x86"
	case "arm64", "aarch64":
		return "arm64"
	default:
		return ""
	}
}

func plannerFamilyClass(value string) string {
	switch normalizeFamilyCategory(value) {
	case "compute-optimized":
		return "compute"
	case "general-purpose":
		return "general"
	case "memory-optimized":
		return "memory"
	case "burstable-performance":
		return "burstable"
	default:
		return ""
	}
}

func plannerGenerationRank(instanceType string, generation string) string {
	trimmed := strings.TrimSpace(instanceType)
	for i := 0; i < len(trimmed); i++ {
		if !unicode.IsDigit(rune(trimmed[i])) {
			continue
		}
		j := i
		for j < len(trimmed) && unicode.IsDigit(rune(trimmed[j])) {
			j++
		}
		if value, err := strconv.ParseFloat(trimmed[i:j], 64); err == nil && value > 0 {
			return strconv.FormatFloat(value, 'f', -1, 64)
		}
	}
	switch normalizeToken(generation) {
	case "current":
		return "4"
	case "previous":
		return "2"
	default:
		return ""
	}
}

func plannerMemoryPerVCPU(vcpu float64, memoryGiB float64) string {
	if vcpu <= 0 || memoryGiB <= 0 {
		return ""
	}
	return strconv.FormatFloat(memoryGiB*1024.0/vcpu, 'f', -1, 64)
}

func availabilityZoneID(zoneIDToName map[string]string, zoneName string) string {
	for zoneID, candidate := range zoneIDToName {
		if strings.EqualFold(strings.TrimSpace(candidate), strings.TrimSpace(zoneName)) {
			return strings.TrimSpace(zoneID)
		}
	}
	return ""
}

func sleepWithContextAWS(ctx context.Context, delay time.Duration) bool {
	if delay <= 0 {
		return true
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
