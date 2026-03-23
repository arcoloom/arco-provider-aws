package aws

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/arcoloom/arco-provider-aws/internal/provider"
	awsv2 "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

var activeInstanceStates = []string{"pending", "running", "stopping", "stopped"}

const optionInventoryCursor = "inventory_cursor"
const optionIncrementalInventory = "incremental_inventory"

func (s *Service) ListActiveInstances(ctx context.Context, req provider.ListActiveInstancesRequest) (provider.ListActiveInstancesResult, error) {
	req = normalizeListActiveInstancesRequest(req)
	if err := validateListActiveInstancesRequest(req); err != nil {
		return provider.ListActiveInstancesResult{}, err
	}

	baseRegion := effectiveListActiveInstancesBaseRegion(req)
	cfg, err := s.clientFactory.NewConfig(ctx, *req.Credentials.AWS, effectiveEndpointRegion(req.Scope, baseRegion), req.Scope.Endpoint)
	if err != nil {
		return provider.ListActiveInstancesResult{}, err
	}

	baseClient := s.clientFactory.NewEC2(ec2ClientOptions{
		Config:   cfg,
		Endpoint: req.Scope.Endpoint,
	})
	scanPlan, err := resolveListActiveInstancesScan(ctx, baseClient, req)
	if err != nil {
		return provider.ListActiveInstancesResult{}, err
	}

	items := make([]provider.ActiveInstance, 0)
	for _, region := range scanPlan.Regions {
		regionCfg, err := s.clientFactory.NewConfig(ctx, *req.Credentials.AWS, effectiveEndpointRegion(req.Scope, region), req.Scope.Endpoint)
		if err != nil {
			return provider.ListActiveInstancesResult{}, fmt.Errorf("build ec2 config for region %s: %w", region, err)
		}

		regionItems, err := listActiveInstancesInRegion(ctx, s.clientFactory.NewEC2(ec2ClientOptions{
			Config:   regionCfg,
			Endpoint: req.Scope.Endpoint,
		}), region, req)
		if err != nil {
			return provider.ListActiveInstancesResult{}, err
		}
		items = append(items, regionItems...)
	}

	sort.Slice(items, func(i, j int) bool {
		left := items[i]
		right := items[j]
		switch {
		case left.Region != right.Region:
			return left.Region < right.Region
		case left.AvailabilityZone != right.AvailabilityZone:
			return left.AvailabilityZone < right.AvailabilityZone
		case left.Name != right.Name:
			return left.Name < right.Name
		default:
			return left.InstanceID < right.InstanceID
		}
	})

	return provider.ListActiveInstancesResult{
		Items:          items,
		NextCursor:     scanPlan.NextCursor,
		CoveredRegions: append([]string(nil), scanPlan.CoveredRegions...),
	}, nil
}

func validateListActiveInstancesRequest(req provider.ListActiveInstancesRequest) error {
	if req.Credentials.AWS == nil {
		return errors.New("aws iam credentials are required")
	}

	return nil
}

func normalizeListActiveInstancesRequest(req provider.ListActiveInstancesRequest) provider.ListActiveInstancesRequest {
	req.Regions = normalizeListFilterValues(req.Regions)
	req.AvailabilityZones = normalizeListFilterValues(req.AvailabilityZones)
	req.InstanceTypes = normalizeListFilterValues(req.InstanceTypes)
	req.Tags = normalizeTagFilters(req.Tags)

	return req
}

func normalizeListFilterValues(values []string) []string {
	if len(values) == 0 {
		return nil
	}

	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		key := normalizeToken(trimmed)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, trimmed)
	}

	if len(result) == 0 {
		return nil
	}

	return result
}

func normalizeTagFilters(tags []provider.InstanceTag) []provider.InstanceTag {
	if len(tags) == 0 {
		return nil
	}

	result := make([]provider.InstanceTag, 0, len(tags))
	seen := make(map[string]struct{}, len(tags))
	for _, tag := range tags {
		key := strings.TrimSpace(tag.Key)
		if key == "" {
			continue
		}
		value := strings.TrimSpace(tag.Value)
		seenKey := key + "\x00" + value
		if _, ok := seen[seenKey]; ok {
			continue
		}
		seen[seenKey] = struct{}{}
		result = append(result, provider.InstanceTag{Key: key, Value: value})
	}

	sort.Slice(result, func(i, j int) bool {
		switch {
		case result[i].Key != result[j].Key:
			return result[i].Key < result[j].Key
		default:
			return result[i].Value < result[j].Value
		}
	})

	if len(result) == 0 {
		return nil
	}

	return result
}

func effectiveListActiveInstancesBaseRegion(req provider.ListActiveInstancesRequest) string {
	if len(req.Regions) > 0 && !containsAllSelector(req.Regions) {
		return req.Regions[0]
	}

	return effectiveDiscoveryBaseRegion("")
}

