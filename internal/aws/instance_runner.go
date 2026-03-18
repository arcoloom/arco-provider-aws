package aws

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/arcoloom/arco-provider-aws/internal/provider"
	awsv2 "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	ssmtypes "github.com/aws/aws-sdk-go-v2/service/ssm/types"
)

const (
	managedByTagValue          = "arcoloom"
	stackTagKey                = "ArcoloomStack"
	debian13AMIPathRoot        = "/aws/service/debian/release/13/latest"
	dryRunOptionKey            = "dry_run"
	dryRunInstanceIDPrefix     = "dry-run-"
	warningCodeDryRun          = "DRY_RUN"
	warningCodeInstancesAbsent = "INSTANCES_NOT_FOUND"
)

type ec2InstanceLifecycleRunner struct {
	factory clientFactory
}

func newInstanceLifecycleRunner(factory clientFactory) instanceLifecycleRunner {
	if factory == nil {
		factory = newAWSClientFactory()
	}
	return ec2InstanceLifecycleRunner{factory: factory}
}

func (r ec2InstanceLifecycleRunner) Start(ctx context.Context, req provider.StartInstanceRequest) (provider.StartInstanceResult, error) {
	if dryRunEnabled(req.Options) {
		return provider.StartInstanceResult{
			StackName:  req.StackName,
			InstanceID: dryRunInstanceID(req.StackName),
			Warnings: []provider.Warning{{
				Code:    warningCodeDryRun,
				Message: "start_instance executed in dry-run mode; no AWS resources were created",
			}},
		}, nil
	}

	cfg, ec2Client, ssmClient, err := r.clients(ctx, req.Credentials, req.Region, req.Scope)
	if err != nil {
		return provider.StartInstanceResult{}, err
	}

	amiID, err := resolveAMI(ctx, ec2Client, ssmClient, req)
	if err != nil {
		return provider.StartInstanceResult{}, err
	}

	runConfig, err := resolveRunInstancesConfig(ctx, ec2Client, req, amiID)
	if err != nil {
		return provider.StartInstanceResult{}, err
	}

	input := buildRunInstancesInput(req, amiID, runConfig)
	output, err := ec2Client.RunInstances(ctx, input)
	if err != nil {
		return provider.StartInstanceResult{}, fmt.Errorf("run instances for stack %s in region %s: %w", req.StackName, cfg.Region, err)
	}
	if len(output.Instances) == 0 {
		return provider.StartInstanceResult{}, fmt.Errorf("run instances for stack %s returned no instances", req.StackName)
	}

	instance := output.Instances[0]
	return provider.StartInstanceResult{
		StackName:  req.StackName,
		InstanceID: awsv2.ToString(instance.InstanceId),
		PublicIP:   awsv2.ToString(instance.PublicIpAddress),
		PrivateIP:  awsv2.ToString(instance.PrivateIpAddress),
	}, nil
}

