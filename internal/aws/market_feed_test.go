package aws

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/arcoloom/arco-provider-aws/internal/provider"
	awsv2 "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	awspricing "github.com/aws/aws-sdk-go-v2/service/pricing"
	"github.com/aws/smithy-go"
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

func TestBuildMarketSnapshotMarksInvalidZeroOnDemandPrice(t *testing.T) {
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
            "description": "$0.000 per On Demand Linux c6a.large Instance Hour",
            "pricePerUnit": {
              "USD": "0.0000000000"
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
	if len(items) != 1 {
		t.Fatalf("expected one offering, got %d", len(items))
	}
	if got := items[0].Attributes["health_state"]; got != "blocked" {
		t.Fatalf("health_state = %q, want %q", got, "blocked")
	}
	if got := items[0].Attributes["price_confidence"]; got != "invalid" {
		t.Fatalf("price_confidence = %q, want %q", got, "invalid")
	}
	if len(warnings) != 1 || warnings[0].Code != warningCodePriceInvalid {
		t.Fatalf("expected one invalid price warning, got %+v", warnings)
	}
}

func TestChunkInstanceTypesForPricingRespectsJoinedLength(t *testing.T) {
	chunks := chunkInstanceTypesForPricing([]string{
		"c7i-flex.12xlarge",
		"c7i-flex.16xlarge",
		"c7i-flex.2xlarge",
		"c7i-flex.4xlarge",
		"c7i-flex.8xlarge",
		strings.Repeat("x", 1000),
	}, onDemandPriceBatchSize, 1024)

	if len(chunks) < 2 {
		t.Fatalf("expected multiple chunks, got %d", len(chunks))
	}
	for _, chunk := range chunks {
		joined := strings.Join(chunk, ",")
		if len(joined) > 1024 {
			t.Fatalf("chunk exceeds 1024 characters: %d", len(joined))
		}
	}
}

func TestBuildMarketSnapshotSkipsRecoverableSpotBatchErrors(t *testing.T) {
	service := &Service{
		version: "test",
		clientFactory: fakeClientFactory{
			clients: map[string]ec2API{
				"us-east-1": fakeEC2Client{
					regionsOutput: &ec2.DescribeRegionsOutput{
						Regions: []ec2types.Region{
							{RegionName: awsv2.String("us-east-1")},
							{RegionName: awsv2.String("us-west-2")},
						},
					},
					availabilityZonesOutput: &ec2.DescribeAvailabilityZonesOutput{
						AvailabilityZones: []ec2types.AvailabilityZone{
							{ZoneName: awsv2.String("us-east-1a"), ZoneId: awsv2.String("use1-az1")},
						},
					},
					instanceTypeOfferingsOutput: &ec2.DescribeInstanceTypeOfferingsOutput{
						InstanceTypeOfferings: []ec2types.InstanceTypeOffering{
							{InstanceType: ec2types.InstanceType("c7g.large"), Location: awsv2.String("us-east-1a")},
						},
					},
					spotPriceHistoryOutput: &ec2.DescribeSpotPriceHistoryOutput{
						SpotPriceHistory: []ec2types.SpotPrice{
							{
								InstanceType:     ec2types.InstanceType("c7g.large"),
								AvailabilityZone: awsv2.String("us-east-1a"),
								SpotPrice:        awsv2.String("0.011000"),
								Timestamp:        awsv2.Time(time.Date(2026, 3, 13, 1, 2, 3, 0, time.UTC)),
							},
						},
					},
				},
				"us-west-2": fakeEC2Client{
					availabilityZonesOutput: &ec2.DescribeAvailabilityZonesOutput{
						AvailabilityZones: []ec2types.AvailabilityZone{
							{ZoneName: awsv2.String("us-west-2a"), ZoneId: awsv2.String("usw2-az1")},
						},
					},
					instanceTypeOfferingsErr: &smithy.GenericAPIError{
						Code:    "InvalidParameterValue",
						Message: "unsupported instance type in region",
					},
				},
			},
			pricingClient: &fakePricingClient{},
		},
		catalog: &catalogRepository{
			cached: &catalogSnapshot{
				metadataByType: map[string]catalogInstanceMetadataRecord{
					"c7g.large": {
						InstanceType: "c7g.large",
						VCPU:         2,
						Memory:       4,
					},
				},
				regionsByType: map[string][]provider.Region{
					"c7g.large": {
						{Code: "us-east-1", Name: "US East (N. Virginia)"},
						{Code: "us-west-2", Name: "US West (Oregon)"},
					},
				},
				regionNames: map[string]string{
					"us-east-1": "US East (N. Virginia)",
					"us-west-2": "US West (Oregon)",
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
	if len(items) != 1 {
		t.Fatalf("expected healthy region to survive recoverable failure, got %d items", len(items))
	}
	if items[0].Region != "us-east-1" {
		t.Fatalf("unexpected surviving item set: %+v", items)
	}
	if len(warnings) != 1 || warnings[0].Code != warningCodeMarketBatchSkipped {
		t.Fatalf("expected one recoverable skip warning, got %+v", warnings)
	}
}

func TestBuildMarketSnapshotReturnsErrorForNonRecoverableSpotBatchErrors(t *testing.T) {
	service := &Service{
		version: "test",
		clientFactory: fakeClientFactory{
			clients: map[string]ec2API{
				"us-east-1": fakeEC2Client{
					regionsOutput: &ec2.DescribeRegionsOutput{
						Regions: []ec2types.Region{
							{RegionName: awsv2.String("us-east-1")},
						},
					},
					availabilityZonesOutput: &ec2.DescribeAvailabilityZonesOutput{
						AvailabilityZones: []ec2types.AvailabilityZone{
							{ZoneName: awsv2.String("us-east-1a"), ZoneId: awsv2.String("use1-az1")},
						},
					},
					instanceTypeOfferingsErr: errors.New("dial tcp timeout"),
				},
			},
			pricingClient: &fakePricingClient{},
		},
		catalog: &catalogRepository{
			cached: &catalogSnapshot{
				metadataByType: map[string]catalogInstanceMetadataRecord{
					"c7g.large": {
						InstanceType: "c7g.large",
						VCPU:         2,
						Memory:       4,
					},
				},
				regionsByType: map[string][]provider.Region{
					"c7g.large": {
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

	_, _, err := service.buildMarketSnapshot(context.Background(), provider.WatchMarketFeedRequest{
		Credentials: provider.Credentials{
			AWS: &provider.AWSCredentials{
				AccessKeyID:     "ak",
				SecretAccessKey: "sk",
			},
		},
	})
	if err == nil {
		t.Fatal("expected non-recoverable error to abort snapshot")
	}
}

func TestBuildMarketOfferingAttributesNormalizesArchitectures(t *testing.T) {
	t.Parallel()

	attributes := buildMarketOfferingAttributes(catalogInstanceMetadataRecord{
		Arch: []string{"x86_64", "amd64", "aarch64"},
	}, "pricing_api", "0.068", time.Date(2026, 3, 23, 10, 0, 0, 0, time.UTC), provider.SpotInventory{})

	if got, want := attributes["architecture"], ""; got != want {
		t.Fatalf("attributes[architecture] = %q, want %q when multiple architectures exist", got, want)
	}
	if got, want := attributes["architectures"], "x86,arm64"; got != want {
		t.Fatalf("attributes[architectures] = %q, want %q", got, want)
	}
}

func TestNormalizeMarketArchitectures(t *testing.T) {
	t.Parallel()

	got := normalizeMarketArchitectures([]string{"x86_64", "amd64", "arm64", "riscv64"})
	want := []string{"x86", "arm64"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("normalizeMarketArchitectures() = %#v, want %#v", got, want)
	}
}
