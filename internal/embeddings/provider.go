// Package embeddings provides text embedding generation using local ONNX models.
package embeddings

import "context"

// Provider generates vector embeddings from text.
type Provider interface {
	Embed(ctx context.Context, text string) ([]float32, error)
	EmbedBatch(ctx context.Context, texts []string) ([][]float32, error)
	Dimensions() int
}
