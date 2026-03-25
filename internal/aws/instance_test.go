package aws

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"testing"

	"github.com/arcoloom/arco-provider-aws/internal/provider"
	awsv2 "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	ssmtypes "github.com/aws/aws-sdk-go-v2/service/ssm/types"
)

func TestStartInstanceDelegatesToRunner(t *testing.T) {
	runner := &fakeInstanceLifecycleRunner{
		startResult: provider.StartInstanceResult{
			StackName:  "stack-a",
			InstanceID: "i-123",
			PublicIP:   "1.2.3.4",
		},
	}
	service := &Service{
		version:        "test",
		clientFactory:  newAWSClientFactory(),
		instanceRunner: runner,
	}

	result, err := service.StartInstance(context.Background(), provider.StartInstanceRequest{
		Context: provider.RequestContext{
			RequestID: "start-instance:stack-a",
		},
		Credentials: provider.Credentials{
			AWS: &provider.AWSCredentials{
				AccessKeyID:     "ak",
				SecretAccessKey: "sk",
			},
		},
		Region:       " us-west-2 ",
		StackName:    " stack-a ",
		InstanceType: " m7i.large ",
		MarketType:   provider.InstanceMarketTypeSpot,
		UserData:     " echo hi ",
	})
	if err != nil {
		t.Fatalf("StartInstance returned error: %v", err)
	}

	if result.InstanceID != "i-123" {
		t.Fatalf("unexpected instance id: %+v", result)
	}
	if runner.startReq.Region != "us-west-2" {
		t.Fatalf("expected region to be normalized, got %q", runner.startReq.Region)
	}
	if runner.startReq.InstanceName != "stack-a" {
		t.Fatalf("expected instance name to default from stack name, got %+v", runner.startReq)
	}
	if runner.startReq.InstanceType != "m7i.large" {
		t.Fatalf("expected instance type to be normalized, got %+v", runner.startReq)
	}
	if runner.startReq.UserData != "echo hi" {
		t.Fatalf("expected user data to be trimmed, got %+v", runner.startReq)
	}
	if runner.startReq.MarketType != provider.InstanceMarketTypeSpot {
		t.Fatalf("unexpected market type: %s", runner.startReq.MarketType)
	}
}

func TestStartInstanceRequiresCoreFields(t *testing.T) {
	service := &Service{instanceRunner: &fakeInstanceLifecycleRunner{}}

	result, err := service.StartInstance(context.Background(), provider.StartInstanceRequest{})
	if err != nil {
		t.Fatalf("expected structured launch failure, got error: %v", err)
	}
	if result.LaunchFailure == nil {
		t.Fatal("expected launch failure")
	}
	if result.LaunchFailure.Class != provider.LaunchFailureClassAuth {
		t.Fatalf("launch failure class = %q, want %q", result.LaunchFailure.Class, provider.LaunchFailureClassAuth)
	}
	if result.LaunchFailure.Scope != provider.LaunchFailureScopeAccount {
		t.Fatalf("launch failure scope = %q, want %q", result.LaunchFailure.Scope, provider.LaunchFailureScopeAccount)
	}
}

func TestStartInstanceRequiresExplicitRegion(t *testing.T) {
	service := &Service{instanceRunner: &fakeInstanceLifecycleRunner{}}

	result, err := service.StartInstance(context.Background(), provider.StartInstanceRequest{
		Credentials: provider.Credentials{
			AWS: &provider.AWSCredentials{
				AccessKeyID:     "ak",
				SecretAccessKey: "sk",
			},
		},
		StackName:    "stack-a",
		InstanceType: "t3.micro",
		Scope: provider.ConnectionScope{
			Region: "us-west-2",
		},
	})
	if err != nil {
		t.Fatalf("expected structured launch failure, got %v", err)
	}
	if result.LaunchFailure == nil || result.LaunchFailure.Class != provider.LaunchFailureClassConfig {
		t.Fatalf("expected config launch failure, got %+v", result.LaunchFailure)
	}
	if !strings.Contains(result.LaunchFailure.Message, "region is required") {
		t.Fatalf("expected region validation message, got %+v", result.LaunchFailure)
	}
}

func TestStopInstanceDelegatesToRunner(t *testing.T) {
	runner := &fakeInstanceLifecycleRunner{
		stopResult: provider.StopInstanceResult{
			InstanceID: "i-stack-a",
			Destroyed:  true,
		},
	}
	service := &Service{
		version:        "test",
		clientFactory:  newAWSClientFactory(),
		instanceRunner: runner,
	}

	result, err := service.StopInstance(context.Background(), provider.StopInstanceRequest{
		Credentials: provider.Credentials{
			AWS: &provider.AWSCredentials{
				AccessKeyID:     "ak",
				SecretAccessKey: "sk",
			},
		},
		InstanceID: "i-stack-a",
		Region:     "us-west-2",
	})
	if err != nil {
		t.Fatalf("StopInstance returned error: %v", err)
	}

	if !result.Destroyed {
		t.Fatalf("expected destroyed result, got %+v", result)
	}
	if runner.stopReq.InstanceID != "i-stack-a" || runner.stopReq.Region != "us-west-2" {
		t.Fatalf("unexpected stop request: %+v", runner.stopReq)
	}
}

func TestStopInstanceDoesNotRequireScopeRegion(t *testing.T) {
	service := &Service{
		instanceRunner: &fakeInstanceLifecycleRunner{
			stopResult: provider.StopInstanceResult{
				InstanceID: "i-stack-a",
				Destroyed:  true,
			},
		},
	}

	result, err := service.StopInstance(context.Background(), provider.StopInstanceRequest{
		Credentials: provider.Credentials{
			AWS: &provider.AWSCredentials{
				AccessKeyID:     "ak",
				SecretAccessKey: "sk",
			},
		},
		InstanceID: "i-stack-a",
		Region:     "us-west-2",
	})
	if err != nil {
		t.Fatalf("expected stop request without scope region to pass validation, got %v", err)
	}
	if !result.Destroyed {
		t.Fatalf("expected destroyed result, got %+v", result)
	}
}

func TestStartInstanceRoutesSelectedAccountCredentials(t *testing.T) {
	runner := &fakeInstanceLifecycleRunner{
		startResult: provider.StartInstanceResult{
			StackName:  "stack-a",
			InstanceID: "i-123",
		},
	}
	service := &Service{
		version:        "test",
		clientFactory:  newAWSClientFactory(),
		instanceRunner: runner,
	}

	req := provider.StartInstanceRequest{
		Credentials: provider.Credentials{
			AWSAccounts: []provider.AWSAccount{
				{
					Name: "acct-a",
					Credentials: provider.AWSCredentials{
						AccessKeyID:     "ak-a",
						SecretAccessKey: "sk-a",
					},
				},
				{
					Name: "acct-b",
					Credentials: provider.AWSCredentials{
						AccessKeyID:     "ak-b",
						SecretAccessKey: "sk-b",
					},
				},
			},
		},
		ScopeID:      resolveInternalScopeID("acct-b", provider.AWSCredentials{AccessKeyID: "ak-b", SecretAccessKey: "sk-b"}, provider.ConnectionScope{}),
		Region:       "us-west-2",
		StackName:    "stack-a",
		InstanceType: "m7i.large",
	}

	if _, err := service.StartInstance(context.Background(), req); err != nil {
		t.Fatalf("StartInstance returned error: %v", err)
	}
	if runner.startReq.Credentials.AWS == nil {
		t.Fatal("expected routed AWS credentials")
	}
	if runner.startReq.Credentials.AWS.AccessKeyID != "ak-b" {
		t.Fatalf("routed access key = %q, want %q", runner.startReq.Credentials.AWS.AccessKeyID, "ak-b")
	}
	if runner.startReq.ScopeID != req.ScopeID {
		t.Fatalf("routed scope id = %q, want %q", runner.startReq.ScopeID, req.ScopeID)
	}
}

