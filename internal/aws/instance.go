package aws

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/arcoloom/arco-provider-aws/internal/provider"
)

const defaultInstanceLifecycleRegion = defaultAWSRegion

type instanceLifecycleRunner interface {
	Start(context.Context, provider.StartInstanceRequest) (provider.StartInstanceResult, error)
	Stop(context.Context, provider.StopInstanceRequest) (provider.StopInstanceResult, error)
}

func (s *Service) StartInstance(ctx context.Context, req provider.StartInstanceRequest) (provider.StartInstanceResult, error) {
	if err := validateStartInstanceRequest(req); err != nil {
		return provider.StartInstanceResult{}, err
	}

	return s.instanceRunner.Start(ctx, normalizeStartInstanceRequest(req))
}

func (s *Service) StopInstance(ctx context.Context, req provider.StopInstanceRequest) (provider.StopInstanceResult, error) {
	if strings.TrimSpace(req.StackName) == "" {
		return provider.StopInstanceResult{}, errors.New("stack name is required")
	}
	if req.Credentials.AWS == nil {
		return provider.StopInstanceResult{}, errors.New("aws iam credentials are required")
	}

	return s.instanceRunner.Stop(ctx, req)
}

func validateStartInstanceRequest(req provider.StartInstanceRequest) error {
	if req.Credentials.AWS == nil {
		return errors.New("aws iam credentials are required")
	}
	if strings.TrimSpace(req.StackName) == "" {
		return errors.New("stack name is required")
	}
	if strings.TrimSpace(req.InstanceType) == "" {
		return errors.New("instance type is required")
	}
	if req.MarketType != "" && req.MarketType != provider.InstanceMarketTypeOnDemand && req.MarketType != provider.InstanceMarketTypeSpot {
		return fmt.Errorf("unsupported market type %q", req.MarketType)
	}

	return nil
}

func normalizeStartInstanceRequest(req provider.StartInstanceRequest) provider.StartInstanceRequest {
	if req.Region == "" {
		switch {
		case req.Scope.Region != "":
			req.Region = req.Scope.Region
		default:
			req.Region = defaultInstanceLifecycleRegion
		}
	}
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
	req.AMI = strings.TrimSpace(req.AMI)
	req.InstanceType = strings.TrimSpace(req.InstanceType)
	req.SubnetID = strings.TrimSpace(req.SubnetID)
	req.KeyName = strings.TrimSpace(req.KeyName)
	req.UserData = strings.TrimSpace(req.UserData)

	return req
}
