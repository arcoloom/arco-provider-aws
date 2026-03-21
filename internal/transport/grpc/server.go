package grpcserver

import (
	"context"
	"log/slog"
	"time"

	providerv1 "github.com/arcoloom/arco-provider-aws/gen/proto/arco/provider/v1"
	"github.com/arcoloom/arco-provider-aws/internal/provider"
)

type Server struct {
	providerv1.UnimplementedProviderServiceServer

	logger  *slog.Logger
	service provider.Service
}

func NewServer(logger *slog.Logger, service provider.Service) *Server {
	return &Server{
		logger:  logger,
		service: service,
	}
}

func (s *Server) GetProviderInfo(ctx context.Context, _ *providerv1.GetProviderInfoRequest) (*providerv1.GetProviderInfoResponse, error) {
	metadata, err := s.service.Metadata(ctx)
	if err != nil {
		return nil, err
	}

	s.logger.Info("provider info requested", "provider", metadata.Name, "cloud", metadata.Cloud)

	return &providerv1.GetProviderInfoResponse{
		Metadata: toProtoMetadata(metadata),
	}, nil
}

func (s *Server) GetProviderSchema(ctx context.Context, _ *providerv1.GetProviderSchemaRequest) (*providerv1.GetProviderSchemaResponse, error) {
	resources, err := s.service.Schema(ctx)
	if err != nil {
		return nil, err
	}

	s.logger.Info("provider schema requested", "resources", len(resources))

	return &providerv1.GetProviderSchemaResponse{
		Resources: toProtoResourceSchemas(resources),
	}, nil
}

func (s *Server) ValidateConnection(ctx context.Context, req *providerv1.ValidateConnectionRequest) (*providerv1.ValidateConnectionResponse, error) {
	domainReq := provider.ValidateConnectionRequest{
		Context:     toDomainContext(req.GetContext()),
		Credentials: toDomainCredentials(req.GetCredentials()),
		Scope:       toDomainScope(req.GetScope()),
		Options:     req.GetOptions(),
	}

	result, err := s.service.ValidateConnection(ctx, domainReq)
	if err != nil {
		return nil, err
	}

	s.logger.Info(
		"connection validation requested",
		"request_id", domainReq.Context.RequestID,
		"accepted", result.Accepted,
		"region", domainReq.Scope.Region,
	)

	return &providerv1.ValidateConnectionResponse{
		Accepted: result.Accepted,
		Message:  result.Message,
		Warnings: toProtoWarnings(result.Warnings),
	}, nil
}

func (s *Server) Ping(ctx context.Context, req *providerv1.PingRequest) (*providerv1.PingResponse, error) {
	result, err := s.service.Ping(ctx, req.GetPayload())
	if err != nil {
		return nil, err
	}

	metadata, err := s.service.Metadata(ctx)
	if err != nil {
		return nil, err
	}

	s.logger.Debug("ping handled", "payload", req.GetPayload())

	return &providerv1.PingResponse{
		Payload:   result.Payload,
		Timestamp: result.Timestamp.Format(time.RFC3339),
		Metadata:  toProtoMetadata(metadata),
	}, nil
}

func (s *Server) ListRegions(ctx context.Context, req *providerv1.ListRegionsRequest) (*providerv1.ListRegionsResponse, error) {
	domainReq := provider.ListRegionsRequest{
		Context:     toDomainContext(req.GetContext()),
		Credentials: toDomainCredentials(req.GetCredentials()),
		Scope:       toDomainScope(req.GetScope()),
		Options:     req.GetOptions(),
	}

	result, err := s.service.ListRegions(ctx, domainReq)
	if err != nil {
		return nil, err
	}

	s.logger.Info(
		"regions listed",
		"request_id", domainReq.Context.RequestID,
		"items", len(result.Items),
	)

	return &providerv1.ListRegionsResponse{
		Items:    toProtoRegions(result.Items),
		Warnings: toProtoWarnings(result.Warnings),
	}, nil
}

func (s *Server) ListAvailabilityZones(ctx context.Context, req *providerv1.ListAvailabilityZonesRequest) (*providerv1.ListAvailabilityZonesResponse, error) {
	domainReq := provider.ListAvailabilityZonesRequest{
		Context:           toDomainContext(req.GetContext()),
		Credentials:       toDomainCredentials(req.GetCredentials()),
		Scope:             toDomainScope(req.GetScope()),
		Region:            req.GetRegion(),
		AvailabilityZones: req.GetAvailabilityZones(),
		Options:           req.GetOptions(),
	}

	result, err := s.service.ListAvailabilityZones(ctx, domainReq)
	if err != nil {
		return nil, err
	}

	s.logger.Info(
		"availability zones listed",
		"request_id", domainReq.Context.RequestID,
		"region", domainReq.Region,
		"availability_zones", len(domainReq.AvailabilityZones),
		"items", len(result.Items),
	)

	return &providerv1.ListAvailabilityZonesResponse{
		Items:    toProtoAvailabilityZones(result.Items),
		Warnings: toProtoWarnings(result.Warnings),
	}, nil
}

