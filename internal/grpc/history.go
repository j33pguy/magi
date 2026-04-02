package grpc

import (
	"path/filepath"

	"github.com/j33pguy/magi/internal/secretstore"
	"github.com/j33pguy/magi/internal/vcs"
)

// SetGitRepo enables git versioning endpoints on the gRPC server.
func (s *Server) SetGitRepo(repo *vcs.Repo) {
	s.gitRepo = repo
}

// SetSecretManager enables secret externalization for remember writes.
func (s *Server) SetSecretManager(manager secretstore.Manager) {
	s.secretManager = manager
}

// MemoryHistory returns the git log for a memory. Returns nil if git is disabled.
func (s *Server) MemoryHistory(id string) ([]vcs.CommitInfo, error) {
	if s.gitRepo == nil {
		return nil, nil
	}
	relPath := filepath.Join("memories", id+".json")
	return s.gitRepo.Log(relPath)
}

// MemoryDiff returns the diff between two commits for a memory. Returns nil if git is disabled.
func (s *Server) MemoryDiff(id, fromHash, toHash string) (*vcs.FileDiff, error) {
	if s.gitRepo == nil {
		return nil, nil
	}
	relPath := filepath.Join("memories", id+".json")
	return s.gitRepo.Diff(relPath, fromHash, toHash)
}
