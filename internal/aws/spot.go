package aws

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/arcoloom/arco-provider-aws/internal/provider"
	awsv2 "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/sts"
)

const (
	defaultAWSRegion           = "us-east-1"
	defaultSpotProduct         = "Linux/UNIX"
	defaultTargetCapacity      = int32(1)
	regionSelectorAll          = "all"
	inventoryStatusUnavailable = "unavailable"
	inventoryStatusHigh        = "high"
	inventoryStatusMedium      = "medium"
	inventoryStatusLow         = "low"
	inventoryStatusAvailable   = "available"
	inventoryStatusUnknown     = "unknown"
	defaultAssumeRoleSession   = "arco-provider-aws"
)

type clientFactory interface {
	NewConfig(context.Context, provider.AWSCredentials, string, string) (awsv2.Config, error)
	NewEC2(ec2ClientOptions) ec2API
	NewSSM(awsv2.Config) ssmAPI
	NewSTS(awsv2.Config) stsAPI
}

type ec2ClientOptions struct {
	Config   awsv2.Config
	Endpoint string
}

type ec2API interface {
	DescribeRegions(context.Context, *ec2.DescribeRegionsInput, ...func(*ec2.Options)) (*ec2.DescribeRegionsOutput, error)
	DescribeAvailabilityZones(context.Context, *ec2.DescribeAvailabilityZonesInput, ...func(*ec2.Options)) (*ec2.DescribeAvailabilityZonesOutput, error)
	DescribeInstanceTypeOfferings(context.Context, *ec2.DescribeInstanceTypeOfferingsInput, ...func(*ec2.Options)) (*ec2.DescribeInstanceTypeOfferingsOutput, error)
	DescribeInstanceTypes(context.Context, *ec2.DescribeInstanceTypesInput, ...func(*ec2.Options)) (*ec2.DescribeInstanceTypesOutput, error)
	DescribeInstances(context.Context, *ec2.DescribeInstancesInput, ...func(*ec2.Options)) (*ec2.DescribeInstancesOutput, error)
	DescribeSpotPriceHistory(context.Context, *ec2.DescribeSpotPriceHistoryInput, ...func(*ec2.Options)) (*ec2.DescribeSpotPriceHistoryOutput, error)
	GetSpotPlacementScores(context.Context, *ec2.GetSpotPlacementScoresInput, ...func(*ec2.Options)) (*ec2.GetSpotPlacementScoresOutput, error)
	RunInstances(context.Context, *ec2.RunInstancesInput, ...func(*ec2.Options)) (*ec2.RunInstancesOutput, error)
	TerminateInstances(context.Context, *ec2.TerminateInstancesInput, ...func(*ec2.Options)) (*ec2.TerminateInstancesOutput, error)
}

type ssmAPI interface {
	GetParameter(context.Context, *ssm.GetParameterInput, ...func(*ssm.Options)) (*ssm.GetParameterOutput, error)
}

type stsAPI interface {
	GetCallerIdentity(context.Context, *sts.GetCallerIdentityInput, ...func(*sts.Options)) (*sts.GetCallerIdentityOutput, error)
}

type awsClientFactory struct{}

func newAWSClientFactory() clientFactory {
	return awsClientFactory{}
}