func (s *Server) GetSpotData(ctx context.Context, req *providerv1.GetSpotDataRequest) (*providerv1.GetSpotDataResponse, error) {
	domainReq := provider.GetSpotDataRequest{
		Context:           toDomainContext(req.GetContext()),
		Credentials:       toDomainCredentials(req.GetCredentials()),
		Scope:             toDomainScope(req.GetScope()),
		InstanceTypes:     req.GetInstanceTypes(),
		Region:            req.GetRegion(),
		AvailabilityZones: req.GetAvailabilityZones(),
		Options:           req.GetOptions(),
	}

	result, err := s.service.GetSpotData(ctx, domainReq)
	if err != nil {
		return nil, err
	}

	s.logger.Info(
		"spot data requested",
		"request_id", domainReq.Context.RequestID,
		"instance_types", len(domainReq.InstanceTypes),
		"region", domainReq.Region,
		"availability_zones", len(domainReq.AvailabilityZones),
		"items", len(result.Items),
	)

	return &providerv1.GetSpotDataResponse{
		Items:    toProtoSpotData(result.Items),
		Warnings: toProtoWarnings(result.Warnings),
	}, nil
}

func (s *Server) StartInstance(ctx context.Context, req *providerv1.StartInstanceRequest) (*providerv1.StartInstanceResponse, error) {
	domainReq := provider.StartInstanceRequest{
		Context:          toDomainContext(req.GetContext()),
		Credentials:      toDomainCredentials(req.GetCredentials()),
		Scope:            toDomainScope(req.GetScope()),
		StackName:        req.GetStackName(),
		InstanceName:     req.GetInstanceName(),
		Region:           req.GetRegion(),
		AvailabilityZone: req.GetAvailabilityZone(),
		InstanceType:     req.GetInstanceType(),
		MarketType:       toDomainInstanceMarketType(req.GetMarketType()),
		UserData:         req.GetUserData(),
		Options:          req.GetOptions(),
		Tags:             toDomainInstanceTags(req.GetTags()),
		ProviderConfig:   toDomainProviderConfig(req.GetProviderConfig()),
	}

	result, err := s.service.StartInstance(ctx, domainReq)
	if err != nil {
		return nil, err
	}

	s.logger.Info(
		"instance start requested",
		"request_id", domainReq.Context.RequestID,
		"stack_name", domainReq.StackName,
		"instance_name", domainReq.InstanceName,
		"region", domainReq.Region,
		"market_type", domainReq.MarketType,
	)

	return &providerv1.StartInstanceResponse{
		StackName:  result.StackName,
		InstanceId: result.InstanceID,
		Urn:        result.URN,
		PublicIp:   result.PublicIP,
		PrivateIp:  result.PrivateIP,
		Warnings:   toProtoWarnings(result.Warnings),
	}, nil
}

func (s *Server) StopInstance(ctx context.Context, req *providerv1.StopInstanceRequest) (*providerv1.StopInstanceResponse, error) {
	domainReq := provider.StopInstanceRequest{
		Context:     toDomainContext(req.GetContext()),
		Credentials: toDomainCredentials(req.GetCredentials()),
		Scope:       toDomainScope(req.GetScope()),
		StackName:   req.GetStackName(),
		Options:     req.GetOptions(),
	}

	result, err := s.service.StopInstance(ctx, domainReq)
	if err != nil {
		return nil, err
	}

	s.logger.Info(
		"instance stop requested",
		"request_id", domainReq.Context.RequestID,
		"stack_name", domainReq.StackName,
		"destroyed", result.Destroyed,
	)

	return &providerv1.StopInstanceResponse{
		StackName: result.StackName,
		Destroyed: result.Destroyed,
		Warnings:  toProtoWarnings(result.Warnings),
	}, nil
}

