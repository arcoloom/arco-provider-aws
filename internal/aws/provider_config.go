package aws

import (
	"fmt"
	"strconv"
	"strings"
)

const (
	providerConfigAMI              = "ami"
	providerConfigOS               = "os"
	providerConfigNetworkMode      = "network_mode"
	providerConfigSubnetID         = "subnet_id"
	providerConfigSecurityGroupIDs = "security_group_ids"
	providerConfigKeyName          = "key_name"
	providerAttributeSubnetID      = "subnet_id"
	providerAttributeVPCID         = "vpc_id"
	providerOSDebian13             = "debian-13"
	providerOSUbuntu2404LTS        = "ubuntu-24.04-lts"
	providerNetworkModeIPv4        = "ipv4"
	providerNetworkModeIPv6        = "ipv6"
	providerNetworkModeDualStack   = "ipv4+ipv6"
)

type startInstanceProviderConfig struct {
	AMI              string
	OS               string
	NetworkMode      string
	SubnetID         string
	SecurityGroupIDs []string
	KeyName          string
	LaunchOptions    startInstanceLaunchOptions
}

func parseStartInstanceProviderConfig(config map[string]any) (startInstanceProviderConfig, error) {
	result := startInstanceProviderConfig{}
	var err error

	result.AMI = parseStringProviderConfig(config, providerConfigAMI)
	result.OS, err = parseOSProviderConfig(config)
	if err != nil {
		return startInstanceProviderConfig{}, err
	}
	result.NetworkMode, err = parseNetworkModeProviderConfig(config)
	if err != nil {
		return startInstanceProviderConfig{}, err
	}
	result.SubnetID = parseStringProviderConfig(config, providerConfigSubnetID)
	result.KeyName = parseStringProviderConfig(config, providerConfigKeyName)

	result.SecurityGroupIDs, err = parseStringListProviderConfig(config, providerConfigSecurityGroupIDs)
	if err != nil {
		return startInstanceProviderConfig{}, err
	}

	result.LaunchOptions, err = parseStartInstanceLaunchProviderConfig(config, result.NetworkMode)
	if err != nil {
		return startInstanceProviderConfig{}, err
	}

	return result, nil
}

func parseOSProviderConfig(config map[string]any) (string, error) {
	value, ok := lookupProviderConfigValue(config, providerConfigOS)
	if !ok {
		return providerOSDebian13, nil
	}

	switch comparableOS(strings.TrimSpace(asString(value))) {
	case "debian13":
		return providerOSDebian13, nil
	case "ubuntu2404lts":
		return providerOSUbuntu2404LTS, nil
	default:
		return "", fmt.Errorf("provider_config.%s must be one of %s or %s", providerConfigOS, providerOSDebian13, providerOSUbuntu2404LTS)
	}
}

func comparableOS(value string) string {
	return strings.NewReplacer(" ", "", "-", "", "_", "", ".", "").Replace(strings.ToLower(strings.TrimSpace(value)))
}

