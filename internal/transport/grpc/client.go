package grpcserver

import (
	"context"
	"fmt"
	"time"

	providerv1 "github.com/arcoloom/arco-provider-aws/gen/proto/arco/provider/v1"
	"github.com/arcoloom/arco-provider-aws/internal/provider"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

type Client struct {
	conn   *grpc.ClientConn
	client providerv1.ProviderServiceClient
	token  string
}

func Dial(address string, token string) (*Client, error) {
	conn, err := grpc.NewClient(address, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("dial grpc provider: %w", err)
	}

	return &Client{
		conn:   conn,
		client: providerv1.NewProviderServiceClient(conn),
		token:  token,
	}, nil
}

func (c *Client) Close() error {
	return c.conn.Close()
}

func (c *Client) GetProviderInfo(ctx context.Context) (provider.Metadata, error) {
	resp, err := c.client.GetProviderInfo(c.withAuth(ctx), &providerv1.GetProviderInfoRequest{})
	if err != nil {
		return provider.Metadata{}, err
	}

	return toDomainMetadata(resp.GetMetadata()), nil
}

func (c *Client) ValidateConnection(ctx context.Context, req provider.ValidateConnectionRequest) (provider.ValidateConnectionResult, error) {
	resp, err := c.client.ValidateConnection(c.withAuth(ctx), &providerv1.ValidateConnectionRequest{
		Context:     toProtoContext(req.Context),
		Credentials: toProtoCredentials(req.Credentials),
		Scope:       toProtoScope(req.Scope),
		Options:     req.Options,
	})
	if err != nil {
		return provider.ValidateConnectionResult{}, err
	}

	return toDomainValidateConnectionResult(resp), nil
}

func (c *Client) Ping(ctx context.Context, req provider.RequestContext, payload string) (provider.PingResult, error) {
	resp, err := c.client.Ping(c.withAuth(ctx), &providerv1.PingRequest{
		Context: toProtoContext(req),
		Payload: payload,
	})
	if err != nil {
		return provider.PingResult{}, err
	}

	timestamp, err := time.Parse(time.RFC3339, resp.GetTimestamp())
	if err != nil {
		return provider.PingResult{}, fmt.Errorf("parse ping timestamp: %w", err)
	}

	return provider.PingResult{
		Payload:   resp.GetPayload(),
		Timestamp: timestamp,
	}, nil
}

func (c *Client) GetSpotData(ctx context.Context, req provider.GetSpotDataRequest) (provider.GetSpotDataResult, error) {
	resp, err := c.client.GetSpotData(c.withAuth(ctx), &providerv1.GetSpotDataRequest{
		Context:           toProtoContext(req.Context),
		Credentials:       toProtoCredentials(req.Credentials),
		Scope:             toProtoScope(req.Scope),
		InstanceTypes:     req.InstanceTypes,
		Region:            req.Region,
		AvailabilityZones: req.AvailabilityZones,
		Options:           req.Options,
	})
	if err != nil {
		return provider.GetSpotDataResult{}, err
	}

	return toDomainGetSpotDataResult(resp)
}

func (c *Client) StartInstance(ctx context.Context, req provider.StartInstanceRequest) (provider.StartInstanceResult, error) {
	resp, err := c.client.StartInstance(c.withAuth(ctx), &providerv1.StartInstanceRequest{
		Context:          toProtoContext(req.Context),
		Credentials:      toProtoCredentials(req.Credentials),
		Scope:            toProtoScope(req.Scope),
		StackName:        req.StackName,
		InstanceName:     req.InstanceName,
		Region:           req.Region,
		AvailabilityZone: req.AvailabilityZone,
		Ami:              req.AMI,
		InstanceType:     req.InstanceType,
		MarketType:       toProtoInstanceMarketType(req.MarketType),
		SubnetId:         req.SubnetID,
		SecurityGroupIds: req.SecurityGroupIDs,
		KeyName:          req.KeyName,
		UserData:         req.UserData,
		Options:          req.Options,
		Tags:             toProtoInstanceTags(req.Tags),
	})
	if err != nil {
		return provider.StartInstanceResult{}, err
	}

	return toDomainStartInstanceResult(resp), nil
}

func (c *Client) StopInstance(ctx context.Context, req provider.StopInstanceRequest) (provider.StopInstanceResult, error) {
	resp, err := c.client.StopInstance(c.withAuth(ctx), &providerv1.StopInstanceRequest{
		Context:     toProtoContext(req.Context),
		Credentials: toProtoCredentials(req.Credentials),
		Scope:       toProtoScope(req.Scope),
		StackName:   req.StackName,
		Options:     req.Options,
	})
	if err != nil {
		return provider.StopInstanceResult{}, err
	}

	return toDomainStopInstanceResult(resp), nil
}

func (c *Client) ListInstanceTypes(ctx context.Context, req provider.ListInstanceTypesRequest) (provider.ListInstanceTypesResult, error) {
	resp, err := c.client.ListInstanceTypes(c.withAuth(ctx), &providerv1.ListInstanceTypesRequest{
		Context:       toProtoContext(req.Context),
		Scope:         toProtoScope(req.Scope),
		Region:        req.Region,
		Series:        req.Series,
		InstanceTypes: req.InstanceTypes,
		Architectures: req.Architectures,
		Generation:    req.Generation,
		Options:       req.Options,
	})
	if err != nil {
		return provider.ListInstanceTypesResult{}, err
	}

	return toDomainListInstanceTypesResult(resp), nil
}

func (c *Client) GetInstanceTypeInfo(ctx context.Context, req provider.GetInstanceTypeInfoRequest) (provider.GetInstanceTypeInfoResult, error) {
	resp, err := c.client.GetInstanceTypeInfo(c.withAuth(ctx), &providerv1.GetInstanceTypeInfoRequest{
		Context:       toProtoContext(req.Context),
		Scope:         toProtoScope(req.Scope),
		Region:        req.Region,
		Series:        req.Series,
		InstanceTypes: req.InstanceTypes,
		Options:       req.Options,
	})
	if err != nil {
		return provider.GetInstanceTypeInfoResult{}, err
	}

	return toDomainGetInstanceTypeInfoResult(resp), nil
}

func (c *Client) GetInstancePrices(ctx context.Context, req provider.GetInstancePricesRequest) (provider.GetInstancePricesResult, error) {
	resp, err := c.client.GetInstancePrices(c.withAuth(ctx), &providerv1.GetInstancePricesRequest{
		Context:              toProtoContext(req.Context),
		Scope:                toProtoScope(req.Scope),
		Region:               req.Region,
		InstanceTypes:        req.InstanceTypes,
		PurchaseOption:       toProtoPurchaseOption(req.PurchaseOption),
		OperatingSystem:      req.OperatingSystem,
		Tenancy:              req.Tenancy,
		PreinstalledSoftware: req.PreinstalledSoftware,
		LicenseModel:         req.LicenseModel,
		Currency:             req.Currency,
		Options:              req.Options,
	})
	if err != nil {
		return provider.GetInstancePricesResult{}, err
	}

	return toDomainGetInstancePricesResult(resp)
}

func (c *Client) withAuth(ctx context.Context) context.Context {
	return metadata.AppendToOutgoingContext(ctx, authorizationMetadataKey, bearerPrefix+c.token)
}
