package aws

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	defaultRegistryChannel   = "latest"
	registryBaseURLEnvVar    = "ARCO_REGISTRY_BASE_URL"
	registryChannelEnvVar    = "ARCO_REGISTRY_CHANNEL"
	registryConfigPathEnvVar = "ARCO_CONFIG_PATH"
	registryProviderName     = "aws"
	registryDatasetName      = "ec2"
	catalogResolveCachePath  = "catalog/resolve.json"
	catalogMetadataFileAlias = "instance_metadata"
	catalogRegionsFileAlias  = "instance_regions"
	catalogSeriesModelsAlias = "series_models"
)

type registryDatasetSource struct {
	baseURL string
	channel string
}

type registryDatasetResolveResponse struct {
	Schema             string                         `json:"schema"`
	Channel            string                         `json:"channel"`
	Dataset            registryDatasetIdentity        `json:"dataset"`
	Files              map[string]registryDatasetFile `json:"files"`
	Metadata           map[string]string              `json:"metadata"`
	ManifestURL        string                         `json:"manifest_url"`
	VersionManifestURL string                         `json:"version_manifest_url"`
}

type registryDatasetIdentity struct {
	Provider string `json:"provider"`
	Name     string `json:"name"`
	Version  string `json:"version"`
}

type registryDatasetFile struct {
	Key         string `json:"key"`
	Filename    string `json:"filename"`
	SHA256      string `json:"sha256"`
	Size        int64  `json:"size"`
	ContentType string `json:"content_type"`
	DownloadURL string `json:"download_url"`
}

func defaultRegistryDatasetSource() registryDatasetSource {
	baseURL := strings.TrimRight(strings.TrimSpace(os.Getenv(registryBaseURLEnvVar)), "/")
	channel := strings.TrimSpace(os.Getenv(registryChannelEnvVar))
	if baseURL == "" || channel == "" {
		configSource := loadRegistryDatasetSourceFromConfig()
		if baseURL == "" {
			baseURL = configSource.baseURL
		}
		if channel == "" {
			channel = configSource.channel
		}
	}
	return registryDatasetSource{
		baseURL: baseURL,
		channel: channel,
	}.normalized()
}

func (s registryDatasetSource) normalized() registryDatasetSource {
	baseURL := strings.TrimRight(strings.TrimSpace(s.baseURL), "/")
	channel := strings.TrimSpace(s.channel)
	if channel == "" {
		channel = defaultRegistryChannel
	}
	return registryDatasetSource{
		baseURL: baseURL,
		channel: channel,
	}
}

func (r *catalogRepository) loadRegistryCatalog(ctx context.Context) ([]catalogInstanceMetadataRecord, []catalogRegionRecord, []catalogSeriesModelsRecord, error) {
	if _, err := r.syncRegistryDataset(ctx); err != nil {
		if ok, localErr := r.localCatalogFilesExist(); localErr != nil || !ok {
			return nil, nil, nil, err
		}
	}

	var metadata []catalogInstanceMetadataRecord
	if err := r.loadLocalJSON(catalogLocalFilePath(catalogMetadataFileAlias), &metadata); err != nil {
		return nil, nil, nil, err
	}

	var regions []catalogRegionRecord
	if err := r.loadLocalJSON(catalogLocalFilePath(catalogRegionsFileAlias), &regions); err != nil {
		return nil, nil, nil, err
	}

	var seriesModels []catalogSeriesModelsRecord
	if err := r.loadLocalJSON(catalogLocalFilePath(catalogSeriesModelsAlias), &seriesModels); err != nil {
		return nil, nil, nil, err
	}

	return metadata, regions, seriesModels, nil
}

func (r *catalogRepository) syncRegistryDataset(ctx context.Context) (*registryDatasetResolveResponse, error) {
	resolved, err := r.resolveRegistryDataset(ctx)
	if err != nil {
		return nil, err
	}
	if err := validateRegistryDatasetResolve(resolved); err != nil {
		return nil, err
	}

	cached, _ := r.loadCachedRegistryResolve()
	if cached != nil && registryDatasetVersionMatches(cached, resolved) {
		if ok, err := r.localRegistryFilesMatch(resolved); err == nil && ok {
			return resolved, nil
		}
	}

	if err := r.downloadRegistryFiles(ctx, resolved); err != nil {
		return nil, err
	}
	if err := writeCatalogResolveCache(filepath.Join(r.baseDir, catalogResolveCachePath), resolved); err != nil {
		return nil, err
	}
	return resolved, nil
}

