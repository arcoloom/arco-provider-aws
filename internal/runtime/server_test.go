package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/arcoloom/arco-provider-aws/internal/aws"
)

func TestWriteStartupInfo(t *testing.T) {
	info := StartupInfo{
		Protocol: "grpc",
		Address:  "127.0.0.1:34567",
		Port:     34567,
		Token:    "token",
		PID:      42,
	}

	var buffer bytes.Buffer
	if err := writeStartupInfo(&buffer, info); err != nil {
		t.Fatalf("writeStartupInfo returned error: %v", err)
	}

	var decoded StartupInfo
	if err := json.Unmarshal(buffer.Bytes(), &decoded); err != nil {
		t.Fatalf("json.Unmarshal returned error: %v", err)
	}

	if decoded.Port != info.Port || decoded.Token != info.Token || decoded.Address != info.Address {
		t.Fatalf("unexpected startup info: %+v", decoded)
	}
}

func TestNewServerGeneratesToken(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	server, err := NewServer(logger, aws.NewService("test"))
	if err != nil {
		t.Fatalf("NewServer returned error: %v", err)
	}

	if server.token == "" {
		t.Fatal("expected token to be generated")
	}

	if _, err := server.service.Metadata(context.Background()); err != nil {
		t.Fatalf("service metadata returned error: %v", err)
	}
}