func parseStartInstanceLaunchProviderConfig(config map[string]any, networkMode string) (startInstanceLaunchOptions, error) {
	result := startInstanceLaunchOptions{
		rootVolumeSizeGiB: defaultRootVolumeSizeGiB,
	}
	var (
		err                 error
		hasAssignPublicIPv6 bool
	)

	if result.useDefaultVPC, _, err = parseBoolProviderConfig(config, optionUseDefaultVPC); err != nil {
		return startInstanceLaunchOptions{}, err
	}
	if result.useDefaultSubnet, _, err = parseBoolProviderConfig(config, optionUseDefaultSubnet); err != nil {
		return startInstanceLaunchOptions{}, err
	}
	if result.useDefaultSecurityGroup, _, err = parseBoolProviderConfig(config, optionUseDefaultSecurityGroup); err != nil {
		return startInstanceLaunchOptions{}, err
	}
	if result.associatePublicIPv4, result.hasAssociatePublicIPv4, err = parseBoolProviderConfig(config, optionAssociatePublicIPv4); err != nil {
		return startInstanceLaunchOptions{}, err
	}
	if result.assignPublicIPv6, hasAssignPublicIPv6, err = parseBoolProviderConfig(config, optionAssignPublicIPv6); err != nil {
		return startInstanceLaunchOptions{}, err
	}
	if result.ipv6AddressCount, result.hasIPv6AddressCount, err = parseNonNegativeInt32ProviderConfig(config, optionIPv6AddressCount); err != nil {
		return startInstanceLaunchOptions{}, err
	}
	if result.rootVolumeSizeGiB, _, err = parsePositiveInt32ProviderConfig(config, optionRootVolumeSizeGiB); err != nil {
		return startInstanceLaunchOptions{}, err
	}
	if result.rootVolumeSizeGiB == 0 {
		result.rootVolumeSizeGiB = defaultRootVolumeSizeGiB
	}

	if result.useDefaultVPC {
		result.useDefaultSubnet = true
	}
	switch networkMode {
	case providerNetworkModeIPv4:
		if !result.hasAssociatePublicIPv4 {
			result.associatePublicIPv4 = true
			result.hasAssociatePublicIPv4 = true
		}
		result.assignPublicIPv6 = false
		result.hasIPv6AddressCount = true
		result.ipv6AddressCount = 0
	case providerNetworkModeIPv6:
		result.associatePublicIPv4 = false
		result.hasAssociatePublicIPv4 = true
		result.assignPublicIPv6 = true
		result.hasIPv6AddressCount = true
		if result.ipv6AddressCount == 0 {
			result.ipv6AddressCount = 1
		}
	case providerNetworkModeDualStack:
		if !result.hasAssociatePublicIPv4 {
			result.associatePublicIPv4 = true
			result.hasAssociatePublicIPv4 = true
		}
		result.assignPublicIPv6 = true
		result.hasIPv6AddressCount = true
		if result.ipv6AddressCount == 0 {
			result.ipv6AddressCount = 1
		}
	}
	if result.assignPublicIPv6 && result.ipv6AddressCount == 0 {
		result.hasIPv6AddressCount = true
		result.ipv6AddressCount = 1
	}
	if !hasAssignPublicIPv6 && networkMode == "" && result.hasIPv6AddressCount && result.ipv6AddressCount > 0 {
		result.assignPublicIPv6 = true
	}

	return result, nil
}

func parseNetworkModeProviderConfig(config map[string]any) (string, error) {
	value, ok := lookupProviderConfigValue(config, providerConfigNetworkMode)
	if !ok {
		return "", nil
	}

	normalized := strings.ToLower(strings.TrimSpace(asString(value)))
	comparable := strings.NewReplacer(" ", "", "-", "", "_", "").Replace(normalized)
	switch comparable {
	case "ipv4":
		return providerNetworkModeIPv4, nil
	case "ipv6":
		return providerNetworkModeIPv6, nil
	case "ipv4+ipv6", "ipv6+ipv4", "dual", "dualstack":
		return providerNetworkModeDualStack, nil
	default:
		return "", fmt.Errorf("provider_config.%s must be one of ipv4, ipv6, or ipv4+ipv6", providerConfigNetworkMode)
	}
}

func parseStringProviderConfig(config map[string]any, target string) string {
	value, ok := lookupProviderConfigValue(config, target)
	if !ok {
		return ""
	}
	return strings.TrimSpace(asString(value))
}

