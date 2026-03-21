package aws

import "github.com/arcoloom/arco-provider-aws/internal/provider"

func awsComputeInstanceSchema() provider.ResourceSchema {
	return provider.ResourceSchema{
		Type:        "compute_instance",
		Description: "Provider-defined attributes for launching an AWS EC2 instance.",
		Attributes: []provider.SchemaAttribute{
			{
				Name:        "ami",
				Type:        provider.SchemaAttributeTypeString,
				Optional:    true,
				Description: "AMI identifier. When omitted, the provider resolves the latest Debian 13 image for the selected instance architecture.",
			},
			{
				Name:        "subnet_id",
				Type:        provider.SchemaAttributeTypeString,
				Optional:    true,
				Description: "Subnet identifier used for instance placement.",
			},
			{
				Name:        "security_group_ids",
				Type:        provider.SchemaAttributeTypeStringList,
				Optional:    true,
				Description: "Security group identifiers to attach to the instance network interface.",
			},
			{
				Name:        "key_name",
				Type:        provider.SchemaAttributeTypeString,
				Optional:    true,
				Description: "EC2 key pair name for SSH access.",
			},
			{
				Name:         "use_default_vpc",
				Type:         provider.SchemaAttributeTypeBool,
				Optional:     true,
				Description:  "Resolve the account default VPC and reuse it for network placement.",
				DefaultValue: false,
			},
			{
				Name:         "use_default_subnet",
				Type:         provider.SchemaAttributeTypeBool,
				Optional:     true,
				Description:  "Resolve a default subnet in the selected region or availability zone.",
				DefaultValue: false,
			},
			{
				Name:         "use_default_security_group",
				Type:         provider.SchemaAttributeTypeBool,
				Optional:     true,
				Description:  "Resolve the default security group for the selected VPC.",
				DefaultValue: false,
			},
			{
				Name:        "associate_public_ipv4",
				Type:        provider.SchemaAttributeTypeBool,
				Optional:    true,
				Description: "Whether the primary network interface should receive a public IPv4 address.",
			},
			{
				Name:         "assign_public_ipv6",
				Type:         provider.SchemaAttributeTypeBool,
				Optional:     true,
				Description:  "Whether the instance should request a public IPv6 address.",
				DefaultValue: false,
			},
			{
				Name:        "ipv6_address_count",
				Type:        provider.SchemaAttributeTypeInt64,
				Optional:    true,
				Description: "Number of IPv6 addresses to allocate on the primary network interface.",
			},
			{
				Name:        "root_volume_size_gib",
				Type:        provider.SchemaAttributeTypeInt64,
				Optional:    true,
				Description: "Root volume size in GiB for the launched instance.",
			},
		},
	}
}
