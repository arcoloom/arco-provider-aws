package aws

import (
	"context"
	"testing"

	"github.com/arcoloom/arco-provider-aws/internal/provider"
)

func TestMetadata(t *testing.T) {
	service := NewService("test")

	metadata, err := service.Metadata(context.Background())
	if err != nil {
		t.Fatalf("metadata returned error: %v", err)
	}

	if metadata.Cloud != string(provider.CloudAWS) {
		t.Fatalf("unexpected cloud: %s", metadata.Cloud)
	}

	if metadata.Name != "arco-provider-aws" {
		t.Fatalf("unexpected provider name: %s", metadata.Name)
	}
	if metadata.Capabilities["schema_mode"] != "provider-defined" {
		t.Fatalf("unexpected schema mode capability: %+v", metadata.Capabilities)
	}

	authMethods := make(map[string]bool, len(metadata.AuthMethods))
	for _, method := range metadata.AuthMethods {
		authMethods[method.Name] = true
	}
	for _, requiredMethod := range []string{
		provider.AuthMethodAWSDefaultCredentials,
		provider.AuthMethodAWSStaticAccessKey,
		provider.AuthMethodAWSAssumeRole,
	} {
		if !authMethods[requiredMethod] {
			t.Fatalf("expected auth method %q in metadata, got %+v", requiredMethod, metadata.AuthMethods)
		}
	}
}

func TestSchema(t *testing.T) {
	service := NewService("test")

	resources, err := service.Schema(context.Background())
	if err != nil {
		t.Fatalf("schema returned error: %v", err)
	}
	if len(resources) != 1 {
		t.Fatalf("unexpected resource schema count: %+v", resources)
	}
	if resources[0].Type != "compute_instance" {
		t.Fatalf("unexpected resource schema: %+v", resources[0])
	}

	foundAMI := false
	foundOS := false
	foundNetworkMode := false
	foundSecurityGroups := false
	foundRootVolumeSize := false
	for _, attribute := range resources[0].Attributes {
		switch attribute.Name {
		case "ami":
			foundAMI = true
			if attribute.Type != provider.SchemaAttributeTypeString || !attribute.Optional {
				t.Fatalf("unexpected ami attribute: %+v", attribute)
			}
		case "security_group_ids":
			foundSecurityGroups = true
			if attribute.Type != provider.SchemaAttributeTypeStringList {
				t.Fatalf("unexpected security_group_ids attribute: %+v", attribute)
			}
		case "os":
			foundOS = true
			if attribute.Type != provider.SchemaAttributeTypeString || !attribute.Optional || attribute.DefaultValue != providerOSDebian13 {
				t.Fatalf("unexpected os attribute: %+v", attribute)
			}
		case "network_mode":
			foundNetworkMode = true
			if attribute.Type != provider.SchemaAttributeTypeString || !attribute.Optional {
				t.Fatalf("unexpected network_mode attribute: %+v", attribute)
			}
		case "root_volume_size_gib":
			foundRootVolumeSize = true
			if attribute.Type != provider.SchemaAttributeTypeInt64 || !attribute.Optional || attribute.DefaultValue != int64(defaultRootVolumeSizeGiB) {
				t.Fatalf("unexpected root_volume_size_gib attribute: %+v", attribute)
			}
		}
	}
	if !foundAMI || !foundOS || !foundNetworkMode || !foundSecurityGroups || !foundRootVolumeSize {
		t.Fatalf("expected aws launch attributes in schema, got %+v", resources[0].Attributes)
	}
}
