package aws

import "github.com/arcoloom/arco-provider-aws/internal/provider"

func awsAuthMethods() []provider.AuthMethod {
	return []provider.AuthMethod{
		{
			Name:        provider.AuthMethodAWSDefaultCredentials,
			DisplayName: "Default Credential Chain",
			Description: "Authenticate with the AWS SDK default credential chain, including environment variables, shared config/profile, web identity, IAM Identity Center, ECS task roles, and EC2 instance roles. Optionally assume a target role before making API calls.",
			Fields: []provider.SchemaAttribute{
				{
					Name:        "profile",
					Type:        provider.SchemaAttributeTypeString,
					Optional:    true,
					Description: "Optional AWS shared config profile to load before resolving the default credential chain.",
				},
				{
					Name:        "role_arn",
					Type:        provider.SchemaAttributeTypeString,
					Optional:    true,
					Description: "Optional ARN of the IAM role to assume after loading the default credential chain.",
				},
				{
					Name:        "external_id",
					Type:        provider.SchemaAttributeTypeString,
					Optional:    true,
					Sensitive:   true,
					Description: "Optional external ID required by the target role trust policy.",
				},
				{
					Name:        "role_session_name",
					Type:        provider.SchemaAttributeTypeString,
					Optional:    true,
					Description: "Optional STS role session name. When omitted, the provider generates a unique session name.",
				},
				{
					Name:        "source_identity",
					Type:        provider.SchemaAttributeTypeString,
					Optional:    true,
					Description: "Optional STS source identity propagated to CloudTrail and trust-policy conditions.",
				},
			},
		},
		{
			Name:        provider.AuthMethodAWSStaticAccessKey,
			DisplayName: "Static Access Key",
			Description: "Authenticate with an AWS access key ID, secret access key, and optional session token.",
			Fields: []provider.SchemaAttribute{
				{
					Name:        "access_key_id",
					Type:        provider.SchemaAttributeTypeString,
					Required:    true,
					Sensitive:   true,
					Description: "AWS access key ID used for request signing.",
				},
				{
					Name:        "secret_access_key",
					Type:        provider.SchemaAttributeTypeString,
					Required:    true,
					Sensitive:   true,
					Description: "AWS secret access key paired with the access key ID.",
				},
				{
					Name:        "session_token",
					Type:        provider.SchemaAttributeTypeString,
					Optional:    true,
					Sensitive:   true,
					Description: "Optional AWS session token for temporary credentials.",
				},
			},
		},
		{
			Name:        provider.AuthMethodAWSAssumeRole,
			DisplayName: "Assume Role",
			Description: "Authenticate with base AWS credentials, then assume a target role before making API calls.",
			Fields: []provider.SchemaAttribute{
				{
					Name:        "access_key_id",
					Type:        provider.SchemaAttributeTypeString,
					Required:    true,
					Sensitive:   true,
					Description: "AWS access key ID used to obtain the role session.",
				},
				{
					Name:        "secret_access_key",
					Type:        provider.SchemaAttributeTypeString,
					Required:    true,
					Sensitive:   true,
					Description: "AWS secret access key paired with the base access key ID.",
				},
				{
					Name:        "session_token",
					Type:        provider.SchemaAttributeTypeString,
					Optional:    true,
					Sensitive:   true,
					Description: "Optional session token for the base credentials.",
				},
				{
					Name:        "role_arn",
					Type:        provider.SchemaAttributeTypeString,
					Required:    true,
					Description: "ARN of the IAM role to assume.",
				},
				{
					Name:        "external_id",
					Type:        provider.SchemaAttributeTypeString,
					Optional:    true,
					Sensitive:   true,
					Description: "Optional external ID required by the target role trust policy.",
				},
				{
					Name:        "role_session_name",
					Type:        provider.SchemaAttributeTypeString,
					Optional:    true,
					Description: "Optional STS role session name. When omitted, the provider generates a unique session name.",
				},
				{
					Name:        "source_identity",
					Type:        provider.SchemaAttributeTypeString,
					Optional:    true,
					Description: "Optional STS source identity propagated to CloudTrail and trust-policy conditions.",
				},
			},
		},
	}
}

func awsComputeInstanceSchema() provider.ResourceSchema {
	return provider.ResourceSchema{
		Type:        "compute_instance",
		Description: "Provider-defined attributes for launching an AWS EC2 instance.",
		Attributes: []provider.SchemaAttribute{
			{
				Name:        "ami",
				Type:        provider.SchemaAttributeTypeString,
				Optional:    true,
				Description: "AMI identifier. When omitted, the provider resolves the latest image for the selected OS and instance architecture.",
			},
			{
				Name:         "os",
				Type:         provider.SchemaAttributeTypeString,
				Optional:     true,
				Description:  "Operating system image to resolve when ami is omitted. Supported values: debian-13, ubuntu-24.04-lts.",
				DefaultValue: providerOSDebian13,
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
				Name:        "network_mode",
				Type:        provider.SchemaAttributeTypeString,
				Optional:    true,
				Description: "Strict address-family mode for the primary interface. Supported values: ipv4, ipv6, ipv4+ipv6. When set, the provider enforces the requested IP family selection during subnet placement and instance launch.",
			},
			{
				Name:         "cpu_credit_mode",
				Type:         provider.SchemaAttributeTypeString,
				Optional:     true,
				Description:  "CPU performance model selector for burstable capacity. Supported values: standard, burstable, any. Default keeps offerings that do not rely on CPU credits.",
				DefaultValue: "standard",
			},
			{
				Name:         "use_default_vpc",
				Type:         provider.SchemaAttributeTypeBool,
				Optional:     true,
				Description:  "Resolve or create the provider-managed shared VPC (`arco-vpc`) in the selected region and reuse it for network placement.",
				DefaultValue: false,
			},
			{
				Name:         "use_default_subnet",
				Type:         provider.SchemaAttributeTypeBool,
				Optional:     true,
				Description:  "Resolve or create a provider-managed shared subnet (`arco-subnet`) in the selected region or availability zone.",
				DefaultValue: false,
			},
			{
				Name:         "use_default_security_group",
				Type:         provider.SchemaAttributeTypeBool,
				Optional:     true,
				Description:  "Resolve or create the provider-managed outbound-only security group for the selected VPC.",
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
				Description:  "Whether the instance should request IPv6 addresses when network_mode is not set or when compatible with the selected network mode.",
				DefaultValue: false,
			},
			{
				Name:        "ipv6_address_count",
				Type:        provider.SchemaAttributeTypeInt64,
				Optional:    true,
				Description: "Number of IPv6 addresses to allocate on the primary network interface. Use 0 to explicitly suppress IPv6 auto-assignment on compatible subnets.",
			},
			{
				Name:         "root_volume_size_gib",
				Type:         provider.SchemaAttributeTypeInt64,
				Optional:     true,
				Description:  "Root volume size in GiB for the launched instance.",
				DefaultValue: int64(defaultRootVolumeSizeGiB),
			},
		},
	}
}
