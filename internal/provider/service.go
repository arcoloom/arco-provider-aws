package provider

import "context"

type Service interface {
	Metadata(context.Context) (Metadata, error)
	Schema(context.Context) ([]ResourceSchema, error)
	ValidateConnection(context.Context, ValidateConnectionRequest) (ValidateConnectionResult, error)
	Ping(context.Context, string) (PingResult, error)
	ListRegions(context.Context, ListRegionsRequest) (ListRegionsResult, error)
	ListAvailabilityZones(context.Context, ListAvailabilityZonesRequest) (ListAvailabilityZonesResult, error)
	WatchMarketFeed(context.Context, WatchMarketFeedRequest, func(WatchMarketFeedEvent) error) error
	GetSpotData(context.Context, GetSpotDataRequest) (GetSpotDataResult, error)
	StartInstance(context.Context, StartInstanceRequest) (StartInstanceResult, error)
	StopInstance(context.Context, StopInstanceRequest) (StopInstanceResult, error)
	ListActiveInstances(context.Context, ListActiveInstancesRequest) (ListActiveInstancesResult, error)
	ListInstanceTypes(context.Context, ListInstanceTypesRequest) (ListInstanceTypesResult, error)
	GetInstanceTypeInfo(context.Context, GetInstanceTypeInfoRequest) (GetInstanceTypeInfoResult, error)
	GetInstancePrices(context.Context, GetInstancePricesRequest) (GetInstancePricesResult, error)
}
