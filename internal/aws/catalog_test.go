package aws

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/arcoloom/arco-provider-aws/internal/provider"
	awspricing "github.com/aws/aws-sdk-go-v2/service/pricing"
)

func TestListInstanceTypesFiltersByRegionFromCatalog(t *testing.T) {
	service := newCatalogBackedTestService(t, newStaticCatalogFetcher(), &fakePricingClient{})

	result, err := service.ListInstanceTypes(context.Background(), provider.ListInstanceTypesRequest{
		Region: "us-west-2",
	})
	if err != nil {
		t.Fatalf("ListInstanceTypes returned error: %v", err)
	}

	if len(result.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(result.Items))
	}
	if result.Items[0].InstanceType != "c6a.large" {
		t.Fatalf("unexpected instance type: %+v", result.Items[0])
	}
	if result.Items[0].Category != "compute-optimized" {
		t.Fatalf("unexpected normalized category: %+v", result.Items[0])
	}
}

func TestGetInstanceTypeInfoReturnsStandardizedFields(t *testing.T) {
	service := newCatalogBackedTestService(t, newStaticCatalogFetcher(), &fakePricingClient{})

	result, err := service.GetInstanceTypeInfo(context.Background(), provider.GetInstanceTypeInfoRequest{
		InstanceTypes: []string{"g5.xlarge"},
	})
	if err != nil {
		t.Fatalf("GetInstanceTypeInfo returned error: %v", err)
	}

	if len(result.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(result.Items))
	}

	item := result.Items[0]
	if item.CPUManufacturer != "AMD" {
		t.Fatalf("unexpected CPU manufacturer: %+v", item)
	}
	if len(item.Accelerators) != 1 || item.Accelerators[0].Kind != provider.AcceleratorKindGPU {
		t.Fatalf("unexpected accelerators: %+v", item.Accelerators)
	}
	if item.Accelerators[0].Count != 1 {
		t.Fatalf("unexpected accelerator count: %+v", item.Accelerators)
	}
	if len(item.SupportedRegions) != 1 || item.SupportedRegions[0].Code != "us-east-1" {
		t.Fatalf("unexpected supported regions: %+v", item.SupportedRegions)
	}
	if item.Attributes["ebs_baseline_iops"] != "10000" {
		t.Fatalf("unexpected attributes: %+v", item.Attributes)
	}
}

func TestGetInstanceTypeInfoSupportsFractionalGPUCounts(t *testing.T) {
	service := newCatalogBackedTestService(t, newStaticCatalogFetcher(), &fakePricingClient{})

	result, err := service.GetInstanceTypeInfo(context.Background(), provider.GetInstanceTypeInfoRequest{
		InstanceTypes: []string{"g6f.large"},
	})
	if err != nil {
		t.Fatalf("GetInstanceTypeInfo returned error: %v", err)
	}

	if len(result.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(result.Items))
	}

	item := result.Items[0]
	if len(item.Accelerators) != 1 {
		t.Fatalf("unexpected accelerators: %+v", item.Accelerators)
	}
	if item.Accelerators[0].Count != 0.125 {
		t.Fatalf("unexpected fractional accelerator count: %+v", item.Accelerators)
	}
	if item.Accelerators[0].MemoryGiB != 3 {
		t.Fatalf("unexpected fractional accelerator memory: %+v", item.Accelerators)
	}
}