func TestStopInstanceRoutesSelectedAccountCredentials(t *testing.T) {
	runner := &fakeInstanceLifecycleRunner{
		stopResult: provider.StopInstanceResult{
			InstanceID: "i-stack-a",
			Destroyed:  true,
		},
	}
	service := &Service{
		version:        "test",
		clientFactory:  newAWSClientFactory(),
		instanceRunner: runner,
	}

	req := provider.StopInstanceRequest{
		Credentials: provider.Credentials{
			AWSAccounts: []provider.AWSAccount{
				{
					Name: "acct-a",
					Credentials: provider.AWSCredentials{
						AccessKeyID:     "ak-a",
						SecretAccessKey: "sk-a",
					},
				},
				{
					Name: "acct-b",
					Credentials: provider.AWSCredentials{
						AccessKeyID:     "ak-b",
						SecretAccessKey: "sk-b",
					},
				},
			},
		},
		ScopeID:    resolveInternalScopeID("acct-a", provider.AWSCredentials{AccessKeyID: "ak-a", SecretAccessKey: "sk-a"}, provider.ConnectionScope{}),
		InstanceID: "i-stack-a",
		Region:     "us-west-2",
	}

	if _, err := service.StopInstance(context.Background(), req); err != nil {
		t.Fatalf("StopInstance returned error: %v", err)
	}
	if runner.stopReq.Credentials.AWS == nil {
		t.Fatal("expected routed AWS credentials")
	}
	if runner.stopReq.Credentials.AWS.AccessKeyID != "ak-a" {
		t.Fatalf("routed access key = %q, want %q", runner.stopReq.Credentials.AWS.AccessKeyID, "ak-a")
	}
	if runner.stopReq.ScopeID != req.ScopeID {
		t.Fatalf("routed scope id = %q, want %q", runner.stopReq.ScopeID, req.ScopeID)
	}
}

func TestEC2RunnerDryRunStartAndStop(t *testing.T) {
	runner := newInstanceLifecycleRunner(nil)

	startResult, err := runner.Start(context.Background(), provider.StartInstanceRequest{
		Credentials: provider.Credentials{
			AWS: &provider.AWSCredentials{AccessKeyID: "ak", SecretAccessKey: "sk"},
		},
		StackName:    "stack-a",
		InstanceName: "stack-a",
		InstanceType: "t3.nano",
		Options:      map[string]string{"dry_run": "true"},
	})
	if err != nil {
		t.Fatalf("dry-run start returned error: %v", err)
	}
	if !strings.HasPrefix(startResult.InstanceID, dryRunInstanceIDPrefix) {
		t.Fatalf("expected dry-run instance id, got %+v", startResult)
	}
	if len(startResult.Warnings) != 1 || startResult.Warnings[0].Code != warningCodeDryRun {
		t.Fatalf("unexpected dry-run start warnings: %+v", startResult.Warnings)
	}

	stopResult, err := runner.Stop(context.Background(), provider.StopInstanceRequest{
		Credentials: provider.Credentials{
			AWS: &provider.AWSCredentials{AccessKeyID: "ak", SecretAccessKey: "sk"},
		},
		InstanceID: "i-stack-a",
		Region:     "us-west-2",
		Options:    map[string]string{"dry_run": "true"},
	})
	if err != nil {
		t.Fatalf("dry-run stop returned error: %v", err)
	}
	if !stopResult.Destroyed {
		t.Fatalf("expected dry-run stop to report destroyed, got %+v", stopResult)
	}
	if len(stopResult.Warnings) != 1 || stopResult.Warnings[0].Code != warningCodeDryRun {
		t.Fatalf("unexpected dry-run stop warnings: %+v", stopResult.Warnings)
	}
}

func TestStartInstanceCreatesEC2InstanceViaAWSSDK(t *testing.T) {
	ec2Client := &recordingEC2Client{
		fakeEC2Client: fakeEC2Client{
			imagesOutput: &ec2.DescribeImagesOutput{
				Images: []ec2types.Image{
					{RootDeviceName: awsv2.String("/dev/xvda")},
				},
			},
		},
		describeInstanceTypesOutput: &ec2.DescribeInstanceTypesOutput{
			InstanceTypes: []ec2types.InstanceTypeInfo{
				{
					ProcessorInfo: &ec2types.ProcessorInfo{
						SupportedArchitectures: []ec2types.ArchitectureType{ec2types.ArchitectureTypeX8664},
					},
				},
			},
		},
		runInstancesOutput: &ec2.RunInstancesOutput{
			Instances: []ec2types.Instance{
				{
					InstanceId:       awsv2.String("i-1234567890"),
					PublicIpAddress:  awsv2.String("1.2.3.4"),
					PrivateIpAddress: awsv2.String("10.0.0.10"),
				},
			},
		},
	}
	ssmClient := &recordingSSMClient{
		pathOutput: &ssm.GetParametersByPathOutput{
			Parameters: []ssmtypes.Parameter{
				{
					Name:  awsv2.String("/aws/service/debian/release/13/latest/amd64"),
					Value: awsv2.String("ami-1234567890"),
				},
				{
					Name:  awsv2.String("/aws/service/debian/release/13/latest/arm64"),
					Value: awsv2.String("ami-should-not-match"),
				},
			},
		},
	}
	factory := instanceTestClientFactory{
		ec2Client: ec2Client,
		ssmClient: ssmClient,
	}
	service := &Service{
		version:        "test",
		clientFactory:  factory,
		instanceRunner: newInstanceLifecycleRunner(factory),
	}

	result, err := service.StartInstance(context.Background(), provider.StartInstanceRequest{
		Context: provider.RequestContext{
			RequestID: "start-instance:stack-a",
		},
		Credentials: provider.Credentials{
			AWS: &provider.AWSCredentials{
				AccessKeyID:     "ak",
				SecretAccessKey: "sk",
			},
		},
		Region:       "us-west-2",
		StackName:    " stack-a ",
		InstanceType: " t3.micro ",
		Scope:        provider.ConnectionScope{Region: "us-west-2"},
		UserData:     "echo hello",
		Tags: []provider.InstanceTag{
			{Key: "Environment", Value: "test"},
		},
		ProviderConfig: map[string]any{
			"subnet_id":          "subnet-123",
			"security_group_ids": []any{"sg-123"},
			"key_name":           "demo-key",
		},
	})
	if err != nil {
		t.Fatalf("StartInstance returned error: %v", err)
	}

	if result.InstanceID != "i-1234567890" {
		t.Fatalf("unexpected start result: %+v", result)
	}
	if ssmClient.pathInput == nil || awsv2.ToString(ssmClient.pathInput.Path) != debian13AMIPathRoot {
		t.Fatalf("expected debian 13 SSM path lookup, got %+v", ssmClient.pathInput)
	}
	if ec2Client.runInstancesInput == nil {
		t.Fatal("expected RunInstances to be called")
	}
	if got := awsv2.ToString(ec2Client.runInstancesInput.ImageId); got != "ami-1234567890" {
		t.Fatalf("unexpected image id: %q", got)
	}
	if got := string(ec2Client.runInstancesInput.InstanceType); got != "t3.micro" {
		t.Fatalf("unexpected instance type: %q", got)
	}
	if got := awsv2.ToString(ec2Client.runInstancesInput.SubnetId); got != "subnet-123" {
		t.Fatalf("unexpected subnet id: %q", got)
	}
	if got := awsv2.ToString(ec2Client.runInstancesInput.KeyName); got != "demo-key" {
		t.Fatalf("unexpected key name: %q", got)
	}
	if got := awsv2.ToString(ec2Client.runInstancesInput.ClientToken); got != clientTokenForRequestID("start-instance:stack-a") {
		t.Fatalf("unexpected client token: %q", got)
	}
	if got := awsv2.ToString(ec2Client.runInstancesInput.UserData); got != base64.StdEncoding.EncodeToString([]byte("echo hello")) {
		t.Fatalf("unexpected user data: %q", got)
	}
	if got := ec2Client.runInstancesInput.SecurityGroupIds; len(got) != 1 || got[0] != "sg-123" {
		t.Fatalf("unexpected security groups: %+v", got)
	}

	tags := toTagMap(ec2Client.runInstancesInput.TagSpecifications)
	if tags["Name"] != "stack-a" || tags[stackTagKey] != "stack-a" || tags["ManagedBy"] != managedByTagValue || tags["Environment"] != "test" {
		t.Fatalf("unexpected instance tags: %+v", tags)
	}
}

