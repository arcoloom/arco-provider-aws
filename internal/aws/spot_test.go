package aws

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/arcoloom/arco-provider-aws/internal/provider"
	awsv2 "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	awspricing "github.com/aws/aws-sdk-go-v2/service/pricing"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/aws/smithy-go"
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
		Region:            "us-east-1",
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
	clients       map[string]ec2API
	pricingClient pricingAPI
	stsClient     stsAPI
}

func (f fakeClientFactory) NewConfig(_ context.Context, _ provider.AWSCredentials, region string, _ string) (awsv2.Config, error) {
	return awsv2.Config{Region: region}, nil
}

func (f fakeClientFactory) NewEC2(options ec2ClientOptions) ec2API {
	return f.clients[options.Config.Region]
}

func (f fakeClientFactory) NewPricing(awsv2.Config) pricingAPI {
	if f.pricingClient != nil {
		return f.pricingClient
	}
	return &fakePricingClient{}
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
	instanceTypeOfferingsErr    error
	instanceTypeOfferingsOutput *ec2.DescribeInstanceTypeOfferingsOutput
	internetGatewaysOutput      *ec2.DescribeInternetGatewaysOutput
	routeTablesOutput           *ec2.DescribeRouteTablesOutput
	securityGroupsOutput        *ec2.DescribeSecurityGroupsOutput
	spotPriceHistoryErr         error
	spotPriceHistoryOutput      *ec2.DescribeSpotPriceHistoryOutput
	spotPlacementScoresOutput   map[string]*ec2.GetSpotPlacementScoresOutput
	subnetsOutput               *ec2.DescribeSubnetsOutput
	vpcsOutput                  *ec2.DescribeVpcsOutput
}

type fakePricingClient struct {
	outputs []*awspricing.GetProductsOutput
	err     error
	index   int
}

func (f *fakePricingClient) GetProducts(_ context.Context, _ *awspricing.GetProductsInput, _ ...func(*awspricing.Options)) (*awspricing.GetProductsOutput, error) {
	if f.err != nil {
		return nil, f.err
	}
	if len(f.outputs) == 0 {
		return &awspricing.GetProductsOutput{}, nil
	}
	if f.index >= len(f.outputs) {
		return f.outputs[len(f.outputs)-1], nil
	}
	output := f.outputs[f.index]
	f.index++
	return output, nil
}

func (f fakeEC2Client) AssociateRouteTable(context.Context, *ec2.AssociateRouteTableInput, ...func(*ec2.Options)) (*ec2.AssociateRouteTableOutput, error) {
	return &ec2.AssociateRouteTableOutput{}, nil
}

func (f fakeEC2Client) AssociateSubnetCidrBlock(context.Context, *ec2.AssociateSubnetCidrBlockInput, ...func(*ec2.Options)) (*ec2.AssociateSubnetCidrBlockOutput, error) {
	return &ec2.AssociateSubnetCidrBlockOutput{}, nil
}

func (f fakeEC2Client) AssociateVpcCidrBlock(context.Context, *ec2.AssociateVpcCidrBlockInput, ...func(*ec2.Options)) (*ec2.AssociateVpcCidrBlockOutput, error) {
	return &ec2.AssociateVpcCidrBlockOutput{}, nil
}

func (f fakeEC2Client) AttachInternetGateway(context.Context, *ec2.AttachInternetGatewayInput, ...func(*ec2.Options)) (*ec2.AttachInternetGatewayOutput, error) {
	return &ec2.AttachInternetGatewayOutput{}, nil
}

func (f fakeEC2Client) AuthorizeSecurityGroupEgress(context.Context, *ec2.AuthorizeSecurityGroupEgressInput, ...func(*ec2.Options)) (*ec2.AuthorizeSecurityGroupEgressOutput, error) {
	return &ec2.AuthorizeSecurityGroupEgressOutput{}, nil
}

func (f fakeEC2Client) CreateInternetGateway(context.Context, *ec2.CreateInternetGatewayInput, ...func(*ec2.Options)) (*ec2.CreateInternetGatewayOutput, error) {
	return &ec2.CreateInternetGatewayOutput{}, nil
}

func (f fakeEC2Client) CreateRoute(context.Context, *ec2.CreateRouteInput, ...func(*ec2.Options)) (*ec2.CreateRouteOutput, error) {
	return &ec2.CreateRouteOutput{}, nil
}

func (f fakeEC2Client) CreateRouteTable(context.Context, *ec2.CreateRouteTableInput, ...func(*ec2.Options)) (*ec2.CreateRouteTableOutput, error) {
	return &ec2.CreateRouteTableOutput{}, nil
}

func (f fakeEC2Client) CreateSecurityGroup(context.Context, *ec2.CreateSecurityGroupInput, ...func(*ec2.Options)) (*ec2.CreateSecurityGroupOutput, error) {
	return &ec2.CreateSecurityGroupOutput{}, nil
}

func (f fakeEC2Client) CreateSubnet(context.Context, *ec2.CreateSubnetInput, ...func(*ec2.Options)) (*ec2.CreateSubnetOutput, error) {
	return &ec2.CreateSubnetOutput{}, nil
}

