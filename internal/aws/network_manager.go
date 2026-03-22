package aws

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"sort"
	"strings"

	awsv2 "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/smithy-go"
)

const (
	managedNetworkVPCName             = "arco-vpc"
	managedNetworkSubnetName          = "arco-subnet"
	managedNetworkSecurityGroupName   = "arco-security-group"
	managedNetworkRouteTableName      = "arco-route-table"
	managedNetworkInternetGatewayName = "arco-igw"
	managedNetworkVPCCIDR             = "10.77.0.0/16"
	managedNetworkManagedByTagValue   = "arco-provider-aws"
	managedNetworkTagKeyShared        = "ArcoSharedNetwork"
	managedNetworkTagKeyKind          = "ArcoResourceKind"
	managedNetworkTagKeyRegion        = "ArcoRegion"
	managedNetworkTagKeyAZ            = "ArcoAvailabilityZone"
	managedNetworkTagValueShared      = "true"
	managedNetworkKindVPC             = "shared-vpc"
	managedNetworkKindSubnet          = "shared-subnet"
	managedNetworkKindRouteTable      = "shared-route-table"
	managedNetworkKindInternetGateway = "shared-internet-gateway"
	managedNetworkSGDescription       = "Managed by arco-provider-aws for outbound-only EC2 access"
)

type managedNetwork struct {
	vpcID           string
	subnetID        string
	securityGroupID string
}

func ensureManagedNetwork(
	ctx context.Context,
	ec2Client ec2API,
	region string,
	availabilityZone string,
) (managedNetwork, error) {
	vpc, err := ensureManagedVPC(ctx, ec2Client, region)
	if err != nil {
		return managedNetwork{}, err
	}

	igw, err := ensureManagedInternetGateway(ctx, ec2Client, region, awsv2.ToString(vpc.VpcId))
	if err != nil {
		return managedNetwork{}, err
	}

	routeTable, err := ensureManagedRouteTable(ctx, ec2Client, region, awsv2.ToString(vpc.VpcId), awsv2.ToString(igw.InternetGatewayId))
	if err != nil {
		return managedNetwork{}, err
	}

	subnet, err := ensureManagedSubnet(ctx, ec2Client, region, vpc, routeTable, availabilityZone)
	if err != nil {
		return managedNetwork{}, err
	}

	securityGroup, err := ensureManagedSecurityGroup(ctx, ec2Client, region, awsv2.ToString(vpc.VpcId))
	if err != nil {
		return managedNetwork{}, err
	}

	return managedNetwork{
		vpcID:           awsv2.ToString(vpc.VpcId),
		subnetID:        awsv2.ToString(subnet.SubnetId),
		securityGroupID: awsv2.ToString(securityGroup.GroupId),
	}, nil
}

func ensureManagedVPC(ctx context.Context, ec2Client ec2API, region string) (ec2types.Vpc, error) {
	vpc, found, err := findManagedVPC(ctx, ec2Client)
	if err != nil {
		return ec2types.Vpc{}, err
	}
	if !found {
		output, err := ec2Client.CreateVpc(ctx, &ec2.CreateVpcInput{
			CidrBlock:                   awsv2.String(managedNetworkVPCCIDR),
			AmazonProvidedIpv6CidrBlock: awsv2.Bool(true),
			TagSpecifications: managedTagSpecifications(
				ec2types.ResourceTypeVpc,
				managedTags(managedNetworkVPCName, managedNetworkKindVPC, region, ""),
			),
		})
		if err != nil {
			return ec2types.Vpc{}, fmt.Errorf("create %s in region %s: %w", managedNetworkVPCName, region, err)
		}
		if output.Vpc == nil || strings.TrimSpace(awsv2.ToString(output.Vpc.VpcId)) == "" {
			return ec2types.Vpc{}, fmt.Errorf("create %s in region %s returned an empty vpc id", managedNetworkVPCName, region)
		}
		vpc = *output.Vpc
	}

	if err := ensureManagedVPCDNS(ctx, ec2Client, region, awsv2.ToString(vpc.VpcId)); err != nil {
		return ec2types.Vpc{}, err
	}

	updated, err := ensureManagedVPCIPv6(ctx, ec2Client, region, vpc)
	if err != nil {
		return ec2types.Vpc{}, err
	}
	return updated, nil
}