func TestStartInstanceResolvesDefaultNetworkAndRootVolumeFromOptions(t *testing.T) {
	ec2Client := &recordingEC2Client{
		fakeEC2Client: fakeEC2Client{
			vpcsOutput: &ec2.DescribeVpcsOutput{
				Vpcs: []ec2types.Vpc{
					{
						VpcId: awsv2.String("vpc-default"),
						Ipv6CidrBlockAssociationSet: []ec2types.VpcIpv6CidrBlockAssociation{
							{Ipv6CidrBlock: awsv2.String("2600:1f14:abcd:ef00::/56")},
						},
					},
				},
			},
			internetGatewaysOutput: &ec2.DescribeInternetGatewaysOutput{
				InternetGateways: []ec2types.InternetGateway{
					{
						InternetGatewayId: awsv2.String("igw-default"),
						Attachments: []ec2types.InternetGatewayAttachment{
							{
								VpcId: awsv2.String("vpc-default"),
								State: ec2types.AttachmentStatusAttached,
							},
						},
					},
				},
			},
			routeTablesOutput: &ec2.DescribeRouteTablesOutput{
				RouteTables: []ec2types.RouteTable{
					{
						RouteTableId: awsv2.String("rtb-default"),
						Routes: []ec2types.Route{
							{
								DestinationCidrBlock: awsv2.String("0.0.0.0/0"),
								GatewayId:            awsv2.String("igw-default"),
							},
							{
								DestinationIpv6CidrBlock: awsv2.String("::/0"),
								GatewayId:                awsv2.String("igw-default"),
							},
						},
						Associations: []ec2types.RouteTableAssociation{
							{
								SubnetId: awsv2.String("subnet-ipv6"),
							},
						},
					},
				},
			},
			subnetsOutput: &ec2.DescribeSubnetsOutput{
				Subnets: []ec2types.Subnet{
					{
						SubnetId:                    awsv2.String("subnet-ipv6"),
						AvailabilityZone:            awsv2.String("us-east-1b"),
						CidrBlock:                   awsv2.String("10.77.0.0/24"),
						MapPublicIpOnLaunch:         awsv2.Bool(true),
						AssignIpv6AddressOnCreation: awsv2.Bool(true),
						Ipv6CidrBlockAssociationSet: []ec2types.SubnetIpv6CidrBlockAssociation{
							{
								Ipv6CidrBlockState: &ec2types.SubnetCidrBlockState{
									State: ec2types.SubnetCidrBlockStateCodeAssociated,
								},
							},
						},
					},
				},
			},
			securityGroupsOutput: &ec2.DescribeSecurityGroupsOutput{
				SecurityGroups: []ec2types.SecurityGroup{
					{GroupId: awsv2.String("sg-default")},
				},
			},
			imagesOutput: &ec2.DescribeImagesOutput{
				Images: []ec2types.Image{
					{RootDeviceName: awsv2.String("/dev/xvda")},
				},
			},
		},
		describeInstanceTypesOutput: &ec2.DescribeInstanceTypesOutput{
			InstanceTypes: []ec2types.InstanceTypeInfo{
				{
					ProcessorInfo: &ec2types.ProcessorInfo{
						SupportedArchitectures: []ec2types.ArchitectureType{ec2types.ArchitectureTypeArm64},
					},
				},
			},
		},
		runInstancesOutput: &ec2.RunInstancesOutput{
			Instances: []ec2types.Instance{
				{InstanceId: awsv2.String("i-spot")},
			},
		},
	}
	ssmClient := &recordingSSMClient{
		pathOutput: &ssm.GetParametersByPathOutput{
			Parameters: []ssmtypes.Parameter{
				{
					Name:  awsv2.String("/aws/service/debian/release/13/latest/arm64/ami-id"),
					Value: awsv2.String("ami-arm64"),
				},
			},
		},
	}
	factory := instanceTestClientFactory{
		ec2Client: ec2Client,
		ssmClient: ssmClient,
	}
	service := &Service{
		version:        "test",
		clientFactory:  factory,
		instanceRunner: newInstanceLifecycleRunner(factory),
	}

	_, err := service.StartInstance(context.Background(), provider.StartInstanceRequest{
		Credentials: provider.Credentials{
			AWS: &provider.AWSCredentials{
				AccessKeyID:     "ak",
				SecretAccessKey: "sk",
			},
		},
		Region:           "us-east-1",
		AvailabilityZone: "us-east-1b",
		StackName:        "stack-spot",
		InstanceType:     "c7g.medium",
		MarketType:       provider.InstanceMarketTypeSpot,
		ProviderConfig: map[string]any{
			optionUseDefaultVPC:           true,
			optionUseDefaultSecurityGroup: true,
			providerConfigNetworkMode:     providerNetworkModeIPv6,
			optionRootVolumeSizeGiB:       int64(20),
		},
	})
	if err != nil {
		t.Fatalf("StartInstance returned error: %v", err)
	}

	if ec2Client.runInstancesInput == nil {
		t.Fatal("expected RunInstances to be called")
	}
	if ec2Client.runInstancesInput.SubnetId != nil {
		t.Fatalf("expected subnet to be configured through network interfaces, got %+v", ec2Client.runInstancesInput.SubnetId)
	}
	if len(ec2Client.runInstancesInput.SecurityGroupIds) != 0 {
		t.Fatalf("expected security groups to be configured through network interfaces, got %+v", ec2Client.runInstancesInput.SecurityGroupIds)
	}
	if len(ec2Client.runInstancesInput.NetworkInterfaces) != 1 {
		t.Fatalf("expected a single network interface, got %+v", ec2Client.runInstancesInput.NetworkInterfaces)
	}

	networkInterface := ec2Client.runInstancesInput.NetworkInterfaces[0]
	if got := awsv2.ToString(networkInterface.SubnetId); got != "subnet-ipv6" {
		t.Fatalf("unexpected subnet id: %q", got)
	}
	if got := networkInterface.Groups; len(got) != 1 || got[0] != "sg-default" {
		t.Fatalf("unexpected network interface groups: %+v", got)
	}
	if got := awsv2.ToBool(networkInterface.AssociatePublicIpAddress); got {
		t.Fatalf("expected public ipv4 association to be disabled, got %+v", networkInterface.AssociatePublicIpAddress)
	}
	if got := awsv2.ToInt32(networkInterface.Ipv6AddressCount); got != 1 {
		t.Fatalf("expected a single ipv6 address, got %d", got)
	}
	if got := len(ec2Client.runInstancesInput.BlockDeviceMappings); got != 1 {
		t.Fatalf("expected a single block device mapping, got %d", got)
	}

	rootBlockDevice := ec2Client.runInstancesInput.BlockDeviceMappings[0]
	if got := awsv2.ToString(rootBlockDevice.DeviceName); got != "/dev/xvda" {
		t.Fatalf("unexpected root device name: %q", got)
	}
	if rootBlockDevice.Ebs == nil {
		t.Fatal("expected root block device to include ebs configuration")
	}
	if got := awsv2.ToInt32(rootBlockDevice.Ebs.VolumeSize); got != 20 {
		t.Fatalf("unexpected root volume size: %d", got)
	}
	if got := rootBlockDevice.Ebs.VolumeType; got != ec2types.VolumeTypeGp3 {
		t.Fatalf("unexpected root volume type: %s", got)
	}
	if !awsv2.ToBool(rootBlockDevice.Ebs.DeleteOnTermination) {
		t.Fatalf("expected root volume delete_on_termination, got %+v", rootBlockDevice.Ebs)
	}
	if ec2Client.runInstancesInput.InstanceMarketOptions == nil || ec2Client.runInstancesInput.InstanceMarketOptions.MarketType != ec2types.MarketTypeSpot {
		t.Fatalf("expected spot market options, got %+v", ec2Client.runInstancesInput.InstanceMarketOptions)
	}
	if ec2Client.createVpcInput != nil || ec2Client.createInternetGatewayInput != nil || ec2Client.createRouteTableInput != nil || ec2Client.createSubnetInput != nil || ec2Client.createSecurityGroupInput != nil {
		t.Fatalf("expected managed network resources to be reused, got creates vpc=%+v igw=%+v routeTable=%+v subnet=%+v sg=%+v", ec2Client.createVpcInput, ec2Client.createInternetGatewayInput, ec2Client.createRouteTableInput, ec2Client.createSubnetInput, ec2Client.createSecurityGroupInput)
	}
}