func TestGetInstancePricesParsesOnDemandPriceAndUsesCache(t *testing.T) {
	fetcher := newStaticCatalogFetcher()
	pricingClient := &fakePricingClient{
		outputs: []*awspricing.GetProductsOutput{
			{
				PriceList: []string{`{
  "product": {
    "sku": "sku-c6a-large",
    "attributes": {
      "instanceType": "c6a.large",
      "location": "US East (N. Virginia)",
      "regionCode": "us-east-1",
      "operatingSystem": "Linux",
      "tenancy": "Shared",
      "preInstalledSw": "NA",
      "licenseModel": "No License required"
    }
  },
  "terms": {
    "OnDemand": {
      "sku-c6a-large.JRTCKXETXF": {
        "effectiveDate": "2026-03-20T04:29:25Z",
        "priceDimensions": {
          "sku-c6a-large.JRTCKXETXF.6YS6EN2CT7": {
            "unit": "Hrs",
            "description": "$0.068 per On Demand Linux c6a.large Instance Hour",
            "pricePerUnit": {
              "USD": "0.0680000000"
            }
          }
        }
      }
    }
  }
}`},
			},
		},
	}
	service := newCatalogBackedTestService(t, fetcher, pricingClient)

	first, err := service.GetInstancePrices(context.Background(), provider.GetInstancePricesRequest{
		Credentials: provider.Credentials{
			AWS: &provider.AWSCredentials{
				AccessKeyID:     "ak",
				SecretAccessKey: "sk",
			},
		},
		Region:        "us-east-1",
		InstanceTypes: []string{"c6a.large"},
	})
	if err != nil {
		t.Fatalf("GetInstancePrices returned error: %v", err)
	}

	if len(first.Items) != 1 {
		t.Fatalf("expected 1 price item, got %d", len(first.Items))
	}
	if first.Items[0].Price != "0.0680000000" {
		t.Fatalf("unexpected price item: %+v", first.Items[0])
	}

	second, err := service.GetInstancePrices(context.Background(), provider.GetInstancePricesRequest{
		Credentials: provider.Credentials{
			AWS: &provider.AWSCredentials{
				AccessKeyID:     "ak",
				SecretAccessKey: "sk",
			},
		},
		Region:        "us-east-1",
		InstanceTypes: []string{"c6a.large"},
	})
	if err != nil {
		t.Fatalf("GetInstancePrices returned error on second call: %v", err)
	}
	if len(second.Items) != 1 {
		t.Fatalf("expected second price item, got %d", len(second.Items))
	}
}

func TestGetInstancePricesRequiresCredentials(t *testing.T) {
	service := newCatalogBackedTestService(t, newStaticCatalogFetcher(), &fakePricingClient{})

	_, err := service.GetInstancePrices(context.Background(), provider.GetInstancePricesRequest{
		Region:        "us-east-1",
		InstanceTypes: []string{"c6a.large"},
	})
	if err == nil {
		t.Fatal("expected credentials error, got nil")
	}
}

func newCatalogBackedTestService(t *testing.T, fetcher *staticCatalogFetcher, pricingClient pricingAPI) *Service {
	t.Helper()

	repo := &catalogRepository{
		baseDir: filepath.Join(t.TempDir(), "aws-cache"),
		fetcher: fetcher,
		now:     time.Now,
	}

	return &Service{
		version: "test",
		clientFactory: fakeClientFactory{
			pricingClient: pricingClient,
		},
		instanceRunner: &fakeInstanceLifecycleRunner{},
		catalog:        repo,
	}
}

type staticCatalogFetcher struct {
	fail      bool
	responses map[string][]byte
}

