package aws

import (
	"context"
	"testing"
	"time"

	"github.com/arcoloom/arco-provider-aws/internal/provider"
	awsv2 "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

func TestListActiveInstancesWithoutFiltersReturnsAllActiveInstancesAcrossAccountRegions(t *testing.T) {
	eastClient := &listInstancesRecordingClient{
		fakeEC2Client: fakeEC2Client{
			regionsOutput: &ec2.DescribeRegionsOutput{
				Regions: []ec2types.Region{
					{RegionName: awsv2.String("us-east-1")},
					{RegionName: awsv2.String("us-west-2")},
				},
			},
		},
		describeInstancesOutput: &ec2.DescribeInstancesOutput{
			Reservations: []ec2types.Reservation{
				{
					Instances: []ec2types.Instance{
						testInstance("i-east-running", "api-east", "us-east-1a", "use1-az1", "c7g.medium", ec2types.InstanceStateNameRunning, "", []ec2types.Tag{
							{Key: awsv2.String("Name"), Value: awsv2.String("api-east")},
							{Key: awsv2.String("Environment"), Value: awsv2.String("prod")},
						}, []string{"2600:1f18::10"}),
						testInstance("i-east-stopped", "batch-east", "us-east-1b", "use1-az2", "c6g.medium", ec2types.InstanceStateNameStopped, ec2types.InstanceLifecycleTypeSpot, []ec2types.Tag{
							{Key: awsv2.String("Name"), Value: awsv2.String("batch-east")},
							{Key: awsv2.String("Environment"), Value: awsv2.String("dev")},
						}, nil),
						testInstance("i-east-terminated", "gone-east", "us-east-1c", "use1-az3", "c7g.medium", ec2types.InstanceStateNameTerminated, "", nil, nil),
					},
				},
			},
		},
	}
	westClient := &listInstancesRecordingClient{
		describeInstancesOutput: &ec2.DescribeInstancesOutput{
			Reservations: []ec2types.Reservation{
				{
					Instances: []ec2types.Instance{
						testInstance("i-west-running", "api-west", "us-west-2b", "usw2-az2", "c7g.medium", ec2types.InstanceStateNameRunning, "", []ec2types.Tag{
							{Key: awsv2.String("Name"), Value: awsv2.String("api-west")},
							{Key: awsv2.String("ArcoManaged"), Value: awsv2.String("true")},
						}, []string{"2600:1f14::20"}),
					},
				},
			},
		},
	}

	service := &Service{
		version: "test",
		clientFactory: fakeClientFactory{
			clients: map[string]ec2API{
				"us-east-1": eastClient,
				"us-west-2": westClient,
			},
		},
	}

	result, err := service.ListActiveInstances(context.Background(), provider.ListActiveInstancesRequest{
		Credentials: provider.Credentials{
			AWS: &provider.AWSCredentials{
				AccessKeyID:     "ak",
				SecretAccessKey: "sk",
			},
		},
	})
	if err != nil {
		t.Fatalf("ListActiveInstances returned error: %v", err)
	}

	if len(result.Items) != 3 {
		t.Fatalf("expected 3 active instances, got %d", len(result.Items))
	}
	if result.Items[0].InstanceID != "i-east-running" || result.Items[1].InstanceID != "i-east-stopped" || result.Items[2].InstanceID != "i-west-running" {
		t.Fatalf("unexpected item ordering: %+v", result.Items)
	}
	if result.NextCursor != "" {
		t.Fatalf("next cursor = %q, want empty for a full inventory response", result.NextCursor)
	}
	if len(result.CoveredRegions) != 2 || result.CoveredRegions[0] != "us-east-1" || result.CoveredRegions[1] != "us-west-2" {
		t.Fatalf("covered regions = %v, want [us-east-1 us-west-2]", result.CoveredRegions)
	}

	eastRunning := result.Items[0]
	if eastRunning.Name != "api-east" || eastRunning.Region != "us-east-1" || eastRunning.AvailabilityZone != "us-east-1a" {
		t.Fatalf("unexpected east instance identity: %+v", eastRunning)
	}
	if eastRunning.MarketType != provider.InstanceMarketTypeOnDemand {
		t.Fatalf("expected on-demand market type, got %+v", eastRunning)
	}
	if len(eastRunning.IPv6Addresses) != 1 || eastRunning.IPv6Addresses[0] != "2600:1f18::10" {
		t.Fatalf("unexpected ipv6 addresses: %+v", eastRunning.IPv6Addresses)
	}
	if len(eastClient.describeInstancesInputs) != 1 || len(westClient.describeInstancesInputs) != 1 {
		t.Fatalf("expected one DescribeInstances call per region, got east=%d west=%d", len(eastClient.describeInstancesInputs), len(westClient.describeInstancesInputs))
	}
	if !hasFilter(eastClient.describeInstancesInputs[0].Filters, "instance-state-name", activeInstanceStates...) {
		t.Fatalf("expected active state filter, got %+v", eastClient.describeInstancesInputs[0].Filters)
	}
}

func TestListActiveInstancesIncrementalInventoryScansOneRegionPerCall(t *testing.T) {
	eastClient := &listInstancesRecordingClient{
		fakeEC2Client: fakeEC2Client{
			regionsOutput: &ec2.DescribeRegionsOutput{
				Regions: []ec2types.Region{
					{RegionName: awsv2.String("us-east-1")},
					{RegionName: awsv2.String("us-west-2")},
				},
			},
		},
		describeInstancesOutput: &ec2.DescribeInstancesOutput{
			Reservations: []ec2types.Reservation{{
				Instances: []ec2types.Instance{
					testInstance("i-east-running", "api-east", "us-east-1a", "use1-az1", "c7g.medium", ec2types.InstanceStateNameRunning, "", nil, nil),
				},
			}},
		},
	}
	westClient := &listInstancesRecordingClient{
		describeInstancesOutput: &ec2.DescribeInstancesOutput{
			Reservations: []ec2types.Reservation{{
				Instances: []ec2types.Instance{
					testInstance("i-west-running", "api-west", "us-west-2b", "usw2-az2", "c7g.medium", ec2types.InstanceStateNameRunning, "", nil, nil),
				},
			}},
		},
	}

	service := &Service{
		version: "test",
		clientFactory: fakeClientFactory{
			clients: map[string]ec2API{
				"us-east-1": eastClient,
				"us-west-2": westClient,
			},
		},
	}

	result, err := service.ListActiveInstances(context.Background(), provider.ListActiveInstancesRequest{
		Credentials: provider.Credentials{
			AWS: &provider.AWSCredentials{
				AccessKeyID:     "ak",
				SecretAccessKey: "sk",
			},
		},
		Options: map[string]string{
			optionIncrementalInventory: "true",
		},
	})
	if err != nil {
		t.Fatalf("ListActiveInstances returned error: %v", err)
	}

	if len(result.Items) != 1 || result.Items[0].InstanceID != "i-east-running" {
		t.Fatalf("items = %+v, want first region slice only", result.Items)
	}
	if result.NextCursor != "us-west-2" {
		t.Fatalf("next cursor = %q, want us-west-2", result.NextCursor)
	}
	if len(result.CoveredRegions) != 1 || result.CoveredRegions[0] != "us-east-1" {
		t.Fatalf("covered regions = %v, want [us-east-1]", result.CoveredRegions)
	}
	if len(eastClient.describeInstancesInputs) != 1 || len(westClient.describeInstancesInputs) != 0 {
		t.Fatalf("expected one DescribeInstances call for east only, got east=%d west=%d", len(eastClient.describeInstancesInputs), len(westClient.describeInstancesInputs))
	}
}

func TestListActiveInstancesCursorAdvancesToNextRegion(t *testing.T) {
	eastClient := &listInstancesRecordingClient{
		fakeEC2Client: fakeEC2Client{
			regionsOutput: &ec2.DescribeRegionsOutput{
				Regions: []ec2types.Region{
					{RegionName: awsv2.String("us-east-1")},
					{RegionName: awsv2.String("us-west-2")},
				},
			},
		},
		describeInstancesOutput: &ec2.DescribeInstancesOutput{
			Reservations: []ec2types.Reservation{{
				Instances: []ec2types.Instance{
					testInstance("i-east-running", "api-east", "us-east-1a", "use1-az1", "c7g.medium", ec2types.InstanceStateNameRunning, "", nil, nil),
				},
			}},
		},
	}
	westClient := &listInstancesRecordingClient{
		describeInstancesOutput: &ec2.DescribeInstancesOutput{
			Reservations: []ec2types.Reservation{{
				Instances: []ec2types.Instance{
					testInstance("i-west-running", "api-west", "us-west-2b", "usw2-az2", "c7g.medium", ec2types.InstanceStateNameRunning, "", nil, nil),
				},
			}},
		},
	}

	service := &Service{
		version: "test",
		clientFactory: fakeClientFactory{
			clients: map[string]ec2API{
				"us-east-1": eastClient,
				"us-west-2": westClient,
			},
		},
	}

	result, err := service.ListActiveInstances(context.Background(), provider.ListActiveInstancesRequest{
		Credentials: provider.Credentials{
			AWS: &provider.AWSCredentials{
				AccessKeyID:     "ak",
				SecretAccessKey: "sk",
			},
		},
		Options: map[string]string{
			optionIncrementalInventory: "true",
			optionInventoryCursor:      "us-west-2",
		},
	})
	if err != nil {
		t.Fatalf("ListActiveInstances returned error: %v", err)
	}

	if len(result.Items) != 1 || result.Items[0].InstanceID != "i-west-running" {
		t.Fatalf("items = %+v, want west slice only", result.Items)
	}
	if result.NextCursor != "" {
		t.Fatalf("next cursor = %q, want empty at the end of the cycle", result.NextCursor)
	}
	if len(result.CoveredRegions) != 1 || result.CoveredRegions[0] != "us-west-2" {
		t.Fatalf("covered regions = %v, want [us-west-2]", result.CoveredRegions)
	}
	if len(eastClient.describeInstancesInputs) != 0 || len(westClient.describeInstancesInputs) != 1 {
		t.Fatalf("expected one DescribeInstances call for west only, got east=%d west=%d", len(eastClient.describeInstancesInputs), len(westClient.describeInstancesInputs))
	}
}

func TestListActiveInstancesAppliesRegionAZTypeAndTagFilters(t *testing.T) {
	eastClient := &listInstancesRecordingClient{
		describeInstancesOutput: &ec2.DescribeInstancesOutput{
			Reservations: []ec2types.Reservation{
				{
					Instances: []ec2types.Instance{
						testInstance("i-east-match", "match-east", "us-east-1a", "use1-az1", "c7g.medium", ec2types.InstanceStateNameRunning, "", []ec2types.Tag{
							{Key: awsv2.String("Name"), Value: awsv2.String("match-east")},
							{Key: awsv2.String("Environment"), Value: awsv2.String("prod")},
							{Key: awsv2.String("ArcoManaged"), Value: awsv2.String("true")},
						}, nil),
						testInstance("i-east-wrong-az", "skip-east-az", "us-east-1c", "use1-az6", "c7g.medium", ec2types.InstanceStateNameRunning, "", []ec2types.Tag{
							{Key: awsv2.String("Environment"), Value: awsv2.String("prod")},
							{Key: awsv2.String("ArcoManaged"), Value: awsv2.String("true")},
						}, nil),
					},
				},
			},
		},
	}
	westClient := &listInstancesRecordingClient{
		describeInstancesOutput: &ec2.DescribeInstancesOutput{
			Reservations: []ec2types.Reservation{
				{
					Instances: []ec2types.Instance{
						testInstance("i-west-match", "match-west", "us-west-2b", "usw2-az2", "c7g.medium", ec2types.InstanceStateNameRunning, ec2types.InstanceLifecycleTypeSpot, []ec2types.Tag{
							{Key: awsv2.String("Name"), Value: awsv2.String("match-west")},
							{Key: awsv2.String("Environment"), Value: awsv2.String("prod")},
							{Key: awsv2.String("ArcoManaged"), Value: awsv2.String("true")},
						}, nil),
						testInstance("i-west-wrong-type", "skip-west-type", "us-west-2b", "usw2-az2", "c6g.medium", ec2types.InstanceStateNameRunning, "", []ec2types.Tag{
							{Key: awsv2.String("Environment"), Value: awsv2.String("prod")},
							{Key: awsv2.String("ArcoManaged"), Value: awsv2.String("true")},
						}, nil),
						testInstance("i-west-wrong-tag", "skip-west-tag", "us-west-2b", "usw2-az2", "c7g.medium", ec2types.InstanceStateNameRunning, "", []ec2types.Tag{
							{Key: awsv2.String("Environment"), Value: awsv2.String("dev")},
							{Key: awsv2.String("ArcoManaged"), Value: awsv2.String("true")},
						}, nil),
					},
				},
			},
		},
	}

	service := &Service{
		version: "test",
		clientFactory: fakeClientFactory{
			clients: map[string]ec2API{
				"us-east-1": eastClient,
				"us-west-2": westClient,
			},
		},
	}

	result, err := service.ListActiveInstances(context.Background(), provider.ListActiveInstancesRequest{
		Credentials: provider.Credentials{
			AWS: &provider.AWSCredentials{
				AccessKeyID:     "ak",
				SecretAccessKey: "sk",
			},
		},
		Regions:           []string{" us-west-2 ", "us-east-1"},
		AvailabilityZones: []string{"us-east-1a", "us-west-2b"},
		InstanceTypes:     []string{" c7g.medium "},
		Tags: []provider.InstanceTag{
			{Key: "Environment", Value: "prod"},
			{Key: "ArcoManaged"},
		},
	})
	if err != nil {
		t.Fatalf("ListActiveInstances returned error: %v", err)
	}

	if len(result.Items) != 2 {
		t.Fatalf("expected 2 matching instances, got %d", len(result.Items))
	}
	if result.Items[0].InstanceID != "i-east-match" || result.Items[1].InstanceID != "i-west-match" {
		t.Fatalf("unexpected filtered items: %+v", result.Items)
	}
	if result.Items[1].MarketType != provider.InstanceMarketTypeSpot {
		t.Fatalf("expected west instance to be spot, got %+v", result.Items[1])
	}

	eastFilters := eastClient.describeInstancesInputs[0].Filters
	if !hasFilter(eastFilters, "instance-state-name", activeInstanceStates...) {
		t.Fatalf("expected active state filter, got %+v", eastFilters)
	}
	if !hasFilter(eastFilters, "instance-type", "c7g.medium") {
		t.Fatalf("expected instance type filter, got %+v", eastFilters)
	}
	if !hasFilter(eastFilters, "tag:Environment", "prod") {
		t.Fatalf("expected environment tag filter, got %+v", eastFilters)
	}
	if hasFilter(eastFilters, "tag:ArcoManaged", "true") {
		t.Fatalf("did not expect key-only tag filter to be pushed to DescribeInstances, got %+v", eastFilters)
	}
}

type listInstancesRecordingClient struct {
	fakeEC2Client
	describeInstancesInputs []*ec2.DescribeInstancesInput
	describeInstancesOutput *ec2.DescribeInstancesOutput
	describeInstancesErr    error
}

func (c *listInstancesRecordingClient) DescribeInstances(_ context.Context, input *ec2.DescribeInstancesInput, _ ...func(*ec2.Options)) (*ec2.DescribeInstancesOutput, error) {
	c.describeInstancesInputs = append(c.describeInstancesInputs, cloneDescribeInstancesInput(input))
	if c.describeInstancesErr != nil {
		return nil, c.describeInstancesErr
	}
	if c.describeInstancesOutput == nil {
		return &ec2.DescribeInstancesOutput{}, nil
	}
	return c.describeInstancesOutput, nil
}

func cloneDescribeInstancesInput(input *ec2.DescribeInstancesInput) *ec2.DescribeInstancesInput {
	if input == nil {
		return nil
	}

	clone := &ec2.DescribeInstancesInput{
		NextToken: input.NextToken,
	}
	if len(input.Filters) > 0 {
		clone.Filters = make([]ec2types.Filter, 0, len(input.Filters))
		for _, filter := range input.Filters {
			clonedFilter := ec2types.Filter{
				Name: filter.Name,
			}
			if len(filter.Values) > 0 {
				clonedFilter.Values = append([]string(nil), filter.Values...)
			}
			clone.Filters = append(clone.Filters, clonedFilter)
		}
	}

	return clone
}

func hasFilter(filters []ec2types.Filter, name string, values ...string) bool {
	for _, filter := range filters {
		if awsv2.ToString(filter.Name) != name {
			continue
		}
		if len(values) == 0 {
			return true
		}
		if len(filter.Values) != len(values) {
			continue
		}
		matched := true
		for index, value := range values {
			if filter.Values[index] != value {
				matched = false
				break
			}
		}
		if matched {
			return true
		}
	}

	return false
}

func testInstance(
	instanceID string,
	name string,
	availabilityZone string,
	availabilityZoneID string,
	instanceType string,
	state ec2types.InstanceStateName,
	lifecycle ec2types.InstanceLifecycleType,
	tags []ec2types.Tag,
	ipv6Addresses []string,
) ec2types.Instance {
	networkInterfaces := make([]ec2types.InstanceNetworkInterface, 0, 1)
	if len(ipv6Addresses) > 0 {
		addresses := make([]ec2types.InstanceIpv6Address, 0, len(ipv6Addresses))
		for _, address := range ipv6Addresses {
			addresses = append(addresses, ec2types.InstanceIpv6Address{
				Ipv6Address: awsv2.String(address),
			})
		}
		networkInterfaces = append(networkInterfaces, ec2types.InstanceNetworkInterface{
			Ipv6Addresses: addresses,
		})
	}

	if name != "" {
		tags = append(tags, ec2types.Tag{
			Key:   awsv2.String("Display"),
			Value: awsv2.String(name),
		})
	}

	return ec2types.Instance{
		InstanceId:        awsv2.String(instanceID),
		InstanceType:      ec2types.InstanceType(instanceType),
		InstanceLifecycle: lifecycle,
		LaunchTime:        awsv2.Time(time.Date(2026, 3, 16, 9, 30, 0, 0, time.UTC)),
		Placement: &ec2types.Placement{
			AvailabilityZone:   awsv2.String(availabilityZone),
			AvailabilityZoneId: awsv2.String(availabilityZoneID),
		},
		PrivateIpAddress:  awsv2.String("10.0.0.10"),
		PublicIpAddress:   awsv2.String("52.0.0.10"),
		State:             &ec2types.InstanceState{Name: state},
		SubnetId:          awsv2.String("subnet-123"),
		VpcId:             awsv2.String("vpc-123"),
		Tags:              tags,
		NetworkInterfaces: networkInterfaces,
	}
}
