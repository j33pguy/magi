package vcs

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// Repo wraps a go-git repository with MAGI-specific operations.
type Repo struct {
	repo *git.Repository
	path string // root directory of the git repo
	mu   sync.Mutex

	// batch mode
	batchMode     bool
	batchInterval time.Duration
	pending       bool // whether there are uncommitted staged changes
	stopBatch     chan struct{}
	batchDone     chan struct{}
}

// CommitInfo represents a single commit in the history of a file.
type CommitInfo struct {
	Hash    string `json:"hash"`
	Message string `json:"message"`
	Date    string `json:"date"`
}

// FileDiff represents the diff of a file between two commits.
type FileDiff struct {
	From    string `json:"from"`
	To      string `json:"to"`
	Content string `json:"content"` // unified diff text
}

// Init opens or creates a git repository at the given path.
// Creates the directory structure and .magi-meta/version file if needed.
func Init(cfg *Config) (*Repo, error) {
	// Ensure directory structure
	for _, dir := range []string{"memories", "links", ".magi-meta"} {
		if err := os.MkdirAll(filepath.Join(cfg.Path, dir), 0o755); err != nil {
			return nil, fmt.Errorf("creating directory %s: %w", dir, err)
		}
	}

	// Write version file if it doesn't exist
	versionFile := filepath.Join(cfg.Path, ".magi-meta", "version")
	if _, err := os.Stat(versionFile); os.IsNotExist(err) {
		if err := os.WriteFile(versionFile, []byte("1\n"), 0o644); err != nil {
			return nil, fmt.Errorf("writing version file: %w", err)
		}
	}

	// Open or init repo
	repo, err := git.PlainOpen(cfg.Path)
	if err != nil {
		repo, err = git.PlainInit(cfg.Path, false)
		if err != nil {
			return nil, fmt.Errorf("initializing git repo: %w", err)
		}

		// Initial commit with directory structure
		wt, err := repo.Worktree()
		if err != nil {
			return nil, fmt.Errorf("getting worktree: %w", err)
		}
		if _, err := wt.Add(".magi-meta/version"); err != nil {
			return nil, fmt.Errorf("staging version file: %w", err)
		}
		if _, err := wt.Commit("init: magi memories repository", &git.CommitOptions{
			Author: magiSignature(),
		}); err != nil {
			return nil, fmt.Errorf("initial commit: %w", err)
		}
	}

	r := &Repo{
		repo:          repo,
		path:          cfg.Path,
		batchMode:     cfg.CommitMode == "batch",
		batchInterval: cfg.BatchInterval,
	}

	if r.batchMode {
		r.stopBatch = make(chan struct{})
		r.batchDone = make(chan struct{})
		go r.batchLoop()
	}

	return r, nil
}

// WriteAndCommit writes a file at relPath (relative to repo root) and commits it.
// In batch mode, the file is staged but committed later.
func (r *Repo) WriteAndCommit(relPath string, data []byte, message string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	absPath := filepath.Join(r.path, relPath)
	if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
		return fmt.Errorf("creating parent dirs: %w", err)
	}
	if err := os.WriteFile(absPath, data, 0o644); err != nil {
		return fmt.Errorf("writing file: %w", err)
	}

	wt, err := r.repo.Worktree()
	if err != nil {
		return fmt.Errorf("getting worktree: %w", err)
	}
	if _, err := wt.Add(relPath); err != nil {
		return fmt.Errorf("staging file: %w", err)
	}

	if r.batchMode {
		r.pending = true
		return nil
	}

	_, err = wt.Commit(message, &git.CommitOptions{
		Author: magiSignature(),
	})
	return err
}

// RemoveAndCommit removes a file at relPath and commits the deletion.
func (r *Repo) RemoveAndCommit(relPath string, message string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	absPath := filepath.Join(r.path, relPath)
	if _, err := os.Stat(absPath); os.IsNotExist(err) {
		return nil // nothing to remove
	}
	if err := os.Remove(absPath); err != nil {
		return fmt.Errorf("removing file: %w", err)
	}

	wt, err := r.repo.Worktree()
	if err != nil {
		return fmt.Errorf("getting worktree: %w", err)
	}
	if _, err := wt.Add(relPath); err != nil {
		return fmt.Errorf("staging removal: %w", err)
	}

	if r.batchMode {
		r.pending = true
		return nil
	}

	_, err = wt.Commit(message, &git.CommitOptions{
		Author: magiSignature(),
	})
	return err
}

