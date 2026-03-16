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
		Cloud:             provider.CloudAWS,
		SupportedAuth:     []provider.AuthScheme{provider.AuthSchemeAWSIAM},
		SupportedServices: []string{"location", "spot", "compute", "catalog", "pricing"},
		Capabilities: map[string]string{
			"transport":  "grpc",
			"runtime":    "provider",
			"extensible": "true",
		},
	}, nil
}

const warningCodeRegionValidationSkipped = "REGION_VALIDATION_SKIPPED"

func (s *Service) ValidateConnection(ctx context.Context, req provider.ValidateConnectionRequest) (provider.ValidateConnectionResult, error) {
	if req.Credentials.AWS == nil {
		return rejectedValidation("aws iam credentials are required"), nil
	}

	resourceRegion := strings.TrimSpace(req.Scope.Region)
	cfg, err := s.clientFactory.NewConfig(ctx, *req.Credentials.AWS, effectiveEndpointRegion(req.Scope, resourceRegion), req.Scope.Endpoint)
	if err != nil {
		return rejectedValidation(fmt.Sprintf("build aws client config: %v", err)), nil
	}

	identity, err := s.clientFactory.NewSTS(cfg).GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		return rejectedValidation(fmt.Sprintf("validate aws caller identity: %v", err)), nil
	}

	accountID := strings.TrimSpace(awsv2.ToString(identity.Account))
	if accountID == "" {
		return rejectedValidation("validate aws caller identity: STS returned an empty account id"), nil
	}

	expectedAccountID := strings.TrimSpace(req.Scope.AccountID)
	if expectedAccountID != "" && expectedAccountID != accountID {
		return rejectedValidation(fmt.Sprintf(
			"validated aws account %s does not match requested scope account %s",
			accountID,
			expectedAccountID,
		)), nil
	}

	warnings := make([]provider.Warning, 0, 1)
	region := strings.TrimSpace(cfg.Region)
	ec2Client := s.clientFactory.NewEC2(ec2ClientOptions{
		Config:   cfg,
		Endpoint: req.Scope.Endpoint,
	})
	if err := validateRegionAccess(ctx, ec2Client); err != nil {
		if isRegionDescribePermissionError(err) {
			warnings = append(warnings, provider.Warning{
				Code: warningCodeRegionValidationSkipped,
				Message: fmt.Sprintf(
					"validated caller identity for account %s, but could not verify EC2 read access in region %s because DescribeAvailabilityZones was denied: %v",
					accountID,
					region,
					err,
				),
			})
		} else {
			return rejectedValidation(fmt.Sprintf("validate EC2 region %s: %v", region, err)), nil
		}
	}

	return provider.ValidateConnectionResult{
		Accepted: true,
		Message:  fmt.Sprintf("validated aws credentials for account %s in region %s", accountID, region),
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