func parseStringListProviderConfig(config map[string]any, target string) ([]string, error) {
	value, ok := lookupProviderConfigValue(config, target)
	if !ok {
		return nil, nil
	}

	switch typed := value.(type) {
	case []string:
		result := make([]string, 0, len(typed))
		for _, item := range typed {
			if trimmed := strings.TrimSpace(item); trimmed != "" {
				result = append(result, trimmed)
			}
		}
		if len(result) == 0 {
			return nil, nil
		}
		return result, nil
	case []any:
		result := make([]string, 0, len(typed))
		for _, item := range typed {
			text := strings.TrimSpace(asString(item))
			if text == "" {
				return nil, fmt.Errorf("provider_config.%s entries must be strings", target)
			}
			result = append(result, text)
		}
		if len(result) == 0 {
			return nil, nil
		}
		return result, nil
	case string:
		text := strings.TrimSpace(typed)
		if text == "" {
			return nil, nil
		}
		return []string{text}, nil
	default:
		return nil, fmt.Errorf("provider_config.%s must be a list of strings", target)
	}
}

func parseBoolProviderConfig(config map[string]any, key string) (bool, bool, error) {
	value, ok := lookupProviderConfigValue(config, key)
	if !ok {
		return false, false, nil
	}

	switch typed := value.(type) {
	case bool:
		return typed, true, nil
	case string:
		switch normalizeToken(typed) {
		case "1", "true", "yes", "on":
			return true, true, nil
		case "0", "false", "no", "off":
			return false, true, nil
		default:
			return false, true, fmt.Errorf("provider_config.%s must be a boolean, got %q", key, typed)
		}
	case float64:
		if typed == 1 {
			return true, true, nil
		}
		if typed == 0 {
			return false, true, nil
		}
	}

	return false, true, fmt.Errorf("provider_config.%s must be a boolean", key)
}

func parsePositiveInt32ProviderConfig(config map[string]any, key string) (int32, bool, error) {
	value, ok := lookupProviderConfigValue(config, key)
	if !ok {
		return 0, false, nil
	}

	var parsed int
	switch typed := value.(type) {
	case float64:
		parsed = int(typed)
		if float64(parsed) != typed {
			return 0, true, fmt.Errorf("provider_config.%s must be an integer, got %v", key, typed)
		}
	case int:
		parsed = typed
	case int32:
		parsed = int(typed)
	case int64:
		parsed = int(typed)
	case string:
		var err error
		parsed, err = strconv.Atoi(normalizeToken(typed))
		if err != nil {
			return 0, true, fmt.Errorf("provider_config.%s must be an integer, got %q", key, typed)
		}
	default:
		return 0, true, fmt.Errorf("provider_config.%s must be an integer", key)
	}

	if parsed <= 0 {
		return 0, true, fmt.Errorf("provider_config.%s must be greater than zero, got %d", key, parsed)
	}

	return int32(parsed), true, nil
}

func parseNonNegativeInt32ProviderConfig(config map[string]any, key string) (int32, bool, error) {
	value, ok := lookupProviderConfigValue(config, key)
	if !ok {
		return 0, false, nil
	}

	var parsed int
	switch typed := value.(type) {
	case float64:
		parsed = int(typed)
		if float64(parsed) != typed {
			return 0, true, fmt.Errorf("provider_config.%s must be an integer, got %v", key, typed)
		}
	case int:
		parsed = typed
	case int32:
		parsed = int(typed)
	case int64:
		parsed = int(typed)
	case string:
		var err error
		parsed, err = strconv.Atoi(normalizeToken(typed))
		if err != nil {
			return 0, true, fmt.Errorf("provider_config.%s must be an integer, got %q", key, typed)
		}
	default:
		return 0, true, fmt.Errorf("provider_config.%s must be an integer", key)
	}

	if parsed < 0 {
		return 0, true, fmt.Errorf("provider_config.%s must be zero or greater, got %d", key, parsed)
	}

	return int32(parsed), true, nil
}

func lookupProviderConfigValue(config map[string]any, target string) (any, bool) {
	target = normalizeToken(target)
	for key, value := range config {
		if normalizeToken(key) == target {
			return value, true
		}
	}
	return nil, false
}

func asString(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	default:
		return fmt.Sprint(value)
	}
}
