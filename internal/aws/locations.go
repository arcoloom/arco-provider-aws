package aws

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/arcoloom/arco-provider-aws/internal/provider"
	awsv2 "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

func (s *Service) ListRegions(ctx context.Context, req provider.ListRegionsRequest) (provider.ListRegionsResult, error) {
	if req.Credentials.AWS == nil {
		return provider.ListRegionsResult{}, errors.New("aws iam credentials are required")
	}

	baseRegion := effectiveDiscoveryBaseRegion("", req.Scope.Region)
	cfg, err := s.clientFactory.NewConfig(ctx, *req.Credentials.AWS, effectiveEndpointRegion(req.Scope, baseRegion), req.Scope.Endpoint)
	if err != nil {
		return provider.ListRegionsResult{}, err
	}

	regions, err := listAccountRegions(ctx, s.clientFactory.NewEC2(ec2ClientOptions{
		Config:   cfg,
		Endpoint: req.Scope.Endpoint,
	}))
	if err != nil {
		return provider.ListRegionsResult{}, err
	}

	nameByCode := s.regionNameLookup(ctx)
	items := make([]provider.Region, 0, len(regions))
	for _, code := range regions {
		name := strings.TrimSpace(nameByCode[code])
		if name == "" {
			name = code
		}
		items = append(items, provider.Region{
			Code: code,
			Name: name,
		})
	}

	return provider.ListRegionsResult{Items: items}, nil
}

func (s *Service) ListAvailabilityZones(ctx context.Context, req provider.ListAvailabilityZonesRequest) (provider.ListAvailabilityZonesResult, error) {
	if req.Credentials.AWS == nil {
		return provider.ListAvailabilityZonesResult{}, errors.New("aws iam credentials are required")
	}

	baseRegion := effectiveDiscoveryBaseRegion(req.Region, req.Scope.Region)
	cfg, err := s.clientFactory.NewConfig(ctx, *req.Credentials.AWS, effectiveEndpointRegion(req.Scope, baseRegion), req.Scope.Endpoint)
	if err != nil {
		return provider.ListAvailabilityZonesResult{}, err
	}

	baseClient := s.clientFactory.NewEC2(ec2ClientOptions{
		Config:   cfg,
		Endpoint: req.Scope.Endpoint,
	})
	regions, err := resolveAccountRegions(ctx, baseClient, req.Region, req.Scope.Region)
	if err != nil {
		return provider.ListAvailabilityZonesResult{}, err
	}

	items := make([]provider.AvailabilityZone, 0)
	warnings := make([]provider.Warning, 0)
	for _, region := range regions {
		regionCfg, err := s.clientFactory.NewConfig(ctx, *req.Credentials.AWS, effectiveEndpointRegion(req.Scope, region), req.Scope.Endpoint)
		if err != nil {
			return provider.ListAvailabilityZonesResult{}, fmt.Errorf("build ec2 config for region %s: %w", region, err)
		}

		availabilityZones, err := describeAvailabilityZones(ctx, s.clientFactory.NewEC2(ec2ClientOptions{
			Config:   regionCfg,
			Endpoint: req.Scope.Endpoint,
		}), region)
		if err != nil {
			return provider.ListAvailabilityZonesResult{}, err
		}

		selected, _, zoneWarnings := selectAvailabilityZones(region, availabilityZones, req.AvailabilityZones)
		warnings = append(warnings, zoneWarnings...)
		for _, zone := range selected {
			items = append(items, provider.AvailabilityZone{
				Name:               awsv2.ToString(zone.ZoneName),
				ZoneID:             awsv2.ToString(zone.ZoneId),
				Region:             region,
				State:              string(zone.State),
				ZoneType:           awsv2.ToString(zone.ZoneType),
				GroupName:          awsv2.ToString(zone.GroupName),
				NetworkBorderGroup: awsv2.ToString(zone.NetworkBorderGroup),
				ParentZoneID:       awsv2.ToString(zone.ParentZoneId),
				ParentZoneName:     awsv2.ToString(zone.ParentZoneName),
				OptInStatus:        string(zone.OptInStatus),
			})
		}
	}

	return provider.ListAvailabilityZonesResult{
		Items:    items,
		Warnings: warnings,
	}, nil
}

func effectiveDiscoveryBaseRegion(requestedRegion string, scopeRegion string) string {
	switch {
	case strings.TrimSpace(requestedRegion) != "" && !isAllSelector(requestedRegion):
		return strings.TrimSpace(requestedRegion)
	case strings.TrimSpace(scopeRegion) != "" && !isAllSelector(scopeRegion):
		return strings.TrimSpace(scopeRegion)
	default:
		return defaultAWSRegion
	}
}

func resolveAccountRegions(ctx context.Context, client ec2API, requestedRegion string, scopeRegion string) ([]string, error) {
	targetRegion := strings.TrimSpace(requestedRegion)
	if targetRegion == "" {
		targetRegion = strings.TrimSpace(scopeRegion)
	}

	if targetRegion != "" && !isAllSelector(targetRegion) {
		return []string{targetRegion}, nil
	}

	return listAccountRegions(ctx, client)
}

func effectiveDiscoveryBaseRegionWithOptions(requestedRegion string, scopeRegion string, options map[string]string) (string, error) {
	regions, hasOption, err := explicitRegionsOption(options)
	if err != nil {
		return "", err
	}
	if hasOption && len(regions) > 0 && !containsAllSelector(regions) {
		return regions[0], nil
	}

	return effectiveDiscoveryBaseRegion(requestedRegion, scopeRegion), nil
}

