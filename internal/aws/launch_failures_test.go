package aws

import (
	"errors"
	"testing"

	"github.com/arcoloom/arco-provider-aws/internal/provider"
	"github.com/aws/smithy-go"
)

func TestClassifyStartInstanceErrorMapsCapacityScopeFromZone(t *testing.T) {
	failure := classifyStartInstanceError(provider.StartInstanceRequest{
		AccountID:        "acct-1",
		Region:           "ap-northeast-2",
		AvailabilityZone: "ap-northeast-2a",
		InstanceType:     "g6.2xlarge",
		MarketType:       provider.InstanceMarketTypeSpot,
	}, &smithy.GenericAPIError{
		Code:    "InsufficientInstanceCapacity",
		Message: "no capacity",
	})
	if failure == nil {
		t.Fatal("expected launch failure")
	}
	if failure.Class != provider.LaunchFailureClassCapacity {
		t.Fatalf("failure class = %q, want %q", failure.Class, provider.LaunchFailureClassCapacity)
	}
	if failure.Scope != provider.LaunchFailureScopeZone {
		t.Fatalf("failure scope = %q, want %q", failure.Scope, provider.LaunchFailureScopeZone)
	}
	if !failure.Retryable {
		t.Fatal("expected capacity failure to be retryable")
	}
}

func TestClassifyStartInstanceErrorMapsSpotQuotaToAccount(t *testing.T) {
	failure := classifyStartInstanceError(provider.StartInstanceRequest{
		AccountID:    "acct-1",
		Region:       "ap-northeast-2",
		InstanceType: "g6.2xlarge",
		MarketType:   provider.InstanceMarketTypeSpot,
	}, &smithy.GenericAPIError{
		Code:    "MaxSpotInstanceCountExceeded",
		Message: "Max spot instance count exceeded",
	})
	if failure == nil {
		t.Fatal("expected launch failure")
	}
	if failure.Class != provider.LaunchFailureClassQuota {
		t.Fatalf("failure class = %q, want %q", failure.Class, provider.LaunchFailureClassQuota)
	}
	if failure.Scope != provider.LaunchFailureScopeAccount {
		t.Fatalf("failure scope = %q, want %q", failure.Scope, provider.LaunchFailureScopeAccount)
	}
	if failure.Retryable {
		t.Fatal("expected quota failure to be non-retryable")
	}
	if got := failure.Attributes["pricing_model"]; got != string(provider.InstanceMarketTypeSpot) {
		t.Fatalf("pricing_model attribute = %q, want %q", got, provider.InstanceMarketTypeSpot)
	}
}

func TestClassifyStartInstanceErrorFallsBackToProviderFailure(t *testing.T) {
	failure := classifyStartInstanceError(provider.StartInstanceRequest{
		Region:       "us-east-1",
		InstanceType: "c7g.large",
	}, errors.New("connection reset by peer"))
	if failure == nil {
		t.Fatal("expected launch failure")
	}
	if failure.Class != provider.LaunchFailureClassProvider {
		t.Fatalf("failure class = %q, want %q", failure.Class, provider.LaunchFailureClassProvider)
	}
	if failure.Scope != provider.LaunchFailureScopeRegion {
		t.Fatalf("failure scope = %q, want %q", failure.Scope, provider.LaunchFailureScopeRegion)
	}
}