func TestStartInstanceCreatesManagedNetworkWhenMissing(t *testing.T) {
	ec2Client := &recordingEC2Client{
		fakeEC2Client: fakeEC2Client{
			vpcsOutput:             &ec2.DescribeVpcsOutput{},
			internetGatewaysOutput: &ec2.DescribeInternetGatewaysOutput{},
			routeTablesOutput:      &ec2.DescribeRouteTablesOutput{},
			subnetsOutput:          &ec2.DescribeSubnetsOutput{},
			securityGroupsOutput:   &ec2.DescribeSecurityGroupsOutput{},
			imagesOutput: &ec2.DescribeImagesOutput{
				Images: []ec2types.Image{
					{RootDeviceName: awsv2.String("/dev/xvda")},
				},
			},
		},
		createVpcOutput: &ec2.CreateVpcOutput{
			Vpc: &ec2types.Vpc{
				VpcId: awsv2.String("vpc-arco"),
				Ipv6CidrBlockAssociationSet: []ec2types.VpcIpv6CidrBlockAssociation{
					{Ipv6CidrBlock: awsv2.String("2600:1f14:abcd:ef00::/56")},
				},
			},
		},
		createInternetGatewayOutput: &ec2.CreateInternetGatewayOutput{
			InternetGateway: &ec2types.InternetGateway{
				InternetGatewayId: awsv2.String("igw-arco"),
			},
		},
		createRouteTableOutput: &ec2.CreateRouteTableOutput{
			RouteTable: &ec2types.RouteTable{
				RouteTableId: awsv2.String("rtb-arco"),
			},
		},
		createSubnetOutput: &ec2.CreateSubnetOutput{
			Subnet: &ec2types.Subnet{
				SubnetId:                    awsv2.String("subnet-arco"),
				AvailabilityZone:            awsv2.String("us-west-1d"),
				CidrBlock:                   awsv2.String("10.77.0.0/24"),
				MapPublicIpOnLaunch:         awsv2.Bool(false),
				AssignIpv6AddressOnCreation: awsv2.Bool(false),
				Ipv6CidrBlockAssociationSet: []ec2types.SubnetIpv6CidrBlockAssociation{
					{
						Ipv6CidrBlock: awsv2.String("2600:1f14:abcd:ef00::/64"),
						Ipv6CidrBlockState: &ec2types.SubnetCidrBlockState{
							State: ec2types.SubnetCidrBlockStateCodeAssociated,
						},
					},
				},
			},
		},
		createSecurityGroupOutput: &ec2.CreateSecurityGroupOutput{
			GroupId: awsv2.String("sg-arco"),
		},
		describeInstanceTypesOutput: &ec2.DescribeInstanceTypesOutput{
			InstanceTypes: []ec2types.InstanceTypeInfo{
				{
					ProcessorInfo: &ec2types.ProcessorInfo{
						SupportedArchitectures: []ec2types.ArchitectureType{ec2types.ArchitectureTypeX8664},
					},
				},
			},
		},
		runInstancesOutput: &ec2.RunInstancesOutput{
			Instances: []ec2types.Instance{
				{InstanceId: awsv2.String("i-arco")},
			},
		},
	}
	ssmClient := &recordingSSMClient{
		pathOutput: &ssm.GetParametersByPathOutput{
			Parameters: []ssmtypes.Parameter{
				{
					Name:  awsv2.String("/aws/service/debian/release/13/latest/amd64/ami-id"),
					Value: awsv2.String("ami-arco"),
				},
			},
		},
	}
	factory := instanceTestClientFactory{
		ec2Client: ec2Client,
		ssmClient: ssmClient,
	}
	service := &Service{
		version:        "test",
		clientFactory:  factory,
		instanceRunner: newInstanceLifecycleRunner(factory),
	}

	_, err := service.StartInstance(context.Background(), provider.StartInstanceRequest{
		Credentials: provider.Credentials{
			AWS: &provider.AWSCredentials{
				AccessKeyID:     "ak",
				SecretAccessKey: "sk",
			},
		},
		Region:           "us-west-1",
		AvailabilityZone: "us-west-1d",
		StackName:        "stack-shared",
		InstanceType:     "c7i.large",
	})
	if err != nil {
		t.Fatalf("StartInstance returned error: %v", err)
	}

	if ec2Client.createVpcInput == nil {
		t.Fatal("expected managed vpc to be created")
	}
	if got := awsv2.ToString(ec2Client.createVpcInput.CidrBlock); got != managedNetworkVPCCIDR {
		t.Fatalf("unexpected managed vpc cidr: %q", got)
	}
	if tags := toTagMap(ec2Client.createVpcInput.TagSpecifications); tags["Name"] != managedNetworkVPCName {
		t.Fatalf("unexpected managed vpc tags: %+v", tags)
	}
	if ec2Client.attachInternetGatewayInput == nil || awsv2.ToString(ec2Client.attachInternetGatewayInput.VpcId) != "vpc-arco" {
		t.Fatalf("expected internet gateway attachment to vpc-arco, got %+v", ec2Client.attachInternetGatewayInput)
	}
	if ec2Client.createRouteTableInput == nil || awsv2.ToString(ec2Client.createRouteTableInput.VpcId) != "vpc-arco" {
		t.Fatalf("expected managed route table for vpc-arco, got %+v", ec2Client.createRouteTableInput)
	}
	if len(ec2Client.createRouteInputs) != 2 {
		t.Fatalf("expected ipv4 and ipv6 default routes, got %+v", ec2Client.createRouteInputs)
	}
	if ec2Client.createSubnetInput == nil {
		t.Fatal("expected managed subnet to be created")
	}
	if got := awsv2.ToString(ec2Client.createSubnetInput.AvailabilityZone); got != "us-west-1d" {
		t.Fatalf("unexpected managed subnet availability zone: %q", got)
	}
	if tags := toTagMap(ec2Client.createSubnetInput.TagSpecifications); tags["Name"] != managedNetworkSubnetName {
		t.Fatalf("unexpected managed subnet tags: %+v", tags)
	}
	if ec2Client.createSecurityGroupInput == nil {
		t.Fatal("expected managed security group to be created")
	}
	if got := awsv2.ToString(ec2Client.createSecurityGroupInput.GroupName); got != managedNetworkSecurityGroupName {
		t.Fatalf("unexpected managed security group name: %q", got)
	}
	authorizedEgress := flattenAuthorizedEgressPermissions(ec2Client.authorizeSecurityGroupEgressInputs)
	if got := len(authorizedEgress); got != 2 {
		t.Fatalf("expected ipv4 and ipv6 outbound rules to be enforced on the new managed security group, got %+v", authorizedEgress)
	}
	if !containsEgressCIDR(authorizedEgress, "0.0.0.0/0", "") {
		t.Fatalf("expected ipv4 outbound rule on the new managed security group, got %+v", authorizedEgress)
	}
	if !containsEgressCIDR(authorizedEgress, "", "::/0") {
		t.Fatalf("expected ipv6 outbound rule on the new managed security group, got %+v", authorizedEgress)
	}
	if got := len(ec2Client.modifyVpcAttributeInputs); got != 2 {
		t.Fatalf("expected dns support and hostnames to be enabled on the managed vpc, got %d calls", got)
	}
	if got := len(ec2Client.modifySubnetAttributeInputs); got != 2 {
		t.Fatalf("expected public ipv4 and ipv6 auto-assignment to be enabled on the managed subnet, got %d calls", got)
	}
	if ec2Client.runInstancesInput == nil {
		t.Fatal("expected RunInstances to be called")
	}
	if got := awsv2.ToString(ec2Client.runInstancesInput.SubnetId); got != "subnet-arco" {
		t.Fatalf("unexpected run subnet id: %q", got)
	}
	if got := ec2Client.runInstancesInput.SecurityGroupIds; len(got) != 1 || got[0] != "sg-arco" {
		t.Fatalf("unexpected run security groups: %+v", got)
	}
	if ec2Client.runInstancesInput.Placement == nil || awsv2.ToString(ec2Client.runInstancesInput.Placement.AvailabilityZone) != "us-west-1d" {
		t.Fatalf("unexpected instance placement: %+v", ec2Client.runInstancesInput.Placement)
	}
}

