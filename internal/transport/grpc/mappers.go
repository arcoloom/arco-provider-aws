package grpcserver

import (
	"strings"
	"time"

	providerv1 "github.com/arcoloom/arco-proto/gen/go/arcoloom/provider/v1"
	"github.com/arcoloom/arco-provider-aws/internal/provider"
	"google.golang.org/protobuf/types/known/structpb"
)

func toProtoMetadata(metadata provider.Metadata) *providerv1.ProviderMetadata {
	return &providerv1.ProviderMetadata{
		Name:              metadata.Name,
		Version:           metadata.Version,
		Cloud:             metadata.Cloud,
		AuthMethods:       toProtoAuthMethods(metadata.AuthMethods),
		SupportedServices: metadata.SupportedServices,
		Capabilities:      metadata.Capabilities,
		ResourcePlanes:    toProtoResourcePlanes(metadata.ResourcePlanes),
	}
}

func toProtoAuthMethods(methods []provider.AuthMethod) []*providerv1.ProviderAuthMethod {
	result := make([]*providerv1.ProviderAuthMethod, 0, len(methods))
	for _, method := range methods {
		result = append(result, &providerv1.ProviderAuthMethod{
			Name:        method.Name,
			DisplayName: method.DisplayName,
			Description: method.Description,
			Fields:      toProtoSchemaAttributes(method.Fields),
		})
	}
	return result
}

func toProtoResourceSchemas(resources []provider.ResourceSchema) []*providerv1.ProviderResourceSchema {
	result := make([]*providerv1.ProviderResourceSchema, 0, len(resources))
	for _, resource := range resources {
		result = append(result, toProtoResourceSchema(resource))
	}
	return result
}

func toProtoResourceSchema(resource provider.ResourceSchema) *providerv1.ProviderResourceSchema {
	return &providerv1.ProviderResourceSchema{
		Type:        resource.Type,
		Description: resource.Description,
		Attributes:  toProtoSchemaAttributes(resource.Attributes),
	}
}

func toProtoSchemaAttributes(attributes []provider.SchemaAttribute) []*providerv1.SchemaAttribute {
	result := make([]*providerv1.SchemaAttribute, 0, len(attributes))
	for _, attribute := range attributes {
		result = append(result, toProtoSchemaAttribute(attribute))
	}
	return result
}

func toProtoSchemaAttribute(attribute provider.SchemaAttribute) *providerv1.SchemaAttribute {
	return &providerv1.SchemaAttribute{
		Name:         attribute.Name,
		Type:         toProtoSchemaAttributeType(attribute.Type),
		Required:     attribute.Required,
		Optional:     attribute.Optional,
		Computed:     attribute.Computed,
		Sensitive:    attribute.Sensitive,
		Description:  attribute.Description,
		DefaultValue: toProtoDefaultValue(attribute.DefaultValue),
	}
}

func toProtoSchemaAttributeType(value provider.SchemaAttributeType) providerv1.SchemaAttributeType {
	switch value {
	case provider.SchemaAttributeTypeString:
		return providerv1.SchemaAttributeType_SCHEMA_ATTRIBUTE_TYPE_STRING
	case provider.SchemaAttributeTypeBool:
		return providerv1.SchemaAttributeType_SCHEMA_ATTRIBUTE_TYPE_BOOL
	case provider.SchemaAttributeTypeInt64:
		return providerv1.SchemaAttributeType_SCHEMA_ATTRIBUTE_TYPE_INT64
	case provider.SchemaAttributeTypeFloat64:
		return providerv1.SchemaAttributeType_SCHEMA_ATTRIBUTE_TYPE_FLOAT64
	case provider.SchemaAttributeTypeStringList:
		return providerv1.SchemaAttributeType_SCHEMA_ATTRIBUTE_TYPE_STRING_LIST
	case provider.SchemaAttributeTypeStringMap:
		return providerv1.SchemaAttributeType_SCHEMA_ATTRIBUTE_TYPE_STRING_MAP
	default:
		return providerv1.SchemaAttributeType_SCHEMA_ATTRIBUTE_TYPE_UNSPECIFIED
	}
}

func toProtoDefaultValue(value any) *structpb.Value {
	if value == nil {
		return nil
	}

	protoValue, err := structpb.NewValue(value)
	if err != nil {
		return nil
	}
	return protoValue
}

func toProtoResourcePlanes(values []provider.ResourcePlane) []providerv1.ResourcePlane {
	result := make([]providerv1.ResourcePlane, 0, len(values))
	for _, value := range values {
		result = append(result, toProtoResourcePlane(value))
	}
	return result
}

