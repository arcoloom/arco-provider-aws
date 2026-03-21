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

	if metadata.Cloud != provider.CloudAWS {
		t.Fatalf("unexpected cloud: %s", metadata.Cloud)
	}

	if metadata.Name != "arco-provider-aws" {
		t.Fatalf("unexpected provider name: %s", metadata.Name)
	}
	if metadata.Capabilities["schema_mode"] != "provider-defined" {
		t.Fatalf("unexpected schema mode capability: %+v", metadata.Capabilities)
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
	foundSecurityGroups := false
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
		}
	}
	if !foundAMI || !foundSecurityGroups {
		t.Fatalf("expected aws launch attributes in schema, got %+v", resources[0].Attributes)
	}
}
