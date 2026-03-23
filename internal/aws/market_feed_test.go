package aws

import (
	"context"
	"testing"
	"time"

	"github.com/arcoloom/arco-provider-aws/internal/provider"
	awsv2 "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	awspricing "github.com/aws/aws-sdk-go-v2/service/pricing"
)

func TestBuildMarketSnapshotUsesLiveOnDemandPricing(t *testing.T) {
	service := &Service{
		version: "test",
		clientFactory: fakeClientFactory{
			clients: map[string]ec2API{
				defaultAWSRegion: fakeEC2Client{
					regionsOutput: &ec2.DescribeRegionsOutput{
						Regions: []ec2types.Region{
							{RegionName: awsv2.String("us-east-1")},
						},
					},
				},
			},
			pricingClient: &fakePricingClient{
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
			},
		},
		catalog: &catalogRepository{
			cached: &catalogSnapshot{
				metadataByType: map[string]catalogInstanceMetadataRecord{
					"c6a.large": {
						InstanceType: "c6a.large",
						VCPU:         2,
						Memory:       4,
					},
				},
				regionsByType: map[string][]provider.Region{
					"c6a.large": {
						{Code: "us-east-1", Name: "US East (N. Virginia)"},
					},
				},
				regionNames: map[string]string{
					"us-east-1": "US East (N. Virginia)",
				},
			},
			cachedAt:    time.Now(),
			now:         time.Now,
			cacheWindow: time.Hour,
		},
	}

	items, warnings, err := service.buildMarketSnapshot(context.Background(), provider.WatchMarketFeedRequest{
		Credentials: provider.Credentials{
			AWS: &provider.AWSCredentials{
				AccessKeyID:     "ak",
				SecretAccessKey: "sk",
			},
		},
	})
	if err != nil {
		t.Fatalf("buildMarketSnapshot returned error: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %+v", warnings)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 market offering, got %d", len(items))
	}
	if items[0].PurchaseOption != provider.PurchaseOptionOnDemand {
		t.Fatalf("unexpected purchase option: %+v", items[0])
	}
	if items[0].HourlyPriceUSD != 0.068 {
		t.Fatalf("unexpected hourly price: %+v", items[0])
	}
	if items[0].Attributes["source"] != "pricing_api" {
		t.Fatalf("expected pricing_api source, got %+v", items[0].Attributes)
	}
}
