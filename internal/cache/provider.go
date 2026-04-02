package cache

import (
	"context"

	"github.com/j33pguy/magi/internal/embeddings"
	"github.com/j33pguy/magi/internal/metrics"
)

// Provider wraps an embeddings.Provider with an embedding cache.
type Provider struct {
	delegate embeddings.Provider
	cache    *EmbeddingCache
}

// NewProvider wraps an embeddings provider with an LRU embedding cache.
func NewProvider(delegate embeddings.Provider, maxSize int) *Provider {
	return &Provider{
		delegate: delegate,
		cache:    NewEmbeddingCache(maxSize),
	}
}

func (p *Provider) Embed(ctx context.Context, text string) ([]float32, error) {
	if cached := p.cache.Get(text); cached != nil {
		metrics.CacheHits.WithLabelValues("embedding").Inc()
		return cached, nil
	}
	metrics.CacheMisses.WithLabelValues("embedding").Inc()

	embedding, err := p.delegate.Embed(ctx, text)
	if err != nil {
		return nil, err
	}
	p.cache.Set(text, embedding)
	return cloneFloat32s(embedding), nil
}

func (p *Provider) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	var missingTexts []string
	var missingIdx []int

	for i, text := range texts {
		if cached := p.cache.Get(text); cached != nil {
			metrics.CacheHits.WithLabelValues("embedding").Inc()
			out[i] = cached
			continue
		}
		metrics.CacheMisses.WithLabelValues("embedding").Inc()
		missingTexts = append(missingTexts, text)
		missingIdx = append(missingIdx, i)
	}

	if len(missingTexts) > 0 {
		embeddingsOut, err := p.delegate.EmbedBatch(ctx, missingTexts)
		if err != nil {
			return nil, err
		}
		for i, embedding := range embeddingsOut {
			idx := missingIdx[i]
			p.cache.Set(texts[idx], embedding)
			out[idx] = cloneFloat32s(embedding)
		}
	}

	return out, nil
}

func (p *Provider) Dimensions() int {
	return p.delegate.Dimensions()
}

func (p *Provider) Destroy() {
	if managed, ok := p.delegate.(embeddings.ManagedProvider); ok {
		managed.Destroy()
	}
}