func TestEnsureManagedSecurityGroupEnforcesOutboundOnly(t *testing.T) {
	ec2Client := &recordingEC2Client{
		fakeEC2Client: fakeEC2Client{
			securityGroupsOutput: &ec2.DescribeSecurityGroupsOutput{
				SecurityGroups: []ec2types.SecurityGroup{
					{
						GroupId:   awsv2.String("sg-arco"),
						GroupName: awsv2.String(managedNetworkSecurityGroupName),
						VpcId:     awsv2.String("vpc-arco"),
						IpPermissions: []ec2types.IpPermission{
							{
								IpProtocol: awsv2.String("tcp"),
								FromPort:   awsv2.Int32(22),
								ToPort:     awsv2.Int32(22),
								IpRanges: []ec2types.IpRange{
									{CidrIp: awsv2.String("0.0.0.0/0")},
								},
							},
						},
					},
				},
			},
		},
	}

	securityGroup, err := ensureManagedSecurityGroup(context.Background(), ec2Client, "us-west-1", "vpc-arco")
	if err != nil {
		t.Fatalf("ensureManagedSecurityGroup() error = %v", err)
	}
	if got := awsv2.ToString(securityGroup.GroupId); got != "sg-arco" {
		t.Fatalf("unexpected managed security group id: %q", got)
	}
	if ec2Client.revokeSecurityGroupIngressInput == nil {
		t.Fatal("expected ingress rules to be revoked")
	}
	if got := len(ec2Client.revokeSecurityGroupIngressInput.IpPermissions); got != 1 {
		t.Fatalf("expected a single ingress rule revoke, got %d", got)
	}
	authorizedEgress := flattenAuthorizedEgressPermissions(ec2Client.authorizeSecurityGroupEgressInputs)
	if got := len(authorizedEgress); got != 2 {
		t.Fatalf("expected ipv4 and ipv6 outbound rules, got %+v", authorizedEgress)
	}
	if !containsEgressCIDR(authorizedEgress, "0.0.0.0/0", "") {
		t.Fatalf("expected ipv4 egress rule, got %+v", authorizedEgress)
	}
	if !containsEgressCIDR(authorizedEgress, "", "::/0") {
		t.Fatalf("expected ipv6 egress rule, got %+v", authorizedEgress)
	}
}

