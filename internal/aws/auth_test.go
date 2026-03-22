package aws

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/arcoloom/arco-provider-aws/internal/provider"
)

func TestAWSClientFactoryNewConfigUsesDefaultCredentialChain(t *testing.T) {
	t.Setenv("AWS_ACCESS_KEY_ID", "env-access-key")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "env-secret-key")
	t.Setenv("AWS_SESSION_TOKEN", "env-session-token")
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
	t.Setenv("AWS_PROFILE", "")

	cfg, err := newAWSClientFactory().NewConfig(context.Background(), provider.AWSCredentials{
		UseDefaultCredentialsChain: true,
	}, "us-east-1", "")
	if err != nil {
		t.Fatalf("NewConfig returned error: %v", err)
	}

	creds, err := cfg.Credentials.Retrieve(context.Background())
	if err != nil {
		t.Fatalf("retrieve credentials: %v", err)
	}
	if creds.AccessKeyID != "env-access-key" {
		t.Fatalf("unexpected access key id: %q", creds.AccessKeyID)
	}
	if creds.SecretAccessKey != "env-secret-key" {
		t.Fatalf("unexpected secret access key: %q", creds.SecretAccessKey)
	}
	if creds.SessionToken != "env-session-token" {
		t.Fatalf("unexpected session token: %q", creds.SessionToken)
	}
}

func TestAWSClientFactoryNewConfigUsesStaticCredentialsForAssumeRole(t *testing.T) {
	server, requests := newFakeSTSAssumeRoleServer(t)
	defer server.Close()

	cfg, err := newAWSClientFactory().NewConfig(context.Background(), provider.AWSCredentials{
		AccessKeyID:     "static-access-key",
		SecretAccessKey: "static-secret-key",
		SessionToken:    "static-session-token",
		RoleARN:         "arn:aws:iam::123456789012:role/arco-test",
		ExternalID:      "external-123",
		RoleSessionName: "arco-session-01",
		SourceIdentity:  "scheduler-01",
	}, "us-east-1", server.URL)
	if err != nil {
		t.Fatalf("NewConfig returned error: %v", err)
	}

	creds, err := cfg.Credentials.Retrieve(context.Background())
	if err != nil {
		t.Fatalf("retrieve credentials: %v", err)
	}
	if creds.AccessKeyID != "assumed-access-key" {
		t.Fatalf("unexpected assumed access key: %q", creds.AccessKeyID)
	}

	values := <-requests
	if values.Get("Action") != "AssumeRole" {
		t.Fatalf("unexpected STS action: %q", values.Get("Action"))
	}
	if values.Get("RoleArn") != "arn:aws:iam::123456789012:role/arco-test" {
		t.Fatalf("unexpected role arn: %q", values.Get("RoleArn"))
	}
	if values.Get("RoleSessionName") != "arco-session-01" {
		t.Fatalf("unexpected role session name: %q", values.Get("RoleSessionName"))
	}
	if values.Get("ExternalId") != "external-123" {
		t.Fatalf("unexpected external id: %q", values.Get("ExternalId"))
	}
	if values.Get("SourceIdentity") != "scheduler-01" {
		t.Fatalf("unexpected source identity: %q", values.Get("SourceIdentity"))
	}
}

func TestAWSClientFactoryNewConfigUsesDefaultCredentialChainForAssumeRole(t *testing.T) {
	t.Setenv("AWS_ACCESS_KEY_ID", "env-access-key")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "env-secret-key")
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
	t.Setenv("AWS_PROFILE", "")

	server, requests := newFakeSTSAssumeRoleServer(t)
	defer server.Close()

	cfg, err := newAWSClientFactory().NewConfig(context.Background(), provider.AWSCredentials{
		UseDefaultCredentialsChain: true,
		RoleARN:                    "arn:aws:iam::123456789012:role/arco-test",
		SourceIdentity:             "planner-01",
	}, "us-east-1", server.URL)
	if err != nil {
		t.Fatalf("NewConfig returned error: %v", err)
	}

	creds, err := cfg.Credentials.Retrieve(context.Background())
	if err != nil {
		t.Fatalf("retrieve credentials: %v", err)
	}
	if creds.AccessKeyID != "assumed-access-key" {
		t.Fatalf("unexpected assumed access key: %q", creds.AccessKeyID)
	}

	values := <-requests
	if values.Get("Action") != "AssumeRole" {
		t.Fatalf("unexpected STS action: %q", values.Get("Action"))
	}
	if values.Get("RoleSessionName") == "" {
		t.Fatal("expected generated role session name")
	}
	if values.Get("RoleSessionName") == defaultAssumeRoleSessionPrefix {
		t.Fatalf("expected generated role session name, got bare prefix %q", values.Get("RoleSessionName"))
	}
	if values.Get("SourceIdentity") != "planner-01" {
		t.Fatalf("unexpected source identity: %q", values.Get("SourceIdentity"))
	}
}

func TestAWSClientFactoryNewConfigRejectsInvalidAuthConfiguration(t *testing.T) {
	tests := []struct {
		name        string
		credentials provider.AWSCredentials
	}{
		{
			name: "default chain with explicit keys",
			credentials: provider.AWSCredentials{
				UseDefaultCredentialsChain: true,
				AccessKeyID:                "ak",
				SecretAccessKey:            "sk",
			},
		},
		{
			name: "static credentials missing secret",
			credentials: provider.AWSCredentials{
				AccessKeyID: "ak",
			},
		},
		{
			name: "source identity without role",
			credentials: provider.AWSCredentials{
				UseDefaultCredentialsChain: true,
				SourceIdentity:             "planner-01",
			},
		},
		{
			name: "profile with static credentials",
			credentials: provider.AWSCredentials{
				Profile:         "dev",
				AccessKeyID:     "ak",
				SecretAccessKey: "sk",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := newAWSClientFactory().NewConfig(context.Background(), tc.credentials, "us-east-1", "")
			if err == nil {
				t.Fatal("expected NewConfig to fail")
			}
		})
	}
}

func newFakeSTSAssumeRoleServer(t *testing.T) (*httptest.Server, <-chan url.Values) {
	t.Helper()

	requests := make(chan url.Values, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse STS form: %v", err)
		}

		select {
		case requests <- r.Form:
		default:
		}

		if r.Form.Get("Action") != "AssumeRole" {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = fmt.Fprint(w, "unexpected action")
			return
		}

		w.Header().Set("Content-Type", "text/xml")
		_, _ = fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?>
<AssumeRoleResponse xmlns="https://sts.amazonaws.com/doc/2011-06-15/">
  <AssumeRoleResult>
    <Credentials>
      <AccessKeyId>assumed-access-key</AccessKeyId>
      <SecretAccessKey>assumed-secret-key</SecretAccessKey>
      <SessionToken>assumed-session-token</SessionToken>
      <Expiration>%s</Expiration>
    </Credentials>
    <AssumedRoleUser>
      <Arn>arn:aws:sts::123456789012:assumed-role/arco-test/session</Arn>
      <AssumedRoleId>AROATEST:session</AssumedRoleId>
    </AssumedRoleUser>
  </AssumeRoleResult>
  <ResponseMetadata>
    <RequestId>req-assume-role-001</RequestId>
  </ResponseMetadata>
</AssumeRoleResponse>`, time.Now().UTC().Add(15*time.Minute).Format(time.RFC3339))
	}))

	return server, requests
}
