// Package remember provides shared enrichment logic for remember operations.
package remember

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"

	"github.com/j33pguy/magi/internal/classify"
	"github.com/j33pguy/magi/internal/contradiction"
	"github.com/j33pguy/magi/internal/db"
	"github.com/j33pguy/magi/internal/embeddings"
	"github.com/j33pguy/magi/internal/secretstore"
)

// TagMode controls how tag write failures are handled.
type TagMode int

const (
	TagModeFail TagMode = iota
	TagModeWarn
)

// Input holds the remember request data.
type Input struct {
	Content       string
	Summary       string
	Project       string
	Type          string
	Visibility    string
	Source        string
	SourceFile    string
	Speaker       string
	Area          string
	SubArea       string
	Tags          []string
	Owner         string
	Team          string
	Workspace     string
	Machine       string
	Agent         string
	Environment   string
	Transport     string
	ImportedFrom  string
	HumanAuthored bool
}

// Options configures the remember enrichment process.
type Options struct {
	DedupThreshold         *float64
	ContradictionThreshold float64
	TagMode                TagMode
	Logger                 *slog.Logger
	SecretManager          secretstore.Manager
}

// Result captures the outcome of remember enrichment.
type Result struct {
	Saved          *db.Memory
	Deduplicated   bool
	Match          *db.VectorResult
	Tags           []string
	TagWarning     string
	Contradictions []contradiction.Candidate
}

// PreparedWrite captures the canonical remember pipeline state before persistence.
type PreparedWrite struct {
	Memory         *db.Memory
	Envelope       Envelope
	Deduplicated   bool
	Match          *db.VectorResult
	Tags           []string
	Contradictions []contradiction.Candidate
}

// SecretError indicates content may contain secrets.
type SecretError struct {
	Warning string
}

func (e *SecretError) Error() string {
	return fmt.Sprintf("Content may contain secrets: %s. Remove sensitive data before storing.", e.Warning)
}

// Prepare runs the shared remember enrichment pipeline up to, but not including, persistence.
func Prepare(ctx context.Context, store db.Store, embedder embeddings.Provider, input Input, opts Options) (*PreparedWrite, error) {
	if input.Content == "" {
		return nil, fmt.Errorf("content is required")
	}

	if input.Type == "" {
		input.Type = "memory"
	}
	if input.Speaker == "" {
		input.Speaker = "assistant"
	}

	if input.Area == "" || input.SubArea == "" {
		c := classify.Infer(input.Content)
		if input.Area == "" {
			input.Area = c.Area
		}
		if input.SubArea == "" {
			input.SubArea = c.SubArea
		}
	}

	if warning := DetectSecrets(input.Content); warning != "" {
		if opts.SecretManager != nil {
			externalized, err := opts.SecretManager.Externalize(ctx, input.Project, input.Content)
			if err != nil {
				return nil, &SecretError{Warning: warning}
			}
			input.Content = externalized.RedactedContent
			for _, ref := range externalized.Refs {
				input.Tags = append(input.Tags,
					"secret_backend:"+ref.Backend,
					"secret_ref:"+ref.Path+"#"+ref.Key,
				)
			}
		} else {
			return nil, &SecretError{Warning: warning}
		}
	}

	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}

	embedding, err := embedder.Embed(ctx, input.Content)
	if err != nil {
		return nil, fmt.Errorf("generating embedding: %w", err)
	}

	dedupThreshold := 0.95
	if opts.DedupThreshold != nil {
		dedupThreshold = *opts.DedupThreshold
	}
	if dedupThreshold < 0 || dedupThreshold > 1 {
		dedupThreshold = 0.95
	}

	maxDistance := 1.0 - dedupThreshold
	groupDistance := 0.15 // 1.0 - 0.85

	match, err := store.FindSimilar(embedding, groupDistance)
	if err != nil {
		logger.Warn("dedup check failed, proceeding with insert", "error", err)
	} else if match != nil && match.Distance <= maxDistance {
		if match.Memory.Project != input.Project {
			logger.Debug("skipping cross-project dedup candidate", "existing_id", match.Memory.ID, "existing_project", match.Memory.Project, "project", input.Project)
		} else {
			logger.Info("deduplicated memory", "existing_id", match.Memory.ID, "distance", match.Distance)
			return &PreparedWrite{Deduplicated: true, Match: match}, nil
		}
	}

	memory := &db.Memory{
		Content:    input.Content,
		Summary:    input.Summary,
		Embedding:  embedding,
		Project:    input.Project,
		Type:       input.Type,
		Visibility: input.Visibility,
		Source:     input.Source,
		SourceFile: input.SourceFile,
		Speaker:    input.Speaker,
		Area:       input.Area,
		SubArea:    input.SubArea,
		TokenCount: len(input.Content) / 4,
	}

	if match != nil {
		memory.ParentID = match.Memory.ID
		logger.Info("linking memory to similar parent", "parent_id", match.Memory.ID, "distance", match.Distance)
	}

	threshold := opts.ContradictionThreshold
	if threshold <= 0 {
		threshold = 0.85
	}
	detector := &contradiction.Detector{Threshold: threshold}
	candidates, cErr := detector.Check(ctx, store, embedder, input.Content, input.Area, input.SubArea)
	if cErr != nil {
		logger.Warn("contradiction detection failed", "error", cErr)
	}

	prepared := &PreparedWrite{Memory: memory, Envelope: BuildEnvelope(input), Match: match, Tags: BuildTags(input)}
	if len(candidates) > 0 {
		prepared.Contradictions = candidates
	}
	return prepared, nil
}

