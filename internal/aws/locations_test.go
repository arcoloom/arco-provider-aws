package aws

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/arcoloom/arco-provider-aws/internal/provider"
	awsv2 "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

func TestListRegionsReturnsAccountEnabledRegions(t *testing.T) {
	service := &Service{
		version: "test",
		clientFactory: fakeClientFactory{
			clients: map[string]ec2API{
				"default": fakeEC2Client{},
				"us-east-1": fakeEC2Client{
					regionsOutput: &ec2.DescribeRegionsOutput{
						Regions: []ec2types.Region{
							{RegionName: awsv2.String("us-west-2")},
							{RegionName: awsv2.String("us-east-1")},
						},
					},
				},
			},
		},
		catalog: testCatalogRepository(map[string]string{
			"us-east-1": "US East (N. Virginia)",
			"us-west-2": "US West (Oregon)",
		}),
	}

	result, err := service.ListRegions(context.Background(), provider.ListRegionsRequest{
		Credentials: provider.Credentials{
			AWS: &provider.AWSCredentials{
				AccessKeyID:     "ak",
				SecretAccessKey: "sk",
			},
		},
		Scope: provider.ConnectionScope{
			Region: "us-east-1",
		},
	})
	if err != nil {
		t.Fatalf("ListRegions returned error: %v", err)
	}

	if len(result.Warnings) != 0 {
		t.Fatalf("expected no warnings, got %+v", result.Warnings)
	}
	if len(result.Items) != 2 {
		t.Fatalf("expected 2 regions, got %+v", result.Items)
	}
	if result.Items[0].Code != "us-east-1" || result.Items[0].Name != "US East (N. Virginia)" {
		t.Fatalf("unexpected first region: %+v", result.Items[0])
	}
	if result.Items[1].Code != "us-west-2" || result.Items[1].Name != "US West (Oregon)" {
		t.Fatalf("unexpected second region: %+v", result.Items[1])
	}
}

func TestListAvailabilityZonesAcrossAllRegions(t *testing.T) {
	service := &Service{
		version: "test",
		clientFactory: fakeClientFactory{
			clients: map[string]ec2API{
				"us-east-1": fakeEC2Client{
					regionsOutput: &ec2.DescribeRegionsOutput{
						Regions: []ec2types.Region{
							{RegionName: awsv2.String("us-west-2")},
							{RegionName: awsv2.String("us-east-1")},
						},
					},
					availabilityZonesOutput: &ec2.DescribeAvailabilityZonesOutput{
						AvailabilityZones: []ec2types.AvailabilityZone{
							{
								ZoneName:           awsv2.String("us-east-1b"),
								ZoneId:             awsv2.String("use1-az2"),
								RegionName:         awsv2.String("us-east-1"),
								State:              ec2types.AvailabilityZoneStateAvailable,
								ZoneType:           awsv2.String("availability-zone"),
								NetworkBorderGroup: awsv2.String("us-east-1"),
								OptInStatus:        ec2types.AvailabilityZoneOptInStatusOptInNotRequired,
								GroupName:          awsv2.String("us-east-1-zg-1"),
								ParentZoneId:       awsv2.String("use1-az1"),
								ParentZoneName:     awsv2.String("us-east-1a"),
							},
							{
								ZoneName:           awsv2.String("us-east-1a"),
								ZoneId:             awsv2.String("use1-az1"),
								RegionName:         awsv2.String("us-east-1"),
								State:              ec2types.AvailabilityZoneStateAvailable,
								ZoneType:           awsv2.String("availability-zone"),
								NetworkBorderGroup: awsv2.String("us-east-1"),
								OptInStatus:        ec2types.AvailabilityZoneOptInStatusOptInNotRequired,
								GroupName:          awsv2.String("us-east-1-zg-1"),
							},
						},
					},
				},
				"us-west-2": fakeEC2Client{
					availabilityZonesOutput: &ec2.DescribeAvailabilityZonesOutput{
						AvailabilityZones: []ec2types.AvailabilityZone{
							{
								ZoneName:           awsv2.String("us-west-2a"),
								ZoneId:             awsv2.String("usw2-az1"),
								RegionName:         awsv2.String("us-west-2"),
								State:              ec2types.AvailabilityZoneStateAvailable,
								ZoneType:           awsv2.String("availability-zone"),
								NetworkBorderGroup: awsv2.String("us-west-2"),
								OptInStatus:        ec2types.AvailabilityZoneOptInStatusOptInNotRequired,
								GroupName:          awsv2.String("us-west-2-zg-1"),
							},
						},
					},
				},
			},
		},
		catalog: testCatalogRepository(nil),
	}

	result, err := service.ListAvailabilityZones(context.Background(), provider.ListAvailabilityZonesRequest{
		Credentials: provider.Credentials{
			AWS: &provider.AWSCredentials{
				AccessKeyID:     "ak",
				SecretAccessKey: "sk",
			},
		},
		Region: "all",
	})
	if err != nil {
		t.Fatalf("ListAvailabilityZones returned error: %v", err)
	}

	if len(result.Warnings) != 0 {
		t.Fatalf("expected no warnings, got %+v", result.Warnings)
	}
	if len(result.Items) != 3 {
		t.Fatalf("expected 3 availability zones, got %+v", result.Items)
	}

	if result.Items[0].Name != "us-east-1a" || result.Items[0].ZoneID != "use1-az1" || result.Items[0].Region != "us-east-1" {
		t.Fatalf("unexpected first availability zone: %+v", result.Items[0])
	}
	if result.Items[1].Name != "us-east-1b" || result.Items[1].ParentZoneName != "us-east-1a" {
		t.Fatalf("unexpected second availability zone: %+v", result.Items[1])
	}
	if result.Items[2].Name != "us-west-2a" || result.Items[2].ZoneID != "usw2-az1" || result.Items[2].Region != "us-west-2" {
		t.Fatalf("unexpected third availability zone: %+v", result.Items[2])
	}
}

