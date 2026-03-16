package grpcserver

import (
	"context"
	"testing"

	"google.golang.org/grpc/metadata"
)

func TestAuthorize(t *testing.T) {
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs(
		authorizationMetadataKey, bearerPrefix+"secret",
	))

	if err := authorize(ctx, "secret"); err != nil {
		t.Fatalf("authorize returned error: %v", err)
	}
}

func TestAuthorizeRejectsMissingToken(t *testing.T) {
	if err := authorize(context.Background(), "secret"); err == nil {
		t.Fatal("expected authorize to fail")
	}
}