// Persist saves a prepared write and applies tags under the configured tag mode.
func Persist(store db.Store, prepared *PreparedWrite, opts Options) (*Result, error) {
	if prepared == nil {
		return nil, fmt.Errorf("prepared write is required")
	}
	if prepared.Deduplicated {
		return &Result{Deduplicated: true, Match: prepared.Match}, nil
	}
	if prepared.Memory == nil {
		return nil, fmt.Errorf("prepared memory is required")
	}

	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}

	var saved *db.Memory
	var tagWarning string
	if persister, ok := store.(preparedMemoryPersister); ok {
		ctxRecord := envelopeRecord("", prepared.Envelope)
		persisted, err := persister.PersistPreparedMemory(db.PersistPreparedMemoryInput{
			Memory:  prepared.Memory,
			Tags:    prepared.Tags,
			Context: ctxRecord,
		})
		if err != nil {
			return nil, fmt.Errorf("saving memory: %w", err)
		}
		saved = persisted.Saved
		tagWarning = persisted.TagWarning
	} else {
		var err error
		saved, err = store.SaveMemory(prepared.Memory)
		if err != nil {
			return nil, fmt.Errorf("saving memory: %w", err)
		}

		persistEnvelope(store, saved.ID, prepared.Envelope, logger)

		if len(prepared.Tags) > 0 {
			if err := store.SetTags(saved.ID, prepared.Tags); err != nil {
				if opts.TagMode == TagModeWarn {
					logger.Warn("setting tags failed (non-fatal)", "error", err, "memory_id", saved.ID)
					tagWarning = err.Error()
				} else {
					return nil, fmt.Errorf("setting tags: %w", err)
				}
			}
		}
	}

	if tagWarning != "" && opts.TagMode != TagModeWarn {
		return nil, fmt.Errorf("setting tags: %w", errors.New(tagWarning))
	}
	if tagWarning != "" && opts.TagMode == TagModeWarn {
		logger.Warn("setting tags failed (non-fatal)", "error", tagWarning, "memory_id", saved.ID)
	}

	return &Result{
		Saved:          saved,
		Match:          prepared.Match,
		Tags:           prepared.Tags,
		TagWarning:     tagWarning,
		Contradictions: prepared.Contradictions,
	}, nil
}

// Remember runs enrichment, dedup, save, tags, and contradiction detection.
func Remember(ctx context.Context, store db.Store, embedder embeddings.Provider, input Input, opts Options) (*Result, error) {
	prepared, err := Prepare(ctx, store, embedder, input, opts)
	if err != nil {
		return nil, err
	}
	return Persist(store, prepared, opts)
}

type memoryContextSaver interface {
	SaveMemoryContext(record *db.MemoryContextRecord) error
}

type preparedMemoryPersister interface {
	PersistPreparedMemory(input db.PersistPreparedMemoryInput) (*db.PersistPreparedMemoryResult, error)
}

func envelopeRecord(memoryID string, env Envelope) *db.MemoryContextRecord {
	return &db.MemoryContextRecord{
		MemoryID: memoryID,
		Repository: db.RepositoryRecord{
			Host:          env.Repository.Host,
			Owner:         env.Repository.Owner,
			Name:          env.Repository.Name,
			CanonicalName: env.Repository.Canonical,
			DisplayName:   env.Repository.Canonical,
		},
		ScopeOwner:              env.Scope.Owner,
		ScopeTeam:               env.Scope.Team,
		ScopeWorkspace:          env.Scope.Workspace,
		ScopeMachine:            env.Scope.Machine,
		ScopeAgent:              env.Scope.Agent,
		ScopeEnvironment:        env.Scope.Environment,
		ProvenanceTransport:     env.Provenance.Transport,
		ProvenanceImportedFrom:  env.Provenance.ImportedFrom,
		ProvenanceHumanAuthored: env.Provenance.HumanAuthored,
		DurableAt:               env.Provenance.DurableAt,
	}
}

func persistEnvelope(store db.Store, memoryID string, env Envelope, logger *slog.Logger) {
	saver, ok := store.(memoryContextSaver)
	if !ok {
		return
	}
	record := envelopeRecord(memoryID, env)
	if err := saver.SaveMemoryContext(record); err != nil && logger != nil {
		logger.Warn("persisting memory context failed (non-fatal)", "memory_id", memoryID, "error", err)
	}
}

var secretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(api[_-]?key|api[_-]?secret|access[_-]?token|auth[_-]?token|secret[_-]?key)\s*[:=]\s*\S+`),
	regexp.MustCompile(`(?i)(password|passwd|pwd)\s*[:=]\s*\S+`),
	regexp.MustCompile(`(?i)bearer\s+[a-zA-Z0-9\-._~+/]+=*`),
	regexp.MustCompile(`ghp_[a-zA-Z0-9]{36}`),
	regexp.MustCompile(`sk-[a-zA-Z0-9]{32,}`),
	regexp.MustCompile(`-----BEGIN (RSA |EC )?PRIVATE KEY-----`),
}

// DetectSecrets returns a warning string if the content appears to contain secrets.
func DetectSecrets(content string) string {
	var found []string
	for _, pat := range secretPatterns {
		if pat.MatchString(content) {
			found = append(found, pat.String())
		}
	}
	if len(found) > 0 {
		return strings.Join(found, "; ")
	}
	return ""
}
