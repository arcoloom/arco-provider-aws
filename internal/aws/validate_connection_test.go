package aws

import (
	"context"
	"strings"
	"testing"

	"github.com/arcoloom/arco-provider-aws/internal/provider"
	awsv2 "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/aws/smithy-go"
)

func TestValidateConnectionAcceptsOpaqueAccountIDAndRegion(t *testing.T) {
	service := &Service{
		version: "test",
		clientFactory: fakeClientFactory{
			clients: map[string]ec2API{
				"us-east-1": fakeEC2Client{
					availabilityZonesOutput: &ec2.DescribeAvailabilityZonesOutput{},
				},
			},
			stsClient: fakeSTSIdentityClient{
				output: &sts.GetCallerIdentityOutput{
					Account: awsv2.String("123456789012"),
					Arn:     awsv2.String("arn:aws:sts::123456789012:assumed-role/arcoloom/test"),
				},
			},
		},
	}

	result, err := service.ValidateConnection(context.Background(), provider.ValidateConnectionRequest{
		Credentials: provider.Credentials{
			AWS: &provider.AWSCredentials{
				AccessKeyID:     "ak",
				SecretAccessKey: "sk",
			},
		},
		Scope: provider.ConnectionScope{
			AccountID: "acct-prod",
			Region:    "us-east-1",
		},
	})
	if err != nil {
		t.Fatalf("ValidateConnection returned error: %v", err)
	}
	if !result.Accepted {
		t.Fatalf("expected validation success, got %+v", result)
	}
	if len(result.Warnings) != 0 {
		t.Fatalf("expected no warnings, got %+v", result.Warnings)
	}
	if !strings.Contains(result.Message, "us-east-1") {
		t.Fatalf("unexpected validation message: %q", result.Message)
	}
}

func TestValidateConnectionDoesNotCompareOpaqueAccountIDToCloudAccount(t *testing.T) {
	service := &Service{
		version: "test",
		clientFactory: fakeClientFactory{
			clients: map[string]ec2API{
				"us-east-1": fakeEC2Client{
					availabilityZonesOutput: &ec2.DescribeAvailabilityZonesOutput{},
				},
			},
			stsClient: fakeSTSIdentityClient{
				output: &sts.GetCallerIdentityOutput{
					Account: awsv2.String("123456789012"),
				},
			},
		},
	}

	result, err := service.ValidateConnection(context.Background(), provider.ValidateConnectionRequest{
		Credentials: provider.Credentials{
			AWS: &provider.AWSCredentials{
				AccessKeyID:     "ak",
				SecretAccessKey: "sk",
			},
		},
		Scope: provider.ConnectionScope{
			AccountID: "acct-other",
			Region:    "us-east-1",
		},
	})
	if err != nil {
		t.Fatalf("ValidateConnection returned error: %v", err)
	}
	if !result.Accepted {
		t.Fatalf("expected validation success, got %+v", result)
	}
	if !strings.Contains(result.Message, "us-east-1") {
		t.Fatalf("unexpected validation message: %q", result.Message)
	}
}

func TestValidateConnectionAddsWarningWhenRegionReadIsDenied(t *testing.T) {
	service := &Service{
		version: "test",
		clientFactory: fakeClientFactory{
			clients: map[string]ec2API{
				"us-east-1": fakeEC2Client{
					availabilityZonesErr: &smithy.GenericAPIError{
						Code:    "AccessDenied",
						Message: "not allowed",
					},
				},
			},
			stsClient: fakeSTSIdentityClient{
				output: &sts.GetCallerIdentityOutput{
					Account: awsv2.String("123456789012"),
				},
			},
		},
	}

	result, err := service.ValidateConnection(context.Background(), provider.ValidateConnectionRequest{
		Credentials: provider.Credentials{
			AWS: &provider.AWSCredentials{
				AccessKeyID:     "ak",
				SecretAccessKey: "sk",
			},
		},
		Scope: provider.ConnectionScope{
			AccountID: "acct-prod",
			Region:    "us-east-1",
		},
	})
	if err != nil {
		t.Fatalf("ValidateConnection returned error: %v", err)
	}
	if !result.Accepted {
		t.Fatalf("expected validation success with warning, got %+v", result)
	}
	if len(result.Warnings) != 1 || result.Warnings[0].Code != warningCodeRegionValidationSkipped {
		t.Fatalf("unexpected validation warnings: %+v", result.Warnings)
	}
}

func TestValidateConnectionRejectsRegionErrors(t *testing.T) {
	service := &Service{
		version: "test",
		clientFactory: fakeClientFactory{
			clients: map[string]ec2API{
				"us-east-1": fakeEC2Client{
					availabilityZonesErr: &smithy.GenericAPIError{
						Code:    "InvalidEndpointException",
						Message: "region does not exist",
					},
				},
			},
			stsClient: fakeSTSIdentityClient{
				output: &sts.GetCallerIdentityOutput{
					Account: awsv2.String("123456789012"),
				},
			},
		},
	}

	result, err := service.ValidateConnection(context.Background(), provider.ValidateConnectionRequest{
		Credentials: provider.Credentials{
			AWS: &provider.AWSCredentials{
				AccessKeyID:     "ak",
				SecretAccessKey: "sk",
			},
		},
		Scope: provider.ConnectionScope{
			AccountID: "acct-prod",
			Region:    "us-east-1",
		},
	})
	if err != nil {
		t.Fatalf("ValidateConnection returned error: %v", err)
	}
	if result.Accepted {
		t.Fatalf("expected validation rejection, got %+v", result)
	}
	if !strings.Contains(result.Message, "validate EC2 region us-east-1") {
		t.Fatalf("unexpected validation message: %q", result.Message)
	}
}

type fakeSTSIdentityClient struct {
	output *sts.GetCallerIdentityOutput
	err    error
}

func (f fakeSTSIdentityClient) GetCallerIdentity(context.Context, *sts.GetCallerIdentityInput, ...func(*sts.Options)) (*sts.GetCallerIdentityOutput, error) {
	if f.err != nil {
		return nil, f.err
	}
	if f.output == nil {
		return &sts.GetCallerIdentityOutput{}, nil
	}
	return f.output, nil
}
