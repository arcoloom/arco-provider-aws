package grpcserver

import (
	"context"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const (
	authorizationMetadataKey = "authorization"
	bearerPrefix             = "Bearer "
)

func UnaryServerAuthInterceptor(expectedToken string) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req any,
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (any, error) {
		if err := authorize(ctx, expectedToken); err != nil {
			return nil, status.Errorf(codes.Unauthenticated, "%s: %s", info.FullMethod, err)
		}

		return handler(ctx, req)
	}
}

func authorize(ctx context.Context, expectedToken string) error {
	metadataValues, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return status.Error(codes.Unauthenticated, "missing metadata")
	}

	values := metadataValues.Get(authorizationMetadataKey)
	if len(values) == 0 {
		return status.Error(codes.Unauthenticated, "missing authorization token")
	}

	token := strings.TrimPrefix(values[0], bearerPrefix)
	if token == values[0] || token != expectedToken {
		return status.Error(codes.Unauthenticated, "invalid authorization token")
	}

	return nil
}
