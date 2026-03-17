package grpcserver

import (
	"fmt"
	"time"

	providerv1 "github.com/arcoloom/arco-provider-aws/gen/proto/arco/provider/v1"
	"github.com/arcoloom/arco-provider-aws/internal/provider"
)

func toProtoContext(ctx provider.RequestContext) *providerv1.RequestContext {
	return &providerv1.RequestContext{
		RequestId: ctx.RequestID,
		Caller:    ctx.Caller,
		TraceId:   ctx.TraceID,
	}
}

func toProtoScope(scope provider.ConnectionScope) *providerv1.ConnectionScope {
	return &providerv1.ConnectionScope{
		AccountId:      scope.AccountID,
		Region:         scope.Region,
		Endpoint:       scope.Endpoint,
		Attributes:     scope.Attributes,
		EndpointRegion: scope.EndpointRegion,
	}
}

func toProtoCredentials(credentials provider.Credentials) *providerv1.Credentials {
	switch {
	case credentials.AWS != nil:
		return &providerv1.Credentials{
			Auth: &providerv1.Credentials_AwsIam{
				AwsIam: &providerv1.AwsIamCredentials{
					AccessKeyId:     credentials.AWS.AccessKeyID,
					SecretAccessKey: credentials.AWS.SecretAccessKey,
					SessionToken:    credentials.AWS.SessionToken,
					RoleArn:         credentials.AWS.RoleARN,
					ExternalId:      credentials.AWS.ExternalID,
				},
			},
		}
	case credentials.Azure != nil:
		return &providerv1.Credentials{
			Auth: &providerv1.Credentials_AzureClientSecret{
				AzureClientSecret: &providerv1.AzureClientSecretCredentials{
					TenantId:       credentials.Azure.TenantID,
					ClientId:       credentials.Azure.ClientID,
					ClientSecret:   credentials.Azure.ClientSecret,
					SubscriptionId: credentials.Azure.SubscriptionID,
				},
			},
		}
	case credentials.GCP != nil:
		return &providerv1.Credentials{
			Auth: &providerv1.Credentials_GcpServiceAccount{
				GcpServiceAccount: &providerv1.GcpServiceAccountCredentials{
					ProjectId:    credentials.GCP.ProjectID,
					ClientEmail:  credentials.GCP.ClientEmail,
					PrivateKey:   credentials.GCP.PrivateKey,
					PrivateKeyId: credentials.GCP.PrivateKeyID,
				},
			},
		}
	default:
		return &providerv1.Credentials{}
	}
}

func toDomainMetadata(metadata *providerv1.ProviderMetadata) provider.Metadata {
	if metadata == nil {
		return provider.Metadata{}
	}

	authSchemes := make([]provider.AuthScheme, 0, len(metadata.GetSupportedAuthSchemes()))
	for _, scheme := range metadata.GetSupportedAuthSchemes() {
		authSchemes = append(authSchemes, toDomainAuthScheme(scheme))
	}

	return provider.Metadata{
		Name:              metadata.GetName(),
		Version:           metadata.GetVersion(),
		Cloud:             toDomainCloud(metadata.GetCloud()),
		SupportedAuth:     authSchemes,
		SupportedServices: metadata.GetSupportedServices(),
		Capabilities:      metadata.GetCapabilities(),
		ResourcePlanes:    toDomainResourcePlanes(metadata.GetResourcePlanes()),
	}
}

func toDomainResourcePlanes(values []providerv1.ResourcePlane) []provider.ResourcePlane {
	result := make([]provider.ResourcePlane, 0, len(values))
	for _, value := range values {
		result = append(result, toDomainResourcePlane(value))
	}
	return result
}

func toDomainResourcePlane(value providerv1.ResourcePlane) provider.ResourcePlane {
	switch value {
	case providerv1.ResourcePlane_RESOURCE_PLANE_COMPUTE:
		return provider.ResourcePlaneCompute
	case providerv1.ResourcePlane_RESOURCE_PLANE_STORAGE:
		return provider.ResourcePlaneStorage
	case providerv1.ResourcePlane_RESOURCE_PLANE_NETWORK:
		return provider.ResourcePlaneNetwork
	default:
		return ""
	}
}