func (r ec2InstanceLifecycleRunner) Stop(ctx context.Context, req provider.StopInstanceRequest) (provider.StopInstanceResult, error) {
	if dryRunEnabled(req.Options) {
		return provider.StopInstanceResult{
			StackName: req.StackName,
			Destroyed: true,
			Warnings: []provider.Warning{{
				Code:    warningCodeDryRun,
				Message: "stop_instance executed in dry-run mode; no AWS resources were terminated",
			}},
		}, nil
	}

	baseRegion, err := effectiveDiscoveryBaseRegionWithOptions("", req.Options)
	if err != nil {
		return provider.StopInstanceResult{}, err
	}

	_, baseClient, _, err := r.clients(ctx, req.Credentials, baseRegion, req.Scope)
	if err != nil {
		return provider.StopInstanceResult{}, err
	}

	regions, err := resolveAccountRegionsWithOptions(ctx, baseClient, "", req.Options)
	if err != nil {
		return provider.StopInstanceResult{}, err
	}

	destroyedAny := false
	for _, region := range regions {
		_, ec2Client, _, err := r.clients(ctx, req.Credentials, region, req.Scope)
		if err != nil {
			return provider.StopInstanceResult{}, fmt.Errorf("build ec2 client for region %s: %w", region, err)
		}

		instanceIDs, err := managedInstanceIDs(ctx, ec2Client, req.StackName)
		if err != nil {
			return provider.StopInstanceResult{}, fmt.Errorf("find managed instances for stack %s in region %s: %w", req.StackName, region, err)
		}
		if len(instanceIDs) == 0 {
			continue
		}

		if _, err := ec2Client.TerminateInstances(ctx, &ec2.TerminateInstancesInput{
			InstanceIds: instanceIDs,
		}); err != nil {
			return provider.StopInstanceResult{}, fmt.Errorf("terminate instances for stack %s in region %s: %w", req.StackName, region, err)
		}

		destroyedAny = true
	}

	if !destroyedAny {
		return provider.StopInstanceResult{
			StackName: req.StackName,
			Destroyed: true,
			Warnings: []provider.Warning{{
				Code:    warningCodeInstancesAbsent,
				Message: fmt.Sprintf("no running instances were found for stack %s", req.StackName),
			}},
		}, nil
	}

	return provider.StopInstanceResult{
		StackName: req.StackName,
		Destroyed: true,
	}, nil
}

func (r ec2InstanceLifecycleRunner) clients(
	ctx context.Context,
	credentials provider.Credentials,
	region string,
	scope provider.ConnectionScope,
) (awsv2.Config, ec2API, ssmAPI, error) {
	if credentials.AWS == nil {
		return awsv2.Config{}, nil, nil, errors.New("aws iam credentials are required")
	}

	cfg, err := r.factory.NewConfig(ctx, *credentials.AWS, effectiveEndpointRegion(scope, region), scope.Endpoint)
	if err != nil {
		return awsv2.Config{}, nil, nil, err
	}

	return cfg, r.factory.NewEC2(ec2ClientOptions{Config: cfg, Endpoint: scope.Endpoint}), r.factory.NewSSM(cfg), nil
}

func resolveAMI(ctx context.Context, ec2Client ec2API, ssmClient ssmAPI, req provider.StartInstanceRequest) (string, error) {
	if strings.TrimSpace(req.AMI) != "" {
		return strings.TrimSpace(req.AMI), nil
	}

	architecture, err := defaultAMIArchitecture(ctx, ec2Client, req.InstanceType)
	if err != nil {
		return "", err
	}

	return resolveDebian13AMI(ctx, ssmClient, architecture)
}

func defaultAMIArchitecture(ctx context.Context, ec2Client ec2API, instanceType string) (string, error) {
	output, err := ec2Client.DescribeInstanceTypes(ctx, &ec2.DescribeInstanceTypesInput{
		InstanceTypes: []ec2types.InstanceType{ec2types.InstanceType(instanceType)},
	})
	if err != nil {
		return "", fmt.Errorf("describe instance type %s for default debian 13 ami selection: %w", instanceType, err)
	}
	if len(output.InstanceTypes) == 0 {
		return "", fmt.Errorf("instance type %s was not found", instanceType)
	}

	for _, arch := range output.InstanceTypes[0].ProcessorInfo.SupportedArchitectures {
		switch arch {
		case ec2types.ArchitectureTypeArm64:
			return "arm64", nil
		case ec2types.ArchitectureTypeX8664:
			return "amd64", nil
		}
	}

	return "", fmt.Errorf("instance type %s does not report a supported debian 13 architecture", instanceType)
}

