package aws

import (
	"errors"
	"strings"

	"github.com/arcoloom/arco-provider-aws/internal/provider"
	"github.com/aws/smithy-go"
)

const (
	launchFailureCodeCapacityUnavailable = "CAPACITY_UNAVAILABLE"
	launchFailureCodeQuotaExceeded       = "QUOTA_EXCEEDED"
	launchFailureCodePriceInvalid        = "PRICE_INVALID"
	launchFailureCodeAPIThrottled        = "API_THROTTLED"
	launchFailureCodeAuthInvalid         = "AUTH_INVALID"
	launchFailureCodeConfigInvalid       = "CONFIG_INVALID"
	launchFailureCodeProviderUnavailable = "PROVIDER_UNAVAILABLE"
)

func launchFailureFromRequestError(req provider.StartInstanceRequest, err error) *provider.LaunchFailure {
	if err == nil {
		return nil
	}

	message := strings.TrimSpace(err.Error())
	lower := strings.ToLower(message)
	switch {
	case strings.Contains(lower, "credentials are required"):
		return newLaunchFailure(
			req,
			launchFailureCodeAuthInvalid,
			provider.LaunchFailureClassAuth,
			provider.LaunchFailureScopeAccount,
			false,
			message,
			"",
		)
	case strings.Contains(lower, "account_id is required"), strings.Contains(lower, "unknown account_id"):
		return newLaunchFailure(
			req,
			launchFailureCodeConfigInvalid,
			provider.LaunchFailureClassConfig,
			provider.LaunchFailureScopeJob,
			false,
			message,
			"",
		)
	default:
		return newLaunchFailure(
			req,
			launchFailureCodeConfigInvalid,
			provider.LaunchFailureClassConfig,
			provider.LaunchFailureScopeJob,
			false,
			message,
			"",
		)
	}
}

func classifyStartInstanceError(req provider.StartInstanceRequest, err error) *provider.LaunchFailure {
	if err == nil {
		return nil
	}

	var apiErr smithy.APIError
	if !errors.As(err, &apiErr) {
		return newLaunchFailure(
			req,
			launchFailureCodeProviderUnavailable,
			provider.LaunchFailureClassProvider,
			provider.LaunchFailureScopeRegion,
			true,
			strings.TrimSpace(err.Error()),
			"",
		)
	}

	rawCode := strings.TrimSpace(apiErr.ErrorCode())
	normalizedCode := strings.ToLower(rawCode)
	message := strings.TrimSpace(apiErr.ErrorMessage())
	if message == "" {
		message = strings.TrimSpace(err.Error())
	}

	switch normalizedCode {
	case "insufficientinstancecapacity":
		scope := provider.LaunchFailureScopeRegion
		if strings.TrimSpace(req.AvailabilityZone) != "" {
			scope = provider.LaunchFailureScopeZone
		}
		return newLaunchFailure(req, launchFailureCodeCapacityUnavailable, provider.LaunchFailureClassCapacity, scope, true, message, rawCode)
	case "maxspotinstancecountexceeded", "vcpulimitexceeded":
		return newLaunchFailure(req, launchFailureCodeQuotaExceeded, provider.LaunchFailureClassQuota, provider.LaunchFailureScopeAccount, false, message, rawCode)
	case "requestlimitexceeded", "throttling", "throttlingexception", "requestthrottled":
		return newLaunchFailure(req, launchFailureCodeAPIThrottled, provider.LaunchFailureClassAPI, provider.LaunchFailureScopeRegion, true, message, rawCode)
	case "accessdenied", "accessdeniedexception", "unauthorizedoperation", "client.unauthorizedoperation", "authfailure":
		return newLaunchFailure(req, launchFailureCodeAuthInvalid, provider.LaunchFailureClassAuth, provider.LaunchFailureScopeAccount, false, message, rawCode)
	case "invalidparametervalue", "invalidparametercombination", "unsupported", "unsupportedoperation", "unsupportedinstancetype", "invalidinstancetype.notfound":
		return newLaunchFailure(req, launchFailureCodeConfigInvalid, provider.LaunchFailureClassConfig, provider.LaunchFailureScopeJob, false, message, rawCode)
	default:
		return newLaunchFailure(req, launchFailureCodeProviderUnavailable, provider.LaunchFailureClassProvider, provider.LaunchFailureScopeRegion, true, message, rawCode)
	}
}

func newLaunchFailure(
	req provider.StartInstanceRequest,
	code string,
	class provider.LaunchFailureClass,
	scope provider.LaunchFailureScope,
	retryable bool,
	message string,
	rawCode string,
) *provider.LaunchFailure {
	attributes := map[string]string{
		"provider":       "aws",
		"account_id":     strings.TrimSpace(req.AccountID),
		"region":         strings.TrimSpace(req.Region),
		"zone":           strings.TrimSpace(req.AvailabilityZone),
		"instance_type":  strings.TrimSpace(req.InstanceType),
		"pricing_model":  string(req.MarketType),
		"stack_name":     strings.TrimSpace(req.StackName),
		"instance_name":  strings.TrimSpace(req.InstanceName),
	}
	if strings.TrimSpace(rawCode) != "" {
		attributes["raw_code"] = strings.TrimSpace(rawCode)
	}

	return &provider.LaunchFailure{
		Code:       strings.TrimSpace(code),
		Class:      class,
		Scope:      scope,
		Retryable:  retryable,
		Message:    strings.TrimSpace(message),
		RawCode:    strings.TrimSpace(rawCode),
		Attributes: attributes,
	}
}
