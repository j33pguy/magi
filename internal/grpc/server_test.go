package grpc_test

import (
	"context"
	"log/slog"
	"net"
	"os"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	memgrpc "github.com/j33pguy/magi/internal/grpc"
	pb "github.com/j33pguy/magi/proto/memory/v1"
)

// recoveryInterceptor catches panics from nil deps in test and returns Internal.
func recoveryInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp any, err error) {
		defer func() {
			if r := recover(); r != nil {
				err = status.Errorf(codes.Internal, "panic: %v", r)
			}
		}()
		return handler(ctx, req)
	}
}

const bufSize = 1024 * 1024

func newTestServer(t *testing.T, token string) (pb.MemoryServiceClient, func()) {
	t.Helper()

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	// nil db and embedder — only usable for Health and validation tests
	svc := memgrpc.NewServer(nil, nil, logger)

	lis := bufconn.Listen(bufSize)
	s := grpc.NewServer(grpc.ChainUnaryInterceptor(
		memgrpc.AuthInterceptor(token),
		recoveryInterceptor(),
	))
	pb.RegisterMemoryServiceServer(s, svc)

	go func() {
		if err := s.Serve(lis); err != nil {
			t.Logf("gRPC test server error: %v", err)
		}
	}()

	conn, err := grpc.NewClient("passthrough:///bufconn",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return lis.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("connecting to bufconn: %v", err)
	}

	client := pb.NewMemoryServiceClient(conn)
	cleanup := func() {
		conn.Close()
		s.GracefulStop()
		lis.Close()
	}
	return client, cleanup
}

func TestHealth(t *testing.T) {
	client, cleanup := newTestServer(t, "")
	defer cleanup()

	resp, err := client.Health(context.Background(), &pb.HealthRequest{})
	if err != nil {
		t.Fatalf("Health RPC failed: %v", err)
	}
	if !resp.Ok {
		t.Error("expected ok=true")
	}
	if resp.Version != "0.3.0" {
		t.Errorf("expected version 0.3.0, got %s", resp.Version)
	}
}

func TestRememberValidation(t *testing.T) {
	client, cleanup := newTestServer(t, "")
	defer cleanup()

	_, err := client.Remember(context.Background(), &pb.RememberRequest{Content: ""})
	if err == nil {
		t.Fatal("expected error for empty content")
	}
	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected gRPC status error, got %v", err)
	}
	if st.Code() != codes.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %s", st.Code())
	}
}

func TestRecallValidation(t *testing.T) {
	client, cleanup := newTestServer(t, "")
	defer cleanup()

	_, err := client.Recall(context.Background(), &pb.RecallRequest{Query: ""})
	if err == nil {
		t.Fatal("expected error for empty query")
	}
	st, _ := status.FromError(err)
	if st.Code() != codes.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %s", st.Code())
	}
}

func TestForgetValidation(t *testing.T) {
	client, cleanup := newTestServer(t, "")
	defer cleanup()

	_, err := client.Forget(context.Background(), &pb.ForgetRequest{Id: ""})
	if err == nil {
		t.Fatal("expected error for empty id")
	}
	st, _ := status.FromError(err)
	if st.Code() != codes.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %s", st.Code())
	}
}

func TestAuthInterceptor_NoToken(t *testing.T) {
	// No token configured = dev mode, all requests pass
	client, cleanup := newTestServer(t, "")
	defer cleanup()

	resp, err := client.Health(context.Background(), &pb.HealthRequest{})
	if err != nil {
		t.Fatalf("Health should succeed without token: %v", err)
	}
	if !resp.Ok {
		t.Error("expected ok=true")
	}
}

func TestAuthInterceptor_ValidToken(t *testing.T) {
	client, cleanup := newTestServer(t, "test-secret")
	defer cleanup()

	// Health should work without auth
	resp, err := client.Health(context.Background(), &pb.HealthRequest{})
	if err != nil {
		t.Fatalf("Health should succeed without auth: %v", err)
	}
	if !resp.Ok {
		t.Error("expected ok=true from health")
	}

	// Remember should fail without auth
	_, err = client.Remember(context.Background(), &pb.RememberRequest{Content: "test"})
	if err == nil {
		t.Fatal("expected auth error")
	}
	st, _ := status.FromError(err)
	if st.Code() != codes.Unauthenticated {
		t.Errorf("expected Unauthenticated, got %s", st.Code())
	}

	// Remember with valid token should pass auth (will fail on nil embedder, but not on auth)
	ctx := metadata.AppendToOutgoingContext(context.Background(), "authorization", "Bearer test-secret")
	_, err = client.Remember(ctx, &pb.RememberRequest{Content: "test"})
	if err == nil {
		// With nil embedder this would panic or fail differently
		t.Log("remember succeeded (unexpected with nil deps)")
	} else {
		st, _ = status.FromError(err)
		if st.Code() == codes.Unauthenticated {
			t.Error("should have passed auth with valid token")
		}
		// Internal error from nil embedder is expected
	}
}

func TestAuthInterceptor_InvalidToken(t *testing.T) {
	client, cleanup := newTestServer(t, "correct-token")
	defer cleanup()

	ctx := metadata.AppendToOutgoingContext(context.Background(), "authorization", "Bearer wrong-token")
	_, err := client.Remember(ctx, &pb.RememberRequest{Content: "test"})
	if err == nil {
		t.Fatal("expected auth error with wrong token")
	}
	st, _ := status.FromError(err)
	if st.Code() != codes.Unauthenticated {
		t.Errorf("expected Unauthenticated, got %s", st.Code())
	}
}

func TestCreateConversationValidation(t *testing.T) {
	client, cleanup := newTestServer(t, "")
	defer cleanup()

	// Missing summary
	_, err := client.CreateConversation(context.Background(), &pb.CreateConversationRequest{Channel: "test"})
	if err == nil {
		t.Fatal("expected error for empty summary")
	}
	st, _ := status.FromError(err)
	if st.Code() != codes.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %s", st.Code())
	}

	// Missing channel
	_, err = client.CreateConversation(context.Background(), &pb.CreateConversationRequest{Summary: "test"})
	if err == nil {
		t.Fatal("expected error for empty channel")
	}
	st, _ = status.FromError(err)
	if st.Code() != codes.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %s", st.Code())
	}
}

func TestSearchConversationsValidation(t *testing.T) {
	client, cleanup := newTestServer(t, "")
	defer cleanup()

	_, err := client.SearchConversations(context.Background(), &pb.SearchConversationsRequest{Query: ""})
	if err == nil {
		t.Fatal("expected error for empty query")
	}
	st, _ := status.FromError(err)
	if st.Code() != codes.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %s", st.Code())
	}
}
