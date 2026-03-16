package aws

import (
	"context"
	"strings"
	"testing"

	"github.com/arcoloom/arco-provider-aws/internal/provider"
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
		Credentials: provider.Credentials{
			AWS: &provider.AWSCredentials{
				AccessKeyID:     "ak",
				SecretAccessKey: "sk",
			},
		},
		StackName:    " stack-a ",
		InstanceType: " m7i.large ",
		MarketType:   provider.InstanceMarketTypeSpot,
		Scope: provider.ConnectionScope{
			Region: "us-west-2",
		},
		UserData: " echo hi ",
	})
	if err != nil {
		t.Fatalf("StartInstance returned error: %v", err)
	}

	if result.InstanceID != "i-123" {
		t.Fatalf("unexpected instance id: %+v", result)
	}
	if runner.startReq.Region != "us-west-2" {
		t.Fatalf("expected region to default from scope, got %q", runner.startReq.Region)
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

	_, err := service.StartInstance(context.Background(), provider.StartInstanceRequest{})
	if err == nil {
		t.Fatal("expected validation error")
	}
}

func TestStopInstanceDelegatesToRunner(t *testing.T) {
	runner := &fakeInstanceLifecycleRunner{
		stopResult: provider.StopInstanceResult{
			StackName: "stack-a",
			Destroyed: true,
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
		StackName: "stack-a",
	})
	if err != nil {
		t.Fatalf("StopInstance returned error: %v", err)
	}

	if !result.Destroyed {
		t.Fatalf("expected destroyed result, got %+v", result)
	}
	if runner.stopReq.StackName != "stack-a" {
		t.Fatalf("unexpected stop request: %+v", runner.stopReq)
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
		StackName: "stack-a",
		Options:   map[string]string{"dry_run": "true"},
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