func findManagedVPC(ctx context.Context, ec2Client ec2API) (ec2types.Vpc, bool, error) {
	output, err := ec2Client.DescribeVpcs(ctx, &ec2.DescribeVpcsInput{
		Filters: []ec2types.Filter{
			tagFilter("Name", managedNetworkVPCName),
			tagFilter("ManagedBy", managedNetworkManagedByTagValue),
			tagFilter(managedNetworkTagKeyShared, managedNetworkTagValueShared),
			tagFilter(managedNetworkTagKeyKind, managedNetworkKindVPC),
		},
	})
	if err != nil {
		return ec2types.Vpc{}, false, fmt.Errorf("describe managed vpcs: %w", err)
	}
	if len(output.Vpcs) == 0 {
		return ec2types.Vpc{}, false, nil
	}

	vpcs := append([]ec2types.Vpc(nil), output.Vpcs...)
	sort.Slice(vpcs, func(i, j int) bool {
		return awsv2.ToString(vpcs[i].VpcId) < awsv2.ToString(vpcs[j].VpcId)
	})

	return vpcs[0], true, nil
}

func ensureManagedVPCDNS(ctx context.Context, ec2Client ec2API, region string, vpcID string) error {
	if _, err := ec2Client.ModifyVpcAttribute(ctx, &ec2.ModifyVpcAttributeInput{
		VpcId: awsv2.String(vpcID),
		EnableDnsSupport: &ec2types.AttributeBooleanValue{
			Value: awsv2.Bool(true),
		},
	}); err != nil {
		return fmt.Errorf("enable dns support on vpc %s in region %s: %w", vpcID, region, err)
	}
	if _, err := ec2Client.ModifyVpcAttribute(ctx, &ec2.ModifyVpcAttributeInput{
		VpcId: awsv2.String(vpcID),
		EnableDnsHostnames: &ec2types.AttributeBooleanValue{
			Value: awsv2.Bool(true),
		},
	}); err != nil {
		return fmt.Errorf("enable dns hostnames on vpc %s in region %s: %w", vpcID, region, err)
	}

	return nil
}

func ensureManagedVPCIPv6(ctx context.Context, ec2Client ec2API, region string, vpc ec2types.Vpc) (ec2types.Vpc, error) {
	if managedVPCIPv6CIDR(vpc) != "" {
		return vpc, nil
	}

	output, err := ec2Client.AssociateVpcCidrBlock(ctx, &ec2.AssociateVpcCidrBlockInput{
		VpcId:                       awsv2.String(awsv2.ToString(vpc.VpcId)),
		AmazonProvidedIpv6CidrBlock: awsv2.Bool(true),
	})
	if err != nil {
		return ec2types.Vpc{}, fmt.Errorf("associate ipv6 cidr with vpc %s in region %s: %w", awsv2.ToString(vpc.VpcId), region, err)
	}
	if output.Ipv6CidrBlockAssociation != nil {
		vpc.Ipv6CidrBlockAssociationSet = append(vpc.Ipv6CidrBlockAssociationSet, *output.Ipv6CidrBlockAssociation)
	}
	if managedVPCIPv6CIDR(vpc) == "" {
		return ec2types.Vpc{}, fmt.Errorf("managed vpc %s in region %s did not expose an ipv6 cidr block", awsv2.ToString(vpc.VpcId), region)
	}

	return vpc, nil
}