func (awsClientFactory) NewConfig(ctx context.Context, creds provider.AWSCredentials, region string, endpoint string) (awsv2.Config, error) {
	if region == "" {
		region = defaultAWSRegion
	}

	loadOptions := []func(*config.LoadOptions) error{
		config.WithRegion(region),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			creds.AccessKeyID,
			creds.SecretAccessKey,
			creds.SessionToken,
		)),
	}

	if endpoint != "" {
		targetService := endpointServiceID(endpoint)
		resolver := awsv2.EndpointResolverWithOptionsFunc(func(service, resolvedRegion string, _ ...interface{}) (awsv2.Endpoint, error) {
			if resolvedRegion == region && shouldUseCustomEndpoint(service, targetService) {
				return awsv2.Endpoint{
					URL:           endpoint,
					SigningRegion: region,
				}, nil
			}

			return awsv2.Endpoint{}, &awsv2.EndpointNotFoundError{}
		})
		loadOptions = append(loadOptions, config.WithEndpointResolverWithOptions(resolver))
	}

	cfg, err := config.LoadDefaultConfig(ctx, loadOptions...)
	if err != nil {
		return awsv2.Config{}, fmt.Errorf("load aws config for region %s: %w", region, err)
	}

	if roleARN := strings.TrimSpace(creds.RoleARN); roleARN != "" {
		externalID := strings.TrimSpace(creds.ExternalID)
		assumeRoleProvider := stscreds.NewAssumeRoleProvider(sts.NewFromConfig(cfg), roleARN, func(options *stscreds.AssumeRoleOptions) {
			options.RoleSessionName = defaultAssumeRoleSession
			if externalID != "" {
				options.ExternalID = awsv2.String(externalID)
			}
		})
		cfg.Credentials = awsv2.NewCredentialsCache(assumeRoleProvider)
	}

	return cfg, nil
}

func (awsClientFactory) NewEC2(options ec2ClientOptions) ec2API {
	return ec2.NewFromConfig(options.Config)
}

func (awsClientFactory) NewSSM(cfg awsv2.Config) ssmAPI {
	return ssm.NewFromConfig(cfg)
}

func (awsClientFactory) NewSTS(cfg awsv2.Config) stsAPI {
	return sts.NewFromConfig(cfg)
}

func endpointServiceID(rawEndpoint string) string {
	parsed, err := url.Parse(rawEndpoint)
	if err != nil {
		return ""
	}

	host := strings.ToLower(parsed.Hostname())
	switch {
	case host == "" || strings.Contains(host, "localhost") || strings.HasPrefix(host, "127.") || host == "::1":
		return ""
	case host == ec2.ServiceID || strings.HasPrefix(host, ec2.ServiceID+".") || strings.Contains(host, "."+ec2.ServiceID+"."):
		return ec2.ServiceID
	case host == ssm.ServiceID || strings.HasPrefix(host, ssm.ServiceID+".") || strings.Contains(host, "."+ssm.ServiceID+"."):
		return ssm.ServiceID
	case host == sts.ServiceID || strings.HasPrefix(host, sts.ServiceID+".") || strings.Contains(host, "."+sts.ServiceID+"."):
		return sts.ServiceID
	default:
		return ""
	}
}

func shouldUseCustomEndpoint(service string, targetService string) bool {
	switch targetService {
	case "":
		return service == ec2.ServiceID || service == ssm.ServiceID || service == sts.ServiceID
	default:
		return service == targetService
	}
}

type spotPriceSnapshot struct {
	Price     string
	Currency  string
	Timestamp time.Time
}

type spotPlacementSnapshot struct {
	HasCapacityScore bool
	CapacityScore    int32
}

