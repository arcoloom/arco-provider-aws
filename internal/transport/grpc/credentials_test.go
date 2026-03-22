package grpcserver

import (
	"testing"

	providerv1 "github.com/arcoloom/arco-proto/gen/go/arcoloom/provider/v1"
	"github.com/arcoloom/arco-provider-aws/internal/provider"
	"google.golang.org/protobuf/types/known/structpb"
)

func TestToDomainCredentialsDefaultCredentialChain(t *testing.T) {
	data, err := structpb.NewStruct(map[string]any{
		"profile":           "dev",
		"role_arn":          "arn:aws:iam::123456789012:role/arco-test",
		"external_id":       "external-123",
		"role_session_name": "arco-session-01",
		"source_identity":   "planner-01",
	})
	if err != nil {
		t.Fatalf("build struct: %v", err)
	}

	credentials := toDomainCredentials(&providerv1.Credentials{
		AuthMethod: provider.AuthMethodAWSDefaultCredentials,
		Data:       data,
	})

	if credentials.AWS == nil {
		t.Fatal("expected AWS credentials")
	}
	if !credentials.AWS.UseDefaultCredentialsChain {
		t.Fatal("expected default credential chain flag")
	}
	if credentials.AWS.Profile != "dev" {
		t.Fatalf("unexpected profile: %q", credentials.AWS.Profile)
	}
	if credentials.AWS.RoleSessionName != "arco-session-01" {
		t.Fatalf("unexpected role session name: %q", credentials.AWS.RoleSessionName)
	}
	if credentials.AWS.SourceIdentity != "planner-01" {
		t.Fatalf("unexpected source identity: %q", credentials.AWS.SourceIdentity)
	}
}

func TestToProtoCredentialsDefaultCredentialChain(t *testing.T) {
	credentials := toProtoCredentials(provider.Credentials{
		AWS: &provider.AWSCredentials{
			UseDefaultCredentialsChain: true,
			Profile:                    "dev",
			RoleARN:                    "arn:aws:iam::123456789012:role/arco-test",
			ExternalID:                 "external-123",
			RoleSessionName:            "arco-session-01",
			SourceIdentity:             "planner-01",
		},
	})

	if credentials.GetAuthMethod() != provider.AuthMethodAWSDefaultCredentials {
		t.Fatalf("unexpected auth method: %q", credentials.GetAuthMethod())
	}

	data := credentials.GetData().AsMap()
	if data["profile"] != "dev" {
		t.Fatalf("unexpected profile: %#v", data["profile"])
	}
	if _, ok := data["access_key_id"]; ok {
		t.Fatalf("did not expect access key id in default credential chain payload: %#v", data)
	}
	if data["role_session_name"] != "arco-session-01" {
		t.Fatalf("unexpected role session name: %#v", data["role_session_name"])
	}
	if data["source_identity"] != "planner-01" {
		t.Fatalf("unexpected source identity: %#v", data["source_identity"])
	}
}