func ensureManagedInternetGateway(ctx context.Context, ec2Client ec2API, region string, vpcID string) (ec2types.InternetGateway, error) {
	output, err := ec2Client.DescribeInternetGateways(ctx, &ec2.DescribeInternetGatewaysInput{
		Filters: []ec2types.Filter{{
			Name:   awsv2.String("attachment.vpc-id"),
			Values: []string{vpcID},
		}},
	})
	if err != nil {
		return ec2types.InternetGateway{}, fmt.Errorf("describe internet gateways for vpc %s in region %s: %w", vpcID, region, err)
	}
	if len(output.InternetGateways) > 0 {
		gateways := append([]ec2types.InternetGateway(nil), output.InternetGateways...)
		sort.Slice(gateways, func(i, j int) bool {
			return awsv2.ToString(gateways[i].InternetGatewayId) < awsv2.ToString(gateways[j].InternetGatewayId)
		})
		return gateways[0], nil
	}

	createOutput, err := ec2Client.CreateInternetGateway(ctx, &ec2.CreateInternetGatewayInput{
		TagSpecifications: managedTagSpecifications(
			ec2types.ResourceTypeInternetGateway,
			managedTags(managedNetworkInternetGatewayName, managedNetworkKindInternetGateway, region, ""),
		),
	})
	if err != nil {
		return ec2types.InternetGateway{}, fmt.Errorf("create internet gateway for vpc %s in region %s: %w", vpcID, region, err)
	}
	if createOutput.InternetGateway == nil || strings.TrimSpace(awsv2.ToString(createOutput.InternetGateway.InternetGatewayId)) == "" {
		return ec2types.InternetGateway{}, fmt.Errorf("create internet gateway for vpc %s in region %s returned an empty gateway id", vpcID, region)
	}

	igw := *createOutput.InternetGateway
	if _, err := ec2Client.AttachInternetGateway(ctx, &ec2.AttachInternetGatewayInput{
		InternetGatewayId: awsv2.String(awsv2.ToString(igw.InternetGatewayId)),
		VpcId:             awsv2.String(vpcID),
	}); err != nil && !hasAPIErrorCode(err, "Resource.AlreadyAssociated") {
		return ec2types.InternetGateway{}, fmt.Errorf("attach internet gateway %s to vpc %s in region %s: %w", awsv2.ToString(igw.InternetGatewayId), vpcID, region, err)
	}

	igw.Attachments = append(igw.Attachments, ec2types.InternetGatewayAttachment{
		VpcId: awsv2.String(vpcID),
		State: ec2types.AttachmentStatusAttached,
	})
	return igw, nil
}

func ensureManagedRouteTable(ctx context.Context, ec2Client ec2API, region string, vpcID string, internetGatewayID string) (ec2types.RouteTable, error) {
	output, err := ec2Client.DescribeRouteTables(ctx, &ec2.DescribeRouteTablesInput{
		Filters: []ec2types.Filter{
			{
				Name:   awsv2.String("vpc-id"),
				Values: []string{vpcID},
			},
			tagFilter("Name", managedNetworkRouteTableName),
			tagFilter("ManagedBy", managedNetworkManagedByTagValue),
			tagFilter(managedNetworkTagKeyShared, managedNetworkTagValueShared),
			tagFilter(managedNetworkTagKeyKind, managedNetworkKindRouteTable),
		},
	})
	if err != nil {
		return ec2types.RouteTable{}, fmt.Errorf("describe managed route tables for vpc %s in region %s: %w", vpcID, region, err)
	}

	var routeTable ec2types.RouteTable
	if len(output.RouteTables) > 0 {
		routeTables := append([]ec2types.RouteTable(nil), output.RouteTables...)
		sort.Slice(routeTables, func(i, j int) bool {
			return awsv2.ToString(routeTables[i].RouteTableId) < awsv2.ToString(routeTables[j].RouteTableId)
		})
		routeTable = routeTables[0]
	} else {
		createOutput, err := ec2Client.CreateRouteTable(ctx, &ec2.CreateRouteTableInput{
			VpcId: awsv2.String(vpcID),
			TagSpecifications: managedTagSpecifications(
				ec2types.ResourceTypeRouteTable,
				managedTags(managedNetworkRouteTableName, managedNetworkKindRouteTable, region, ""),
			),
		})
		if err != nil {
			return ec2types.RouteTable{}, fmt.Errorf("create managed route table for vpc %s in region %s: %w", vpcID, region, err)
		}
		if createOutput.RouteTable == nil || strings.TrimSpace(awsv2.ToString(createOutput.RouteTable.RouteTableId)) == "" {
			return ec2types.RouteTable{}, fmt.Errorf("create managed route table for vpc %s in region %s returned an empty route table id", vpcID, region)
		}
		routeTable = *createOutput.RouteTable
	}

	if err := ensureManagedRoute(ctx, ec2Client, region, awsv2.ToString(routeTable.RouteTableId), internetGatewayID, routeTable, "0.0.0.0/0", ""); err != nil {
		return ec2types.RouteTable{}, err
	}
	if err := ensureManagedRoute(ctx, ec2Client, region, awsv2.ToString(routeTable.RouteTableId), internetGatewayID, routeTable, "", "::/0"); err != nil {
		return ec2types.RouteTable{}, err
	}

	return routeTable, nil
}