func (s *Service) GetSpotData(ctx context.Context, req provider.GetSpotDataRequest) (provider.GetSpotDataResult, error) {
	if req.Credentials.AWS == nil {
		return provider.GetSpotDataResult{}, errors.New("aws iam credentials are required")
	}
	if len(req.InstanceTypes) == 0 {
		return provider.GetSpotDataResult{}, errors.New("at least one instance type is required")
	}

	baseRegion := effectiveBaseRegion(req)
	cfg, err := s.clientFactory.NewConfig(ctx, *req.Credentials.AWS, effectiveEndpointRegion(req.Scope, baseRegion), req.Scope.Endpoint)
	if err != nil {
		return provider.GetSpotDataResult{}, err
	}

	baseClient := s.clientFactory.NewEC2(ec2ClientOptions{Config: cfg, Endpoint: req.Scope.Endpoint})
	regions, warnings, err := resolveRegions(ctx, baseClient, req)
	if err != nil {
		return provider.GetSpotDataResult{}, err
	}

	items := make([]provider.SpotData, 0)
	for _, region := range regions {
		regionCfg, err := s.clientFactory.NewConfig(ctx, *req.Credentials.AWS, effectiveEndpointRegion(req.Scope, region), req.Scope.Endpoint)
		if err != nil {
			return provider.GetSpotDataResult{}, fmt.Errorf("build ec2 config for region %s: %w", region, err)
		}
		regionClient := s.clientFactory.NewEC2(ec2ClientOptions{Config: regionCfg, Endpoint: req.Scope.Endpoint})

		availabilityZones, zoneIDToName, zoneWarnings, err := resolveAvailabilityZones(ctx, regionClient, region, req.AvailabilityZones)
		warnings = append(warnings, zoneWarnings...)
		if err != nil {
			return provider.GetSpotDataResult{}, err
		}

		offerings, err := fetchOfferings(ctx, regionClient, req.InstanceTypes)
		if err != nil {
			return provider.GetSpotDataResult{}, err
		}

		prices, err := fetchLatestSpotPrices(ctx, regionClient, req.InstanceTypes)
		if err != nil {
			return provider.GetSpotDataResult{}, err
		}

		placementScores, scoreWarnings, err := fetchPlacementScores(ctx, regionClient, region, req.InstanceTypes, zoneIDToName)
		warnings = append(warnings, scoreWarnings...)
		if err != nil {
			return provider.GetSpotDataResult{}, err
		}

		items = append(items, buildSpotItems(region, req.InstanceTypes, availabilityZones, offerings, prices, placementScores)...)
	}

	return provider.GetSpotDataResult{
		Items:    items,
		Warnings: warnings,
	}, nil
}

func effectiveEndpointRegion(scope provider.ConnectionScope, fallbackRegion string) string {
	if strings.TrimSpace(scope.Endpoint) == "" {
		return fallbackRegion
	}

	if endpointRegion := strings.TrimSpace(scope.EndpointRegion); endpointRegion != "" {
		return endpointRegion
	}

	return fallbackRegion
}

func effectiveBaseRegion(req provider.GetSpotDataRequest) string {
	switch {
	case req.Region != "" && !isAllSelector(req.Region):
		return req.Region
	case req.Scope.Region != "" && !isAllSelector(req.Scope.Region):
		return req.Scope.Region
	default:
		return defaultAWSRegion
	}
}

func resolveRegions(ctx context.Context, client ec2API, req provider.GetSpotDataRequest) ([]string, []provider.Warning, error) {
	targetRegion := req.Region
	if targetRegion == "" {
		targetRegion = req.Scope.Region
	}

	if targetRegion != "" && !isAllSelector(targetRegion) {
		return []string{targetRegion}, nil, nil
	}

	output, err := client.DescribeRegions(ctx, &ec2.DescribeRegionsInput{
		AllRegions: awsv2.Bool(false),
	})
	if err != nil {
		return nil, nil, fmt.Errorf("describe regions: %w", err)
	}

	regions := make([]string, 0, len(output.Regions))
	for _, region := range output.Regions {
		if name := awsv2.ToString(region.RegionName); name != "" {
			regions = append(regions, name)
		}
	}
	slices.Sort(regions)

	return regions, nil, nil
}

func resolveAvailabilityZones(ctx context.Context, client ec2API, region string, requested []string) ([]string, map[string]string, []provider.Warning, error) {
	output, err := client.DescribeAvailabilityZones(ctx, &ec2.DescribeAvailabilityZonesInput{
		AllAvailabilityZones: awsv2.Bool(false),
	})
	if err != nil {
		return nil, nil, nil, fmt.Errorf("describe availability zones for region %s: %w", region, err)
	}

	requestedSet := make(map[string]struct{}, len(requested))
	allZones := len(requested) == 0
	for _, zone := range requested {
		if isAllSelector(zone) {
			allZones = true
			break
		}
		if looksLikeAvailabilityZone(zone) && !strings.HasPrefix(zone, region) {
			continue
		}
		requestedSet[zone] = struct{}{}
	}

	zones := make([]string, 0, len(output.AvailabilityZones))
	zoneIDToName := make(map[string]string, len(output.AvailabilityZones))
	seenRequested := make(map[string]struct{}, len(requestedSet))
	for _, zone := range output.AvailabilityZones {
		name := awsv2.ToString(zone.ZoneName)
		if name == "" {
			continue
		}
		zoneID := awsv2.ToString(zone.ZoneId)
		if zoneID != "" {
			zoneIDToName[zoneID] = name
		}
		if !allZones {
			if _, ok := requestedSet[name]; !ok {
				continue
			}
			seenRequested[name] = struct{}{}
		}
		zones = append(zones, name)
	}
	slices.Sort(zones)

	warnings := make([]provider.Warning, 0)
	if !allZones {
		for zone := range requestedSet {
			if _, ok := seenRequested[zone]; ok {
				continue
			}
			warnings = append(warnings, provider.Warning{
				Code:    "AZ_NOT_FOUND",
				Message: fmt.Sprintf("availability zone %s was not found in region %s", zone, region),
			})
		}
	}

	return zones, zoneIDToName, warnings, nil
}