func (r *catalogRepository) resolveRegistryDataset(ctx context.Context) (*registryDatasetResolveResponse, error) {
	source := r.source.normalized()
	if source.baseURL == "" {
		return nil, fmt.Errorf("registry base URL is not configured")
	}
	resolveURL := fmt.Sprintf(
		"%s/v1/resolve/datasets/%s/%s/%s",
		source.baseURL,
		registryProviderName,
		registryDatasetName,
		source.channel,
	)
	body, err := r.fetcher.Fetch(ctx, resolveURL)
	if err != nil {
		return nil, err
	}

	var resolved registryDatasetResolveResponse
	if err := json.Unmarshal(body, &resolved); err != nil {
		return nil, fmt.Errorf("decode registry dataset resolve response: %w", err)
	}
	return &resolved, nil
}

func loadRegistryDatasetSourceFromConfig() registryDatasetSource {
	for _, path := range registryConfigCandidatePaths() {
		source, ok := readRegistryDatasetSourceFromConfig(path)
		if ok {
			return source
		}
	}
	return registryDatasetSource{}
}

func registryConfigCandidatePaths() []string {
	candidates := make([]string, 0, 5)
	if path := strings.TrimSpace(os.Getenv(registryConfigPathEnvVar)); path != "" {
		candidates = append(candidates, path)
	}
	candidates = append(
		candidates,
		"config.toml",
		"config.toml.example",
		filepath.Join("..", "arcoloom", "config.toml"),
		filepath.Join("..", "arcoloom", "config.toml.example"),
	)
	return candidates
}

func readRegistryDatasetSourceFromConfig(path string) (registryDatasetSource, bool) {
	body, err := os.ReadFile(path)
	if err != nil {
		return registryDatasetSource{}, false
	}

	source := registryDatasetSource{}
	inSourceSection := false
	for _, rawLine := range strings.Split(string(body), "\n") {
		line := strings.TrimSpace(strings.SplitN(rawLine, "#", 2)[0])
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			inSourceSection = line == "[source]"
			continue
		}
		if !inSourceSection {
			continue
		}

		key, value, ok := parseRegistryConfigAssignment(line)
		if !ok {
			continue
		}
		switch key {
		case "registry_base_url":
			source.baseURL = strings.TrimRight(value, "/")
		case "channel":
			source.channel = value
		}
	}
	if source.baseURL == "" && source.channel == "" {
		return registryDatasetSource{}, false
	}
	return source.normalized(), true
}

func parseRegistryConfigAssignment(line string) (string, string, bool) {
	key, rawValue, ok := strings.Cut(line, "=")
	if !ok {
		return "", "", false
	}
	value, err := strconv.Unquote(strings.TrimSpace(rawValue))
	if err != nil {
		return "", "", false
	}
	return strings.TrimSpace(key), strings.TrimSpace(value), true
}

func validateRegistryDatasetResolve(resolved *registryDatasetResolveResponse) error {
	if resolved == nil {
		return fmt.Errorf("registry dataset resolve response is empty")
	}
	if strings.TrimSpace(resolved.Dataset.Provider) != registryProviderName || strings.TrimSpace(resolved.Dataset.Name) != registryDatasetName {
		return fmt.Errorf("registry dataset resolve response did not describe %s/%s", registryProviderName, registryDatasetName)
	}
	for _, alias := range requiredCatalogAliases() {
		file, ok := resolved.Files[alias]
		if !ok {
			return fmt.Errorf("registry dataset resolve response is missing file alias %q", alias)
		}
		if strings.TrimSpace(file.DownloadURL) == "" {
			return fmt.Errorf("registry dataset file %q is missing a download_url", alias)
		}
		if strings.TrimSpace(file.SHA256) == "" {
			return fmt.Errorf("registry dataset file %q is missing a sha256", alias)
		}
		if strings.TrimSpace(file.Filename) == "" {
			return fmt.Errorf("registry dataset file %q is missing a filename", alias)
		}
	}
	return nil
}

func (r *catalogRepository) loadLocalJSON(relativePath string, target any) error {
	body, err := os.ReadFile(filepath.Join(r.baseDir, relativePath))
	if err != nil {
		return fmt.Errorf("read %s: %w", relativePath, err)
	}
	if err := json.Unmarshal(body, target); err != nil {
		return fmt.Errorf("decode %s: %w", relativePath, err)
	}
	return nil
}

func (r *catalogRepository) loadCachedRegistryResolve() (*registryDatasetResolveResponse, error) {
	path := filepath.Join(r.baseDir, catalogResolveCachePath)
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var resolved registryDatasetResolveResponse
	if err := json.Unmarshal(body, &resolved); err != nil {
		return nil, fmt.Errorf("decode %s: %w", catalogResolveCachePath, err)
	}
	return &resolved, nil
}

