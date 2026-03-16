package aws

import (
	"context"
	"testing"
	"time"

	"github.com/arcoloom/arco-provider-aws/internal/provider"
	awsv2 "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/sts"
)

func TestGetSpotDataSingleRegionAndAZ(t *testing.T) {
	service := &Service{
		version: "test",
		clientFactory: fakeClientFactory{
			clients: map[string]ec2API{
				"us-east-1": fakeEC2Client{
					availabilityZonesOutput: &ec2.DescribeAvailabilityZonesOutput{
						AvailabilityZones: []ec2types.AvailabilityZone{
							{ZoneName: awsv2.String("us-east-1a"), ZoneId: awsv2.String("use1-az1")},
							{ZoneName: awsv2.String("us-east-1b"), ZoneId: awsv2.String("use1-az2")},
						},
					},
					instanceTypeOfferingsOutput: &ec2.DescribeInstanceTypeOfferingsOutput{
						InstanceTypeOfferings: []ec2types.InstanceTypeOffering{
							{InstanceType: ec2types.InstanceType("m6i.large"), Location: awsv2.String("us-east-1a")},
						},
					},
					spotPriceHistoryOutput: &ec2.DescribeSpotPriceHistoryOutput{
						SpotPriceHistory: []ec2types.SpotPrice{
							{
								InstanceType:     ec2types.InstanceType("m6i.large"),
								AvailabilityZone: awsv2.String("us-east-1a"),
								SpotPrice:        awsv2.String("0.012300"),
								Timestamp:        awsv2.Time(time.Date(2026, 3, 13, 1, 2, 3, 0, time.UTC)),
							},
						},
					},
					spotPlacementScoresOutput: map[string]*ec2.GetSpotPlacementScoresOutput{
						"m6i.large": {
							SpotPlacementScores: []ec2types.SpotPlacementScore{
								{
									AvailabilityZoneId: awsv2.String("use1-az1"),
									Score:              awsv2.Int32(9),
								},
							},
						},
					},
				},
			},
		},
	}

	result, err := service.GetSpotData(context.Background(), provider.GetSpotDataRequest{
		Credentials: provider.Credentials{
			AWS: &provider.AWSCredentials{
				AccessKeyID:     "ak",
				SecretAccessKey: "sk",
			},
		},
		Scope: provider.ConnectionScope{
			Region: "us-east-1",
		},
		InstanceTypes:     []string{"m6i.large"},
		AvailabilityZones: []string{"us-east-1a"},
	})
	if err != nil {
		t.Fatalf("GetSpotData returned error: %v", err)
	}

	if len(result.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(result.Items))
	}

	item := result.Items[0]
	if item.Region != "us-east-1" || item.AvailabilityZone != "us-east-1a" || item.InstanceType != "m6i.large" {
		t.Fatalf("unexpected spot item identity: %+v", item)
	}
	if !item.HasPrice || item.Price != "0.012300" {
		t.Fatalf("unexpected price payload: %+v", item)
	}
	if !item.Inventory.Offered || !item.Inventory.HasCapacityScore || item.Inventory.CapacityScore != 9 || item.Inventory.Status != inventoryStatusHigh {
		t.Fatalf("unexpected inventory payload: %+v", item.Inventory)
	}
}

func TestGetSpotDataAllRegionsAddsWarningForMissingAZ(t *testing.T) {
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
							{ZoneName: awsv2.String("us-west-2b"), ZoneId: awsv2.String("usw2-az2")},
						},
					},
					instanceTypeOfferingsOutput: &ec2.DescribeInstanceTypeOfferingsOutput{},
					spotPriceHistoryOutput:      &ec2.DescribeSpotPriceHistoryOutput{},
				},
			},
		},
	}

	result, err := service.GetSpotData(context.Background(), provider.GetSpotDataRequest{
		Credentials: provider.Credentials{
			AWS: &provider.AWSCredentials{
				AccessKeyID:     "ak",
				SecretAccessKey: "sk",
			},
		},
		InstanceTypes:     []string{"c7g.large"},
		Region:            "all",
		AvailabilityZones: []string{"us-east-1a", "us-west-2a"},
	})
	if err != nil {
		t.Fatalf("GetSpotData returned error: %v", err)
	}

	if len(result.Items) != 1 {
		t.Fatalf("expected 1 returned item from matching zones, got %d", len(result.Items))
	}
	if len(result.Warnings) != 1 || result.Warnings[0].Code != "AZ_NOT_FOUND" {
		t.Fatalf("unexpected warnings: %+v", result.Warnings)
	}
}

