package aws

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/arcoloom/arco-provider-aws/internal/provider"
)

type instanceLifecycleRunner interface {
	Start(context.Context, provider.StartInstanceRequest) (provider.StartInstanceResult, error)
	Stop(context.Context, provider.StopInstanceRequest) (provider.StopInstanceResult, error)
}

func (s *Service) StartInstance(ctx context.Context, req provider.StartInstanceRequest) (provider.StartInstanceResult, error) {
	req = normalizeStartInstanceRequest(req)
	if err := validateStartInstanceRequest(req); err != nil {
		return provider.StartInstanceResult{
			StackName:     req.StackName,
			LaunchFailure: launchFailureFromRequestError(req, err),
		}, nil
	}
	account, err := routeAWSAccount(req.Credentials, req.AccountID, req.Scope)
	if err != nil {
		return provider.StartInstanceResult{
			StackName:     req.StackName,
			LaunchFailure: launchFailureFromRequestError(req, err),
		}, nil
	}
	req.Credentials = provider.Credentials{AWS: &account.Credentials}
	req.AccountID = account.AccountID

	return s.instanceRunner.Start(ctx, req)
}

func (s *Service) StopInstance(ctx context.Context, req provider.StopInstanceRequest) (provider.StopInstanceResult, error) {
	req = normalizeStopInstanceRequest(req)
	if err := validateStopInstanceRequest(req); err != nil {
		return provider.StopInstanceResult{}, err
	}
	account, err := routeAWSAccount(req.Credentials, req.AccountID, req.Scope)
	if err != nil {
		return provider.StopInstanceResult{}, err
	}
	req.Credentials = provider.Credentials{AWS: &account.Credentials}
	req.AccountID = account.AccountID

	return s.instanceRunner.Stop(ctx, req)
}

func validateStartInstanceRequest(req provider.StartInstanceRequest) error {
	if !hasAnyAWSCredentials(req.Credentials) {
		return errors.New("aws iam credentials are required")
	}
	if strings.TrimSpace(req.StackName) == "" {
		return errors.New("stack name is required")
	}
	if strings.TrimSpace(req.Region) == "" {
		return errors.New("region is required; automatic fallback to scope.region or provider defaults is disabled")
	}
	if strings.TrimSpace(req.InstanceType) == "" {
		return errors.New("instance type is required")
	}
	if req.MarketType != "" && req.MarketType != provider.InstanceMarketTypeOnDemand && req.MarketType != provider.InstanceMarketTypeSpot {
		return fmt.Errorf("unsupported market type %q", req.MarketType)
	}
	if _, err := parseStartInstanceProviderConfig(req.ProviderConfig); err != nil {
		return err
	}

	return nil
}

func validateStopInstanceRequest(req provider.StopInstanceRequest) error {
	if !hasAnyAWSCredentials(req.Credentials) {
		return errors.New("aws iam credentials are required")
	}
	if strings.TrimSpace(req.InstanceID) == "" {
		return errors.New("instance id is required")
	}
	if strings.TrimSpace(req.Region) == "" {
		return errors.New("region is required")
	}

	return nil
}

func normalizeStartInstanceRequest(req provider.StartInstanceRequest) provider.StartInstanceRequest {
	if req.MarketType == "" {
		req.MarketType = provider.InstanceMarketTypeOnDemand
	}
	if req.InstanceName == "" {
		req.InstanceName = req.StackName
	}

	req.StackName = strings.TrimSpace(req.StackName)
	req.InstanceName = strings.TrimSpace(req.InstanceName)
	req.Region = strings.TrimSpace(req.Region)
	req.AvailabilityZone = strings.TrimSpace(req.AvailabilityZone)
	req.InstanceType = strings.TrimSpace(req.InstanceType)
	req.UserData = strings.TrimSpace(req.UserData)

	return req
}

func normalizeStopInstanceRequest(req provider.StopInstanceRequest) provider.StopInstanceRequest {
	req.InstanceID = strings.TrimSpace(req.InstanceID)
	req.Region = strings.TrimSpace(req.Region)
	return req
}

func hasAnyAWSCredentials(credentials provider.Credentials) bool {
	return credentials.AWS != nil || len(credentials.AWSAccounts) != 0
}
