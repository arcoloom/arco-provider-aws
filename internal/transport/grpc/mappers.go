package grpcserver

import (
	"time"

	providerv1 "github.com/arcoloom/arco-provider-aws/gen/proto/arco/provider/v1"
	"github.com/arcoloom/arco-provider-aws/internal/provider"
)

func toProtoMetadata(metadata provider.Metadata) *providerv1.ProviderMetadata {
	authSchemes := make([]providerv1.AuthScheme, 0, len(metadata.SupportedAuth))
	for _, scheme := range metadata.SupportedAuth {
		authSchemes = append(authSchemes, toProtoAuthScheme(scheme))
	}

	return &providerv1.ProviderMetadata{
		Name:                 metadata.Name,
		Version:              metadata.Version,
		Cloud:                toProtoCloud(metadata.Cloud),
		SupportedAuthSchemes: authSchemes,
		SupportedServices:    metadata.SupportedServices,
		Capabilities:         metadata.Capabilities,
	}
}

func toProtoWarnings(warnings []provider.Warning) []*providerv1.Warning {
	result := make([]*providerv1.Warning, 0, len(warnings))
	for _, warning := range warnings {
		result = append(result, &providerv1.Warning{
			Code:    warning.Code,
			Message: warning.Message,
		})
	}

	return result
}

func toProtoInstanceTags(tags []provider.InstanceTag) []*providerv1.InstanceTag {
	result := make([]*providerv1.InstanceTag, 0, len(tags))
	for _, tag := range tags {
		result = append(result, &providerv1.InstanceTag{
			Key:   tag.Key,
			Value: tag.Value,
		})
	}

	return result
}

func toProtoSpotData(items []provider.SpotData) []*providerv1.SpotData {
	result := make([]*providerv1.SpotData, 0, len(items))
	for _, item := range items {
		protoItem := &providerv1.SpotData{
			InstanceType:     item.InstanceType,
			Region:           item.Region,
			AvailabilityZone: item.AvailabilityZone,
			HasPrice:         item.HasPrice,
			Price:            item.Price,
			Currency:         item.Currency,
			Inventory: &providerv1.SpotInventory{
				Offered:          item.Inventory.Offered,
				Status:           item.Inventory.Status,
				HasCapacityScore: item.Inventory.HasCapacityScore,
				CapacityScore:    item.Inventory.CapacityScore,
			},
		}
		if !item.Timestamp.IsZero() {
			protoItem.Timestamp = item.Timestamp.Format(time.RFC3339)
		}

		result = append(result, protoItem)
	}

	return result
}

func toProtoRegions(regions []provider.Region) []*providerv1.CloudRegion {
	result := make([]*providerv1.CloudRegion, 0, len(regions))
	for _, region := range regions {
		result = append(result, &providerv1.CloudRegion{
			Code: region.Code,
			Name: region.Name,
		})
	}

	return result
}

func toProtoAccelerators(accelerators []provider.Accelerator) []*providerv1.Accelerator {
	result := make([]*providerv1.Accelerator, 0, len(accelerators))
	for _, accelerator := range accelerators {
		result = append(result, &providerv1.Accelerator{
			Kind:      toProtoAcceleratorKind(accelerator.Kind),
			Model:     accelerator.Model,
			Count:     accelerator.Count,
			MemoryGib: accelerator.MemoryGiB,
		})
	}

	return result
}

func toProtoLocalStorage(storage *provider.LocalStorage) *providerv1.LocalStorage {
	if storage == nil {
		return nil
	}

	return &providerv1.LocalStorage{
		HasLocalStorage: storage.HasLocalStorage,
		Medium:          storage.Medium,
		DiskCount:       storage.DiskCount,
		TotalSizeGib:    storage.TotalSizeGiB,
	}
}

func toProtoInstanceTypeSummaries(items []provider.InstanceTypeSummary) []*providerv1.InstanceTypeSummary {
	result := make([]*providerv1.InstanceTypeSummary, 0, len(items))
	for _, item := range items {
		result = append(result, &providerv1.InstanceTypeSummary{
			InstanceType:         item.InstanceType,
			Series:               item.Series,
			Family:               item.Family,
			Category:             item.Category,
			DisplayName:          item.DisplayName,
			Generation:           item.Generation,
			Vcpu:                 item.VCPU,
			MemoryGib:            item.MemoryGiB,
			Architectures:        item.Architectures,
			SupportedRegionCount: item.SupportedRegionCount,
		})
	}

	return result
}