func toProtoResourcePlane(value provider.ResourcePlane) providerv1.ResourcePlane {
	switch value {
	case provider.ResourcePlaneCompute:
		return providerv1.ResourcePlane_RESOURCE_PLANE_COMPUTE
	case provider.ResourcePlaneStorage:
		return providerv1.ResourcePlane_RESOURCE_PLANE_STORAGE
	case provider.ResourcePlaneNetwork:
		return providerv1.ResourcePlane_RESOURCE_PLANE_NETWORK
	default:
		return providerv1.ResourcePlane_RESOURCE_PLANE_UNSPECIFIED
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

func toProtoActiveInstances(items []provider.ActiveInstance) []*providerv1.ActiveInstance {
	result := make([]*providerv1.ActiveInstance, 0, len(items))
	for _, item := range items {
		protoItem := &providerv1.ActiveInstance{
			InstanceId:         item.InstanceID,
			Name:               item.Name,
			Region:             item.Region,
			AvailabilityZone:   item.AvailabilityZone,
			InstanceType:       item.InstanceType,
			State:              item.State,
			MarketType:         toProtoInstanceMarketType(item.MarketType),
			PublicIp:           item.PublicIP,
			PrivateIp:          item.PrivateIP,
			Ipv6Addresses:      item.IPv6Addresses,
			Tags:               toProtoInstanceTags(item.Tags),
			ProviderAttributes: item.ProviderAttributes,
		}
		if !item.LaunchTime.IsZero() {
			protoItem.LaunchTime = item.LaunchTime.Format(time.RFC3339)
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

func toProtoAvailabilityZones(zones []provider.AvailabilityZone) []*providerv1.AvailabilityZone {
	result := make([]*providerv1.AvailabilityZone, 0, len(zones))
	for _, zone := range zones {
		result = append(result, &providerv1.AvailabilityZone{
			Name:               zone.Name,
			ZoneId:             zone.ZoneID,
			Region:             zone.Region,
			State:              zone.State,
			ZoneType:           zone.ZoneType,
			GroupName:          zone.GroupName,
			NetworkBorderGroup: zone.NetworkBorderGroup,
			ParentZoneId:       zone.ParentZoneID,
			ParentZoneName:     zone.ParentZoneName,
			OptInStatus:        zone.OptInStatus,
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

func toDomainProviderConfig(config *structpb.Struct) map[string]any {
	if config == nil {
		return nil
	}

	values := config.AsMap()
	if len(values) == 0 {
		return nil
	}

	return values
}

func toDomainCredentials(credentials *providerv1.Credentials) provider.Credentials {
	if credentials == nil {
		return provider.Credentials{}
	}

	data := map[string]any{}
	if credentials.GetData() != nil {
		data = credentials.GetData().AsMap()
	}

	switch strings.TrimSpace(credentials.GetAuthMethod()) {
	case provider.AuthMethodAWSDefaultCredentials:
		return provider.Credentials{
			AWS: &provider.AWSCredentials{
				UseDefaultCredentialsChain: true,
				Profile:                    strings.TrimSpace(asString(data["profile"])),
				RoleARN:                    strings.TrimSpace(asString(data["role_arn"])),
				ExternalID:                 strings.TrimSpace(asString(data["external_id"])),
				RoleSessionName:            strings.TrimSpace(asString(data["role_session_name"])),
				SourceIdentity:             strings.TrimSpace(asString(data["source_identity"])),
			},
		}
	case provider.AuthMethodAWSStaticAccessKey:
		return provider.Credentials{
			AWS: &provider.AWSCredentials{
				AccessKeyID:     strings.TrimSpace(asString(data["access_key_id"])),
				SecretAccessKey: strings.TrimSpace(asString(data["secret_access_key"])),
				SessionToken:    strings.TrimSpace(asString(data["session_token"])),
			},
		}
	case provider.AuthMethodAWSAssumeRole:
		return provider.Credentials{
			AWS: &provider.AWSCredentials{
				AccessKeyID:     strings.TrimSpace(asString(data["access_key_id"])),
				SecretAccessKey: strings.TrimSpace(asString(data["secret_access_key"])),
				SessionToken:    strings.TrimSpace(asString(data["session_token"])),
				RoleARN:         strings.TrimSpace(asString(data["role_arn"])),
				ExternalID:      strings.TrimSpace(asString(data["external_id"])),
				RoleSessionName: strings.TrimSpace(asString(data["role_session_name"])),
				SourceIdentity:  strings.TrimSpace(asString(data["source_identity"])),
			},
		}
	}

	return provider.Credentials{}
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

func toDomainAvailabilityZones(zones []*providerv1.AvailabilityZone) []provider.AvailabilityZone {
	result := make([]provider.AvailabilityZone, 0, len(zones))
	for _, zone := range zones {
		if zone == nil {
			continue
		}
		result = append(result, provider.AvailabilityZone{
			Name:               zone.GetName(),
			ZoneID:             zone.GetZoneId(),
			Region:             zone.GetRegion(),
			State:              zone.GetState(),
			ZoneType:           zone.GetZoneType(),
			GroupName:          zone.GetGroupName(),
			NetworkBorderGroup: zone.GetNetworkBorderGroup(),
			ParentZoneID:       zone.GetParentZoneId(),
			ParentZoneName:     zone.GetParentZoneName(),
			OptInStatus:        zone.GetOptInStatus(),
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

func asString(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	default:
		return ""
	}
}