func fetchOfferings(ctx context.Context, client ec2API, instanceTypes []string) (map[string]map[string]bool, error) {
	offerings := make(map[string]map[string]bool)
	input := &ec2.DescribeInstanceTypeOfferingsInput{
		LocationType: ec2types.LocationTypeAvailabilityZone,
		MaxResults:   awsv2.Int32(1000),
		Filters: []ec2types.Filter{
			{
				Name:   awsv2.String("instance-type"),
				Values: instanceTypes,
			},
		},
	}

	for {
		output, err := client.DescribeInstanceTypeOfferings(ctx, input)
		if err != nil {
			return nil, fmt.Errorf("describe instance type offerings: %w", err)
		}

		for _, offering := range output.InstanceTypeOfferings {
			instanceType := string(offering.InstanceType)
			location := awsv2.ToString(offering.Location)
			if instanceType == "" || location == "" {
				continue
			}
			if _, ok := offerings[instanceType]; !ok {
				offerings[instanceType] = make(map[string]bool)
			}
			offerings[instanceType][location] = true
		}

		if output.NextToken == nil || awsv2.ToString(output.NextToken) == "" {
			break
		}
		input.NextToken = output.NextToken
	}

	return offerings, nil
}

func fetchLatestSpotPrices(ctx context.Context, client ec2API, instanceTypes []string) (map[string]map[string]spotPriceSnapshot, error) {
	prices := make(map[string]map[string]spotPriceSnapshot)
	input := &ec2.DescribeSpotPriceHistoryInput{
		InstanceTypes:       toEC2InstanceTypes(instanceTypes),
		ProductDescriptions: []string{defaultSpotProduct},
		StartTime:           awsv2.Time(time.Now().UTC()),
		MaxResults:          awsv2.Int32(1000),
	}

	for {
		output, err := client.DescribeSpotPriceHistory(ctx, input)
		if err != nil {
			return nil, fmt.Errorf("describe spot price history: %w", err)
		}

		for _, entry := range output.SpotPriceHistory {
			instanceType := string(entry.InstanceType)
			zone := awsv2.ToString(entry.AvailabilityZone)
			price := awsv2.ToString(entry.SpotPrice)
			if instanceType == "" || zone == "" || price == "" {
				continue
			}
			if _, ok := prices[instanceType]; !ok {
				prices[instanceType] = make(map[string]spotPriceSnapshot)
			}
			if _, exists := prices[instanceType][zone]; exists {
				continue
			}
			prices[instanceType][zone] = spotPriceSnapshot{
				Price:     price,
				Currency:  "USD",
				Timestamp: awsv2.ToTime(entry.Timestamp),
			}
		}

		if output.NextToken == nil || awsv2.ToString(output.NextToken) == "" {
			break
		}
		input.NextToken = output.NextToken
	}

	return prices, nil
}