func (f fakeEC2Client) CreateVpc(context.Context, *ec2.CreateVpcInput, ...func(*ec2.Options)) (*ec2.CreateVpcOutput, error) {
	return &ec2.CreateVpcOutput{}, nil
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
	if f.instanceTypeOfferingsErr != nil {
		return nil, f.instanceTypeOfferingsErr
	}
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

func (f fakeEC2Client) DescribeInternetGateways(context.Context, *ec2.DescribeInternetGatewaysInput, ...func(*ec2.Options)) (*ec2.DescribeInternetGatewaysOutput, error) {
	if f.internetGatewaysOutput == nil {
		return &ec2.DescribeInternetGatewaysOutput{}, nil
	}
	return f.internetGatewaysOutput, nil
}

func (f fakeEC2Client) DescribeInstances(context.Context, *ec2.DescribeInstancesInput, ...func(*ec2.Options)) (*ec2.DescribeInstancesOutput, error) {
	return &ec2.DescribeInstancesOutput{}, nil
}

func (f fakeEC2Client) DescribeRouteTables(context.Context, *ec2.DescribeRouteTablesInput, ...func(*ec2.Options)) (*ec2.DescribeRouteTablesOutput, error) {
	if f.routeTablesOutput == nil {
		return &ec2.DescribeRouteTablesOutput{}, nil
	}
	return f.routeTablesOutput, nil
}

func (f fakeEC2Client) DescribeSecurityGroups(context.Context, *ec2.DescribeSecurityGroupsInput, ...func(*ec2.Options)) (*ec2.DescribeSecurityGroupsOutput, error) {
	if f.securityGroupsOutput == nil {
		return &ec2.DescribeSecurityGroupsOutput{}, nil
	}
	return f.securityGroupsOutput, nil
}

func (f fakeEC2Client) DescribeSpotPriceHistory(context.Context, *ec2.DescribeSpotPriceHistoryInput, ...func(*ec2.Options)) (*ec2.DescribeSpotPriceHistoryOutput, error) {
	if f.spotPriceHistoryErr != nil {
		return nil, f.spotPriceHistoryErr
	}
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

func (f fakeEC2Client) ModifySubnetAttribute(context.Context, *ec2.ModifySubnetAttributeInput, ...func(*ec2.Options)) (*ec2.ModifySubnetAttributeOutput, error) {
	return &ec2.ModifySubnetAttributeOutput{}, nil
}

func (f fakeEC2Client) ModifyVpcAttribute(context.Context, *ec2.ModifyVpcAttributeInput, ...func(*ec2.Options)) (*ec2.ModifyVpcAttributeOutput, error) {
	return &ec2.ModifyVpcAttributeOutput{}, nil
}

func (f fakeEC2Client) RevokeSecurityGroupIngress(context.Context, *ec2.RevokeSecurityGroupIngressInput, ...func(*ec2.Options)) (*ec2.RevokeSecurityGroupIngressOutput, error) {
	return &ec2.RevokeSecurityGroupIngressOutput{}, nil
}

func (f fakeEC2Client) TerminateInstances(context.Context, *ec2.TerminateInstancesInput, ...func(*ec2.Options)) (*ec2.TerminateInstancesOutput, error) {
	return &ec2.TerminateInstancesOutput{}, nil
}

func TestGetSpotDataSkipsRecoverableBatchErrors(t *testing.T) {
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
		},
	}

	result, err := service.GetSpotData(context.Background(), provider.GetSpotDataRequest{
		Credentials: provider.Credentials{
			AWS: &provider.AWSCredentials{
				AccessKeyID:     "ak",
				SecretAccessKey: "sk",
			},
		},
		InstanceTypes: []string{"c7g.large"},
		Options: map[string]string{
			optionRegions: "us-east-1, us-west-2",
		},
	})
	if err != nil {
		t.Fatalf("GetSpotData returned error: %v", err)
	}

	if len(result.Items) != 1 {
		t.Fatalf("expected 1 item after skipping recoverable failure, got %d", len(result.Items))
	}
	if result.Items[0].Region != "us-east-1" {
		t.Fatalf("unexpected spot result after partial failure: %+v", result.Items)
	}
	if len(result.Warnings) != 1 || result.Warnings[0].Code != warningCodeMarketBatchSkipped {
		t.Fatalf("expected recoverable skip warning, got %+v", result.Warnings)
	}
}

func TestGetSpotDataReturnsErrorForNonRecoverableBatchFailures(t *testing.T) {
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
					instanceTypeOfferingsErr: errors.New("dial tcp timeout"),
				},
			},
		},
	}

	_, err := service.GetSpotData(context.Background(), provider.GetSpotDataRequest{
		Credentials: provider.Credentials{
			AWS: &provider.AWSCredentials{
				AccessKeyID:     "ak",
				SecretAccessKey: "sk",
			},
		},
		Region:        "us-east-1",
		InstanceTypes: []string{"c7g.large"},
	})
	if err == nil {
		t.Fatal("expected non-recoverable batch failure to be returned")
	}
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
