package grpc

import (
	"context"
	"errors"
	"testing"

	"google.golang.org/grpc/codes"

	"github.com/j33pguy/magi/internal/db"
	pb "github.com/j33pguy/magi/proto/memory/v1"
)

// archiveFailStore wraps a real db.Store but forces ArchiveMemory to return an
// error.  Every other method delegates to the embedded Store.
type archiveFailStore struct {
	db.Store
}

func (s *archiveFailStore) ArchiveMemory(_ string) error {
	return errors.New("disk on fire")
}

// TestForget_ArchiveMemoryInternalError exercises the branch at server.go:170-173
// where GetMemory succeeds but ArchiveMemory returns an error, which should
// produce a codes.Internal gRPC status.
func TestForget_ArchiveMemoryInternalError(t *testing.T) {
	// Stand up a real server so we can create a memory.
	srv, _ := newTestGRPCServer(t)
	ctx := context.Background()

	// Create a memory that GetMemory can find.
	rem, err := srv.Remember(ctx, &pb.RememberRequest{Content: "archive-fail test"})
	if err != nil {
		t.Fatalf("Remember: %v", err)
	}

	// Swap the DB for one that fails on ArchiveMemory only.
	srv.db = &archiveFailStore{Store: srv.db}

	_, err = srv.Forget(ctx, &pb.ForgetRequest{Id: rem.Id})
	assertGRPCCode(t, err, codes.Internal)
}
