package aws

import (
	"context"
	"fmt"
	"sort"

	"github.com/arcoloom/arco-provider-aws/internal/provider"
	awsv2 "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

const (
	optionUseDefaultVPC           = "use_default_vpc"
	optionUseDefaultSubnet        = "use_default_subnet"
	optionUseDefaultSecurityGroup = "use_default_security_group"
	optionAssociatePublicIPv4     = "associate_public_ipv4"
	optionAssignPublicIPv6        = "assign_public_ipv6"
	optionIPv6AddressCount        = "ipv6_address_count"
	optionRootVolumeSizeGiB       = "root_volume_size_gib"
	defaultRootVolumeSizeGiB      = 20
)

type startInstanceLaunchOptions struct {
	useDefaultVPC           bool
	useDefaultSubnet        bool
	useDefaultSecurityGroup bool
	hasAssociatePublicIPv4  bool
	associatePublicIPv4     bool
	assignPublicIPv6        bool
	ipv6AddressCount        int32
	rootVolumeSizeGiB       int32
}

type resolvedRunInstancesConfig struct {
	subnetID            string
	securityGroupIDs    []string
	useNetworkInterface bool
	associatePublicIPv4 *bool
	ipv6AddressCount    int32
	rootVolumeSizeGiB   int32
	rootDeviceName      string
}

func resolveRunInstancesConfig(
	ctx context.Context,
	ec2Client ec2API,
	req provider.StartInstanceRequest,
	amiID string,
	config startInstanceProviderConfig,
) (resolvedRunInstancesConfig, error) {
	var err error

	result := resolvedRunInstancesConfig{
		subnetID:          config.SubnetID,
		securityGroupIDs:  append([]string(nil), config.SecurityGroupIDs...),
		rootVolumeSizeGiB: config.LaunchOptions.rootVolumeSizeGiB,
	}

	options := config.LaunchOptions
	needsIPv6 := options.ipv6AddressCount > 0
	needsNetworkInterface := options.hasAssociatePublicIPv4 || needsIPv6
	needsManagedNetwork := result.subnetID == "" && (options.useDefaultVPC || options.useDefaultSubnet || needsNetworkInterface || len(result.securityGroupIDs) == 0)
	sharedNetwork := managedNetwork{}
	if needsManagedNetwork {
		sharedNetwork, err = ensureManagedNetwork(ctx, ec2Client, req.Region, req.AvailabilityZone)
		if err != nil {
			return resolvedRunInstancesConfig{}, err
		}
	}

	if result.subnetID == "" && sharedNetwork.subnetID != "" {
		result.subnetID = sharedNetwork.subnetID
	}

	if len(result.securityGroupIDs) == 0 && sharedNetwork.securityGroupID != "" {
		result.securityGroupIDs = []string{sharedNetwork.securityGroupID}
	} else if len(result.securityGroupIDs) == 0 && options.useDefaultSecurityGroup {
		defaultVPCID, err := resolveDefaultVPCID(ctx, ec2Client, req.Region)
		if err != nil {
			return resolvedRunInstancesConfig{}, err
		}

		defaultSecurityGroupID, err := resolveDefaultSecurityGroupID(ctx, ec2Client, req.Region, defaultVPCID)
		if err != nil {
			return resolvedRunInstancesConfig{}, err
		}
		result.securityGroupIDs = []string{defaultSecurityGroupID}
	}

	if needsNetworkInterface {
		if result.subnetID == "" {
			return resolvedRunInstancesConfig{}, fmt.Errorf(
				"start instance options require a subnet; provide subnet_id or enable %s",
				optionUseDefaultSubnet,
			)
		}

		result.useNetworkInterface = true
		if options.hasAssociatePublicIPv4 {
			result.associatePublicIPv4 = awsv2.Bool(options.associatePublicIPv4)
		}
		result.ipv6AddressCount = options.ipv6AddressCount
	}

	if options.rootVolumeSizeGiB > 0 {
		rootDeviceName, err := resolveRootDeviceName(ctx, ec2Client, amiID)
		if err != nil {
			return resolvedRunInstancesConfig{}, err
		}
		result.rootDeviceName = rootDeviceName
	}

	return result, nil
}

func lookupOptionValue(options map[string]string, target string) (string, bool) {
	target = normalizeToken(target)
	for key, value := range options {
		if normalizeToken(key) == target {
			return value, true
		}
	}
	return "", false
}

func resolveDefaultVPCID(ctx context.Context, ec2Client ec2API, region string) (string, error) {
	output, err := ec2Client.DescribeVpcs(ctx, &ec2.DescribeVpcsInput{
		Filters: []ec2types.Filter{{
			Name:   awsv2.String("is-default"),
			Values: []string{"true"},
		}},
	})
	if err != nil {
		return "", fmt.Errorf("describe default vpc in region %s: %w", region, err)
	}
	if len(output.Vpcs) == 0 {
		return "", fmt.Errorf("no default vpc was found in region %s", region)
	}

	vpcs := append([]ec2types.Vpc(nil), output.Vpcs...)
	sort.Slice(vpcs, func(i, j int) bool {
		return awsv2.ToString(vpcs[i].VpcId) < awsv2.ToString(vpcs[j].VpcId)
	})

	vpcID := awsv2.ToString(vpcs[0].VpcId)
	if vpcID == "" {
		return "", fmt.Errorf("default vpc lookup in region %s returned an empty vpc id", region)
	}

	return vpcID, nil
}

func resolveDefaultSecurityGroupID(ctx context.Context, ec2Client ec2API, region string, vpcID string) (string, error) {
	output, err := ec2Client.DescribeSecurityGroups(ctx, &ec2.DescribeSecurityGroupsInput{
		Filters: []ec2types.Filter{
			{
				Name:   awsv2.String("vpc-id"),
				Values: []string{vpcID},
			},
			{
				Name:   awsv2.String("group-name"),
				Values: []string{"default"},
			},
		},
	})
	if err != nil {
		return "", fmt.Errorf("describe default security group in region %s: %w", region, err)
	}
	if len(output.SecurityGroups) == 0 {
		return "", fmt.Errorf("no default security group was found in region %s for vpc %s", region, vpcID)
	}

	securityGroups := append([]ec2types.SecurityGroup(nil), output.SecurityGroups...)
	sort.Slice(securityGroups, func(i, j int) bool {
		return awsv2.ToString(securityGroups[i].GroupId) < awsv2.ToString(securityGroups[j].GroupId)
	})

	groupID := awsv2.ToString(securityGroups[0].GroupId)
	if groupID == "" {
		return "", fmt.Errorf("default security group lookup in region %s returned an empty group id", region)
	}

	return groupID, nil
}

func resolveRootDeviceName(ctx context.Context, ec2Client ec2API, amiID string) (string, error) {
	output, err := ec2Client.DescribeImages(ctx, &ec2.DescribeImagesInput{
		ImageIds: []string{amiID},
	})
	if err != nil {
		return "", fmt.Errorf("describe image %s for root device lookup: %w", amiID, err)
	}
	if len(output.Images) == 0 {
		return "", fmt.Errorf("describe image %s returned no images", amiID)
	}

	image := output.Images[0]
	if rootDeviceName := awsv2.ToString(image.RootDeviceName); rootDeviceName != "" {
		return rootDeviceName, nil
	}

	for _, mapping := range image.BlockDeviceMappings {
		if deviceName := awsv2.ToString(mapping.DeviceName); deviceName != "" {
			return deviceName, nil
		}
	}

	return "", fmt.Errorf("image %s did not report a root device name", amiID)
}

func subnetSupportsIPv6(subnet ec2types.Subnet) bool {
	for _, association := range subnet.Ipv6CidrBlockAssociationSet {
		state := ""
		if association.Ipv6CidrBlockState != nil {
			state = normalizeToken(string(association.Ipv6CidrBlockState.State))
		}
		if state == "" || state == "associated" {
			return true
		}
	}

	return false
}