func toProtoInstanceTypeInfos(items []provider.InstanceTypeInfo) []*providerv1.InstanceTypeInfo {
	result := make([]*providerv1.InstanceTypeInfo, 0, len(items))
	for _, item := range items {
		result = append(result, &providerv1.InstanceTypeInfo{
			InstanceType:              item.InstanceType,
			Series:                    item.Series,
			Family:                    item.Family,
			Category:                  item.Category,
			DisplayName:               item.DisplayName,
			Generation:                item.Generation,
			Vcpu:                      item.VCPU,
			MemoryGib:                 item.MemoryGiB,
			Architectures:             item.Architectures,
			CpuManufacturer:           item.CPUManufacturer,
			CpuModel:                  item.CPUModel,
			CpuClockSpeedGhz:          item.CPUClockSpeedGHz,
			NetworkPerformance:        item.NetworkPerformance,
			EnhancedNetworking:        item.EnhancedNetworking,
			Ipv6Supported:             item.IPv6Supported,
			PlacementGroupSupported:   item.PlacementGroupSupported,
			VpcOnly:                   item.VPCOnly,
			EbsOptimized:              item.EBSOptimized,
			SupportedRegions:          toProtoRegions(item.SupportedRegions),
			SupportedOperatingSystems: item.SupportedOperatingSystems,
			Accelerators:              toProtoAccelerators(item.Accelerators),
			LocalStorage:              toProtoLocalStorage(item.LocalStorage),
			Attributes:                item.Attributes,
		})
	}

	return result
}

func toProtoInstancePrices(items []provider.InstancePrice) []*providerv1.InstancePrice {
	result := make([]*providerv1.InstancePrice, 0, len(items))
	for _, item := range items {
		protoItem := &providerv1.InstancePrice{
			InstanceType:         item.InstanceType,
			Region:               &providerv1.CloudRegion{Code: item.Region.Code, Name: item.Region.Name},
			PurchaseOption:       toProtoPurchaseOption(item.PurchaseOption),
			OperatingSystem:      item.OperatingSystem,
			Tenancy:              item.Tenancy,
			PreinstalledSoftware: item.PreinstalledSoftware,
			LicenseModel:         item.LicenseModel,
			BillingUnit:          item.BillingUnit,
			Currency:             item.Currency,
			Price:                item.Price,
			Sku:                  item.SKU,
			Description:          item.Description,
		}
		if !item.EffectiveAt.IsZero() {
			protoItem.EffectiveAt = item.EffectiveAt.Format(time.RFC3339)
		}
		result = append(result, protoItem)
	}

	return result
}

func toDomainContext(ctx *providerv1.RequestContext) provider.RequestContext {
	if ctx == nil {
		return provider.RequestContext{}
	}

	return provider.RequestContext{
		RequestID: ctx.GetRequestId(),
		Caller:    ctx.GetCaller(),
		TraceID:   ctx.GetTraceId(),
	}
}

func toDomainScope(scope *providerv1.ConnectionScope) provider.ConnectionScope {
	if scope == nil {
		return provider.ConnectionScope{}
	}

	return provider.ConnectionScope{
		AccountID:      scope.GetAccountId(),
		Region:         scope.GetRegion(),
		Endpoint:       scope.GetEndpoint(),
		Attributes:     scope.GetAttributes(),
		EndpointRegion: scope.GetEndpointRegion(),
	}
}

func toDomainCredentials(credentials *providerv1.Credentials) provider.Credentials {
	if credentials == nil {
		return provider.Credentials{}
	}

	result := provider.Credentials{}

	switch auth := credentials.GetAuth().(type) {
	case *providerv1.Credentials_AwsIam:
		result.AWS = &provider.AWSCredentials{
			AccessKeyID:     auth.AwsIam.GetAccessKeyId(),
			SecretAccessKey: auth.AwsIam.GetSecretAccessKey(),
			SessionToken:    auth.AwsIam.GetSessionToken(),
			RoleARN:         auth.AwsIam.GetRoleArn(),
			ExternalID:      auth.AwsIam.GetExternalId(),
		}
	case *providerv1.Credentials_AzureClientSecret:
		result.Azure = &provider.AzureCredentials{
			TenantID:       auth.AzureClientSecret.GetTenantId(),
			ClientID:       auth.AzureClientSecret.GetClientId(),
			ClientSecret:   auth.AzureClientSecret.GetClientSecret(),
			SubscriptionID: auth.AzureClientSecret.GetSubscriptionId(),
		}
	case *providerv1.Credentials_GcpServiceAccount:
		result.GCP = &provider.GCPCredentials{
			ProjectID:    auth.GcpServiceAccount.GetProjectId(),
			ClientEmail:  auth.GcpServiceAccount.GetClientEmail(),
			PrivateKey:   auth.GcpServiceAccount.GetPrivateKey(),
			PrivateKeyID: auth.GcpServiceAccount.GetPrivateKeyId(),
		}
	}

	return result
}

func toDomainInstanceTags(tags []*providerv1.InstanceTag) []provider.InstanceTag {
	result := make([]provider.InstanceTag, 0, len(tags))
	for _, tag := range tags {
		if tag == nil {
			continue
		}
		result = append(result, provider.InstanceTag{
			Key:   tag.GetKey(),
			Value: tag.GetValue(),
		})
	}

	return result
}

func toDomainRegions(regions []*providerv1.CloudRegion) []provider.Region {
	result := make([]provider.Region, 0, len(regions))
	for _, region := range regions {
		if region == nil {
			continue
		}
		result = append(result, provider.Region{
			Code: region.GetCode(),
			Name: region.GetName(),
		})
	}

	return result
}

