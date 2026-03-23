package aws

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/arcoloom/arco-provider-aws/internal/provider"
	"github.com/aws/smithy-go"
)

const (
	warningCodeMarketRegionSkipped = "MARKET_REGION_SKIPPED"
	warningCodeMarketBatchSkipped  = "MARKET_BATCH_SKIPPED"
)

func shouldSkipMarketRegionError(err error) bool {
	if isRecoverableMarketAPIError(err) {
		return true
	}
	return strings.Contains(strings.ToLower(strings.TrimSpace(err.Error())), "not available in catalog metadata")
}

func shouldSkipMarketBatchError(err error) bool {
	return isRecoverableMarketAPIError(err)
}

func isRecoverableMarketAPIError(err error) bool {
	if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}

	var apiErr smithy.APIError
	if !errors.As(err, &apiErr) {
		return false
	}

	code := strings.ToLower(strings.TrimSpace(apiErr.ErrorCode()))
	switch code {
	case "accessdenied", "accessdeniedexception", "unauthorizedoperation", "client.unauthorizedoperation",
		"authfailure", "optinrequired", "invalidparametervalue", "invalidparametercombination",
		"unsupportedoperation":
		return true
	}

	if strings.Contains(code, "invalidinstancetype") || strings.Contains(code, "unsupported") {
		return true
	}

	return false
}

func marketRegionWarning(kind string, region string, err error) provider.Warning {
	return provider.Warning{
		Code: warningCodeMarketRegionSkipped,
		Message: fmt.Sprintf(
			"skipped %s market sync for region %s: %v",
			strings.TrimSpace(kind),
			strings.TrimSpace(region),
			err,
		),
	}
}

func marketBatchWarning(region string, instanceTypes []string, operation string, err error) provider.Warning {
	return provider.Warning{
		Code: warningCodeMarketBatchSkipped,
		Message: fmt.Sprintf(
			"skipped market batch in region %s for %s (%s): %v",
			strings.TrimSpace(region),
			summarizeInstanceTypes(instanceTypes),
			strings.TrimSpace(operation),
			err,
		),
	}
}

func summarizeInstanceTypes(instanceTypes []string) string {
	trimmed := make([]string, 0, len(instanceTypes))
	for _, instanceType := range instanceTypes {
		if value := strings.TrimSpace(instanceType); value != "" {
			trimmed = append(trimmed, value)
		}
	}
	if len(trimmed) == 0 {
		return "no instance types"
	}
	if len(trimmed) <= 3 {
		return strings.Join(trimmed, ", ")
	}
	return fmt.Sprintf("%s, %s, %s (+%d more)", trimmed[0], trimmed[1], trimmed[2], len(trimmed)-3)
}
