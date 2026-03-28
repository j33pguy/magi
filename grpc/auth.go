package grpc

import (
	"context"
	"crypto/subtle"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// AuthInterceptor returns a unary server interceptor that checks for a bearer
// token in the "authorization" gRPC metadata. The Health RPC is exempt.
// If token is empty, all requests are allowed (dev mode).
func AuthInterceptor(token string) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if token == "" {
			return handler(ctx, req)
		}

		// Health check is always unauthenticated.
		if info.FullMethod == "/memory.v1.MemoryService/Health" {
			return handler(ctx, req)
		}

		md, ok := metadata.FromIncomingContext(ctx)
		if !ok {
			return nil, status.Error(codes.Unauthenticated, "missing metadata")
		}

		values := md.Get("authorization")
		if len(values) == 0 {
			return nil, status.Error(codes.Unauthenticated, "missing authorization header")
		}

		provided := values[0]
		const prefix = "Bearer "
		if len(provided) < len(prefix) || provided[:len(prefix)] != prefix {
			return nil, status.Error(codes.Unauthenticated, "invalid authorization format")
		}

		if subtle.ConstantTimeCompare([]byte(provided[len(prefix):]), []byte(token)) != 1 {
			return nil, status.Error(codes.Unauthenticated, "invalid token")
		}

		return handler(ctx, req)
	}
}