func TestEnsureManagedVPCIPv6WaitsForAssociatedCIDR(t *testing.T) {
	ec2Client := &recordingEC2Client{
		describeVpcsOutputs: []*ec2.DescribeVpcsOutput{
			{
				Vpcs: []ec2types.Vpc{
					{
						VpcId: awsv2.String("vpc-arco"),
						Ipv6CidrBlockAssociationSet: []ec2types.VpcIpv6CidrBlockAssociation{
							{
								Ipv6CidrBlock: awsv2.String("2600:1f14:abcd:ef00::/56"),
								Ipv6CidrBlockState: &ec2types.VpcCidrBlockState{
									State: ec2types.VpcCidrBlockStateCodeAssociating,
								},
							},
						},
					},
				},
			},
			{
				Vpcs: []ec2types.Vpc{
					{
						VpcId: awsv2.String("vpc-arco"),
						Ipv6CidrBlockAssociationSet: []ec2types.VpcIpv6CidrBlockAssociation{
							{
								Ipv6CidrBlock: awsv2.String("2600:1f14:abcd:ef00::/56"),
								Ipv6CidrBlockState: &ec2types.VpcCidrBlockState{
									State: ec2types.VpcCidrBlockStateCodeAssociated,
								},
							},
						},
					},
				},
			},
		},
	}

	vpc, err := ensureManagedVPCIPv6(context.Background(), ec2Client, "us-west-1", ec2types.Vpc{
		VpcId: awsv2.String("vpc-arco"),
		Ipv6CidrBlockAssociationSet: []ec2types.VpcIpv6CidrBlockAssociation{
			{
				Ipv6CidrBlock: awsv2.String("2600:1f14:abcd:ef00::/56"),
				Ipv6CidrBlockState: &ec2types.VpcCidrBlockState{
					State: ec2types.VpcCidrBlockStateCodeAssociating,
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("ensureManagedVPCIPv6() error = %v", err)
	}
	if ec2Client.associateVpcCidrBlockInput != nil {
		t.Fatalf("expected existing ipv6 association to be awaited, not recreated: %+v", ec2Client.associateVpcCidrBlockInput)
	}
	if got := managedVPCIPv6CIDR(vpc); got != "2600:1f14:abcd:ef00::/56" {
		t.Fatalf("unexpected managed vpc ipv6 cidr after wait: %q", got)
	}
}

func TestEnsureManagedSubnetReadyWaitsForAssociatedCIDRBeforeIPv6AutoAssign(t *testing.T) {
	ec2Client := &recordingEC2Client{
		describeSubnetsOutputs: []*ec2.DescribeSubnetsOutput{
			{
				Subnets: []ec2types.Subnet{
					{
						SubnetId:         awsv2.String("subnet-arco"),
						CidrBlock:        awsv2.String("10.77.0.0/24"),
						AvailabilityZone: awsv2.String("us-west-1d"),
						Ipv6CidrBlockAssociationSet: []ec2types.SubnetIpv6CidrBlockAssociation{
							{
								Ipv6CidrBlock: awsv2.String("2600:1f14:abcd:ef00::/64"),
								Ipv6CidrBlockState: &ec2types.SubnetCidrBlockState{
									State: ec2types.SubnetCidrBlockStateCodeAssociating,
								},
							},
						},
					},
				},
			},
			{
				Subnets: []ec2types.Subnet{
					{
						SubnetId:                    awsv2.String("subnet-arco"),
						CidrBlock:                   awsv2.String("10.77.0.0/24"),
						AvailabilityZone:            awsv2.String("us-west-1d"),
						MapPublicIpOnLaunch:         awsv2.Bool(false),
						AssignIpv6AddressOnCreation: awsv2.Bool(false),
						Ipv6CidrBlockAssociationSet: []ec2types.SubnetIpv6CidrBlockAssociation{
							{
								Ipv6CidrBlock: awsv2.String("2600:1f14:abcd:ef00::/64"),
								Ipv6CidrBlockState: &ec2types.SubnetCidrBlockState{
									State: ec2types.SubnetCidrBlockStateCodeAssociated,
								},
							},
						},
					},
				},
			},
		},
	}

	subnet, err := ensureManagedSubnetReady(
		context.Background(),
		ec2Client,
		"us-west-1",
		ec2types.Vpc{
			VpcId: awsv2.String("vpc-arco"),
			Ipv6CidrBlockAssociationSet: []ec2types.VpcIpv6CidrBlockAssociation{
				{
					Ipv6CidrBlock: awsv2.String("2600:1f14:abcd:ef00::/56"),
					Ipv6CidrBlockState: &ec2types.VpcCidrBlockState{
						State: ec2types.VpcCidrBlockStateCodeAssociated,
					},
				},
			},
		},
		ec2types.RouteTable{},
		ec2types.Subnet{
			SubnetId:         awsv2.String("subnet-arco"),
			CidrBlock:        awsv2.String("10.77.0.0/24"),
			AvailabilityZone: awsv2.String("us-west-1d"),
			Ipv6CidrBlockAssociationSet: []ec2types.SubnetIpv6CidrBlockAssociation{
				{
					Ipv6CidrBlock: awsv2.String("2600:1f14:abcd:ef00::/64"),
					Ipv6CidrBlockState: &ec2types.SubnetCidrBlockState{
						State: ec2types.SubnetCidrBlockStateCodeAssociating,
					},
				},
			},
		},
		nil,
	)
	if err != nil {
		t.Fatalf("ensureManagedSubnetReady() error = %v", err)
	}
	if ec2Client.associateSubnetCidrBlockInput != nil {
		t.Fatalf("expected existing subnet ipv6 association to be awaited, not recreated: %+v", ec2Client.associateSubnetCidrBlockInput)
	}
	if !subnetSupportsIPv6(subnet) {
		t.Fatalf("expected subnet to report ipv6 support after wait, got %+v", subnet.Ipv6CidrBlockAssociationSet)
	}
	if got := len(ec2Client.modifySubnetAttributeInputs); got != 2 {
		t.Fatalf("expected subnet attribute updates after ipv6 association completed, got %d", got)
	}
}

func TestResolveDebianAMIPrefersMatchingArchitecture(t *testing.T) {
	amiID, parameterName := pickDebianAMI([]ssmtypes.Parameter{
		{
			Name:  awsv2.String("/aws/service/debian/release/13/latest/arm64"),
			Value: awsv2.String("ami-arm64"),
		},
		{
			Name:  awsv2.String("/aws/service/debian/release/13/latest/amd64"),
			Value: awsv2.String("ami-amd64"),
		},
		{
			Name:  awsv2.String("/aws/service/debian/release/13/latest/amd64/ami-id"),
			Value: awsv2.String("ami-amd64-best"),
		},
	}, "amd64", debian13AMIPathRoot)
	if amiID != "ami-amd64-best" || parameterName != "/aws/service/debian/release/13/latest/amd64/ami-id" {
		t.Fatalf("unexpected Debian 13 AMI selection: ami=%q parameter=%q", amiID, parameterName)
	}
}

func TestResolveUbuntu2404AMIUsesCanonicalParameter(t *testing.T) {
	ssmClient := &recordingSSMClient{
		output: &ssm.GetParameterOutput{
			Parameter: &ssmtypes.Parameter{
				Value: awsv2.String("ami-ubuntu2404"),
			},
		},
	}

	amiID, err := resolveUbuntu2404AMI(context.Background(), ssmClient, "arm64")
	if err != nil {
		t.Fatalf("resolveUbuntu2404AMI() error = %v", err)
	}
	if amiID != "ami-ubuntu2404" {
		t.Fatalf("amiID = %q, want %q", amiID, "ami-ubuntu2404")
	}
	if ssmClient.input == nil || awsv2.ToString(ssmClient.input.Name) != fmt.Sprintf(ubuntu2404AMIPathFormat, "arm64") {
		t.Fatalf("unexpected ubuntu 24.04 parameter lookup: %+v", ssmClient.input)
	}
}

func TestStopInstanceTerminatesMatchedInstances(t *testing.T) {
	ec2Client := &recordingEC2Client{
		describeInstancesOutput: &ec2.DescribeInstancesOutput{
			Reservations: []ec2types.Reservation{
				{
					Instances: []ec2types.Instance{
						{InstanceId: awsv2.String("i-b")},
						{InstanceId: awsv2.String("i-a")},
					},
				},
			},
		},
		terminateInstancesOutput: &ec2.TerminateInstancesOutput{},
	}
	factory := instanceTestClientFactory{ec2Client: ec2Client, ssmClient: &recordingSSMClient{}}
	service := &Service{
		version:        "test",
		clientFactory:  factory,
		instanceRunner: newInstanceLifecycleRunner(factory),
	}

	result, err := service.StopInstance(context.Background(), provider.StopInstanceRequest{
		Credentials: provider.Credentials{
			AWS: &provider.AWSCredentials{
				AccessKeyID:     "ak",
				SecretAccessKey: "sk",
			},
		},
		InstanceID: "i-a",
		Region:     "us-west-2",
	})
	if err != nil {
		t.Fatalf("StopInstance returned error: %v", err)
	}
	if !result.Destroyed {
		t.Fatalf("expected destroyed result, got %+v", result)
	}
	if ec2Client.terminateInstancesInput == nil {
		t.Fatal("expected TerminateInstances to be called")
	}
	if got := ec2Client.terminateInstancesInput.InstanceIds; len(got) != 1 || got[0] != "i-a" {
		t.Fatalf("unexpected terminated instance ids: %+v", got)
	}
}

type fakeInstanceLifecycleRunner struct {
	startReq    provider.StartInstanceRequest
	startResult provider.StartInstanceResult
	startErr    error
	stopReq     provider.StopInstanceRequest
	stopResult  provider.StopInstanceResult
	stopErr     error
}

func (f *fakeInstanceLifecycleRunner) Start(_ context.Context, req provider.StartInstanceRequest) (provider.StartInstanceResult, error) {
	f.startReq = req
	if f.startErr != nil {
		return provider.StartInstanceResult{}, f.startErr
	}
	return f.startResult, nil
}

func (f *fakeInstanceLifecycleRunner) Stop(_ context.Context, req provider.StopInstanceRequest) (provider.StopInstanceResult, error) {
	f.stopReq = req
	if f.stopErr != nil {
		return provider.StopInstanceResult{}, f.stopErr
	}
	return f.stopResult, nil
}

type instanceTestClientFactory struct {
	ec2Client     ec2API
	pricingClient pricingAPI
	ssmClient     ssmAPI
}

func (f instanceTestClientFactory) NewConfig(_ context.Context, _ provider.AWSCredentials, region string, _ string) (awsv2.Config, error) {
	return awsv2.Config{Region: region}, nil
}

func (f instanceTestClientFactory) NewEC2(ec2ClientOptions) ec2API {
	return f.ec2Client
}

func (f instanceTestClientFactory) NewPricing(awsv2.Config) pricingAPI {
	if f.pricingClient != nil {
		return f.pricingClient
	}
	return &fakePricingClient{}
}

func (f instanceTestClientFactory) NewSSM(awsv2.Config) ssmAPI {
	return f.ssmClient
}

func (f instanceTestClientFactory) NewSTS(awsv2.Config) stsAPI {
	return noopSTSClient{}
}

type recordingEC2Client struct {
	fakeEC2Client
	associateRouteTableInput           *ec2.AssociateRouteTableInput
	associateRouteTableOutput          *ec2.AssociateRouteTableOutput
	associateRouteTableErr             error
	associateSubnetCidrBlockInput      *ec2.AssociateSubnetCidrBlockInput
	associateSubnetCidrBlockOutput     *ec2.AssociateSubnetCidrBlockOutput
	associateSubnetCidrBlockErr        error
	associateVpcCidrBlockInput         *ec2.AssociateVpcCidrBlockInput
	associateVpcCidrBlockOutput        *ec2.AssociateVpcCidrBlockOutput
	associateVpcCidrBlockErr           error
	attachInternetGatewayInput         *ec2.AttachInternetGatewayInput
	attachInternetGatewayOutput        *ec2.AttachInternetGatewayOutput
	attachInternetGatewayErr           error
	authorizeSecurityGroupEgressInputs []*ec2.AuthorizeSecurityGroupEgressInput
	authorizeSecurityGroupEgressOutput *ec2.AuthorizeSecurityGroupEgressOutput
	authorizeSecurityGroupEgressErr    error
	createInternetGatewayInput         *ec2.CreateInternetGatewayInput
	createInternetGatewayOutput        *ec2.CreateInternetGatewayOutput
	createInternetGatewayErr           error
	createRouteInputs                  []*ec2.CreateRouteInput
	createRouteOutput                  *ec2.CreateRouteOutput
	createRouteErr                     error
	createRouteTableInput              *ec2.CreateRouteTableInput
	createRouteTableOutput             *ec2.CreateRouteTableOutput
	createRouteTableErr                error
	createSecurityGroupInput           *ec2.CreateSecurityGroupInput
	createSecurityGroupOutput          *ec2.CreateSecurityGroupOutput
	createSecurityGroupErr             error
	createSubnetInput                  *ec2.CreateSubnetInput
	createSubnetOutput                 *ec2.CreateSubnetOutput
	createSubnetErr                    error
	createVpcInput                     *ec2.CreateVpcInput
	createVpcOutput                    *ec2.CreateVpcOutput
	createVpcErr                       error
	describeSubnetsOutputs             []*ec2.DescribeSubnetsOutput
	describeSubnetsCallCount           int
	describeInstanceTypesOutput        *ec2.DescribeInstanceTypesOutput
	describeInstanceTypesErr           error
	describeVpcsOutputs                []*ec2.DescribeVpcsOutput
	describeVpcsCallCount              int
	runInstancesInput                  *ec2.RunInstancesInput
	runInstancesOutput                 *ec2.RunInstancesOutput
	runInstancesErr                    error
	describeInstancesOutput            *ec2.DescribeInstancesOutput
	describeInstancesErr               error
	modifySubnetAttributeInputs        []*ec2.ModifySubnetAttributeInput
	modifySubnetAttributeErr           error
	modifyVpcAttributeInputs           []*ec2.ModifyVpcAttributeInput
	modifyVpcAttributeErr              error
	revokeSecurityGroupIngressInput    *ec2.RevokeSecurityGroupIngressInput
	revokeSecurityGroupIngressOutput   *ec2.RevokeSecurityGroupIngressOutput
	revokeSecurityGroupIngressErr      error
	terminateInstancesInput            *ec2.TerminateInstancesInput
	terminateInstancesOutput           *ec2.TerminateInstancesOutput
	terminateInstancesErr              error
}

func (r *recordingEC2Client) AssociateRouteTable(_ context.Context, input *ec2.AssociateRouteTableInput, _ ...func(*ec2.Options)) (*ec2.AssociateRouteTableOutput, error) {
	r.associateRouteTableInput = input
	if r.associateRouteTableErr != nil {
		return nil, r.associateRouteTableErr
	}
	if r.associateRouteTableOutput == nil {
		return &ec2.AssociateRouteTableOutput{}, nil
	}
	return r.associateRouteTableOutput, nil
}

func (r *recordingEC2Client) AssociateSubnetCidrBlock(_ context.Context, input *ec2.AssociateSubnetCidrBlockInput, _ ...func(*ec2.Options)) (*ec2.AssociateSubnetCidrBlockOutput, error) {
	r.associateSubnetCidrBlockInput = input
	if r.associateSubnetCidrBlockErr != nil {
		return nil, r.associateSubnetCidrBlockErr
	}
	if r.associateSubnetCidrBlockOutput == nil {
		return &ec2.AssociateSubnetCidrBlockOutput{}, nil
	}
	return r.associateSubnetCidrBlockOutput, nil
}

func (r *recordingEC2Client) AssociateVpcCidrBlock(_ context.Context, input *ec2.AssociateVpcCidrBlockInput, _ ...func(*ec2.Options)) (*ec2.AssociateVpcCidrBlockOutput, error) {
	r.associateVpcCidrBlockInput = input
	if r.associateVpcCidrBlockErr != nil {
		return nil, r.associateVpcCidrBlockErr
	}
	if r.associateVpcCidrBlockOutput == nil {
		return &ec2.AssociateVpcCidrBlockOutput{}, nil
	}
	return r.associateVpcCidrBlockOutput, nil
}

func (r *recordingEC2Client) AttachInternetGateway(_ context.Context, input *ec2.AttachInternetGatewayInput, _ ...func(*ec2.Options)) (*ec2.AttachInternetGatewayOutput, error) {
	r.attachInternetGatewayInput = input
	if r.attachInternetGatewayErr != nil {
		return nil, r.attachInternetGatewayErr
	}
	if r.attachInternetGatewayOutput == nil {
		return &ec2.AttachInternetGatewayOutput{}, nil
	}
	return r.attachInternetGatewayOutput, nil
}

func (r *recordingEC2Client) AuthorizeSecurityGroupEgress(_ context.Context, input *ec2.AuthorizeSecurityGroupEgressInput, _ ...func(*ec2.Options)) (*ec2.AuthorizeSecurityGroupEgressOutput, error) {
	r.authorizeSecurityGroupEgressInputs = append(r.authorizeSecurityGroupEgressInputs, input)
	if r.authorizeSecurityGroupEgressErr != nil {
		return nil, r.authorizeSecurityGroupEgressErr
	}
	if r.authorizeSecurityGroupEgressOutput == nil {
		return &ec2.AuthorizeSecurityGroupEgressOutput{}, nil
	}
	return r.authorizeSecurityGroupEgressOutput, nil
}

func (r *recordingEC2Client) CreateInternetGateway(_ context.Context, input *ec2.CreateInternetGatewayInput, _ ...func(*ec2.Options)) (*ec2.CreateInternetGatewayOutput, error) {
	r.createInternetGatewayInput = input
	if r.createInternetGatewayErr != nil {
		return nil, r.createInternetGatewayErr
	}
	if r.createInternetGatewayOutput == nil {
		return &ec2.CreateInternetGatewayOutput{}, nil
	}
	return r.createInternetGatewayOutput, nil
}

func (r *recordingEC2Client) CreateRoute(_ context.Context, input *ec2.CreateRouteInput, _ ...func(*ec2.Options)) (*ec2.CreateRouteOutput, error) {
	r.createRouteInputs = append(r.createRouteInputs, input)
	if r.createRouteErr != nil {
		return nil, r.createRouteErr
	}
	if r.createRouteOutput == nil {
		return &ec2.CreateRouteOutput{}, nil
	}
	return r.createRouteOutput, nil
}

func (r *recordingEC2Client) CreateRouteTable(_ context.Context, input *ec2.CreateRouteTableInput, _ ...func(*ec2.Options)) (*ec2.CreateRouteTableOutput, error) {
	r.createRouteTableInput = input
	if r.createRouteTableErr != nil {
		return nil, r.createRouteTableErr
	}
	if r.createRouteTableOutput == nil {
		return &ec2.CreateRouteTableOutput{}, nil
	}
	return r.createRouteTableOutput, nil
}

func (r *recordingEC2Client) CreateSecurityGroup(_ context.Context, input *ec2.CreateSecurityGroupInput, _ ...func(*ec2.Options)) (*ec2.CreateSecurityGroupOutput, error) {
	r.createSecurityGroupInput = input
	if r.createSecurityGroupErr != nil {
		return nil, r.createSecurityGroupErr
	}
	if r.createSecurityGroupOutput == nil {
		return &ec2.CreateSecurityGroupOutput{}, nil
	}
	return r.createSecurityGroupOutput, nil
}

func (r *recordingEC2Client) CreateSubnet(_ context.Context, input *ec2.CreateSubnetInput, _ ...func(*ec2.Options)) (*ec2.CreateSubnetOutput, error) {
	r.createSubnetInput = input
	if r.createSubnetErr != nil {
		return nil, r.createSubnetErr
	}
	if r.createSubnetOutput == nil {
		return &ec2.CreateSubnetOutput{}, nil
	}
	return r.createSubnetOutput, nil
}

func (r *recordingEC2Client) CreateVpc(_ context.Context, input *ec2.CreateVpcInput, _ ...func(*ec2.Options)) (*ec2.CreateVpcOutput, error) {
	r.createVpcInput = input
	if r.createVpcErr != nil {
		return nil, r.createVpcErr
	}
	if r.createVpcOutput == nil {
		return &ec2.CreateVpcOutput{}, nil
	}
	return r.createVpcOutput, nil
}

func (r *recordingEC2Client) DescribeInstanceTypes(context.Context, *ec2.DescribeInstanceTypesInput, ...func(*ec2.Options)) (*ec2.DescribeInstanceTypesOutput, error) {
	if r.describeInstanceTypesErr != nil {
		return nil, r.describeInstanceTypesErr
	}
	if r.describeInstanceTypesOutput == nil {
		return &ec2.DescribeInstanceTypesOutput{}, nil
	}
	return r.describeInstanceTypesOutput, nil
}

func (r *recordingEC2Client) DescribeSubnets(ctx context.Context, input *ec2.DescribeSubnetsInput, optFns ...func(*ec2.Options)) (*ec2.DescribeSubnetsOutput, error) {
	if len(r.describeSubnetsOutputs) > 0 {
		idx := r.describeSubnetsCallCount
		if idx >= len(r.describeSubnetsOutputs) {
			idx = len(r.describeSubnetsOutputs) - 1
		}
		r.describeSubnetsCallCount++
		if r.describeSubnetsOutputs[idx] == nil {
			return &ec2.DescribeSubnetsOutput{}, nil
		}
		return r.describeSubnetsOutputs[idx], nil
	}
	return r.fakeEC2Client.DescribeSubnets(ctx, input, optFns...)
}

func (r *recordingEC2Client) DescribeVpcs(ctx context.Context, input *ec2.DescribeVpcsInput, optFns ...func(*ec2.Options)) (*ec2.DescribeVpcsOutput, error) {
	if len(r.describeVpcsOutputs) > 0 {
		idx := r.describeVpcsCallCount
		if idx >= len(r.describeVpcsOutputs) {
			idx = len(r.describeVpcsOutputs) - 1
		}
		r.describeVpcsCallCount++
		if r.describeVpcsOutputs[idx] == nil {
			return &ec2.DescribeVpcsOutput{}, nil
		}
		return r.describeVpcsOutputs[idx], nil
	}
	return r.fakeEC2Client.DescribeVpcs(ctx, input, optFns...)
}

func (r *recordingEC2Client) DescribeInstances(context.Context, *ec2.DescribeInstancesInput, ...func(*ec2.Options)) (*ec2.DescribeInstancesOutput, error) {
	if r.describeInstancesErr != nil {
		return nil, r.describeInstancesErr
	}
	if r.describeInstancesOutput == nil {
		return &ec2.DescribeInstancesOutput{}, nil
	}
	return r.describeInstancesOutput, nil
}

func (r *recordingEC2Client) RunInstances(_ context.Context, input *ec2.RunInstancesInput, _ ...func(*ec2.Options)) (*ec2.RunInstancesOutput, error) {
	r.runInstancesInput = input
	if r.runInstancesErr != nil {
		return nil, r.runInstancesErr
	}
	if r.runInstancesOutput == nil {
		return &ec2.RunInstancesOutput{}, nil
	}
	return r.runInstancesOutput, nil
}

func (r *recordingEC2Client) ModifySubnetAttribute(_ context.Context, input *ec2.ModifySubnetAttributeInput, _ ...func(*ec2.Options)) (*ec2.ModifySubnetAttributeOutput, error) {
	r.modifySubnetAttributeInputs = append(r.modifySubnetAttributeInputs, input)
	if r.modifySubnetAttributeErr != nil {
		return nil, r.modifySubnetAttributeErr
	}
	return &ec2.ModifySubnetAttributeOutput{}, nil
}

func (r *recordingEC2Client) ModifyVpcAttribute(_ context.Context, input *ec2.ModifyVpcAttributeInput, _ ...func(*ec2.Options)) (*ec2.ModifyVpcAttributeOutput, error) {
	r.modifyVpcAttributeInputs = append(r.modifyVpcAttributeInputs, input)
	if r.modifyVpcAttributeErr != nil {
		return nil, r.modifyVpcAttributeErr
	}
	return &ec2.ModifyVpcAttributeOutput{}, nil
}

func (r *recordingEC2Client) RevokeSecurityGroupIngress(_ context.Context, input *ec2.RevokeSecurityGroupIngressInput, _ ...func(*ec2.Options)) (*ec2.RevokeSecurityGroupIngressOutput, error) {
	r.revokeSecurityGroupIngressInput = input
	if r.revokeSecurityGroupIngressErr != nil {
		return nil, r.revokeSecurityGroupIngressErr
	}
	if r.revokeSecurityGroupIngressOutput == nil {
		return &ec2.RevokeSecurityGroupIngressOutput{}, nil
	}
	return r.revokeSecurityGroupIngressOutput, nil
}

func (r *recordingEC2Client) TerminateInstances(_ context.Context, input *ec2.TerminateInstancesInput, _ ...func(*ec2.Options)) (*ec2.TerminateInstancesOutput, error) {
	r.terminateInstancesInput = input
	if r.terminateInstancesErr != nil {
		return nil, r.terminateInstancesErr
	}
	if r.terminateInstancesOutput == nil {
		return &ec2.TerminateInstancesOutput{}, nil
	}
	return r.terminateInstancesOutput, nil
}

type recordingSSMClient struct {
	input      *ssm.GetParameterInput
	output     *ssm.GetParameterOutput
	pathInput  *ssm.GetParametersByPathInput
	pathOutput *ssm.GetParametersByPathOutput
	err        error
}

func (r *recordingSSMClient) GetParameter(_ context.Context, input *ssm.GetParameterInput, _ ...func(*ssm.Options)) (*ssm.GetParameterOutput, error) {
	r.input = input
	if r.err != nil {
		return nil, r.err
	}
	if r.output == nil {
		return &ssm.GetParameterOutput{}, nil
	}
	return r.output, nil
}

func (r *recordingSSMClient) GetParametersByPath(_ context.Context, input *ssm.GetParametersByPathInput, _ ...func(*ssm.Options)) (*ssm.GetParametersByPathOutput, error) {
	r.pathInput = input
	if r.err != nil {
		return nil, r.err
	}
	if r.pathOutput == nil {
		return &ssm.GetParametersByPathOutput{}, nil
	}
	return r.pathOutput, nil
}

func toTagMap(specs []ec2types.TagSpecification) map[string]string {
	result := make(map[string]string)
	for _, spec := range specs {
		for _, tag := range spec.Tags {
			result[awsv2.ToString(tag.Key)] = awsv2.ToString(tag.Value)
		}
	}
	return result
}

func containsEgressCIDR(permissions []ec2types.IpPermission, ipv4CIDR string, ipv6CIDR string) bool {
	for _, permission := range permissions {
		if awsv2.ToString(permission.IpProtocol) != "-1" {
			continue
		}
		for _, ipRange := range permission.IpRanges {
			if ipv4CIDR != "" && awsv2.ToString(ipRange.CidrIp) == ipv4CIDR {
				return true
			}
		}
		for _, ipRange := range permission.Ipv6Ranges {
			if ipv6CIDR != "" && awsv2.ToString(ipRange.CidrIpv6) == ipv6CIDR {
				return true
			}
		}
	}
	return false
}

func flattenAuthorizedEgressPermissions(inputs []*ec2.AuthorizeSecurityGroupEgressInput) []ec2types.IpPermission {
	permissions := make([]ec2types.IpPermission, 0)
	for _, input := range inputs {
		permissions = append(permissions, input.IpPermissions...)
	}
	return permissions
}
