package grpcserver

import (
	"fmt"
	"time"

	providerv1 "github.com/arcoloom/arco-proto/gen/go/arcoloom/provider/v1"
	"github.com/arcoloom/arco-provider-aws/internal/provider"
	"google.golang.org/protobuf/types/known/structpb"
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
		ScopeId:        scope.ScopeID,
		Region:         scope.Region,
		Endpoint:       scope.Endpoint,
		Attributes:     scope.Attributes,
		EndpointRegion: scope.EndpointRegion,
	}
}

func toProtoProviderConfig(config map[string]any) *structpb.Struct {
	if len(config) == 0 {
		return nil
	}

	protoConfig, err := structpb.NewStruct(config)
	if err != nil {
		return nil
	}
	return protoConfig
}

func toProtoCredentials(credentials provider.Credentials) *providerv1.Credentials {
	if len(credentials.AWSAccounts) != 0 {
		accounts := make([]any, 0, len(credentials.AWSAccounts))
		for _, account := range credentials.AWSAccounts {
			item := map[string]any{
				"name": account.Name,
			}
			switch {
			case account.Credentials.UseDefaultCredentialsChain:
				item["auth_method"] = provider.AuthMethodAWSDefaultCredentials
				if account.Credentials.Profile != "" {
					item["profile"] = account.Credentials.Profile
				}
			case account.Credentials.RoleARN != "":
				item["auth_method"] = provider.AuthMethodAWSAssumeRole
				item["access_key_id"] = account.Credentials.AccessKeyID
				item["secret_access_key"] = account.Credentials.SecretAccessKey
				if account.Credentials.SessionToken != "" {
					item["session_token"] = account.Credentials.SessionToken
				}
				item["role_arn"] = account.Credentials.RoleARN
			default:
				item["auth_method"] = provider.AuthMethodAWSStaticAccessKey
				item["access_key_id"] = account.Credentials.AccessKeyID
				item["secret_access_key"] = account.Credentials.SecretAccessKey
				if account.Credentials.SessionToken != "" {
					item["session_token"] = account.Credentials.SessionToken
				}
			}
			if account.Credentials.ExternalID != "" {
				item["external_id"] = account.Credentials.ExternalID
			}
			if account.Credentials.RoleSessionName != "" {
				item["role_session_name"] = account.Credentials.RoleSessionName
			}
			if account.Credentials.SourceIdentity != "" {
				item["source_identity"] = account.Credentials.SourceIdentity
			}
			accounts = append(accounts, item)
		}
		return &providerv1.Credentials{
			Data: toProtoProviderConfig(map[string]any{
				"accounts": accounts,
			}),
		}
	}

	switch {
	case credentials.AWS != nil:
		authMethod := provider.AuthMethodAWSStaticAccessKey
		if credentials.AWS.UseDefaultCredentialsChain {
			authMethod = provider.AuthMethodAWSDefaultCredentials
		} else if credentials.AWS.RoleARN != "" {
			authMethod = provider.AuthMethodAWSAssumeRole
		}
		data := map[string]any{}
		if !credentials.AWS.UseDefaultCredentialsChain {
			data["access_key_id"] = credentials.AWS.AccessKeyID
			data["secret_access_key"] = credentials.AWS.SecretAccessKey
		}
		if credentials.AWS.Profile != "" {
			data["profile"] = credentials.AWS.Profile
		}
		if credentials.AWS.SessionToken != "" && !credentials.AWS.UseDefaultCredentialsChain {
			data["session_token"] = credentials.AWS.SessionToken
		}
		if credentials.AWS.RoleARN != "" {
			data["role_arn"] = credentials.AWS.RoleARN
		}
		if credentials.AWS.ExternalID != "" {
			data["external_id"] = credentials.AWS.ExternalID
		}
		if credentials.AWS.RoleSessionName != "" {
			data["role_session_name"] = credentials.AWS.RoleSessionName
		}
		if credentials.AWS.SourceIdentity != "" {
			data["source_identity"] = credentials.AWS.SourceIdentity
		}
		return &providerv1.Credentials{
			AuthMethod: authMethod,
			Data:       toProtoProviderConfig(data),
		}
	default:
		return &providerv1.Credentials{}
	}
}

func toDomainMetadata(metadata *providerv1.ProviderMetadata) provider.Metadata {
	if metadata == nil {
		return provider.Metadata{}
	}

	return provider.Metadata{
		Name:              metadata.GetName(),
		Version:           metadata.GetVersion(),
		Cloud:             metadata.GetCloud(),
		AuthMethods:       toDomainAuthMethods(metadata.GetAuthMethods()),
		SupportedServices: metadata.GetSupportedServices(),
		Capabilities:      metadata.GetCapabilities(),
		ResourcePlanes:    toDomainResourcePlanes(metadata.GetResourcePlanes()),
	}
}

