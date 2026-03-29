package embeddings

import (
	"context"
	"testing"
)

// MockProvider implements Provider for testing.
type MockProvider struct {
	dim int
}

func (m *MockProvider) Embed(_ context.Context, _ string) ([]float32, error) {
	v := make([]float32, m.dim)
	for i := range v {
		v[i] = 0.1
	}
	return v, nil
}

func (m *MockProvider) EmbedBatch(_ context.Context, texts []string) ([][]float32, error) {
	results := make([][]float32, len(texts))
	for i := range texts {
		v, _ := m.Embed(context.Background(), texts[i])
		results[i] = v
	}
	return results, nil
}

func (m *MockProvider) Dimensions() int {
	return m.dim
}

func TestProviderInterface(t *testing.T) {
	var p Provider = &MockProvider{dim: 384}

	// Test Embed
	vec, err := p.Embed(context.Background(), "test")
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if len(vec) != 384 {
		t.Errorf("Embed returned %d dims, want 384", len(vec))
	}

	// Test EmbedBatch
	vecs, err := p.EmbedBatch(context.Background(), []string{"a", "b"})
	if err != nil {
		t.Fatalf("EmbedBatch: %v", err)
	}
	if len(vecs) != 2 {
		t.Errorf("EmbedBatch returned %d results, want 2", len(vecs))
	}

	// Test Dimensions
	if p.Dimensions() != 384 {
		t.Errorf("Dimensions = %d, want 384", p.Dimensions())
	}
}
