package aws

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/arcoloom/arco-provider-aws/internal/provider"
	awsv2 "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/aws/smithy-go"
)

type Service struct {
	version        string
	clientFactory  clientFactory
	instanceRunner instanceLifecycleRunner
	catalog        *catalogRepository
}

func NewService(version string) *Service {
	factory := newAWSClientFactory()
	return &Service{
		version:        version,
		clientFactory:  factory,
		instanceRunner: newInstanceLifecycleRunner(factory),
		catalog:        newCatalogRepository(),
	}
}

func (s *Service) Metadata(context.Context) (provider.Metadata, error) {
	return provider.Metadata{
		Name:              "arco-provider-aws",
		Version:           s.version,
		Cloud:             string(provider.CloudAWS),
		AuthMethods:       awsAuthMethods(),
		SupportedServices: []string{"location", "spot", "compute", "catalog", "pricing", "market"},
		Capabilities: map[string]string{
			"transport":      "grpc",
			"runtime":        "provider",
			"extensible":     "true",
			"schema_mode":    "provider-defined",
			"market_feed_v1": "stream_snapshot",
		},
		ResourcePlanes: []provider.ResourcePlane{provider.ResourcePlaneCompute},
	}, nil
}

func (s *Service) Schema(context.Context) ([]provider.ResourceSchema, error) {
	return []provider.ResourceSchema{awsComputeInstanceSchema()}, nil
}

const warningCodeRegionValidationSkipped = "REGION_VALIDATION_SKIPPED"

func (s *Service) ValidateConnection(ctx context.Context, req provider.ValidateConnectionRequest) (provider.ValidateConnectionResult, error) {
	accounts := routeAWSAccounts(req.Credentials, req.Scope)
	if len(accounts) == 0 {
		return rejectedValidation("aws iam credentials are required"), nil
	}

	warnings := make([]provider.Warning, 0, 1)
	regionsValidated := make(map[string]struct{})
	for _, account := range accounts {
		cfg, err := s.clientFactory.NewConfig(ctx, account.Credentials, effectiveEndpointRegion(req.Scope, defaultAWSRegion), req.Scope.Endpoint)
		if err != nil {
			return rejectedValidation(fmt.Sprintf("build aws client config: %v", err)), nil
		}

		identity, err := s.clientFactory.NewSTS(cfg).GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
		if err != nil {
			return rejectedValidation(fmt.Sprintf("validate aws caller identity: %v", err)), nil
		}
		if strings.TrimSpace(awsv2.ToString(identity.Account)) == "" {
			return rejectedValidation("validate aws caller identity: STS returned an empty account id"), nil
		}

		region := strings.TrimSpace(cfg.Region)
		regionsValidated[region] = struct{}{}
		ec2Client := s.clientFactory.NewEC2(ec2ClientOptions{
			Config:   cfg,
			Endpoint: req.Scope.Endpoint,
		})
		if err := validateRegionAccess(ctx, ec2Client); err != nil {
			if isRegionDescribePermissionError(err) {
				warnings = append(warnings, provider.Warning{
					Code: warningCodeRegionValidationSkipped,
					Message: fmt.Sprintf(
						"validated aws credentials, but could not verify EC2 read access in region %s because DescribeAvailabilityZones was denied: %v",
						region,
						err,
					),
				})
				continue
			}
			return rejectedValidation(fmt.Sprintf("validate EC2 region %s: %v", region, err)), nil
		}
	}

	region := defaultAWSRegion
	for candidate := range regionsValidated {
		region = candidate
		break
	}
	return provider.ValidateConnectionResult{
		Accepted: true,
		Message:  fmt.Sprintf("validated aws credentials in region %s", region),
		Warnings: warnings,
	}, nil
}

func (s *Service) Ping(_ context.Context, payload string) (provider.PingResult, error) {
	return provider.PingResult{
		Payload:   fmt.Sprintf("pong:%s", payload),
		Timestamp: time.Now().UTC(),
	}, nil
}

func validateRegionAccess(ctx context.Context, client ec2API) error {
	_, err := client.DescribeAvailabilityZones(ctx, &ec2.DescribeAvailabilityZonesInput{
		AllAvailabilityZones: awsv2.Bool(false),
	})
	return err
}

func isRegionDescribePermissionError(err error) bool {
	var apiErr smithy.APIError
	if !errors.As(err, &apiErr) {
		return false
	}

	switch apiErr.ErrorCode() {
	case "AccessDenied", "AccessDeniedException", "UnauthorizedOperation", "Client.UnauthorizedOperation":
		return true
	default:
		return false
	}
}

func rejectedValidation(message string) provider.ValidateConnectionResult {
	return provider.ValidateConnectionResult{
		Accepted: false,
		Message:  message,
	}
}