func (s *Server) ListActiveInstances(ctx context.Context, req *providerv1.ListActiveInstancesRequest) (*providerv1.ListActiveInstancesResponse, error) {
	domainReq := provider.ListActiveInstancesRequest{
		Context:           toDomainContext(req.GetContext()),
		Credentials:       toDomainCredentials(req.GetCredentials()),
		Scope:             toDomainScope(req.GetScope()),
		Regions:           req.GetRegions(),
		AvailabilityZones: req.GetAvailabilityZones(),
		InstanceTypes:     req.GetInstanceTypes(),
		Tags:              toDomainInstanceTags(req.GetTags()),
		Options:           req.GetOptions(),
	}

	result, err := s.service.ListActiveInstances(ctx, domainReq)
	if err != nil {
		return nil, err
	}

	s.logger.Info(
		"active instances listed",
		"request_id", domainReq.Context.RequestID,
		"regions", len(domainReq.Regions),
		"availability_zones", len(domainReq.AvailabilityZones),
		"instance_types", len(domainReq.InstanceTypes),
		"tags", len(domainReq.Tags),
		"items", len(result.Items),
	)

	return &providerv1.ListActiveInstancesResponse{
		Items:    toProtoActiveInstances(result.Items),
		Warnings: toProtoWarnings(result.Warnings),
	}, nil
}

func (s *Server) ListInstanceTypes(ctx context.Context, req *providerv1.ListInstanceTypesRequest) (*providerv1.ListInstanceTypesResponse, error) {
	domainReq := provider.ListInstanceTypesRequest{
		Context:       toDomainContext(req.GetContext()),
		Scope:         toDomainScope(req.GetScope()),
		Region:        req.GetRegion(),
		Series:        req.GetSeries(),
		InstanceTypes: req.GetInstanceTypes(),
		Architectures: req.GetArchitectures(),
		Generation:    req.GetGeneration(),
		Options:       req.GetOptions(),
	}

	result, err := s.service.ListInstanceTypes(ctx, domainReq)
	if err != nil {
		return nil, err
	}

	s.logger.Info(
		"instance types listed",
		"request_id", domainReq.Context.RequestID,
		"region", domainReq.Region,
		"items", len(result.Items),
	)

	return &providerv1.ListInstanceTypesResponse{
		Items:    toProtoInstanceTypeSummaries(result.Items),
		Warnings: toProtoWarnings(result.Warnings),
	}, nil
}

func (s *Server) GetInstanceTypeInfo(ctx context.Context, req *providerv1.GetInstanceTypeInfoRequest) (*providerv1.GetInstanceTypeInfoResponse, error) {
	domainReq := provider.GetInstanceTypeInfoRequest{
		Context:       toDomainContext(req.GetContext()),
		Scope:         toDomainScope(req.GetScope()),
		Region:        req.GetRegion(),
		Series:        req.GetSeries(),
		InstanceTypes: req.GetInstanceTypes(),
		Options:       req.GetOptions(),
	}

	result, err := s.service.GetInstanceTypeInfo(ctx, domainReq)
	if err != nil {
		return nil, err
	}

	s.logger.Info(
		"instance type info requested",
		"request_id", domainReq.Context.RequestID,
		"region", domainReq.Region,
		"instance_types", len(domainReq.InstanceTypes),
		"items", len(result.Items),
	)

	return &providerv1.GetInstanceTypeInfoResponse{
		Items:    toProtoInstanceTypeInfos(result.Items),
		Warnings: toProtoWarnings(result.Warnings),
	}, nil
}

func (s *Server) GetInstancePrices(ctx context.Context, req *providerv1.GetInstancePricesRequest) (*providerv1.GetInstancePricesResponse, error) {
	domainReq := provider.GetInstancePricesRequest{
		Context:              toDomainContext(req.GetContext()),
		Scope:                toDomainScope(req.GetScope()),
		Region:               req.GetRegion(),
		InstanceTypes:        req.GetInstanceTypes(),
		PurchaseOption:       toDomainPurchaseOption(req.GetPurchaseOption()),
		OperatingSystem:      req.GetOperatingSystem(),
		Tenancy:              req.GetTenancy(),
		PreinstalledSoftware: req.GetPreinstalledSoftware(),
		LicenseModel:         req.GetLicenseModel(),
		Currency:             req.GetCurrency(),
		Options:              req.GetOptions(),
	}

	result, err := s.service.GetInstancePrices(ctx, domainReq)
	if err != nil {
		return nil, err
	}

	s.logger.Info(
		"instance prices requested",
		"request_id", domainReq.Context.RequestID,
		"region", domainReq.Region,
		"instance_types", len(domainReq.InstanceTypes),
		"items", len(result.Items),
	)

	return &providerv1.GetInstancePricesResponse{
		Items:    toProtoInstancePrices(result.Items),
		Warnings: toProtoWarnings(result.Warnings),
	}, nil
}
