package aws

import "testing"

func TestValidateRegistryDatasetResolveAllowsEmptyVersion(t *testing.T) {
	resolved := &registryDatasetResolveResponse{
		Dataset: registryDatasetIdentity{
			Provider: registryProviderName,
			Name:     registryDatasetName,
		},
		Files: map[string]registryDatasetFile{
			catalogMetadataFileAlias: {
				Filename:    "instance_metadata.json",
				SHA256:      "aaa",
				DownloadURL: "https://example.com/instance_metadata.json",
			},
			catalogRegionsFileAlias: {
				Filename:    "instance_regions.json",
				SHA256:      "bbb",
				DownloadURL: "https://example.com/instance_regions.json",
			},
			catalogSeriesModelsAlias: {
				Filename:    "series_models.json",
				SHA256:      "ccc",
				DownloadURL: "https://example.com/series_models.json",
			},
		},
	}

	if err := validateRegistryDatasetResolve(resolved); err != nil {
		t.Fatalf("validateRegistryDatasetResolve() error = %v", err)
	}
}

func TestRegistryDatasetVersionMatchesIgnoresVersionWhenFilesMatch(t *testing.T) {
	left := &registryDatasetResolveResponse{
		Dataset: registryDatasetIdentity{
			Provider: registryProviderName,
			Name:     registryDatasetName,
			Version:  "",
		},
		Files: map[string]registryDatasetFile{
			catalogMetadataFileAlias: {Filename: "instance_metadata.json", SHA256: "aaa", Size: 1},
			catalogRegionsFileAlias:  {Filename: "instance_regions.json", SHA256: "bbb", Size: 2},
			catalogSeriesModelsAlias: {Filename: "series_models.json", SHA256: "ccc", Size: 3},
		},
	}
	right := &registryDatasetResolveResponse{
		Dataset: registryDatasetIdentity{
			Provider: registryProviderName,
			Name:     registryDatasetName,
			Version:  "2026-03-25",
		},
		Files: map[string]registryDatasetFile{
			catalogMetadataFileAlias: {Filename: "instance_metadata.json", SHA256: "aaa", Size: 1},
			catalogRegionsFileAlias:  {Filename: "instance_regions.json", SHA256: "bbb", Size: 2},
			catalogSeriesModelsAlias: {Filename: "series_models.json", SHA256: "ccc", Size: 3},
		},
	}

	if !registryDatasetVersionMatches(left, right) {
		t.Fatal("expected matching file descriptors to be treated as unchanged")
	}
}