type activeInstanceScanPlan struct {
	Regions        []string
	CoveredRegions []string
	NextCursor     string
}

func resolveListActiveInstancesScan(ctx context.Context, client ec2API, req provider.ListActiveInstancesRequest) (activeInstanceScanPlan, error) {
	if len(req.Regions) > 0 && !containsAllSelector(req.Regions) {
		regions := append([]string(nil), req.Regions...)
		return activeInstanceScanPlan{
			Regions:        regions,
			CoveredRegions: regions,
		}, nil
	}

	regions, err := listAccountRegions(ctx, client)
	if err != nil {
		return activeInstanceScanPlan{}, err
	}
	if len(regions) == 0 {
		return activeInstanceScanPlan{}, nil
	}
	if !listActiveInstancesUsesIncrementalInventory(req.Options) {
		return activeInstanceScanPlan{
			Regions:        append([]string(nil), regions...),
			CoveredRegions: append([]string(nil), regions...),
		}, nil
	}

	cursor, _ := lookupOptionValue(req.Options, optionInventoryCursor)
	index := nextActiveInstanceScanIndex(regions, cursor)
	nextCursor := ""
	if index+1 < len(regions) {
		nextCursor = regions[index+1]
	}

	region := regions[index]
	return activeInstanceScanPlan{
		Regions:        []string{region},
		CoveredRegions: []string{region},
		NextCursor:     nextCursor,
	}, nil
}

func listActiveInstancesUsesIncrementalInventory(options map[string]string) bool {
	value, ok := lookupOptionValue(options, optionIncrementalInventory)
	if !ok {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(value), "true")
}

func nextActiveInstanceScanIndex(regions []string, cursor string) int {
	if len(regions) == 0 {
		return 0
	}

	cursor = strings.TrimSpace(cursor)
	if cursor == "" {
		return 0
	}

	for index, region := range regions {
		if strings.EqualFold(region, cursor) {
			return index
		}
	}

	return 0
}

func listActiveInstancesInRegion(
	ctx context.Context,
	client ec2API,
	region string,
	req provider.ListActiveInstancesRequest,
) ([]provider.ActiveInstance, error) {
	filters := []ec2types.Filter{
		{
			Name:   awsv2.String("instance-state-name"),
			Values: activeInstanceStates,
		},
	}
	if len(req.InstanceTypes) > 0 {
		filters = append(filters, ec2types.Filter{
			Name:   awsv2.String("instance-type"),
			Values: append([]string(nil), req.InstanceTypes...),
		})
	}
	for _, tag := range req.Tags {
		if tag.Value == "" {
			continue
		}
		filters = append(filters, ec2types.Filter{
			Name:   awsv2.String("tag:" + tag.Key),
			Values: []string{tag.Value},
		})
	}

	items := make([]provider.ActiveInstance, 0)
	input := &ec2.DescribeInstancesInput{Filters: filters}
	for {
		output, err := client.DescribeInstances(ctx, input)
		if err != nil {
			return nil, fmt.Errorf("describe active instances for region %s: %w", region, err)
		}

		for _, reservation := range output.Reservations {
			for _, instance := range reservation.Instances {
				if !matchesActiveInstanceFilters(instance, region, req) {
					continue
				}
				items = append(items, buildActiveInstance(region, instance))
			}
		}

		nextToken := strings.TrimSpace(awsv2.ToString(output.NextToken))
		if nextToken == "" {
			break
		}
		input.NextToken = awsv2.String(nextToken)
	}

	return items, nil
}