func ensureManagedRoute(
	ctx context.Context,
	ec2Client ec2API,
	region string,
	routeTableID string,
	internetGatewayID string,
	routeTable ec2types.RouteTable,
	ipv4Destination string,
	ipv6Destination string,
) error {
	if routeTableHasDestination(routeTable, ipv4Destination, ipv6Destination) {
		return nil
	}

	input := &ec2.CreateRouteInput{
		RouteTableId: awsv2.String(routeTableID),
		GatewayId:    awsv2.String(internetGatewayID),
	}
	if ipv4Destination != "" {
		input.DestinationCidrBlock = awsv2.String(ipv4Destination)
	}
	if ipv6Destination != "" {
		input.DestinationIpv6CidrBlock = awsv2.String(ipv6Destination)
	}

	if _, err := ec2Client.CreateRoute(ctx, input); err != nil && !hasAPIErrorCode(err, "RouteAlreadyExists") {
		target := ipv4Destination
		if target == "" {
			target = ipv6Destination
		}
		return fmt.Errorf("create route %s via internet gateway %s in region %s: %w", target, internetGatewayID, region, err)
	}

	return nil
}

func ensureManagedSubnet(
	ctx context.Context,
	ec2Client ec2API,
	region string,
	vpc ec2types.Vpc,
	routeTable ec2types.RouteTable,
	availabilityZone string,
) (ec2types.Subnet, error) {
	subnets, err := listManagedSubnets(ctx, ec2Client, awsv2.ToString(vpc.VpcId))
	if err != nil {
		return ec2types.Subnet{}, err
	}

	selectedAZ := strings.TrimSpace(availabilityZone)
	if selectedAZ == "" {
		if subnet, ok := firstManagedSubnet(subnets, ""); ok {
			return ensureManagedSubnetReady(ctx, ec2Client, region, vpc, routeTable, subnet, subnets)
		}
		selectedAZ, err = chooseManagedSubnetAZ(ctx, ec2Client, region)
		if err != nil {
			return ec2types.Subnet{}, err
		}
	} else if subnet, ok := firstManagedSubnet(subnets, selectedAZ); ok {
		return ensureManagedSubnetReady(ctx, ec2Client, region, vpc, routeTable, subnet, subnets)
	}

	index, err := firstAvailableManagedSubnetIndex(subnets)
	if err != nil {
		return ec2types.Subnet{}, err
	}

	ipv4CIDR, err := managedSubnetIPv4CIDR(index)
	if err != nil {
		return ec2types.Subnet{}, err
	}
	ipv6CIDR, err := managedSubnetIPv6CIDR(managedVPCIPv6CIDR(vpc), index)
	if err != nil {
		return ec2types.Subnet{}, err
	}

	createOutput, err := ec2Client.CreateSubnet(ctx, &ec2.CreateSubnetInput{
		VpcId:            awsv2.String(awsv2.ToString(vpc.VpcId)),
		AvailabilityZone: awsv2.String(selectedAZ),
		CidrBlock:        awsv2.String(ipv4CIDR),
		Ipv6CidrBlock:    awsv2.String(ipv6CIDR),
		TagSpecifications: managedTagSpecifications(
			ec2types.ResourceTypeSubnet,
			managedTags(managedNetworkSubnetName, managedNetworkKindSubnet, region, selectedAZ),
		),
	})
	if err != nil {
		return ec2types.Subnet{}, fmt.Errorf("create managed subnet in az %s for region %s: %w", selectedAZ, region, err)
	}
	if createOutput.Subnet == nil || strings.TrimSpace(awsv2.ToString(createOutput.Subnet.SubnetId)) == "" {
		return ec2types.Subnet{}, fmt.Errorf("create managed subnet in az %s for region %s returned an empty subnet id", selectedAZ, region)
	}

	return ensureManagedSubnetReady(ctx, ec2Client, region, vpc, routeTable, *createOutput.Subnet, subnets)
}