func toDomainAccelerators(accelerators []*providerv1.Accelerator) []provider.Accelerator {
	result := make([]provider.Accelerator, 0, len(accelerators))
	for _, accelerator := range accelerators {
		if accelerator == nil {
			continue
		}
		result = append(result, provider.Accelerator{
			Kind:      toDomainAcceleratorKind(accelerator.GetKind()),
			Model:     accelerator.GetModel(),
			Count:     accelerator.GetCount(),
			MemoryGiB: accelerator.GetMemoryGib(),
		})
	}

	return result
}

func toDomainLocalStorage(storage *providerv1.LocalStorage) *provider.LocalStorage {
	if storage == nil {
		return nil
	}

	return &provider.LocalStorage{
		HasLocalStorage: storage.GetHasLocalStorage(),
		Medium:          storage.GetMedium(),
		DiskCount:       storage.GetDiskCount(),
		TotalSizeGiB:    storage.GetTotalSizeGib(),
	}
}

func toProtoInstanceMarketType(marketType provider.InstanceMarketType) providerv1.InstanceMarketType {
	switch marketType {
	case provider.InstanceMarketTypeOnDemand:
		return providerv1.InstanceMarketType_INSTANCE_MARKET_TYPE_ON_DEMAND
	case provider.InstanceMarketTypeSpot:
		return providerv1.InstanceMarketType_INSTANCE_MARKET_TYPE_SPOT
	default:
		return providerv1.InstanceMarketType_INSTANCE_MARKET_TYPE_UNSPECIFIED
	}
}

func toProtoPurchaseOption(option provider.PurchaseOption) providerv1.PurchaseOption {
	switch option {
	case provider.PurchaseOptionOnDemand:
		return providerv1.PurchaseOption_PURCHASE_OPTION_ON_DEMAND
	case provider.PurchaseOptionSpot:
		return providerv1.PurchaseOption_PURCHASE_OPTION_SPOT
	default:
		return providerv1.PurchaseOption_PURCHASE_OPTION_UNSPECIFIED
	}
}

func toDomainPurchaseOption(option providerv1.PurchaseOption) provider.PurchaseOption {
	switch option {
	case providerv1.PurchaseOption_PURCHASE_OPTION_ON_DEMAND:
		return provider.PurchaseOptionOnDemand
	case providerv1.PurchaseOption_PURCHASE_OPTION_SPOT:
		return provider.PurchaseOptionSpot
	default:
		return ""
	}
}

func toProtoAcceleratorKind(kind provider.AcceleratorKind) providerv1.AcceleratorKind {
	switch kind {
	case provider.AcceleratorKindGPU:
		return providerv1.AcceleratorKind_ACCELERATOR_KIND_GPU
	case provider.AcceleratorKindFPGA:
		return providerv1.AcceleratorKind_ACCELERATOR_KIND_FPGA
	default:
		return providerv1.AcceleratorKind_ACCELERATOR_KIND_UNSPECIFIED
	}
}

func toDomainAcceleratorKind(kind providerv1.AcceleratorKind) provider.AcceleratorKind {
	switch kind {
	case providerv1.AcceleratorKind_ACCELERATOR_KIND_GPU:
		return provider.AcceleratorKindGPU
	case providerv1.AcceleratorKind_ACCELERATOR_KIND_FPGA:
		return provider.AcceleratorKindFPGA
	default:
		return ""
	}
}

func toDomainInstanceMarketType(marketType providerv1.InstanceMarketType) provider.InstanceMarketType {
	switch marketType {
	case providerv1.InstanceMarketType_INSTANCE_MARKET_TYPE_ON_DEMAND:
		return provider.InstanceMarketTypeOnDemand
	case providerv1.InstanceMarketType_INSTANCE_MARKET_TYPE_SPOT:
		return provider.InstanceMarketTypeSpot
	default:
		return ""
	}
}

func toProtoCloud(cloud provider.Cloud) providerv1.Cloud {
	switch cloud {
	case provider.CloudAWS:
		return providerv1.Cloud_CLOUD_AWS
	case provider.CloudAzure:
		return providerv1.Cloud_CLOUD_AZURE
	case provider.CloudGCP:
		return providerv1.Cloud_CLOUD_GCP
	default:
		return providerv1.Cloud_CLOUD_UNSPECIFIED
	}
}

func toProtoAuthScheme(scheme provider.AuthScheme) providerv1.AuthScheme {
	switch scheme {
	case provider.AuthSchemeAWSIAM:
		return providerv1.AuthScheme_AUTH_SCHEME_AWS_IAM
	case provider.AuthSchemeAzureClientSecret:
		return providerv1.AuthScheme_AUTH_SCHEME_AZURE_CLIENT_SECRET
	case provider.AuthSchemeGCPServiceAccount:
		return providerv1.AuthScheme_AUTH_SCHEME_GCP_SERVICE_ACCOUNT
	default:
		return providerv1.AuthScheme_AUTH_SCHEME_UNSPECIFIED
	}
}
