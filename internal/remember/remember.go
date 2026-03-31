// Package remember provides shared enrichment logic for remember operations.
package remember

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"strings"

	"github.com/j33pguy/magi/internal/classify"
	"github.com/j33pguy/magi/internal/contradiction"
	"github.com/j33pguy/magi/internal/db"
	"github.com/j33pguy/magi/internal/embeddings"
)

// TagMode controls how tag write failures are handled.
type TagMode int

const (
	TagModeFail TagMode = iota
	TagModeWarn
)

// Input holds the remember request data.
type Input struct {
	Content    string
	Summary    string
	Project    string
	Type       string
	Visibility string
	Source     string
	Speaker    string
	Area       string
	SubArea    string
	Tags       []string
}

// Options configures the remember enrichment process.
type Options struct {
	DedupThreshold         *float64
	ContradictionThreshold float64
	TagMode                TagMode
	Logger                 *slog.Logger
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

// SecretError indicates content may contain secrets.
type SecretError struct {
	Warning string
}

func (e *SecretError) Error() string {
	return fmt.Sprintf("Content may contain secrets: %s. Remove sensitive data before storing.", e.Warning)
}

// Remember runs enrichment, dedup, save, tags, and contradiction detection.
func Remember(ctx context.Context, store db.Store, embedder embeddings.Provider, input Input, opts Options) (*Result, error) {
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
		return nil, &SecretError{Warning: warning}
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
		logger.Info("deduplicated memory", "existing_id", match.Memory.ID, "distance", match.Distance)
		return &Result{Deduplicated: true, Match: match}, nil
	}

	memory := &db.Memory{
		Content:    input.Content,
		Summary:    input.Summary,
		Embedding:  embedding,
		Project:    input.Project,
		Type:       input.Type,
		Visibility: input.Visibility,
		Source:     input.Source,
		Speaker:    input.Speaker,
		Area:       input.Area,
		SubArea:    input.SubArea,
		TokenCount: len(input.Content) / 4,
	}

	if match != nil {
		memory.ParentID = match.Memory.ID
		logger.Info("linking memory to similar parent", "parent_id", match.Memory.ID, "distance", match.Distance)
	}

	saved, err := store.SaveMemory(memory)
	if err != nil {
		return nil, fmt.Errorf("saving memory: %w", err)
	}

	tags := append([]string{}, input.Tags...)
	if input.Speaker != "" {
		tags = append(tags, "speaker:"+input.Speaker)
	}
	if input.Area != "" {
		tags = append(tags, "area:"+input.Area)
	}
	if input.SubArea != "" {
		tags = append(tags, "sub_area:"+input.SubArea)
	}

	var tagWarning string
	if len(tags) > 0 {
		if err := store.SetTags(saved.ID, tags); err != nil {
			if opts.TagMode == TagModeWarn {
				logger.Warn("setting tags failed (non-fatal)", "error", err, "memory_id", saved.ID)
				tagWarning = err.Error()
			} else {
				return nil, fmt.Errorf("setting tags: %w", err)
			}
		}
	}

	result := &Result{Saved: saved, Match: match, Tags: tags, TagWarning: tagWarning}

	threshold := opts.ContradictionThreshold
	if threshold <= 0 {
		threshold = 0.85
	}
	// Best-effort contradiction detection; never blocks writes.
	detector := &contradiction.Detector{Threshold: threshold}
	candidates, cErr := detector.Check(ctx, store, embedder, input.Content, input.Area, input.SubArea)
	if cErr != nil {
		logger.Warn("contradiction detection failed", "error", cErr)
	} else if len(candidates) > 0 {
		result.Contradictions = candidates
	}

	return result, nil
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
