package aws

import "testing"

func TestParseStartInstanceProviderConfigSupportsNetworkMode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                  string
		input                 any
		wantMode              string
		wantAssociatePublicV4 bool
		wantHasAssociateV4    bool
		wantAssignIPv6        bool
		wantIPv6Count         int32
	}{
		{
			name:                  "ipv4 only",
			input:                 "ipv4",
			wantMode:              providerNetworkModeIPv4,
			wantAssociatePublicV4: true,
			wantHasAssociateV4:    true,
			wantAssignIPv6:        false,
			wantIPv6Count:         0,
		},
		{
			name:                  "ipv6 only",
			input:                 "ipv6",
			wantMode:              providerNetworkModeIPv6,
			wantAssociatePublicV4: false,
			wantHasAssociateV4:    true,
			wantAssignIPv6:        true,
			wantIPv6Count:         1,
		},
		{
			name:                  "dual stack shorthand",
			input:                 "dual-stack",
			wantMode:              providerNetworkModeDualStack,
			wantAssociatePublicV4: true,
			wantHasAssociateV4:    true,
			wantAssignIPv6:        true,
			wantIPv6Count:         1,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg, err := parseStartInstanceProviderConfig(map[string]any{
				providerConfigNetworkMode: tt.input,
			})
			if err != nil {
				t.Fatalf("parseStartInstanceProviderConfig() error = %v", err)
			}
			if cfg.NetworkMode != tt.wantMode {
				t.Fatalf("cfg.NetworkMode = %q, want %q", cfg.NetworkMode, tt.wantMode)
			}
			if cfg.LaunchOptions.hasAssociatePublicIPv4 != tt.wantHasAssociateV4 {
				t.Fatalf("cfg.LaunchOptions.hasAssociatePublicIPv4 = %v, want %v", cfg.LaunchOptions.hasAssociatePublicIPv4, tt.wantHasAssociateV4)
			}
			if cfg.LaunchOptions.associatePublicIPv4 != tt.wantAssociatePublicV4 {
				t.Fatalf("cfg.LaunchOptions.associatePublicIPv4 = %v, want %v", cfg.LaunchOptions.associatePublicIPv4, tt.wantAssociatePublicV4)
			}
			if cfg.LaunchOptions.assignPublicIPv6 != tt.wantAssignIPv6 {
				t.Fatalf("cfg.LaunchOptions.assignPublicIPv6 = %v, want %v", cfg.LaunchOptions.assignPublicIPv6, tt.wantAssignIPv6)
			}
			if cfg.LaunchOptions.ipv6AddressCount != tt.wantIPv6Count {
				t.Fatalf("cfg.LaunchOptions.ipv6AddressCount = %d, want %d", cfg.LaunchOptions.ipv6AddressCount, tt.wantIPv6Count)
			}
		})
	}
}

func TestParseStartInstanceProviderConfigRejectsInvalidNetworkMode(t *testing.T) {
	t.Parallel()

	_, err := parseStartInstanceProviderConfig(map[string]any{
		providerConfigNetworkMode: "ipv10",
	})
	if err == nil {
		t.Fatal("expected validation error for invalid network_mode")
	}
}

func TestParseStartInstanceProviderConfigDefaultsRootVolumeSize(t *testing.T) {
	t.Parallel()

	cfg, err := parseStartInstanceProviderConfig(map[string]any{})
	if err != nil {
		t.Fatalf("parseStartInstanceProviderConfig() error = %v", err)
	}
	if cfg.LaunchOptions.rootVolumeSizeGiB != defaultRootVolumeSizeGiB {
		t.Fatalf("cfg.LaunchOptions.rootVolumeSizeGiB = %d, want %d", cfg.LaunchOptions.rootVolumeSizeGiB, defaultRootVolumeSizeGiB)
	}
}

func TestParseStartInstanceProviderConfigDefaultsOS(t *testing.T) {
	t.Parallel()

	cfg, err := parseStartInstanceProviderConfig(map[string]any{})
	if err != nil {
		t.Fatalf("parseStartInstanceProviderConfig() error = %v", err)
	}
	if cfg.OS != providerOSDebian13 {
		t.Fatalf("cfg.OS = %q, want %q", cfg.OS, providerOSDebian13)
	}
}

func TestParseStartInstanceProviderConfigSupportsOSAliases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input any
		want  string
	}{
		{input: "debian13", want: providerOSDebian13},
		{input: "ubuntu-24.04-lts", want: providerOSUbuntu2404LTS},
	}

	for _, tt := range tests {
		cfg, err := parseStartInstanceProviderConfig(map[string]any{
			providerConfigOS: tt.input,
		})
		if err != nil {
			t.Fatalf("parseStartInstanceProviderConfig(%v) error = %v", tt.input, err)
		}
		if cfg.OS != tt.want {
			t.Fatalf("cfg.OS = %q, want %q", cfg.OS, tt.want)
		}
	}
}