func listManagedSubnets(ctx context.Context, ec2Client ec2API, vpcID string) ([]ec2types.Subnet, error) {
	output, err := ec2Client.DescribeSubnets(ctx, &ec2.DescribeSubnetsInput{
		Filters: []ec2types.Filter{
			{
				Name:   awsv2.String("vpc-id"),
				Values: []string{vpcID},
			},
			tagFilter("Name", managedNetworkSubnetName),
			tagFilter("ManagedBy", managedNetworkManagedByTagValue),
			tagFilter(managedNetworkTagKeyShared, managedNetworkTagValueShared),
			tagFilter(managedNetworkTagKeyKind, managedNetworkKindSubnet),
		},
	})
	if err != nil {
		return nil, fmt.Errorf("describe managed subnets for vpc %s: %w", vpcID, err)
	}

	subnets := append([]ec2types.Subnet(nil), output.Subnets...)
	sort.Slice(subnets, func(i, j int) bool {
		leftAZ := awsv2.ToString(subnets[i].AvailabilityZone)
		rightAZ := awsv2.ToString(subnets[j].AvailabilityZone)
		if leftAZ != rightAZ {
			return leftAZ < rightAZ
		}
		return awsv2.ToString(subnets[i].SubnetId) < awsv2.ToString(subnets[j].SubnetId)
	})
	return subnets, nil
}

func firstManagedSubnet(subnets []ec2types.Subnet, availabilityZone string) (ec2types.Subnet, bool) {
	for _, subnet := range subnets {
		if availabilityZone == "" || strings.EqualFold(awsv2.ToString(subnet.AvailabilityZone), availabilityZone) {
			return subnet, true
		}
	}
	return ec2types.Subnet{}, false
}

func chooseManagedSubnetAZ(ctx context.Context, ec2Client ec2API, region string) (string, error) {
	output, err := ec2Client.DescribeAvailabilityZones(ctx, &ec2.DescribeAvailabilityZonesInput{
		AllAvailabilityZones: awsv2.Bool(false),
	})
	if err != nil {
		return "", fmt.Errorf("describe availability zones for managed subnet placement in region %s: %w", region, err)
	}

	names := make([]string, 0, len(output.AvailabilityZones))
	for _, zone := range output.AvailabilityZones {
		name := strings.TrimSpace(awsv2.ToString(zone.ZoneName))
		if name == "" {
			continue
		}
		state := normalizeToken(string(zone.State))
		if state != "" && state != normalizeToken(string(ec2types.AvailabilityZoneStateAvailable)) {
			continue
		}
		zoneType := normalizeToken(awsv2.ToString(zone.ZoneType))
		if zoneType != "" && zoneType != "availabilityzone" {
			continue
		}
		names = append(names, name)
	}
	if len(names) == 0 {
		return "", fmt.Errorf("no available availability zone was found in region %s for managed subnet placement", region)
	}

	sort.Strings(names)
	return names[0], nil
}

func ensureManagedSubnetReady(
	ctx context.Context,
	ec2Client ec2API,
	region string,
	vpc ec2types.Vpc,
	routeTable ec2types.RouteTable,
	subnet ec2types.Subnet,
	allManagedSubnets []ec2types.Subnet,
) (ec2types.Subnet, error) {
	subnetID := awsv2.ToString(subnet.SubnetId)
	if subnetID == "" {
		return ec2types.Subnet{}, fmt.Errorf("managed subnet in region %s returned an empty subnet id", region)
	}

	if !subnetSupportsIPv6(subnet) {
		index, err := managedSubnetIndex(subnet)
		if err != nil {
			return ec2types.Subnet{}, err
		}
		ipv6CIDR, err := managedSubnetIPv6CIDR(managedVPCIPv6CIDR(vpc), index)
		if err != nil {
			return ec2types.Subnet{}, err
		}
		output, err := ec2Client.AssociateSubnetCidrBlock(ctx, &ec2.AssociateSubnetCidrBlockInput{
			SubnetId:      awsv2.String(subnetID),
			Ipv6CidrBlock: awsv2.String(ipv6CIDR),
		})
		if err != nil {
			return ec2types.Subnet{}, fmt.Errorf("associate ipv6 cidr with subnet %s in region %s: %w", subnetID, region, err)
		}
		if output.Ipv6CidrBlockAssociation != nil {
			subnet.Ipv6CidrBlockAssociationSet = append(subnet.Ipv6CidrBlockAssociationSet, *output.Ipv6CidrBlockAssociation)
		}
	}

	if !awsv2.ToBool(subnet.MapPublicIpOnLaunch) {
		if _, err := ec2Client.ModifySubnetAttribute(ctx, &ec2.ModifySubnetAttributeInput{
			SubnetId: awsv2.String(subnetID),
			MapPublicIpOnLaunch: &ec2types.AttributeBooleanValue{
				Value: awsv2.Bool(true),
			},
		}); err != nil {
			return ec2types.Subnet{}, fmt.Errorf("enable public ipv4 mapping on subnet %s in region %s: %w", subnetID, region, err)
		}
	}
	if !awsv2.ToBool(subnet.AssignIpv6AddressOnCreation) {
		if _, err := ec2Client.ModifySubnetAttribute(ctx, &ec2.ModifySubnetAttributeInput{
			SubnetId: awsv2.String(subnetID),
			AssignIpv6AddressOnCreation: &ec2types.AttributeBooleanValue{
				Value: awsv2.Bool(true),
			},
		}); err != nil {
			return ec2types.Subnet{}, fmt.Errorf("enable ipv6 auto-assignment on subnet %s in region %s: %w", subnetID, region, err)
		}
	}

	if !routeTableAssociatesSubnet(routeTable, subnetID) {
		if _, err := ec2Client.AssociateRouteTable(ctx, &ec2.AssociateRouteTableInput{
			RouteTableId: awsv2.String(awsv2.ToString(routeTable.RouteTableId)),
			SubnetId:     awsv2.String(subnetID),
		}); err != nil && !hasAPIErrorCode(err, "Resource.AlreadyAssociated") {
			return ec2types.Subnet{}, fmt.Errorf("associate route table %s with subnet %s in region %s: %w", awsv2.ToString(routeTable.RouteTableId), subnetID, region, err)
		}
	}

	for _, managedSubnet := range allManagedSubnets {
		if awsv2.ToString(managedSubnet.SubnetId) == subnetID {
			return subnet, nil
		}
	}
	return subnet, nil
}