func TestGetSpotDataExplicitRegionOptionFansOutSelectedRegions(t *testing.T) {
	service := &Service{
		version: "test",
		clientFactory: fakeClientFactory{
			clients: map[string]ec2API{
				"us-east-1": fakeEC2Client{
					availabilityZonesOutput: &ec2.DescribeAvailabilityZonesOutput{
						AvailabilityZones: []ec2types.AvailabilityZone{
							{ZoneName: awsv2.String("us-east-1a"), ZoneId: awsv2.String("use1-az1")},
						},
					},
					instanceTypeOfferingsOutput: &ec2.DescribeInstanceTypeOfferingsOutput{
						InstanceTypeOfferings: []ec2types.InstanceTypeOffering{
							{InstanceType: ec2types.InstanceType("c7g.medium"), Location: awsv2.String("us-east-1a")},
						},
					},
					spotPriceHistoryOutput: &ec2.DescribeSpotPriceHistoryOutput{
						SpotPriceHistory: []ec2types.SpotPrice{
							{
								InstanceType:     ec2types.InstanceType("c7g.medium"),
								AvailabilityZone: awsv2.String("us-east-1a"),
								SpotPrice:        awsv2.String("0.010000"),
								Timestamp:        awsv2.Time(time.Date(2026, 3, 13, 1, 2, 3, 0, time.UTC)),
							},
						},
					},
				},
				"us-west-2": fakeEC2Client{
					availabilityZonesOutput: &ec2.DescribeAvailabilityZonesOutput{
						AvailabilityZones: []ec2types.AvailabilityZone{
							{ZoneName: awsv2.String("us-west-2b"), ZoneId: awsv2.String("usw2-az2")},
						},
					},
					instanceTypeOfferingsOutput: &ec2.DescribeInstanceTypeOfferingsOutput{
						InstanceTypeOfferings: []ec2types.InstanceTypeOffering{
							{InstanceType: ec2types.InstanceType("c7g.medium"), Location: awsv2.String("us-west-2b")},
						},
					},
					spotPriceHistoryOutput: &ec2.DescribeSpotPriceHistoryOutput{
						SpotPriceHistory: []ec2types.SpotPrice{
							{
								InstanceType:     ec2types.InstanceType("c7g.medium"),
								AvailabilityZone: awsv2.String("us-west-2b"),
								SpotPrice:        awsv2.String("0.009500"),
								Timestamp:        awsv2.Time(time.Date(2026, 3, 13, 1, 2, 3, 0, time.UTC)),
							},
						},
					},
				},
			},
		},
	}

	result, err := service.GetSpotData(context.Background(), provider.GetSpotDataRequest{
		Credentials: provider.Credentials{
			AWS: &provider.AWSCredentials{
				AccessKeyID:     "ak",
				SecretAccessKey: "sk",
			},
		},
		Scope: provider.ConnectionScope{
			Region: "us-east-1",
		},
		InstanceTypes: []string{"c7g.medium"},
		Options: map[string]string{
			optionRegions: "us-east-1, us-west-2",
		},
	})
	if err != nil {
		t.Fatalf("GetSpotData returned error: %v", err)
	}

	if len(result.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(result.Items))
	}
	if result.Items[0].Region != "us-east-1" || result.Items[1].Region != "us-west-2" {
		t.Fatalf("unexpected regions in result: %+v", result.Items)
	}
}

type fakeClientFactory struct {
	clients   map[string]ec2API
	stsClient stsAPI
}

func (f fakeClientFactory) NewConfig(_ context.Context, _ provider.AWSCredentials, region string, _ string) (awsv2.Config, error) {
	return awsv2.Config{Region: region}, nil
}

func (f fakeClientFactory) NewEC2(options ec2ClientOptions) ec2API {
	return f.clients[options.Config.Region]
}

func (f fakeClientFactory) NewSSM(awsv2.Config) ssmAPI {
	return fakeSSMClient{}
}

func (f fakeClientFactory) NewSTS(awsv2.Config) stsAPI {
	if f.stsClient != nil {
		return f.stsClient
	}
	return noopSTSClient{}
}

type fakeEC2Client struct {
	regionsOutput               *ec2.DescribeRegionsOutput
	availabilityZonesOutput     *ec2.DescribeAvailabilityZonesOutput
	availabilityZonesErr        error
	imagesOutput                *ec2.DescribeImagesOutput
	instanceTypeOfferingsOutput *ec2.DescribeInstanceTypeOfferingsOutput
	securityGroupsOutput        *ec2.DescribeSecurityGroupsOutput
	spotPriceHistoryOutput      *ec2.DescribeSpotPriceHistoryOutput
	spotPlacementScoresOutput   map[string]*ec2.GetSpotPlacementScoresOutput
	subnetsOutput               *ec2.DescribeSubnetsOutput
	vpcsOutput                  *ec2.DescribeVpcsOutput
}

func (f fakeEC2Client) DescribeRegions(context.Context, *ec2.DescribeRegionsInput, ...func(*ec2.Options)) (*ec2.DescribeRegionsOutput, error) {
	if f.regionsOutput == nil {
		return &ec2.DescribeRegionsOutput{}, nil
	}
	return f.regionsOutput, nil
}

func (f fakeEC2Client) DescribeAvailabilityZones(context.Context, *ec2.DescribeAvailabilityZonesInput, ...func(*ec2.Options)) (*ec2.DescribeAvailabilityZonesOutput, error) {
	if f.availabilityZonesErr != nil {
		return nil, f.availabilityZonesErr
	}
	if f.availabilityZonesOutput == nil {
		return &ec2.DescribeAvailabilityZonesOutput{}, nil
	}
	return f.availabilityZonesOutput, nil
}