func resolveAccountRegionsWithOptions(
	ctx context.Context,
	client ec2API,
	requestedRegion string,
	scopeRegion string,
	options map[string]string,
) ([]string, error) {
	regions, hasOption, err := explicitRegionsOption(options)
	if err != nil {
		return nil, err
	}
	if hasOption {
		if containsAllSelector(regions) {
			return listAccountRegions(ctx, client)
		}
		return regions, nil
	}

	return resolveAccountRegions(ctx, client, requestedRegion, scopeRegion)
}

func listAccountRegions(ctx context.Context, client ec2API) ([]string, error) {
	output, err := client.DescribeRegions(ctx, &ec2.DescribeRegionsInput{
		AllRegions: awsv2.Bool(false),
	})
	if err != nil {
		return nil, fmt.Errorf("describe regions: %w", err)
	}

	regions := make([]string, 0, len(output.Regions))
	for _, region := range output.Regions {
		name := strings.TrimSpace(awsv2.ToString(region.RegionName))
		if name == "" {
			continue
		}
		regions = append(regions, name)
	}
	slices.Sort(regions)

	return slices.Compact(regions), nil
}

func explicitRegionsOption(options map[string]string) ([]string, bool, error) {
	value, ok := lookupOptionValue(options, optionRegions)
	if !ok {
		return nil, false, nil
	}

	tokens := strings.FieldsFunc(value, func(r rune) bool {
		switch r {
		case ',', '\n', '\r', '\t', ' ':
			return true
		default:
			return false
		}
	})

	regions := make([]string, 0, len(tokens))
	seen := make(map[string]struct{})
	for _, token := range tokens {
		region := strings.TrimSpace(token)
		if region == "" {
			continue
		}
		key := normalizeToken(region)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		regions = append(regions, region)
	}

	if len(regions) == 0 {
		return nil, true, fmt.Errorf("option %s must contain at least one region", optionRegions)
	}

	return regions, true, nil
}

func containsAllSelector(regions []string) bool {
	for _, region := range regions {
		if isAllSelector(region) {
			return true
		}
	}

	return false
}

func describeAvailabilityZones(ctx context.Context, client ec2API, region string) ([]ec2types.AvailabilityZone, error) {
	output, err := client.DescribeAvailabilityZones(ctx, &ec2.DescribeAvailabilityZonesInput{
		AllAvailabilityZones: awsv2.Bool(false),
	})
	if err != nil {
		return nil, fmt.Errorf("describe availability zones for region %s: %w", region, err)
	}

	return output.AvailabilityZones, nil
}

func selectAvailabilityZones(
	region string,
	availabilityZones []ec2types.AvailabilityZone,
	requested []string,
) ([]ec2types.AvailabilityZone, map[string]string, []provider.Warning) {
	requestedSet := make(map[string]struct{}, len(requested))
	allZones := len(requested) == 0
	for _, zone := range requested {
		zone = strings.TrimSpace(zone)
		if zone == "" {
			continue
		}
		if isAllSelector(zone) {
			allZones = true
			break
		}
		if looksLikeAvailabilityZone(zone) && !strings.HasPrefix(zone, region) {
			continue
		}
		requestedSet[normalizeToken(zone)] = struct{}{}
	}

	selected := make([]ec2types.AvailabilityZone, 0, len(availabilityZones))
	zoneIDToName := make(map[string]string, len(availabilityZones))
	seenRequested := make(map[string]struct{}, len(requestedSet))
	for _, zone := range availabilityZones {
		name := strings.TrimSpace(awsv2.ToString(zone.ZoneName))
		if name == "" {
			continue
		}
		zoneID := strings.TrimSpace(awsv2.ToString(zone.ZoneId))
		if zoneID != "" {
			zoneIDToName[zoneID] = name
		}

		if !allZones {
			nameKey := normalizeToken(name)
			zoneIDKey := normalizeToken(zoneID)
			if _, ok := requestedSet[nameKey]; !ok {
				if _, ok := requestedSet[zoneIDKey]; !ok {
					continue
				}
				seenRequested[zoneIDKey] = struct{}{}
			} else {
				seenRequested[nameKey] = struct{}{}
			}
		}

		selected = append(selected, zone)
	}

	slices.SortFunc(selected, func(a, b ec2types.AvailabilityZone) int {
		return strings.Compare(awsv2.ToString(a.ZoneName), awsv2.ToString(b.ZoneName))
	})

	warnings := make([]provider.Warning, 0)
	if !allZones {
		for _, zone := range requested {
			key := normalizeToken(zone)
			if key == "" {
				continue
			}
			if _, ok := seenRequested[key]; ok {
				continue
			}
			if looksLikeAvailabilityZone(zone) && !strings.HasPrefix(zone, region) {
				continue
			}
			warnings = append(warnings, provider.Warning{
				Code:    "AZ_NOT_FOUND",
				Message: fmt.Sprintf("availability zone %s was not found in region %s", zone, region),
			})
		}
	}

	return selected, zoneIDToName, warnings
}

func (s *Service) regionNameLookup(ctx context.Context) map[string]string {
	if s.catalog == nil {
		return nil
	}

	snapshot, err := s.catalog.loadCatalog(ctx)
	if err != nil {
		return nil
	}

	nameByCode := make(map[string]string)
	for _, regions := range snapshot.regionsByType {
		for _, region := range regions {
			if strings.TrimSpace(region.Code) == "" || strings.TrimSpace(region.Name) == "" {
				continue
			}
			if _, ok := nameByCode[region.Code]; ok {
				continue
			}
			nameByCode[region.Code] = region.Name
		}
	}

	return nameByCode
}