func TestListAvailabilityZonesMatchesZoneIDsAndWarnsOnMissingZones(t *testing.T) {
	service := &Service{
		version: "test",
		clientFactory: fakeClientFactory{
			clients: map[string]ec2API{
				"us-east-1": fakeEC2Client{
					availabilityZonesOutput: &ec2.DescribeAvailabilityZonesOutput{
						AvailabilityZones: []ec2types.AvailabilityZone{
							{
								ZoneName:           awsv2.String("us-east-1a"),
								ZoneId:             awsv2.String("use1-az1"),
								RegionName:         awsv2.String("us-east-1"),
								State:              ec2types.AvailabilityZoneStateAvailable,
								ZoneType:           awsv2.String("availability-zone"),
								NetworkBorderGroup: awsv2.String("us-east-1"),
								OptInStatus:        ec2types.AvailabilityZoneOptInStatusOptInNotRequired,
							},
						},
					},
				},
			},
		},
		catalog: testCatalogRepository(nil),
	}

	result, err := service.ListAvailabilityZones(context.Background(), provider.ListAvailabilityZonesRequest{
		Credentials: provider.Credentials{
			AWS: &provider.AWSCredentials{
				AccessKeyID:     "ak",
				SecretAccessKey: "sk",
			},
		},
		Region:            "us-east-1",
		AvailabilityZones: []string{"use1-az1", "use1-az9"},
	})
	if err != nil {
		t.Fatalf("ListAvailabilityZones returned error: %v", err)
	}

	if len(result.Items) != 1 {
		t.Fatalf("expected 1 availability zone, got %+v", result.Items)
	}
	if result.Items[0].Name != "us-east-1a" || result.Items[0].ZoneID != "use1-az1" {
		t.Fatalf("unexpected availability zone: %+v", result.Items[0])
	}
	if len(result.Warnings) != 1 || result.Warnings[0].Code != "AZ_NOT_FOUND" {
		t.Fatalf("expected AZ_NOT_FOUND warning, got %+v", result.Warnings)
	}
	if !strings.Contains(result.Warnings[0].Message, "use1-az9") {
		t.Fatalf("unexpected warning message: %q", result.Warnings[0].Message)
	}
}

func testCatalogRepository(regionNames map[string]string) *catalogRepository {
	regionsByType := map[string][]provider.Region{}
	if len(regionNames) > 0 {
		items := make([]provider.Region, 0, len(regionNames))
		for code, name := range regionNames {
			items = append(items, provider.Region{Code: code, Name: name})
		}
		regionsByType["test.instance"] = items
	}

	return &catalogRepository{
		cached: &catalogSnapshot{
			regionsByType: regionsByType,
		},
		cachedAt:    time.Now(),
		now:         time.Now,
		cacheWindow: time.Hour,
	}
}
