package aws

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/arcoloom/arco-provider-aws/internal/provider"
	awsv2 "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/service/sts"
)

const defaultAssumeRoleSessionPrefix = "arco-provider-aws"

var stsIdentifierPattern = regexp.MustCompile(`^[\w+=,.@-]{2,64}$`)

func buildAWSLoadOptions(creds provider.AWSCredentials, region string, endpoint string) ([]func(*config.LoadOptions) error, error) {
	normalized := normalizeAWSCredentials(creds)
	if err := validateAWSAuthConfiguration(normalized); err != nil {
		return nil, fmt.Errorf("validate aws auth configuration: %w", err)
	}

	loadOptions := []func(*config.LoadOptions) error{
		config.WithRegion(region),
	}

	if normalized.UseDefaultCredentialsChain {
		if normalized.Profile != "" {
			loadOptions = append(loadOptions, config.WithSharedConfigProfile(normalized.Profile))
		}
	} else {
		loadOptions = append(loadOptions, config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			normalized.AccessKeyID,
			normalized.SecretAccessKey,
			normalized.SessionToken,
		)))
	}

	if endpoint != "" {
		targetService := endpointServiceID(endpoint)
		resolver := awsv2.EndpointResolverWithOptionsFunc(func(service, resolvedRegion string, _ ...interface{}) (awsv2.Endpoint, error) {
			if resolvedRegion == region && shouldUseCustomEndpoint(service, targetService) {
				return awsv2.Endpoint{
					URL:           endpoint,
					SigningRegion: region,
				}, nil
			}

			return awsv2.Endpoint{}, &awsv2.EndpointNotFoundError{}
		})
		loadOptions = append(loadOptions, config.WithEndpointResolverWithOptions(resolver))
	}

	return loadOptions, nil
}

func buildAssumeRoleProvider(cfg awsv2.Config, creds provider.AWSCredentials) (awsv2.CredentialsProvider, error) {
	normalized := normalizeAWSCredentials(creds)
	if normalized.RoleARN == "" {
		return nil, fmt.Errorf("role_arn is required when assume role is enabled")
	}

	roleSessionName := normalized.RoleSessionName
	if roleSessionName == "" {
		roleSessionName = generatedRoleSessionName()
	}

	assumeRoleProvider := stscreds.NewAssumeRoleProvider(sts.NewFromConfig(cfg), normalized.RoleARN, func(options *stscreds.AssumeRoleOptions) {
		options.RoleSessionName = roleSessionName
		if normalized.ExternalID != "" {
			options.ExternalID = awsv2.String(normalized.ExternalID)
		}
		if normalized.SourceIdentity != "" {
			options.SourceIdentity = awsv2.String(normalized.SourceIdentity)
		}
	})

	return assumeRoleProvider, nil
}

func normalizeAWSCredentials(creds provider.AWSCredentials) provider.AWSCredentials {
	creds.Profile = strings.TrimSpace(creds.Profile)
	creds.AccessKeyID = strings.TrimSpace(creds.AccessKeyID)
	creds.SecretAccessKey = strings.TrimSpace(creds.SecretAccessKey)
	creds.SessionToken = strings.TrimSpace(creds.SessionToken)
	creds.RoleARN = strings.TrimSpace(creds.RoleARN)
	creds.ExternalID = strings.TrimSpace(creds.ExternalID)
	creds.RoleSessionName = strings.TrimSpace(creds.RoleSessionName)
	creds.SourceIdentity = strings.TrimSpace(creds.SourceIdentity)
	return creds
}

func validateAWSAuthConfiguration(creds provider.AWSCredentials) error {
	if creds.UseDefaultCredentialsChain {
		if creds.AccessKeyID != "" || creds.SecretAccessKey != "" || creds.SessionToken != "" {
			return fmt.Errorf("default credential chain auth cannot be combined with access keys or session tokens")
		}
	} else {
		if creds.AccessKeyID == "" || creds.SecretAccessKey == "" {
			return fmt.Errorf("access_key_id and secret_access_key are required")
		}
		if creds.Profile != "" {
			return fmt.Errorf("profile is only supported with the default credential chain auth method")
		}
	}

	if creds.RoleARN == "" {
		if creds.ExternalID != "" || creds.RoleSessionName != "" || creds.SourceIdentity != "" {
			return fmt.Errorf("role_arn is required when setting assume-role options")
		}
	}

	if creds.RoleSessionName != "" && !stsIdentifierPattern.MatchString(creds.RoleSessionName) {
		return fmt.Errorf("role_session_name must match [\\w+=,.@-]{2,64}")
	}
	if creds.SourceIdentity != "" && !stsIdentifierPattern.MatchString(creds.SourceIdentity) {
		return fmt.Errorf("source_identity must match [\\w+=,.@-]{2,64}")
	}

	return nil
}

func generatedRoleSessionName() string {
	name := fmt.Sprintf("%s-%d-%d", defaultAssumeRoleSessionPrefix, os.Getpid(), time.Now().UTC().UnixNano())
	if len(name) > 64 {
		return name[:64]
	}
	return name
}