func newStaticCatalogFetcher() *staticCatalogFetcher {
	metadataBody := []byte(`[
  {
    "series": "c6a",
    "instance_type": "c6a.large",
    "family": "Compute optimized",
    "pretty_name": "C6A Large",
    "generation": "current",
    "vCPU": 2,
    "memory": 4,
    "clock_speed_ghz": "3.6 GHz",
    "physical_processor": "AMD EPYC 7R13 Processor",
    "arch": ["x86_64"],
    "network_performance": "Up to 12.5 Gigabit",
    "enhanced_networking": true,
    "vpc_only": true,
    "ipv6_support": true,
    "placement_group_support": true,
    "ebs_optimized": true,
    "ebs_baseline_iops": 10000,
    "GPU": 0,
    "GPU_model": null,
    "GPU_memory": 0,
    "FPGA": 0,
    "storage": null,
    "support_os": ["linux", "ubuntu"]
  },
  {
    "series": "g5",
    "instance_type": "g5.xlarge",
    "family": "Accelerated computing",
    "pretty_name": "G5 Xlarge",
    "generation": "current",
    "vCPU": 4,
    "memory": 16,
    "clock_speed_ghz": "3.3 GHz",
    "physical_processor": "AMD EPYC 7R32 Processor",
    "arch": ["x86_64"],
    "network_performance": "Up to 10 Gigabit",
    "enhanced_networking": true,
    "vpc_only": true,
    "ipv6_support": true,
    "placement_group_support": true,
    "ebs_optimized": true,
    "ebs_baseline_iops": 10000,
    "GPU": 1,
    "GPU_model": "NVIDIA A10G",
    "GPU_memory": 24,
    "FPGA": 0,
    "storage": null,
    "support_os": ["linux", "windows"]
  },
  {
    "series": "g6f",
    "instance_type": "g6f.large",
    "family": "Accelerated computing",
    "pretty_name": "G6F Large",
    "generation": "current",
    "vCPU": 2,
    "memory": 8,
    "clock_speed_ghz": "2.6 GHz",
    "physical_processor": "AMD EPYC 7R13 Processor",
    "arch": ["x86_64"],
    "network_performance": "Up to 10 Gigabit",
    "enhanced_networking": true,
    "vpc_only": true,
    "ipv6_support": true,
    "placement_group_support": true,
    "ebs_optimized": true,
    "ebs_baseline_iops": 3750,
    "GPU": 0.125,
    "GPU_model": "NVIDIA L4",
    "GPU_memory": 3,
    "FPGA": 0,
    "storage": null,
    "support_os": ["linux", "ubuntu"]
  }
]`)
	regionsBody := []byte(`[
  {
    "series": "c6a",
    "instance_type": "c6a.large",
    "region_code": "us-east-1",
    "region_name": "US East (N. Virginia)",
    "on_demand_price": "0.0680000000"
  },
  {
    "series": "c6a",
    "instance_type": "c6a.large",
    "region_code": "us-west-2",
    "region_name": "US West (Oregon)",
    "on_demand_price": "0.0790000000"
  },
  {
    "series": "g5",
    "instance_type": "g5.xlarge",
    "region_code": "us-east-1",
    "region_name": "US East (N. Virginia)",
    "on_demand_price": "1.0060000000"
  },
  {
    "series": "g6f",
    "instance_type": "g6f.large",
    "region_code": "us-east-1",
    "region_name": "US East (N. Virginia)",
    "on_demand_price": "0.7520000000"
  }
]`)
	seriesModelsBody := []byte(`[
  {
    "series": "c6a",
    "instance_count": 1,
    "instance_types": ["c6a.large"]
  },
  {
    "series": "g5",
    "instance_count": 1,
    "instance_types": ["g5.xlarge"]
  },
  {
    "series": "g6f",
    "instance_count": 1,
    "instance_types": ["g6f.large"]
  }
]`)
	resolveBody := []byte(fmt.Sprintf(`{
  "schema": "arco.dataset.resolve.v1",
  "channel": "latest",
  "dataset": {
    "provider": "aws",
    "name": "ec2",
    "version": "test-catalog-version"
  },
  "files": {
    "instance_metadata": {
      "key": "datasets/aws/ec2/test-catalog-version/instance_metadata.json",
      "filename": "instance_metadata.json",
      "sha256": "%s",
      "size": %d,
      "content_type": "application/json",
      "download_url": "%s"
    },
    "instance_regions": {
      "key": "datasets/aws/ec2/test-catalog-version/instance_regions.json",
      "filename": "instance_regions.json",
      "sha256": "%s",
      "size": %d,
      "content_type": "application/json",
      "download_url": "%s"
    },
    "series_models": {
      "key": "datasets/aws/ec2/test-catalog-version/series_models.json",
      "filename": "series_models.json",
      "sha256": "%s",
      "size": %d,
      "content_type": "application/json",
      "download_url": "%s"
    }
  }
}`,
		sha256Hex(metadataBody), len(metadataBody), registryDatasetDownloadURL("instance_metadata.json"),
		sha256Hex(regionsBody), len(regionsBody), registryDatasetDownloadURL("instance_regions.json"),
		sha256Hex(seriesModelsBody), len(seriesModelsBody), registryDatasetDownloadURL("series_models.json"),
	))

	return &staticCatalogFetcher{
		responses: map[string][]byte{
			registryDatasetResolveURL():                          resolveBody,
			registryDatasetDownloadURL("instance_metadata.json"): metadataBody,
			registryDatasetDownloadURL("instance_regions.json"):  regionsBody,
			registryDatasetDownloadURL("series_models.json"):     seriesModelsBody,
		},
	}
}

func registryDatasetResolveURL() string {
	return fmt.Sprintf("%s/v1/resolve/datasets/aws/ec2/latest", defaultRegistryBaseURL)
}

func registryDatasetDownloadURL(filename string) string {
	return fmt.Sprintf("%s/v1/datasets/aws/ec2/test-catalog-version/%s", defaultRegistryBaseURL, filename)
}

func (f *staticCatalogFetcher) Fetch(_ context.Context, url string) ([]byte, error) {
	if f.fail {
		return nil, errors.New("fetch disabled")
	}
	if body, ok := f.responses[url]; ok {
		return body, nil
	}
	return nil, errors.New("unexpected url: " + url)
}
