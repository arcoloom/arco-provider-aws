//go:build !pulumi

package aws

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/arcoloom/arco-provider-aws/internal/provider"
	awsv2 "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
)

const (
	managedByTagValue          = "arcoloom"
	stackTagKey                = "ArcoloomStack"
	defaultAMIParamX8664       = "/aws/service/ami-amazon-linux-latest/al2023-ami-kernel-default-x86_64"
	defaultAMIParamArm64       = "/aws/service/ami-amazon-linux-latest/al2023-ami-kernel-default-arm64"
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

	input := buildRunInstancesInput(req, amiID)
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

	_, ec2Client, _, err := r.clients(ctx, req.Credentials, req.Scope.Region, req.Scope)
	if err != nil {
		return provider.StopInstanceResult{}, err
	}

	instanceIDs, err := managedInstanceIDs(ctx, ec2Client, req.StackName)
	if err != nil {
		return provider.StopInstanceResult{}, err
	}
	if len(instanceIDs) == 0 {
		return provider.StopInstanceResult{
			StackName: req.StackName,
			Destroyed: true,
			Warnings: []provider.Warning{{
				Code:    warningCodeInstancesAbsent,
				Message: fmt.Sprintf("no running instances were found for stack %s", req.StackName),
			}},
		}, nil
	}

	if _, err := ec2Client.TerminateInstances(ctx, &ec2.TerminateInstancesInput{
		InstanceIds: instanceIDs,
	}); err != nil {
		return provider.StopInstanceResult{}, fmt.Errorf("terminate instances for stack %s: %w", req.StackName, err)
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

	parameterName, err := defaultAMIParameter(ctx, ec2Client, req.InstanceType)
	if err != nil {
		return "", err
	}

	output, err := ssmClient.GetParameter(ctx, &ssm.GetParameterInput{
		Name: awsv2.String(parameterName),
	})
	if err != nil {
		return "", fmt.Errorf("resolve default ami from %s: %w", parameterName, err)
	}

	amiID := strings.TrimSpace(awsv2.ToString(output.Parameter.Value))
	if amiID == "" {
		return "", fmt.Errorf("ssm parameter %s returned an empty ami id", parameterName)
	}

	return amiID, nil
}

func defaultAMIParameter(ctx context.Context, ec2Client ec2API, instanceType string) (string, error) {
	output, err := ec2Client.DescribeInstanceTypes(ctx, &ec2.DescribeInstanceTypesInput{
		InstanceTypes: []ec2types.InstanceType{ec2types.InstanceType(instanceType)},
	})
	if err != nil {
		return "", fmt.Errorf("describe instance type %s: %w", instanceType, err)
	}
	if len(output.InstanceTypes) == 0 {
		return "", fmt.Errorf("instance type %s was not found", instanceType)
	}

	for _, arch := range output.InstanceTypes[0].ProcessorInfo.SupportedArchitectures {
		switch arch {
		case ec2types.ArchitectureTypeArm64:
			return defaultAMIParamArm64, nil
		case ec2types.ArchitectureTypeX8664:
			return defaultAMIParamX8664, nil
		}
	}

	return "", fmt.Errorf("instance type %s does not report a supported default architecture", instanceType)
}

func buildRunInstancesInput(req provider.StartInstanceRequest, amiID string) *ec2.RunInstancesInput {
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
	if req.SubnetID != "" {
		input.SubnetId = awsv2.String(req.SubnetID)
	}
	if len(req.SecurityGroupIDs) > 0 {
		input.SecurityGroupIds = append([]string(nil), req.SecurityGroupIDs...)
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
