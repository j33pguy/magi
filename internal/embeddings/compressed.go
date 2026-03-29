package embeddings

import (
	"context"
	"fmt"

	"github.com/j33pguy/magi/internal/polarquant"
)

// CompressedProvider wraps a Provider and adds PolarQuant compression.
type CompressedProvider struct {
	inner Provider
	store *polarquant.Store
}

// NewCompressedProvider creates a provider that compresses embeddings via PolarQuant.
func NewCompressedProvider(inner Provider, bitsPerAngle int) (*CompressedProvider, error) {
	dims := inner.Dimensions()
	store, err := polarquant.NewStore(dims, bitsPerAngle)
	if err != nil {
		return nil, fmt.Errorf("creating polarquant store: %w", err)
	}
	return &CompressedProvider{inner: inner, store: store}, nil
}

func (p *CompressedProvider) Embed(ctx context.Context, text string) ([]float32, error) {
	return p.inner.Embed(ctx, text)
}

func (p *CompressedProvider) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	return p.inner.EmbedBatch(ctx, texts)
}

func (p *CompressedProvider) Dimensions() int {
	return p.inner.Dimensions()
}

// CompressedEmbed returns both full and compressed forms.
func (p *CompressedProvider) CompressedEmbed(ctx context.Context, text string) (full []float32, compressed []byte, err error) {
	full, err = p.inner.Embed(ctx, text)
	if err != nil {
		return nil, nil, err
	}
	compressed, err = p.store.CompressEmbedding(full)
	if err != nil {
		return full, nil, fmt.Errorf("compressing: %w", err)
	}
	return full, compressed, nil
}

// Decompress reconstructs an approximate embedding from compressed bytes.
func (p *CompressedProvider) Decompress(data []byte) ([]float32, error) {
	return p.store.DecompressEmbedding(data)
}

// CompressionStats returns compression ratio and size info.
func (p *CompressedProvider) CompressionStats() map[string]interface{} {
	return p.store.CompressionStats()
}