func (r *catalogRepository) localRegistryFilesMatch(resolved *registryDatasetResolveResponse) (bool, error) {
	for _, alias := range requiredCatalogAliases() {
		descriptor := resolved.Files[alias]
		info, err := os.Stat(filepath.Join(r.baseDir, catalogLocalFilePath(alias)))
		if err != nil {
			if os.IsNotExist(err) {
				return false, nil
			}
			return false, err
		}
		if descriptor.Size > 0 && info.Size() != descriptor.Size {
			return false, nil
		}
	}
	return true, nil
}

func (r *catalogRepository) localCatalogFilesExist() (bool, error) {
	for _, alias := range requiredCatalogAliases() {
		if _, err := os.Stat(filepath.Join(r.baseDir, catalogLocalFilePath(alias))); err != nil {
			if os.IsNotExist(err) {
				return false, nil
			}
			return false, err
		}
	}
	return true, nil
}

func (r *catalogRepository) downloadRegistryFiles(ctx context.Context, resolved *registryDatasetResolveResponse) error {
	if err := os.MkdirAll(filepath.Join(r.baseDir, "catalog"), 0o755); err != nil {
		return fmt.Errorf("create registry catalog cache dir: %w", err)
	}
	tempDir, err := os.MkdirTemp(filepath.Join(r.baseDir, "catalog"), ".registry-*")
	if err != nil {
		return fmt.Errorf("create temporary registry cache dir: %w", err)
	}
	defer os.RemoveAll(tempDir)

	aliases := requiredCatalogAliases()
	for _, alias := range aliases {
		descriptor := resolved.Files[alias]
		body, err := r.fetcher.Fetch(ctx, descriptor.DownloadURL)
		if err != nil {
			return fmt.Errorf("download registry catalog file %q: %w", alias, err)
		}
		if descriptor.Size > 0 && int64(len(body)) != descriptor.Size {
			return fmt.Errorf("registry catalog file %q size mismatch: got %d want %d", alias, len(body), descriptor.Size)
		}
		actualSHA256 := sha256Hex(body)
		if !strings.EqualFold(actualSHA256, descriptor.SHA256) {
			return fmt.Errorf("registry catalog file %q checksum mismatch: got %s want %s", alias, actualSHA256, descriptor.SHA256)
		}
		tempPath := filepath.Join(tempDir, descriptor.Filename)
		if err := os.WriteFile(tempPath, body, 0o644); err != nil {
			return fmt.Errorf("write temporary registry catalog file %q: %w", alias, err)
		}
	}

	for _, alias := range aliases {
		descriptor := resolved.Files[alias]
		tempPath := filepath.Join(tempDir, descriptor.Filename)
		finalPath := filepath.Join(r.baseDir, catalogLocalFilePath(alias))
		body, err := os.ReadFile(tempPath)
		if err != nil {
			return fmt.Errorf("read temporary registry catalog file %q: %w", alias, err)
		}
		if err := writeCacheFile(finalPath, body); err != nil {
			return err
		}
	}
	return nil
}

func writeCatalogResolveCache(path string, resolved *registryDatasetResolveResponse) error {
	body, err := json.MarshalIndent(resolved, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal registry resolve cache: %w", err)
	}
	body = append(body, '\n')
	return writeCacheFile(path, body)
}

func registryDatasetVersionMatches(left *registryDatasetResolveResponse, right *registryDatasetResolveResponse) bool {
	if left == nil || right == nil {
		return false
	}
	for _, alias := range requiredCatalogAliases() {
		leftFile, leftOK := left.Files[alias]
		rightFile, rightOK := right.Files[alias]
		if !leftOK || !rightOK {
			return false
		}
		if leftFile.SHA256 != rightFile.SHA256 || leftFile.Size != rightFile.Size || leftFile.Filename != rightFile.Filename {
			return false
		}
	}
	return true
}

func requiredCatalogAliases() []string {
	return []string{
		catalogMetadataFileAlias,
		catalogRegionsFileAlias,
		catalogSeriesModelsAlias,
	}
}

func catalogLocalFilePath(alias string) string {
	switch alias {
	case catalogMetadataFileAlias:
		return "catalog/instance_metadata.json"
	case catalogRegionsFileAlias:
		return "catalog/instance_regions.json"
	case catalogSeriesModelsAlias:
		return "catalog/series_models.json"
	default:
		return filepath.Join("catalog", alias+".json")
	}
}

func sha256Hex(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}