func matchesActiveInstanceFilters(instance ec2types.Instance, region string, req provider.ListActiveInstancesRequest) bool {
	state := normalizeToken(string(instanceStateName(instance)))
	if !slices.Contains(activeInstanceStates, state) {
		return false
	}

	if len(req.InstanceTypes) > 0 {
		instanceType := strings.TrimSpace(string(instance.InstanceType))
		if instanceType == "" {
			return false
		}

		matched := false
		for _, candidate := range req.InstanceTypes {
			if strings.EqualFold(candidate, instanceType) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}

	if len(req.AvailabilityZones) > 0 && !matchesAvailabilityZoneFilter(region, instance, req.AvailabilityZones) {
		return false
	}

	if len(req.Tags) > 0 && !matchesTagFilters(instance.Tags, req.Tags) {
		return false
	}

	return true
}

func instanceStateName(instance ec2types.Instance) ec2types.InstanceStateName {
	if instance.State == nil {
		return ""
	}

	return instance.State.Name
}

func matchesAvailabilityZoneFilter(region string, instance ec2types.Instance, requested []string) bool {
	zoneName := strings.TrimSpace(awsv2.ToString(instance.Placement.AvailabilityZone))
	zoneID := strings.TrimSpace(awsv2.ToString(instance.Placement.AvailabilityZoneId))
	for _, zone := range requested {
		if isAllSelector(zone) {
			return true
		}
		trimmed := strings.TrimSpace(zone)
		if trimmed == "" {
			continue
		}
		if looksLikeAvailabilityZone(trimmed) && !strings.HasPrefix(trimmed, region) {
			continue
		}
		if strings.EqualFold(trimmed, zoneName) || strings.EqualFold(trimmed, zoneID) {
			return true
		}
	}

	return false
}

func matchesTagFilters(tags []ec2types.Tag, requested []provider.InstanceTag) bool {
	if len(requested) == 0 {
		return true
	}

	valuesByKey := make(map[string][]string, len(tags))
	for _, tag := range tags {
		key := strings.TrimSpace(awsv2.ToString(tag.Key))
		if key == "" {
			continue
		}
		valuesByKey[key] = append(valuesByKey[key], awsv2.ToString(tag.Value))
	}

	for _, filter := range requested {
		values, ok := valuesByKey[filter.Key]
		if !ok {
			return false
		}
		if filter.Value == "" {
			continue
		}
		matched := false
		for _, value := range values {
			if value == filter.Value {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}

	return true
}

func buildActiveInstance(region string, instance ec2types.Instance) provider.ActiveInstance {
	tags := toProviderInstanceTags(instance.Tags)
	providerAttributes := map[string]string{}
	if subnetID := strings.TrimSpace(awsv2.ToString(instance.SubnetId)); subnetID != "" {
		providerAttributes[providerAttributeSubnetID] = subnetID
	}
	if vpcID := strings.TrimSpace(awsv2.ToString(instance.VpcId)); vpcID != "" {
		providerAttributes[providerAttributeVPCID] = vpcID
	}
	if len(providerAttributes) == 0 {
		providerAttributes = nil
	}
	return provider.ActiveInstance{
		InstanceID:         strings.TrimSpace(awsv2.ToString(instance.InstanceId)),
		Name:               activeInstanceName(tags),
		Region:             region,
		AvailabilityZone:   strings.TrimSpace(awsv2.ToString(instance.Placement.AvailabilityZone)),
		InstanceType:       strings.TrimSpace(string(instance.InstanceType)),
		State:              string(instanceStateName(instance)),
		MarketType:         marketTypeFromInstance(instance),
		PublicIP:           strings.TrimSpace(awsv2.ToString(instance.PublicIpAddress)),
		PrivateIP:          strings.TrimSpace(awsv2.ToString(instance.PrivateIpAddress)),
		IPv6Addresses:      instanceIPv6Addresses(instance),
		LaunchTime:         awsv2.ToTime(instance.LaunchTime),
		Tags:               tags,
		ProviderAttributes: providerAttributes,
	}
}

func marketTypeFromInstance(instance ec2types.Instance) provider.InstanceMarketType {
	switch instance.InstanceLifecycle {
	case ec2types.InstanceLifecycleTypeSpot:
		return provider.InstanceMarketTypeSpot
	case "":
		return provider.InstanceMarketTypeOnDemand
	default:
		return ""
	}
}

func activeInstanceName(tags []provider.InstanceTag) string {
	for _, tag := range tags {
		if tag.Key == "Name" {
			return tag.Value
		}
	}

	return ""
}

func instanceIPv6Addresses(instance ec2types.Instance) []string {
	addresses := make([]string, 0)
	seen := make(map[string]struct{})
	for _, networkInterface := range instance.NetworkInterfaces {
		for _, address := range networkInterface.Ipv6Addresses {
			value := strings.TrimSpace(awsv2.ToString(address.Ipv6Address))
			if value == "" {
				continue
			}
			if _, ok := seen[value]; ok {
				continue
			}
			seen[value] = struct{}{}
			addresses = append(addresses, value)
		}
	}

	sort.Strings(addresses)
	return addresses
}

func toProviderInstanceTags(tags []ec2types.Tag) []provider.InstanceTag {
	if len(tags) == 0 {
		return nil
	}

	result := make([]provider.InstanceTag, 0, len(tags))
	for _, tag := range tags {
		key := strings.TrimSpace(awsv2.ToString(tag.Key))
		if key == "" {
			continue
		}
		result = append(result, provider.InstanceTag{
			Key:   key,
			Value: awsv2.ToString(tag.Value),
		})
	}

	sort.Slice(result, func(i, j int) bool {
		switch {
		case result[i].Key != result[j].Key:
			return result[i].Key < result[j].Key
		default:
			return result[i].Value < result[j].Value
		}
	})

	if len(result) == 0 {
		return nil
	}

	return result
}
