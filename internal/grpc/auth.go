package grpc

import (
	"context"

	"github.com/j33pguy/magi/internal/auth"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// Read-only gRPC methods allowed without a token.
var grpcReadOnlyMethods = map[string]bool{
	"/memory.v1.MemoryService/Health":              true,
	"/memory.v1.MemoryService/Recall":              true,
	"/memory.v1.MemoryService/List":                true,
	"/memory.v1.MemoryService/SearchConversations": true,
	"/memory.v1.MemoryService/GetRelated":          true,
}

// AuthInterceptor returns a unary server interceptor that checks for a bearer
// token in the "authorization" gRPC metadata. The Health RPC is exempt.
// If no auth is configured, only read-only RPCs are allowed.
func AuthInterceptor(resolver *auth.Resolver) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if resolver == nil || !resolver.Enabled() {
			if grpcReadOnlyMethods[info.FullMethod] {
				return handler(ctx, req)
			}
			return nil, status.Error(codes.PermissionDenied, "write operations require MAGI_API_TOKEN to be set")
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

		identity, ok := resolver.ResolveBearer(provided[len(prefix):])
		if !ok {
			return nil, status.Error(codes.Unauthenticated, "invalid token")
		}

		return handler(auth.NewContext(ctx, identity), req)
	}
}
