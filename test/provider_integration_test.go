package test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/arcoloom/arco-provider-aws/internal/provider"
	"github.com/arcoloom/arco-provider-aws/internal/runtime"
	grpcclient "github.com/arcoloom/arco-provider-aws/internal/transport/grpc"
)

func TestProviderProcessLifecycleAndBusinessCalls(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	fakeAWS := newFakeAWSQueryServer(t)
	t.Cleanup(fakeAWS.Close)

	binaryPath := buildProviderBinary(t)

	var stderr bytes.Buffer
	process, err := runtime.Launch(ctx, binaryPath, &stderr)
	if err != nil {
		t.Fatalf("launch provider process: %v\nstderr:\n%s", err, stderr.String())
	}
	t.Cleanup(func() {
		if err := process.Stop(); err != nil && !strings.Contains(err.Error(), "no child processes") {
			t.Fatalf("stop provider process: %v", err)
		}
	})

	startup := process.StartupInfo()
	client, err := grpcclient.Dial(startup.Address, startup.Token)
	if err != nil {
		t.Fatalf("dial provider: %v", err)
	}
	t.Cleanup(func() {
		if err := client.Close(); err != nil {
			t.Fatalf("close grpc client: %v", err)
		}
	})

	metadata, err := client.GetProviderInfo(ctx)
	if err != nil {
		t.Fatalf("get provider info: %v", err)
	}
	if metadata.Name != "arco-provider-aws" {
		t.Fatalf("unexpected provider name: %s", metadata.Name)
	}
	if metadata.Cloud != provider.CloudAWS {
		t.Fatalf("unexpected cloud: %s", metadata.Cloud)
	}

	validateResp, err := client.ValidateConnection(ctx, provider.ValidateConnectionRequest{
		Context: provider.RequestContext{
			RequestID: "req-validate-001",
			Caller:    "integration-test",
			TraceID:   "trace-validate-001",
		},
		Credentials: provider.Credentials{
			AWS: &provider.AWSCredentials{
				AccessKeyID:     "test-access-key",
				SecretAccessKey: "test-secret-key",
				SessionToken:    "test-session-token",
			},
		},
		Scope: provider.ConnectionScope{
			AccountID: "123456789012",
			Region:    "us-east-1",
			Endpoint:  fakeAWS.URL,
		},
		Options: map[string]string{
			"service": "spot",
		},
	})
	if err != nil {
		t.Fatalf("validate connection: %v", err)
	}
	if !validateResp.Accepted {
		t.Fatalf("expected validation success, got %+v", validateResp)
	}
	if !strings.Contains(validateResp.Message, "123456789012") || !strings.Contains(validateResp.Message, "us-east-1") {
		t.Fatalf("unexpected validation message: %s", validateResp.Message)
	}
	if len(validateResp.Warnings) != 0 {
		t.Fatalf("unexpected validation warnings: %+v", validateResp.Warnings)
	}

	pingResp, err := client.Ping(ctx, provider.RequestContext{
		RequestID: "req-ping-001",
		Caller:    "integration-test",
		TraceID:   "trace-ping-001",
	}, "hello-provider")
	if err != nil {
		t.Fatalf("ping provider: %v", err)
	}
	if pingResp.Payload != "pong:hello-provider" {
		t.Fatalf("unexpected ping payload: %s", pingResp.Payload)
	}
	if pingResp.Timestamp.IsZero() {
		t.Fatal("expected ping timestamp to be set")
	}
}

func newFakeAWSQueryServer(t *testing.T) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read fake aws request body: %v", err)
		}

		values, err := url.ParseQuery(string(body))
		if err != nil {
			t.Fatalf("parse fake aws request body: %v", err)
		}

		switch values.Get("Action") {
		case "GetCallerIdentity":
			w.Header().Set("Content-Type", "text/xml")
			fmt.Fprint(w, `<?xml version="1.0" encoding="UTF-8"?>
<GetCallerIdentityResponse xmlns="https://sts.amazonaws.com/doc/2011-06-15/">
  <GetCallerIdentityResult>
    <Arn>arn:aws:sts::123456789012:assumed-role/arcoloom/test</Arn>
    <UserId>AROATEST:test</UserId>
    <Account>123456789012</Account>
  </GetCallerIdentityResult>
  <ResponseMetadata>
    <RequestId>req-sts-001</RequestId>
  </ResponseMetadata>
</GetCallerIdentityResponse>`)
		case "DescribeAvailabilityZones":
			w.Header().Set("Content-Type", "text/xml")
			fmt.Fprint(w, `<?xml version="1.0" encoding="UTF-8"?>
<DescribeAvailabilityZonesResponse xmlns="http://ec2.amazonaws.com/doc/2016-11-15/">
  <requestId>req-ec2-001</requestId>
  <availabilityZoneInfo>
    <item>
      <zoneName>us-east-1a</zoneName>
      <zoneId>use1-az1</zoneId>
      <zoneState>available</zoneState>
      <regionName>us-east-1</regionName>
    </item>
  </availabilityZoneInfo>
</DescribeAvailabilityZonesResponse>`)
		default:
			http.Error(w, fmt.Sprintf("unsupported action %q", values.Get("Action")), http.StatusBadRequest)
		}
	}))
}

func buildProviderBinary(t *testing.T) string {
	t.Helper()

	workingDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}

	outputPath := t.TempDir() + "/arco-provider-aws-test-bin"
	cmd := exec.Command("go", "build", "-o", outputPath, "./cmd/arco-provider-aws")
	cmd.Dir = filepath.Dir(workingDir)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("build provider binary: %v\nstderr:\n%s", err, stderr.String())
	}

	return outputPath
}