func resolveDebian13AMI(ctx context.Context, ssmClient ssmAPI, architecture string) (string, error) {
	var (
		nextToken  *string
		candidates []ssmtypes.Parameter
	)

	for {
		output, err := ssmClient.GetParametersByPath(ctx, &ssm.GetParametersByPathInput{
			Path:           awsv2.String(debian13AMIPathRoot),
			Recursive:      awsv2.Bool(true),
			WithDecryption: awsv2.Bool(false),
			NextToken:      nextToken,
		})
		if err != nil {
			return "", fmt.Errorf("resolve default debian 13 ami from %s: %w", debian13AMIPathRoot, err)
		}

		candidates = append(candidates, output.Parameters...)
		if strings.TrimSpace(awsv2.ToString(output.NextToken)) == "" {
			break
		}
		nextToken = output.NextToken
	}

	amiID, parameterName := pickDebian13AMI(candidates, architecture)
	if amiID == "" {
		return "", fmt.Errorf("no debian 13 ami parameter under %s matched architecture %s", debian13AMIPathRoot, architecture)
	}
	if parameterName == "" {
		return "", fmt.Errorf("debian 13 ami resolution for architecture %s returned an unnamed parameter", architecture)
	}

	return amiID, nil
}

func pickDebian13AMI(parameters []ssmtypes.Parameter, architecture string) (string, string) {
	bestScore := -1
	bestVersion := -1
	bestName := ""
	bestValue := ""

	for _, parameter := range parameters {
		name := strings.TrimSpace(awsv2.ToString(parameter.Name))
		value := strings.TrimSpace(awsv2.ToString(parameter.Value))
		if name == "" || value == "" || !strings.HasPrefix(value, "ami-") {
			continue
		}

		score := debian13AMIScore(name, architecture)
		if score < 0 {
			continue
		}

		version := parameterVersion(name)
		if score > bestScore || (score == bestScore && version > bestVersion) || (score == bestScore && version == bestVersion && name > bestName) {
			bestScore = score
			bestVersion = version
			bestName = name
			bestValue = value
		}
	}

	return bestValue, bestName
}

func debian13AMIScore(name string, architecture string) int {
	normalized := strings.ToLower(strings.TrimSpace(name))
	if !strings.HasPrefix(normalized, debian13AMIPathRoot) {
		return -1
	}

	score := 0
	switch architecture {
	case "amd64":
		if strings.Contains(normalized, "/amd64") {
			score += 100
		}
		if strings.Contains(normalized, "/x86_64") {
			score += 80
		}
	case "arm64":
		if strings.Contains(normalized, "/arm64") {
			score += 100
		}
		if strings.Contains(normalized, "/aarch64") {
			score += 80
		}
	}

	if strings.Contains(normalized, "/ami-id") || strings.HasSuffix(normalized, "/id") {
		score += 20
	}
	if strings.Contains(normalized, "/current") || strings.Contains(normalized, "/latest") {
		score += 10
	}
	if score == 0 {
		return -1
	}

	return score
}

func parameterVersion(name string) int {
	if idx := strings.LastIndex(name, ":"); idx >= 0 && idx+1 < len(name) {
		if version, err := strconv.Atoi(name[idx+1:]); err == nil {
			return version
		}
	}
	return 0
}

