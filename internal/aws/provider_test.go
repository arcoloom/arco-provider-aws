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
}