func toDomainAuthMethods(methods []*providerv1.ProviderAuthMethod) []provider.AuthMethod {
	result := make([]provider.AuthMethod, 0, len(methods))
	for _, method := range methods {
		if method == nil {
			continue
		}
		result = append(result, provider.AuthMethod{
			Name:        method.GetName(),
			DisplayName: method.GetDisplayName(),
			Description: method.GetDescription(),
			Fields:      toDomainSchemaAttributes(method.GetFields()),
		})
	}
	return result
}

func toDomainResourceSchemas(resources []*providerv1.ProviderResourceSchema) []provider.ResourceSchema {
	result := make([]provider.ResourceSchema, 0, len(resources))
	for _, resource := range resources {
		if resource == nil {
			continue
		}
		result = append(result, provider.ResourceSchema{
			Type:        resource.GetType(),
			Description: resource.GetDescription(),
			Attributes:  toDomainSchemaAttributes(resource.GetAttributes()),
		})
	}
	return result
}

func toDomainSchemaAttributes(attributes []*providerv1.SchemaAttribute) []provider.SchemaAttribute {
	result := make([]provider.SchemaAttribute, 0, len(attributes))
	for _, attribute := range attributes {
		if attribute == nil {
			continue
		}
		result = append(result, provider.SchemaAttribute{
			Name:         attribute.GetName(),
			Type:         toDomainSchemaAttributeType(attribute.GetType()),
			Required:     attribute.GetRequired(),
			Optional:     attribute.GetOptional(),
			Computed:     attribute.GetComputed(),
			Sensitive:    attribute.GetSensitive(),
			Description:  attribute.GetDescription(),
			DefaultValue: toDomainDefaultValue(attribute.GetDefaultValue()),
		})
	}
	return result
}

func toDomainSchemaAttributeType(value providerv1.SchemaAttributeType) provider.SchemaAttributeType {
	switch value {
	case providerv1.SchemaAttributeType_SCHEMA_ATTRIBUTE_TYPE_STRING:
		return provider.SchemaAttributeTypeString
	case providerv1.SchemaAttributeType_SCHEMA_ATTRIBUTE_TYPE_BOOL:
		return provider.SchemaAttributeTypeBool
	case providerv1.SchemaAttributeType_SCHEMA_ATTRIBUTE_TYPE_INT64:
		return provider.SchemaAttributeTypeInt64
	case providerv1.SchemaAttributeType_SCHEMA_ATTRIBUTE_TYPE_FLOAT64:
		return provider.SchemaAttributeTypeFloat64
	case providerv1.SchemaAttributeType_SCHEMA_ATTRIBUTE_TYPE_STRING_LIST:
		return provider.SchemaAttributeTypeStringList
	case providerv1.SchemaAttributeType_SCHEMA_ATTRIBUTE_TYPE_STRING_MAP:
		return provider.SchemaAttributeTypeStringMap
	default:
		return ""
	}
}