func buildRunInstancesInput(
	req provider.StartInstanceRequest,
	amiID string,
	runConfig resolvedRunInstancesConfig,
) *ec2.RunInstancesInput {
	input := &ec2.RunInstancesInput{
		ImageId:      awsv2.String(amiID),
		InstanceType: ec2types.InstanceType(req.InstanceType),
		MinCount:     awsv2.Int32(1),
		MaxCount:     awsv2.Int32(1),
		TagSpecifications: []ec2types.TagSpecification{
			{
				ResourceType: ec2types.ResourceTypeInstance,
				Tags:         buildEC2Tags(req),
			},
		},
	}

	if req.AvailabilityZone != "" {
		input.Placement = &ec2types.Placement{AvailabilityZone: awsv2.String(req.AvailabilityZone)}
	}
	if runConfig.useNetworkInterface {
		networkInterface := ec2types.InstanceNetworkInterfaceSpecification{
			DeviceIndex:         awsv2.Int32(0),
			DeleteOnTermination: awsv2.Bool(true),
			SubnetId:            awsv2.String(runConfig.subnetID),
		}
		if runConfig.associatePublicIPv4 != nil {
			networkInterface.AssociatePublicIpAddress = runConfig.associatePublicIPv4
		}
		if runConfig.ipv6AddressCount > 0 {
			networkInterface.Ipv6AddressCount = awsv2.Int32(runConfig.ipv6AddressCount)
		}
		if len(runConfig.securityGroupIDs) > 0 {
			networkInterface.Groups = append([]string(nil), runConfig.securityGroupIDs...)
		}
		input.NetworkInterfaces = []ec2types.InstanceNetworkInterfaceSpecification{networkInterface}
	} else {
		if runConfig.subnetID != "" {
			input.SubnetId = awsv2.String(runConfig.subnetID)
		}
		if len(runConfig.securityGroupIDs) > 0 {
			input.SecurityGroupIds = append([]string(nil), runConfig.securityGroupIDs...)
		}
	}
	if runConfig.rootVolumeSizeGiB > 0 {
		input.BlockDeviceMappings = []ec2types.BlockDeviceMapping{{
			DeviceName: awsv2.String(runConfig.rootDeviceName),
			Ebs: &ec2types.EbsBlockDevice{
				DeleteOnTermination: awsv2.Bool(true),
				VolumeSize:          awsv2.Int32(runConfig.rootVolumeSizeGiB),
				VolumeType:          ec2types.VolumeTypeGp3,
			},
		}}
	}
	if req.KeyName != "" {
		input.KeyName = awsv2.String(req.KeyName)
	}
	if req.UserData != "" {
		encoded := base64.StdEncoding.EncodeToString([]byte(req.UserData))
		input.UserData = awsv2.String(encoded)
	}
	if req.MarketType == provider.InstanceMarketTypeSpot {
		input.InstanceMarketOptions = &ec2types.InstanceMarketOptionsRequest{
			MarketType: ec2types.MarketTypeSpot,
		}
	}

	return input
}

func buildEC2Tags(req provider.StartInstanceRequest) []ec2types.Tag {
	tagMap := map[string]string{
		"Name":      req.InstanceName,
		stackTagKey: req.StackName,
		"ManagedBy": managedByTagValue,
	}
	for _, tag := range req.Tags {
		key := strings.TrimSpace(tag.Key)
		if key == "" {
			continue
		}
		tagMap[key] = tag.Value
	}

	keys := make([]string, 0, len(tagMap))
	for key := range tagMap {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	result := make([]ec2types.Tag, 0, len(keys))
	for _, key := range keys {
		value := tagMap[key]
		result = append(result, ec2types.Tag{
			Key:   awsv2.String(key),
			Value: awsv2.String(value),
		})
	}
	return result
}

func managedInstanceIDs(ctx context.Context, ec2Client ec2API, stackName string) ([]string, error) {
	output, err := ec2Client.DescribeInstances(ctx, &ec2.DescribeInstancesInput{
		Filters: []ec2types.Filter{
			{
				Name:   awsv2.String("tag:" + stackTagKey),
				Values: []string{stackName},
			},
			{
				Name:   awsv2.String("instance-state-name"),
				Values: []string{"pending", "running", "stopping", "stopped"},
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("describe instances for stack %s: %w", stackName, err)
	}

	var instanceIDs []string
	for _, reservation := range output.Reservations {
		for _, instance := range reservation.Instances {
			if instance.InstanceId == nil || *instance.InstanceId == "" {
				continue
			}
			instanceIDs = append(instanceIDs, *instance.InstanceId)
		}
	}
	sort.Strings(instanceIDs)
	return instanceIDs, nil
}

func dryRunEnabled(options map[string]string) bool {
	for key, value := range options {
		if !strings.EqualFold(key, dryRunOptionKey) {
			continue
		}

		switch strings.ToLower(strings.TrimSpace(value)) {
		case "1", "true", "yes", "on":
			return true
		}
	}
	return false
}

func dryRunInstanceID(stackName string) string {
	stackName = strings.TrimSpace(stackName)
	if stackName == "" {
		return dryRunInstanceIDPrefix + "instance"
	}
	return dryRunInstanceIDPrefix + stackName
}
