package embeddings

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"testing"
)

// --- meanPool ---

func BenchmarkMeanPool384(b *testing.B) {
	seqLen := 128
	embDim := 384
	data := make([]float32, seqLen*embDim)
	mask := make([]int64, seqLen)
	for i := range data {
		data[i] = rand.Float32()
	}
	for i := range mask {
		if i < 64 {
			mask[i] = 1
		}
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = meanPool(data, mask, seqLen, embDim)
	}
}

func BenchmarkMeanPool_FullMask(b *testing.B) {
	seqLen := 128
	embDim := 384
	data := make([]float32, seqLen*embDim)
	mask := make([]int64, seqLen)
	for i := range data {
		data[i] = rand.Float32()
	}
	for i := range mask {
		mask[i] = 1
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = meanPool(data, mask, seqLen, embDim)
	}
}

// --- MockProvider EmbedBatch (measures goroutine overhead) ---

// slowMockProvider simulates embedding computation with a small busy loop.
type slowMockProvider struct {
	dim int
}

func (m *slowMockProvider) Embed(_ context.Context, text string) ([]float32, error) {
	v := make([]float32, m.dim)
	// Simulate some computation
	for i := range v {
		v[i] = float32(math.Sin(float64(i) + float64(len(text))))
	}
	// L2 normalize
	var norm float32
	for _, f := range v {
		norm += f * f
	}
	norm = float32(math.Sqrt(float64(norm)))
	if norm > 0 {
		for i := range v {
			v[i] /= norm
		}
	}
	return v, nil
}

func (m *slowMockProvider) EmbedBatch(_ context.Context, texts []string) ([][]float32, error) {
	results := make([][]float32, len(texts))
	for i, t := range texts {
		v, err := m.Embed(context.Background(), t)
		if err != nil {
			return nil, err
		}
		results[i] = v
	}
	return results, nil
}

func (m *slowMockProvider) Dimensions() int { return m.dim }

func BenchmarkMockEmbedSingle(b *testing.B) {
	p := &slowMockProvider{dim: 384}
	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := p.Embed(ctx, "test embedding text")
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkMockEmbedBatch10(b *testing.B) {
	p := &slowMockProvider{dim: 384}
	ctx := context.Background()
	texts := make([]string, 10)
	for i := range texts {
		texts[i] = fmt.Sprintf("test embedding text number %d", i)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := p.EmbedBatch(ctx, texts)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkMockEmbedBatch50(b *testing.B) {
	p := &slowMockProvider{dim: 384}
	ctx := context.Background()
	texts := make([]string, 50)
	for i := range texts {
		texts[i] = fmt.Sprintf("test embedding text number %d with more content for realism", i)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := p.EmbedBatch(ctx, texts)
		if err != nil {
			b.Fatal(err)
		}
	}
}