func toDomainDefaultValue(value *structpb.Value) any {
	if value == nil {
		return nil
	}
	return value.AsInterface()
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

func toDomainWatchMarketFeedEvent(resp *providerv1.WatchMarketFeedResponse) provider.WatchMarketFeedEvent {
	if resp == nil {
		return provider.WatchMarketFeedEvent{}
	}

	switch event := resp.GetEvent().(type) {
	case *providerv1.WatchMarketFeedResponse_Begin:
		return provider.WatchMarketFeedEvent{
			Type:          provider.WatchMarketFeedEventTypeBegin,
			SnapshotToken: event.Begin.GetSnapshotToken(),
		}
	case *providerv1.WatchMarketFeedResponse_Chunk:
		return provider.WatchMarketFeedEvent{
			Type:      provider.WatchMarketFeedEventTypeChunk,
			Offerings: toDomainMarketOfferings(event.Chunk.GetOfferings()),
		}
	case *providerv1.WatchMarketFeedResponse_Commit:
		return provider.WatchMarketFeedEvent{
			Type:          provider.WatchMarketFeedEventTypeCommit,
			SnapshotToken: event.Commit.GetSnapshotToken(),
			ResumeToken:   event.Commit.GetResumeToken(),
		}
	case *providerv1.WatchMarketFeedResponse_Heartbeat:
		return provider.WatchMarketFeedEvent{
			Type:        provider.WatchMarketFeedEventTypeHeartbeat,
			ResumeToken: event.Heartbeat.GetResumeToken(),
			Warnings:    toDomainWarnings(event.Heartbeat.GetWarnings()),
		}
	case *providerv1.WatchMarketFeedResponse_Warning:
		return provider.WatchMarketFeedEvent{
			Type: provider.WatchMarketFeedEventTypeWarning,
			Warnings: []provider.Warning{{
				Code:    event.Warning.GetCode(),
				Message: event.Warning.GetMessage(),
			}},
		}
	default:
		return provider.WatchMarketFeedEvent{}
	}
}

func toDomainMarketOfferings(items []*providerv1.MarketOffering) []provider.MarketOffering {
	result := make([]provider.MarketOffering, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		result = append(result, provider.MarketOffering{
			ScopeID:          item.GetScopeId(),
			Region:           item.GetRegion(),
			AvailabilityZone: item.GetAvailabilityZone(),
			ZoneID:           item.GetZoneId(),
			InstanceType:     item.GetInstanceType(),
			PurchaseOption:   toDomainPurchaseOption(item.GetPurchaseOption()),
			CPUMilli:         item.GetCpuMilli(),
			MemoryMiB:        item.GetMemoryMib(),
			GPUCount:         item.GetGpuCount(),
			HourlyPriceUSD:   item.GetHourlyPriceUsd(),
			Attributes:       item.GetAttributes(),
		})
	}
	return result
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
		StackName:     resp.GetStackName(),
		InstanceID:    resp.GetInstanceId(),
		URN:           resp.GetUrn(),
		PublicIP:      resp.GetPublicIp(),
		PrivateIP:     resp.GetPrivateIp(),
		Warnings:      warnings,
		LaunchFailure: toDomainLaunchFailure(resp.GetLaunchFailure()),
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
		InstanceID: resp.GetInstanceId(),
		Destroyed:  resp.GetDestroyed(),
		Warnings:   warnings,
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
			InstanceID:         item.GetInstanceId(),
			Name:               item.GetName(),
			Region:             item.GetRegion(),
			AvailabilityZone:   item.GetAvailabilityZone(),
			InstanceType:       item.GetInstanceType(),
			State:              item.GetState(),
			MarketType:         toDomainInstanceMarketType(item.GetMarketType()),
			PublicIP:           item.GetPublicIp(),
			PrivateIP:          item.GetPrivateIp(),
			IPv6Addresses:      item.GetIpv6Addresses(),
			Tags:               toDomainInstanceTags(item.GetTags()),
			ProviderAttributes: item.GetProviderAttributes(),
			ScopeID:            item.GetScopeId(),
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
		Items:          items,
		Warnings:       warnings,
		NextCursor:     resp.GetNextCursor(),
		CoveredRegions: append([]string(nil), resp.GetCoveredRegions()...),
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

func toDomainWarnings(items []*providerv1.Warning) []provider.Warning {
	warnings := make([]provider.Warning, 0, len(items))
	for _, warning := range items {
		if warning == nil {
			continue
		}
		warnings = append(warnings, provider.Warning{
			Code:    warning.GetCode(),
			Message: warning.GetMessage(),
		})
	}
	return warnings
}

func toDomainLaunchFailureClass(value providerv1.LaunchFailureClass) provider.LaunchFailureClass {
	switch value {
	case providerv1.LaunchFailureClass_LAUNCH_FAILURE_CLASS_CAPACITY:
		return provider.LaunchFailureClassCapacity
	case providerv1.LaunchFailureClass_LAUNCH_FAILURE_CLASS_QUOTA:
		return provider.LaunchFailureClassQuota
	case providerv1.LaunchFailureClass_LAUNCH_FAILURE_CLASS_PRICE:
		return provider.LaunchFailureClassPrice
	case providerv1.LaunchFailureClass_LAUNCH_FAILURE_CLASS_API:
		return provider.LaunchFailureClassAPI
	case providerv1.LaunchFailureClass_LAUNCH_FAILURE_CLASS_AUTH:
		return provider.LaunchFailureClassAuth
	case providerv1.LaunchFailureClass_LAUNCH_FAILURE_CLASS_CONFIG:
		return provider.LaunchFailureClassConfig
	case providerv1.LaunchFailureClass_LAUNCH_FAILURE_CLASS_PROVIDER:
		return provider.LaunchFailureClassProvider
	default:
		return ""
	}
}

func toDomainLaunchFailureScope(value providerv1.LaunchFailureScope) provider.LaunchFailureScope {
	switch value {
	case providerv1.LaunchFailureScope_LAUNCH_FAILURE_SCOPE_PLACEMENT:
		return provider.LaunchFailureScopePlacement
	case providerv1.LaunchFailureScope_LAUNCH_FAILURE_SCOPE_ZONE:
		return provider.LaunchFailureScopeZone
	case providerv1.LaunchFailureScope_LAUNCH_FAILURE_SCOPE_REGION:
		return provider.LaunchFailureScopeRegion
	case providerv1.LaunchFailureScope_LAUNCH_FAILURE_SCOPE_ACCOUNT:
		return provider.LaunchFailureScopeAccount
	case providerv1.LaunchFailureScope_LAUNCH_FAILURE_SCOPE_PROVIDER:
		return provider.LaunchFailureScopeProvider
	case providerv1.LaunchFailureScope_LAUNCH_FAILURE_SCOPE_JOB:
		return provider.LaunchFailureScopeJob
	default:
		return ""
	}
}

func toDomainLaunchFailure(item *providerv1.LaunchFailure) *provider.LaunchFailure {
	if item == nil {
		return nil
	}
	return &provider.LaunchFailure{
		Code:       item.GetCode(),
		Class:      toDomainLaunchFailureClass(item.GetClass()),
		Scope:      toDomainLaunchFailureScope(item.GetScope()),
		Retryable:  item.GetRetryable(),
		Message:    item.GetMessage(),
		RawCode:    item.GetRawCode(),
		Attributes: item.GetAttributes(),
	}
}