// Log returns the commit history for a specific file path (relative to repo root).
func (r *Repo) Log(relPath string) ([]CommitInfo, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	iter, err := r.repo.Log(&git.LogOptions{
		FileName: &relPath,
		Order:    git.LogOrderCommitterTime,
	})
	if err != nil {
		return nil, fmt.Errorf("git log: %w", err)
	}
	defer iter.Close()

	var commits []CommitInfo
	err = iter.ForEach(func(c *object.Commit) error {
		commits = append(commits, CommitInfo{
			Hash:    c.Hash.String(),
			Message: strings.TrimSpace(c.Message),
			Date:    c.Author.When.UTC().Format(time.RFC3339),
		})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("iterating log: %w", err)
	}

	return commits, nil
}

// Diff returns the content of a file at two commits, suitable for diffing.
func (r *Repo) Diff(relPath, fromHash, toHash string) (*FileDiff, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	fromContent, err := r.fileAtCommit(relPath, fromHash)
	if err != nil {
		return nil, fmt.Errorf("reading file at %s: %w", fromHash, err)
	}

	toContent, err := r.fileAtCommit(relPath, toHash)
	if err != nil {
		return nil, fmt.Errorf("reading file at %s: %w", toHash, err)
	}

	return &FileDiff{
		From:    fromContent,
		To:      toContent,
		Content: simpleDiff(fromContent, toContent),
	}, nil
}

// Close stops the batch loop if running.
func (r *Repo) Close() {
	if r.batchMode && r.stopBatch != nil {
		close(r.stopBatch)
		<-r.batchDone
	}
}

// HasMemories checks if there are any .json files in the memories/ directory.
func (r *Repo) HasMemories() bool {
	entries, err := os.ReadDir(filepath.Join(r.path, "memories"))
	if err != nil {
		return false
	}
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") {
			return true
		}
	}
	return false
}

// MemoriesDir returns the absolute path to the memories directory.
func (r *Repo) MemoriesDir() string {
	return filepath.Join(r.path, "memories")
}

// batchLoop runs in a goroutine and periodically commits pending changes.
func (r *Repo) batchLoop() {
	defer close(r.batchDone)
	ticker := time.NewTicker(r.batchInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			r.flushBatch()
		case <-r.stopBatch:
			r.flushBatch()
			return
		}
	}
}

func (r *Repo) flushBatch() {
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.pending {
		return
	}

	wt, err := r.repo.Worktree()
	if err != nil {
		return
	}

	_, _ = wt.Commit("batch: update memories", &git.CommitOptions{
		Author: magiSignature(),
	})
	r.pending = false
}

// fileAtCommit reads a file from a specific commit.
func (r *Repo) fileAtCommit(relPath, hash string) (string, error) {
	commit, err := r.repo.CommitObject(plumbing.NewHash(hash))
	if err != nil {
		return "", fmt.Errorf("resolving commit %s: %w", hash, err)
	}

	tree, err := commit.Tree()
	if err != nil {
		return "", fmt.Errorf("getting tree: %w", err)
	}

	file, err := tree.File(relPath)
	if err != nil {
		return "", fmt.Errorf("getting file %s: %w", relPath, err)
	}

	content, err := file.Contents()
	if err != nil {
		return "", fmt.Errorf("reading file contents: %w", err)
	}

	return content, nil
}

func magiSignature() *object.Signature {
	return &object.Signature{
		Name:  "MAGI",
		Email: "magi@localhost",
		When:  time.Now().UTC(),
	}
}

// simpleDiff produces a basic line-by-line diff summary.
func simpleDiff(from, to string) string {
	if from == to {
		return "(no changes)"
	}

	fromLines := strings.Split(from, "\n")
	toLines := strings.Split(to, "\n")

	var b strings.Builder
	b.WriteString(fmt.Sprintf("--- a (from)\n+++ b (to)\n"))

	// Simple LCS-based diff
	maxLines := len(fromLines)
	if len(toLines) > maxLines {
		maxLines = len(toLines)
	}

	i, j := 0, 0
	for i < len(fromLines) || j < len(toLines) {
		if i < len(fromLines) && j < len(toLines) && fromLines[i] == toLines[j] {
			b.WriteString(" " + fromLines[i] + "\n")
			i++
			j++
		} else if j < len(toLines) && (i >= len(fromLines) || !containsLine(fromLines[i:], toLines[j])) {
			b.WriteString("+" + toLines[j] + "\n")
			j++
		} else {
			b.WriteString("-" + fromLines[i] + "\n")
			i++
		}
	}

	return b.String()
}

func containsLine(lines []string, target string) bool {
	for _, l := range lines {
		if l == target {
			return true
		}
	}
	return false
}