func fetchPlacementScores(ctx context.Context, client ec2API, region string, instanceTypes []string, zoneIDToName map[string]string) (map[string]map[string]spotPlacementSnapshot, []provider.Warning, error) {
	results := make(map[string]map[string]spotPlacementSnapshot)
	warnings := make([]provider.Warning, 0)

	for _, instanceType := range instanceTypes {
		output, err := client.GetSpotPlacementScores(ctx, &ec2.GetSpotPlacementScoresInput{
			InstanceTypes:          []string{instanceType},
			RegionNames:            []string{region},
			SingleAvailabilityZone: awsv2.Bool(true),
			TargetCapacity:         awsv2.Int32(defaultTargetCapacity),
			TargetCapacityUnitType: ec2types.TargetCapacityUnitTypeUnits,
		})
		if err != nil {
			warnings = append(warnings, provider.Warning{
				Code:    "SPOT_CAPACITY_UNAVAILABLE",
				Message: fmt.Sprintf("spot placement score unavailable for %s in %s: %v", instanceType, region, err),
			})
			continue
		}

		if _, ok := results[instanceType]; !ok {
			results[instanceType] = make(map[string]spotPlacementSnapshot)
		}
		for _, score := range output.SpotPlacementScores {
			zoneName := zoneIDToName[awsv2.ToString(score.AvailabilityZoneId)]
			if zoneName == "" {
				continue
			}
			results[instanceType][zoneName] = spotPlacementSnapshot{
				HasCapacityScore: true,
				CapacityScore:    awsv2.ToInt32(score.Score),
			}
		}
	}

	return results, warnings, nil
}

func buildSpotItems(
	region string,
	instanceTypes []string,
	availabilityZones []string,
	offerings map[string]map[string]bool,
	prices map[string]map[string]spotPriceSnapshot,
	placementScores map[string]map[string]spotPlacementSnapshot,
) []provider.SpotData {
	items := make([]provider.SpotData, 0, len(instanceTypes)*max(1, len(availabilityZones)))
	for _, instanceType := range instanceTypes {
		for _, availabilityZone := range availabilityZones {
			offered := offerings[instanceType][availabilityZone]
			item := provider.SpotData{
				InstanceType:     instanceType,
				Region:           region,
				AvailabilityZone: availabilityZone,
				Inventory: provider.SpotInventory{
					Offered: offered,
					Status:  inventoryStatus(offered, spotPlacementSnapshot{}, false),
				},
			}

			if price, ok := prices[instanceType][availabilityZone]; ok {
				item.HasPrice = true
				item.Price = price.Price
				item.Currency = price.Currency
				item.Timestamp = price.Timestamp
			}

			if placement, ok := placementScores[instanceType][availabilityZone]; ok {
				item.Inventory.HasCapacityScore = placement.HasCapacityScore
				item.Inventory.CapacityScore = placement.CapacityScore
				item.Inventory.Status = inventoryStatus(offered, placement, true)
			} else if item.HasPrice {
				item.Inventory.Status = inventoryStatus(offered, spotPlacementSnapshot{}, false)
			}

			items = append(items, item)
		}
	}

	return items
}

func inventoryStatus(offered bool, placement spotPlacementSnapshot, hasPlacement bool) string {
	if !offered {
		return inventoryStatusUnavailable
	}
	if hasPlacement && placement.HasCapacityScore {
		switch {
		case placement.CapacityScore >= 8:
			return inventoryStatusHigh
		case placement.CapacityScore >= 5:
			return inventoryStatusMedium
		case placement.CapacityScore > 0:
			return inventoryStatusLow
		default:
			return inventoryStatusUnknown
		}
	}
	return inventoryStatusAvailable
}

func toEC2InstanceTypes(instanceTypes []string) []ec2types.InstanceType {
	result := make([]ec2types.InstanceType, 0, len(instanceTypes))
	for _, instanceType := range instanceTypes {
		result = append(result, ec2types.InstanceType(instanceType))
	}
	return result
}

func isAllSelector(value string) bool {
	return strings.EqualFold(strings.TrimSpace(value), regionSelectorAll)
}

func looksLikeAvailabilityZone(value string) bool {
	return strings.Count(value, "-") >= 2
}

func max(a int, b int) int {
	if a > b {
		return a
	}
	return b
}