func toDomainValidateConnectionResult(resp *providerv1.ValidateConnectionResponse) provider.ValidateConnectionResult {
	if resp == nil {
		return provider.ValidateConnectionResult{}
	}

	warnings := make([]provider.Warning, 0, len(resp.GetWarnings()))
	for _, warning := range resp.GetWarnings() {
		warnings = append(warnings, provider.Warning{
			Code:    warning.GetCode(),
			Message: warning.GetMessage(),
		})
	}

	return provider.ValidateConnectionResult{
		Accepted: resp.GetAccepted(),
		Message:  resp.GetMessage(),
		Warnings: warnings,
	}
}

func toDomainListRegionsResult(resp *providerv1.ListRegionsResponse) provider.ListRegionsResult {
	if resp == nil {
		return provider.ListRegionsResult{}
	}

	warnings := make([]provider.Warning, 0, len(resp.GetWarnings()))
	for _, warning := range resp.GetWarnings() {
		warnings = append(warnings, provider.Warning{
			Code:    warning.GetCode(),
			Message: warning.GetMessage(),
		})
	}

	return provider.ListRegionsResult{
		Items:    toDomainRegions(resp.GetItems()),
		Warnings: warnings,
	}
}

func toDomainListAvailabilityZonesResult(resp *providerv1.ListAvailabilityZonesResponse) provider.ListAvailabilityZonesResult {
	if resp == nil {
		return provider.ListAvailabilityZonesResult{}
	}

	warnings := make([]provider.Warning, 0, len(resp.GetWarnings()))
	for _, warning := range resp.GetWarnings() {
		warnings = append(warnings, provider.Warning{
			Code:    warning.GetCode(),
			Message: warning.GetMessage(),
		})
	}

	return provider.ListAvailabilityZonesResult{
		Items:    toDomainAvailabilityZones(resp.GetItems()),
		Warnings: warnings,
	}
}

func toDomainGetSpotDataResult(resp *providerv1.GetSpotDataResponse) (provider.GetSpotDataResult, error) {
	if resp == nil {
		return provider.GetSpotDataResult{}, nil
	}

	items := make([]provider.SpotData, 0, len(resp.GetItems()))
	for _, item := range resp.GetItems() {
		parsedItem, err := toDomainSpotData(item)
		if err != nil {
			return provider.GetSpotDataResult{}, err
		}
		items = append(items, parsedItem)
	}

	warnings := make([]provider.Warning, 0, len(resp.GetWarnings()))
	for _, warning := range resp.GetWarnings() {
		warnings = append(warnings, provider.Warning{
			Code:    warning.GetCode(),
			Message: warning.GetMessage(),
		})
	}

	return provider.GetSpotDataResult{
		Items:    items,
		Warnings: warnings,
	}, nil
}

func toDomainStartInstanceResult(resp *providerv1.StartInstanceResponse) provider.StartInstanceResult {
	if resp == nil {
		return provider.StartInstanceResult{}
	}

	warnings := make([]provider.Warning, 0, len(resp.GetWarnings()))
	for _, warning := range resp.GetWarnings() {
		warnings = append(warnings, provider.Warning{
			Code:    warning.GetCode(),
			Message: warning.GetMessage(),
		})
	}

	return provider.StartInstanceResult{
		StackName:  resp.GetStackName(),
		InstanceID: resp.GetInstanceId(),
		URN:        resp.GetUrn(),
		PublicIP:   resp.GetPublicIp(),
		PrivateIP:  resp.GetPrivateIp(),
		Warnings:   warnings,
	}
}

func toDomainStopInstanceResult(resp *providerv1.StopInstanceResponse) provider.StopInstanceResult {
	if resp == nil {
		return provider.StopInstanceResult{}
	}

	warnings := make([]provider.Warning, 0, len(resp.GetWarnings()))
	for _, warning := range resp.GetWarnings() {
		warnings = append(warnings, provider.Warning{
			Code:    warning.GetCode(),
			Message: warning.GetMessage(),
		})
	}

	return provider.StopInstanceResult{
		StackName: resp.GetStackName(),
		Destroyed: resp.GetDestroyed(),
		Warnings:  warnings,
	}
}