func ensureManagedSecurityGroup(ctx context.Context, ec2Client ec2API, region string, vpcID string) (ec2types.SecurityGroup, error) {
	output, err := ec2Client.DescribeSecurityGroups(ctx, &ec2.DescribeSecurityGroupsInput{
		Filters: []ec2types.Filter{
			{
				Name:   awsv2.String("vpc-id"),
				Values: []string{vpcID},
			},
			{
				Name:   awsv2.String("group-name"),
				Values: []string{managedNetworkSecurityGroupName},
			},
		},
	})
	if err != nil {
		return ec2types.SecurityGroup{}, fmt.Errorf("describe managed security groups for vpc %s in region %s: %w", vpcID, region, err)
	}

	if len(output.SecurityGroups) == 0 {
		createOutput, err := ec2Client.CreateSecurityGroup(ctx, &ec2.CreateSecurityGroupInput{
			GroupName:   awsv2.String(managedNetworkSecurityGroupName),
			Description: awsv2.String(managedNetworkSGDescription),
			VpcId:       awsv2.String(vpcID),
			TagSpecifications: managedTagSpecifications(
				ec2types.ResourceTypeSecurityGroup,
				managedTags(managedNetworkSecurityGroupName, "shared-security-group", region, ""),
			),
		})
		if err != nil {
			if hasAPIErrorCode(err, "InvalidGroup.Duplicate") {
				return ensureManagedSecurityGroup(ctx, ec2Client, region, vpcID)
			}
			return ec2types.SecurityGroup{}, fmt.Errorf("create managed security group for vpc %s in region %s: %w", vpcID, region, err)
		}
		groupID := strings.TrimSpace(awsv2.ToString(createOutput.GroupId))
		if groupID == "" {
			return ec2types.SecurityGroup{}, fmt.Errorf("create managed security group for vpc %s in region %s returned an empty group id", vpcID, region)
		}
		return ec2types.SecurityGroup{
			GroupId:   awsv2.String(groupID),
			GroupName: awsv2.String(managedNetworkSecurityGroupName),
			VpcId:     awsv2.String(vpcID),
		}, nil
	}

	securityGroups := append([]ec2types.SecurityGroup(nil), output.SecurityGroups...)
	sort.Slice(securityGroups, func(i, j int) bool {
		return awsv2.ToString(securityGroups[i].GroupId) < awsv2.ToString(securityGroups[j].GroupId)
	})

	securityGroup := securityGroups[0]
	if len(securityGroup.IpPermissions) > 0 {
		if _, err := ec2Client.RevokeSecurityGroupIngress(ctx, &ec2.RevokeSecurityGroupIngressInput{
			GroupId:       awsv2.String(awsv2.ToString(securityGroup.GroupId)),
			IpPermissions: append([]ec2types.IpPermission(nil), securityGroup.IpPermissions...),
		}); err != nil {
			return ec2types.SecurityGroup{}, fmt.Errorf("revoke ingress rules on managed security group %s in region %s: %w", awsv2.ToString(securityGroup.GroupId), region, err)
		}
	}

	missingEgress := missingManagedEgressPermissions(securityGroup)
	if len(missingEgress) > 0 {
		if _, err := ec2Client.AuthorizeSecurityGroupEgress(ctx, &ec2.AuthorizeSecurityGroupEgressInput{
			GroupId:       awsv2.String(awsv2.ToString(securityGroup.GroupId)),
			IpPermissions: missingEgress,
		}); err != nil && !hasAPIErrorCode(err, "InvalidPermission.Duplicate") {
			return ec2types.SecurityGroup{}, fmt.Errorf("authorize outbound rules on managed security group %s in region %s: %w", awsv2.ToString(securityGroup.GroupId), region, err)
		}
	}

	return securityGroup, nil
}

