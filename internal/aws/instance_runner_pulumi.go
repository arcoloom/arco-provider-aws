//go:build pulumi

package aws

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/arcoloom/arco-provider-aws/internal/provider"
	awsprovider "github.com/pulumi/pulumi-aws/sdk/v7/go/aws"
	"github.com/pulumi/pulumi-aws/sdk/v7/go/aws/ec2"
	auto "github.com/pulumi/pulumi/sdk/v3/go/auto"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const pulumiProjectName = "arco-provider-aws"

type pulumiInstanceLifecycleRunner struct{}

func newInstanceLifecycleRunner(clientFactory) instanceLifecycleRunner {
	return pulumiInstanceLifecycleRunner{}
}

func (pulumiInstanceLifecycleRunner) Start(ctx context.Context, req provider.StartInstanceRequest) (provider.StartInstanceResult, error) {
	if err := ensurePulumiCLI(); err != nil {
		return provider.StartInstanceResult{}, err
	}

	stack, err := auto.UpsertStackInlineSource(
		ctx,
		req.StackName,
		pulumiProjectName,
		newEC2InstanceProgram(req),
		auto.WorkDir(stackWorkDir(req.StackName)),
		auto.EnvVars(pulumiEnv(req.Credentials.AWS, req.Region)),
	)
	if err != nil {
		return provider.StartInstanceResult{}, fmt.Errorf("create or select pulumi stack %s: %w", req.StackName, err)
	}

	if err := stack.SetConfig(ctx, "aws:region", auto.ConfigValue{Value: req.Region}); err != nil {
		return provider.StartInstanceResult{}, fmt.Errorf("set aws region config: %w", err)
	}

	upResult, err := stack.Up(ctx)
	if err != nil {
		return provider.StartInstanceResult{}, fmt.Errorf("pulumi up stack %s: %w", req.StackName, err)
	}

	return provider.StartInstanceResult{
		StackName:  req.StackName,
		InstanceID: outputValue(upResult.Outputs, "instanceId"),
		URN:        outputValue(upResult.Outputs, "urn"),
		PublicIP:   outputValue(upResult.Outputs, "publicIp"),
		PrivateIP:  outputValue(upResult.Outputs, "privateIp"),
	}, nil
}

func (pulumiInstanceLifecycleRunner) Stop(ctx context.Context, req provider.StopInstanceRequest) (provider.StopInstanceResult, error) {
	if err := ensurePulumiCLI(); err != nil {
		return provider.StopInstanceResult{}, err
	}

	stack, err := auto.SelectStackInlineSource(
		ctx,
		req.StackName,
		pulumiProjectName,
		func(*pulumi.Context) error { return nil },
		auto.WorkDir(stackWorkDir(req.StackName)),
		auto.EnvVars(pulumiEnv(req.Credentials.AWS, req.Scope.Region)),
	)
	if err != nil {
		return provider.StopInstanceResult{}, fmt.Errorf("select pulumi stack %s: %w", req.StackName, err)
	}

	if region := strings.TrimSpace(req.Scope.Region); region != "" {
		if err := stack.SetConfig(ctx, "aws:region", auto.ConfigValue{Value: region}); err != nil {
			return provider.StopInstanceResult{}, fmt.Errorf("set aws region config: %w", err)
		}
	}

	if _, err := stack.Destroy(ctx); err != nil {
		return provider.StopInstanceResult{}, fmt.Errorf("pulumi destroy stack %s: %w", req.StackName, err)
	}

	return provider.StopInstanceResult{
		StackName: req.StackName,
		Destroyed: true,
	}, nil
}

func ensurePulumiCLI() error {
	if _, err := exec.LookPath("pulumi"); err != nil {
		return errors.New("pulumi CLI was not found in PATH")
	}

	return nil
}

func stackWorkDir(stackName string) string {
	return filepath.Join(os.TempDir(), "arco-provider-aws", "pulumi", stackName)
}

func pulumiEnv(creds *provider.AWSCredentials, region string) map[string]string {
	if region == "" {
		region = defaultInstanceLifecycleRegion
	}

	env := map[string]string{
		"AWS_REGION":         region,
		"AWS_DEFAULT_REGION": region,
	}
	if creds == nil {
		return env
	}
	if creds.AccessKeyID != "" {
		env["AWS_ACCESS_KEY_ID"] = creds.AccessKeyID
	}
	if creds.SecretAccessKey != "" {
		env["AWS_SECRET_ACCESS_KEY"] = creds.SecretAccessKey
	}
	if creds.SessionToken != "" {
		env["AWS_SESSION_TOKEN"] = creds.SessionToken
	}

	return env
}

func newEC2InstanceProgram(req provider.StartInstanceRequest) pulumi.RunFunc {
	return func(ctx *pulumi.Context) error {
		providerArgs := &awsprovider.ProviderArgs{
			Region: pulumi.String(req.Region),
		}
		awsProvider, err := awsprovider.NewProvider(ctx, "aws-provider", providerArgs)
		if err != nil {
			return fmt.Errorf("create aws provider: %w", err)
		}

		tags := map[string]string{
			"Name": req.InstanceName,
		}
		for _, tag := range req.Tags {
			if key := strings.TrimSpace(tag.Key); key != "" {
				tags[key] = tag.Value
			}
		}

		args := &ec2.InstanceArgs{
			Ami:          pulumi.String(req.AMI),
			InstanceType: pulumi.String(req.InstanceType),
			Tags:         pulumi.ToStringMap(tags),
		}
		if req.AvailabilityZone != "" {
			args.AvailabilityZone = pulumi.StringPtr(req.AvailabilityZone)
		}
		if req.SubnetID != "" {
			args.SubnetId = pulumi.StringPtr(req.SubnetID)
		}
		if len(req.SecurityGroupIDs) > 0 {
			args.VpcSecurityGroupIds = pulumi.ToStringArray(req.SecurityGroupIDs)
		}
		if req.KeyName != "" {
			args.KeyName = pulumi.StringPtr(req.KeyName)
		}
		if req.UserData != "" {
			args.UserData = pulumi.StringPtr(req.UserData)
		}
		if req.MarketType == provider.InstanceMarketTypeSpot {
			args.InstanceMarketOptions = &ec2.InstanceInstanceMarketOptionsArgs{
				MarketType: pulumi.String("spot"),
			}
		}

		instance, err := ec2.NewInstance(ctx, req.InstanceName, args, pulumi.Provider(awsProvider))
		if err != nil {
			return fmt.Errorf("create ec2 instance: %w", err)
		}

		ctx.Export("instanceId", instance.ID())
		ctx.Export("urn", instance.URN())
		ctx.Export("publicIp", instance.PublicIp)
		ctx.Export("privateIp", instance.PrivateIp)

		return nil
	}
}

func outputValue(outputs auto.OutputMap, key string) string {
	value, ok := outputs[key]
	if !ok || value.Value == nil {
		return ""
	}

	if str, ok := value.Value.(string); ok {
		return str
	}

	return fmt.Sprint(value.Value)
}