func toDomainListActiveInstancesResult(resp *providerv1.ListActiveInstancesResponse) (provider.ListActiveInstancesResult, error) {
	if resp == nil {
		return provider.ListActiveInstancesResult{}, nil
	}

	items := make([]provider.ActiveInstance, 0, len(resp.GetItems()))
	for _, item := range resp.GetItems() {
		if item == nil {
			continue
		}

		parsed := provider.ActiveInstance{
			InstanceID:       item.GetInstanceId(),
			Name:             item.GetName(),
			Region:           item.GetRegion(),
			AvailabilityZone: item.GetAvailabilityZone(),
			InstanceType:     item.GetInstanceType(),
			State:            item.GetState(),
			MarketType:       toDomainInstanceMarketType(item.GetMarketType()),
			PublicIP:         item.GetPublicIp(),
			PrivateIP:        item.GetPrivateIp(),
			IPv6Addresses:    item.GetIpv6Addresses(),
			SubnetID:         item.GetSubnetId(),
			VPCID:            item.GetVpcId(),
			Tags:             toDomainInstanceTags(item.GetTags()),
		}
		if timestamp := item.GetLaunchTime(); timestamp != "" {
			launchTime, err := time.Parse(time.RFC3339, timestamp)
			if err != nil {
				return provider.ListActiveInstancesResult{}, fmt.Errorf("parse active instance timestamp: %w", err)
			}
			parsed.LaunchTime = launchTime
		}

		items = append(items, parsed)
	}

	warnings := make([]provider.Warning, 0, len(resp.GetWarnings()))
	for _, warning := range resp.GetWarnings() {
		warnings = append(warnings, provider.Warning{
			Code:    warning.GetCode(),
			Message: warning.GetMessage(),
		})
	}

	return provider.ListActiveInstancesResult{
		Items:    items,
		Warnings: warnings,
	}, nil
}

func toDomainListInstanceTypesResult(resp *providerv1.ListInstanceTypesResponse) provider.ListInstanceTypesResult {
	if resp == nil {
		return provider.ListInstanceTypesResult{}
	}

	items := make([]provider.InstanceTypeSummary, 0, len(resp.GetItems()))
	for _, item := range resp.GetItems() {
		if item == nil {
			continue
		}
		items = append(items, provider.InstanceTypeSummary{
			InstanceType:         item.GetInstanceType(),
			Series:               item.GetSeries(),
			Family:               item.GetFamily(),
			Category:             item.GetCategory(),
			DisplayName:          item.GetDisplayName(),
			Generation:           item.GetGeneration(),
			VCPU:                 item.GetVcpu(),
			MemoryGiB:            item.GetMemoryGib(),
			Architectures:        item.GetArchitectures(),
			SupportedRegionCount: item.GetSupportedRegionCount(),
		})
	}

	warnings := make([]provider.Warning, 0, len(resp.GetWarnings()))
	for _, warning := range resp.GetWarnings() {
		warnings = append(warnings, provider.Warning{
			Code:    warning.GetCode(),
			Message: warning.GetMessage(),
		})
	}

	return provider.ListInstanceTypesResult{
		Items:    items,
		Warnings: warnings,
	}
}

func toDomainGetInstanceTypeInfoResult(resp *providerv1.GetInstanceTypeInfoResponse) provider.GetInstanceTypeInfoResult {
	if resp == nil {
		return provider.GetInstanceTypeInfoResult{}
	}

	items := make([]provider.InstanceTypeInfo, 0, len(resp.GetItems()))
	for _, item := range resp.GetItems() {
		if item == nil {
			continue
		}
		items = append(items, provider.InstanceTypeInfo{
			InstanceType:              item.GetInstanceType(),
			Series:                    item.GetSeries(),
			Family:                    item.GetFamily(),
			Category:                  item.GetCategory(),
			DisplayName:               item.GetDisplayName(),
			Generation:                item.GetGeneration(),
			VCPU:                      item.GetVcpu(),
			MemoryGiB:                 item.GetMemoryGib(),
			Architectures:             item.GetArchitectures(),
			CPUManufacturer:           item.GetCpuManufacturer(),
			CPUModel:                  item.GetCpuModel(),
			CPUClockSpeedGHz:          item.GetCpuClockSpeedGhz(),
			NetworkPerformance:        item.GetNetworkPerformance(),
			EnhancedNetworking:        item.GetEnhancedNetworking(),
			IPv6Supported:             item.GetIpv6Supported(),
			PlacementGroupSupported:   item.GetPlacementGroupSupported(),
			VPCOnly:                   item.GetVpcOnly(),
			EBSOptimized:              item.GetEbsOptimized(),
			SupportedRegions:          toDomainRegions(item.GetSupportedRegions()),
			SupportedOperatingSystems: item.GetSupportedOperatingSystems(),
			Accelerators:              toDomainAccelerators(item.GetAccelerators()),
			LocalStorage:              toDomainLocalStorage(item.GetLocalStorage()),
			Attributes:                item.GetAttributes(),
		})
	}

	warnings := make([]provider.Warning, 0, len(resp.GetWarnings()))
	for _, warning := range resp.GetWarnings() {
		warnings = append(warnings, provider.Warning{
			Code:    warning.GetCode(),
			Message: warning.GetMessage(),
		})
	}

	return provider.GetInstanceTypeInfoResult{
		Items:    items,
		Warnings: warnings,
	}
}