func routeTableHasDestination(routeTable ec2types.RouteTable, ipv4Destination string, ipv6Destination string) bool {
	for _, route := range routeTable.Routes {
		if ipv4Destination != "" && strings.TrimSpace(awsv2.ToString(route.DestinationCidrBlock)) == ipv4Destination {
			return true
		}
		if ipv6Destination != "" && strings.TrimSpace(awsv2.ToString(route.DestinationIpv6CidrBlock)) == ipv6Destination {
			return true
		}
	}
	return false
}

func routeTableAssociatesSubnet(routeTable ec2types.RouteTable, subnetID string) bool {
	for _, association := range routeTable.Associations {
		if strings.TrimSpace(awsv2.ToString(association.SubnetId)) == subnetID {
			return true
		}
	}
	return false
}

func managedVPCIPv6CIDR(vpc ec2types.Vpc) string {
	for _, association := range vpc.Ipv6CidrBlockAssociationSet {
		cidr := strings.TrimSpace(awsv2.ToString(association.Ipv6CidrBlock))
		if cidr == "" {
			continue
		}

		state := ""
		if association.Ipv6CidrBlockState != nil {
			state = normalizeToken(string(association.Ipv6CidrBlockState.State))
		}
		switch state {
		case "", "associated", "associating":
			return cidr
		}
	}
	return ""
}

func firstAvailableManagedSubnetIndex(subnets []ec2types.Subnet) (int, error) {
	used := make(map[int]struct{}, len(subnets))
	for _, subnet := range subnets {
		index, err := managedSubnetIndex(subnet)
		if err != nil {
			return 0, err
		}
		used[index] = struct{}{}
	}

	for index := 0; index < 256; index++ {
		if _, exists := used[index]; !exists {
			return index, nil
		}
	}

	return 0, errors.New("no free managed subnet cidr blocks remain in the shared arco vpc")
}

func managedSubnetIndex(subnet ec2types.Subnet) (int, error) {
	cidr := strings.TrimSpace(awsv2.ToString(subnet.CidrBlock))
	prefix, err := netip.ParsePrefix(cidr)
	if err != nil {
		return 0, fmt.Errorf("parse managed subnet cidr %q: %w", cidr, err)
	}
	if prefix.Bits() != 24 {
		return 0, fmt.Errorf("managed subnet cidr %q must be a /24", cidr)
	}

	base, err := netip.ParsePrefix(managedNetworkVPCCIDR)
	if err != nil {
		return 0, err
	}
	addr := prefix.Masked().Addr().As4()
	baseAddr := base.Masked().Addr().As4()
	if addr[0] != baseAddr[0] || addr[1] != baseAddr[1] {
		return 0, fmt.Errorf("managed subnet cidr %q is not within %s", cidr, managedNetworkVPCCIDR)
	}
	return int(addr[2]), nil
}

func managedSubnetIPv4CIDR(index int) (string, error) {
	if index < 0 || index > 255 {
		return "", fmt.Errorf("managed subnet index %d is outside the supported /24 range", index)
	}

	base, err := netip.ParsePrefix(managedNetworkVPCCIDR)
	if err != nil {
		return "", fmt.Errorf("parse managed vpc cidr %s: %w", managedNetworkVPCCIDR, err)
	}
	addr := base.Masked().Addr().As4()
	addr[2] = byte(index)
	addr[3] = 0
	return netip.PrefixFrom(netip.AddrFrom4(addr), 24).String(), nil
}