func (f fakeEC2Client) DescribeInstanceTypeOfferings(context.Context, *ec2.DescribeInstanceTypeOfferingsInput, ...func(*ec2.Options)) (*ec2.DescribeInstanceTypeOfferingsOutput, error) {
	if f.instanceTypeOfferingsOutput == nil {
		return &ec2.DescribeInstanceTypeOfferingsOutput{}, nil
	}
	return f.instanceTypeOfferingsOutput, nil
}

func (f fakeEC2Client) DescribeInstanceTypes(context.Context, *ec2.DescribeInstanceTypesInput, ...func(*ec2.Options)) (*ec2.DescribeInstanceTypesOutput, error) {
	return &ec2.DescribeInstanceTypesOutput{}, nil
}

func (f fakeEC2Client) DescribeImages(context.Context, *ec2.DescribeImagesInput, ...func(*ec2.Options)) (*ec2.DescribeImagesOutput, error) {
	if f.imagesOutput == nil {
		return &ec2.DescribeImagesOutput{}, nil
	}
	return f.imagesOutput, nil
}

func (f fakeEC2Client) DescribeInstances(context.Context, *ec2.DescribeInstancesInput, ...func(*ec2.Options)) (*ec2.DescribeInstancesOutput, error) {
	return &ec2.DescribeInstancesOutput{}, nil
}

func (f fakeEC2Client) DescribeSecurityGroups(context.Context, *ec2.DescribeSecurityGroupsInput, ...func(*ec2.Options)) (*ec2.DescribeSecurityGroupsOutput, error) {
	if f.securityGroupsOutput == nil {
		return &ec2.DescribeSecurityGroupsOutput{}, nil
	}
	return f.securityGroupsOutput, nil
}

func (f fakeEC2Client) DescribeSpotPriceHistory(context.Context, *ec2.DescribeSpotPriceHistoryInput, ...func(*ec2.Options)) (*ec2.DescribeSpotPriceHistoryOutput, error) {
	if f.spotPriceHistoryOutput == nil {
		return &ec2.DescribeSpotPriceHistoryOutput{}, nil
	}
	return f.spotPriceHistoryOutput, nil
}

func (f fakeEC2Client) DescribeSubnets(context.Context, *ec2.DescribeSubnetsInput, ...func(*ec2.Options)) (*ec2.DescribeSubnetsOutput, error) {
	if f.subnetsOutput == nil {
		return &ec2.DescribeSubnetsOutput{}, nil
	}
	return f.subnetsOutput, nil
}

func (f fakeEC2Client) DescribeVpcs(context.Context, *ec2.DescribeVpcsInput, ...func(*ec2.Options)) (*ec2.DescribeVpcsOutput, error) {
	if f.vpcsOutput == nil {
		return &ec2.DescribeVpcsOutput{}, nil
	}
	return f.vpcsOutput, nil
}

func (f fakeEC2Client) GetSpotPlacementScores(_ context.Context, input *ec2.GetSpotPlacementScoresInput, _ ...func(*ec2.Options)) (*ec2.GetSpotPlacementScoresOutput, error) {
	if f.spotPlacementScoresOutput == nil {
		return &ec2.GetSpotPlacementScoresOutput{}, nil
	}
	if len(input.InstanceTypes) == 0 {
		return &ec2.GetSpotPlacementScoresOutput{}, nil
	}
	if output, ok := f.spotPlacementScoresOutput[input.InstanceTypes[0]]; ok {
		return output, nil
	}
	return &ec2.GetSpotPlacementScoresOutput{}, nil
}

func (f fakeEC2Client) RunInstances(context.Context, *ec2.RunInstancesInput, ...func(*ec2.Options)) (*ec2.RunInstancesOutput, error) {
	return &ec2.RunInstancesOutput{}, nil
}

func (f fakeEC2Client) TerminateInstances(context.Context, *ec2.TerminateInstancesInput, ...func(*ec2.Options)) (*ec2.TerminateInstancesOutput, error) {
	return &ec2.TerminateInstancesOutput{}, nil
}

type fakeSSMClient struct{}

func (fakeSSMClient) GetParameter(context.Context, *ssm.GetParameterInput, ...func(*ssm.Options)) (*ssm.GetParameterOutput, error) {
	return &ssm.GetParameterOutput{}, nil
}

func (fakeSSMClient) GetParametersByPath(context.Context, *ssm.GetParametersByPathInput, ...func(*ssm.Options)) (*ssm.GetParametersByPathOutput, error) {
	return &ssm.GetParametersByPathOutput{}, nil
}

type noopSTSClient struct{}

func (noopSTSClient) GetCallerIdentity(context.Context, *sts.GetCallerIdentityInput, ...func(*sts.Options)) (*sts.GetCallerIdentityOutput, error) {
	return &sts.GetCallerIdentityOutput{}, nil
}