func toDomainGetInstancePricesResult(resp *providerv1.GetInstancePricesResponse) (provider.GetInstancePricesResult, error) {
	if resp == nil {
		return provider.GetInstancePricesResult{}, nil
	}

	items := make([]provider.InstancePrice, 0, len(resp.GetItems()))
	for _, item := range resp.GetItems() {
		if item == nil {
			continue
		}

		region := provider.Region{}
		if item.GetRegion() != nil {
			region = provider.Region{
				Code: item.GetRegion().GetCode(),
				Name: item.GetRegion().GetName(),
			}
		}

		parsed := provider.InstancePrice{
			InstanceType:         item.GetInstanceType(),
			Region:               region,
			PurchaseOption:       toDomainPurchaseOption(item.GetPurchaseOption()),
			OperatingSystem:      item.GetOperatingSystem(),
			Tenancy:              item.GetTenancy(),
			PreinstalledSoftware: item.GetPreinstalledSoftware(),
			LicenseModel:         item.GetLicenseModel(),
			BillingUnit:          item.GetBillingUnit(),
			Currency:             item.GetCurrency(),
			Price:                item.GetPrice(),
			SKU:                  item.GetSku(),
			Description:          item.GetDescription(),
		}
		if timestamp := item.GetEffectiveAt(); timestamp != "" {
			effectiveAt, err := time.Parse(time.RFC3339, timestamp)
			if err != nil {
				return provider.GetInstancePricesResult{}, fmt.Errorf("parse instance price timestamp: %w", err)
			}
			parsed.EffectiveAt = effectiveAt
		}

		items = append(items, parsed)
	}

	warnings := make([]provider.Warning, 0, len(resp.GetWarnings()))
	for _, warning := range resp.GetWarnings() {
		warnings = append(warnings, provider.Warning{
			Code:    warning.GetCode(),
			Message: warning.GetMessage(),
		})
	}

	return provider.GetInstancePricesResult{
		Items:    items,
		Warnings: warnings,
	}, nil
}

func toDomainSpotData(item *providerv1.SpotData) (provider.SpotData, error) {
	if item == nil {
		return provider.SpotData{}, nil
	}

	result := provider.SpotData{
		InstanceType:     item.GetInstanceType(),
		Region:           item.GetRegion(),
		AvailabilityZone: item.GetAvailabilityZone(),
		HasPrice:         item.GetHasPrice(),
		Price:            item.GetPrice(),
		Currency:         item.GetCurrency(),
	}

	if timestamp := item.GetTimestamp(); timestamp != "" {
		parsed, err := time.Parse(time.RFC3339, timestamp)
		if err != nil {
			return provider.SpotData{}, fmt.Errorf("parse spot data timestamp: %w", err)
		}
		result.Timestamp = parsed
	}

	if inventory := item.GetInventory(); inventory != nil {
		result.Inventory = provider.SpotInventory{
			Offered:          inventory.GetOffered(),
			Status:           inventory.GetStatus(),
			HasCapacityScore: inventory.GetHasCapacityScore(),
			CapacityScore:    inventory.GetCapacityScore(),
		}
	}

	return result, nil
}

func toDomainCloud(cloud providerv1.Cloud) provider.Cloud {
	switch cloud {
	case providerv1.Cloud_CLOUD_AWS:
		return provider.CloudAWS
	case providerv1.Cloud_CLOUD_AZURE:
		return provider.CloudAzure
	case providerv1.Cloud_CLOUD_GCP:
		return provider.CloudGCP
	default:
		return ""
	}
}

func toDomainAuthScheme(scheme providerv1.AuthScheme) provider.AuthScheme {
	switch scheme {
	case providerv1.AuthScheme_AUTH_SCHEME_AWS_IAM:
		return provider.AuthSchemeAWSIAM
	case providerv1.AuthScheme_AUTH_SCHEME_AZURE_CLIENT_SECRET:
		return provider.AuthSchemeAzureClientSecret
	case providerv1.AuthScheme_AUTH_SCHEME_GCP_SERVICE_ACCOUNT:
		return provider.AuthSchemeGCPServiceAccount
	default:
		return ""
	}
}