func managedSubnetIPv6CIDR(vpcIPv6CIDR string, index int) (string, error) {
	if strings.TrimSpace(vpcIPv6CIDR) == "" {
		return "", errors.New("managed vpc does not have an ipv6 cidr block")
	}
	if index < 0 || index > 255 {
		return "", fmt.Errorf("managed subnet index %d is outside the supported ipv6 /64 range", index)
	}

	base, err := netip.ParsePrefix(vpcIPv6CIDR)
	if err != nil {
		return "", fmt.Errorf("parse managed vpc ipv6 cidr %q: %w", vpcIPv6CIDR, err)
	}
	if base.Bits() != 56 {
		return "", fmt.Errorf("managed vpc ipv6 cidr %q must be a /56", vpcIPv6CIDR)
	}

	addr := base.Masked().Addr().As16()
	addr[7] = byte(index)
	for i := 8; i < len(addr); i++ {
		addr[i] = 0
	}
	return netip.PrefixFrom(netip.AddrFrom16(addr), 64).String(), nil
}

func missingManagedEgressPermissions(securityGroup ec2types.SecurityGroup) []ec2types.IpPermission {
	missing := make([]ec2types.IpPermission, 0, 2)
	if !securityGroupAllowsAllIPv4Egress(securityGroup) {
		missing = append(missing, ec2types.IpPermission{
			IpProtocol: awsv2.String("-1"),
			IpRanges: []ec2types.IpRange{{
				CidrIp: awsv2.String("0.0.0.0/0"),
			}},
		})
	}
	if !securityGroupAllowsAllIPv6Egress(securityGroup) {
		missing = append(missing, ec2types.IpPermission{
			IpProtocol: awsv2.String("-1"),
			Ipv6Ranges: []ec2types.Ipv6Range{{
				CidrIpv6: awsv2.String("::/0"),
			}},
		})
	}
	return missing
}

func securityGroupAllowsAllIPv4Egress(securityGroup ec2types.SecurityGroup) bool {
	for _, permission := range securityGroup.IpPermissionsEgress {
		if strings.TrimSpace(awsv2.ToString(permission.IpProtocol)) != "-1" {
			continue
		}
		for _, ipRange := range permission.IpRanges {
			if strings.TrimSpace(awsv2.ToString(ipRange.CidrIp)) == "0.0.0.0/0" {
				return true
			}
		}
	}
	return false
}

func securityGroupAllowsAllIPv6Egress(securityGroup ec2types.SecurityGroup) bool {
	for _, permission := range securityGroup.IpPermissionsEgress {
		if strings.TrimSpace(awsv2.ToString(permission.IpProtocol)) != "-1" {
			continue
		}
		for _, ipRange := range permission.Ipv6Ranges {
			if strings.TrimSpace(awsv2.ToString(ipRange.CidrIpv6)) == "::/0" {
				return true
			}
		}
	}
	return false
}

func managedTags(name string, kind string, region string, availabilityZone string) []ec2types.Tag {
	tagMap := map[string]string{
		"Name":                     name,
		"ManagedBy":                managedNetworkManagedByTagValue,
		managedNetworkTagKeyShared: managedNetworkTagValueShared,
		managedNetworkTagKeyKind:   kind,
	}
	if region != "" {
		tagMap[managedNetworkTagKeyRegion] = region
	}
	if availabilityZone != "" {
		tagMap[managedNetworkTagKeyAZ] = availabilityZone
	}

	keys := make([]string, 0, len(tagMap))
	for key := range tagMap {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	tags := make([]ec2types.Tag, 0, len(keys))
	for _, key := range keys {
		tags = append(tags, ec2types.Tag{
			Key:   awsv2.String(key),
			Value: awsv2.String(tagMap[key]),
		})
	}
	return tags
}

func managedTagSpecifications(resourceType ec2types.ResourceType, tags []ec2types.Tag) []ec2types.TagSpecification {
	return []ec2types.TagSpecification{{
		ResourceType: resourceType,
		Tags:         tags,
	}}
}

func tagFilter(key string, value string) ec2types.Filter {
	return ec2types.Filter{
		Name:   awsv2.String("tag:" + key),
		Values: []string{value},
	}
}

func hasAPIErrorCode(err error, codes ...string) bool {
	var apiErr smithy.APIError
	if !errors.As(err, &apiErr) {
		return false
	}

	for _, code := range codes {
		if apiErr.ErrorCode() == code {
			return true
		}
	}

	return false
}
