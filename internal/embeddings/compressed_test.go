package embeddings

import (
	"context"
	"math"
	"testing"
)

type stubProvider struct {
	dims int
}

func (s *stubProvider) Embed(_ context.Context, text string) ([]float32, error) {
	emb := make([]float32, s.dims)
	for i := range emb {
		emb[i] = float32(i+len(text)) / float32(s.dims)
	}
	var norm float32
	for _, v := range emb {
		norm += v * v
	}
	norm = float32(math.Sqrt(float64(norm)))
	if norm > 0 {
		for i := range emb {
			emb[i] /= norm
		}
	}
	return emb, nil
}

func (s *stubProvider) EmbedBatch(_ context.Context, texts []string) ([][]float32, error) {
	var results [][]float32
	for _, t := range texts {
		e, _ := s.Embed(context.Background(), t)
		results = append(results, e)
	}
	return results, nil
}

func (s *stubProvider) Dimensions() int { return s.dims }

func TestNewCompressedProvider(t *testing.T) {
	inner := &stubProvider{dims: 384}
	cp, err := NewCompressedProvider(inner, 4)
	if err != nil {
		t.Fatal(err)
	}
	if cp.Dimensions() != 384 {
		t.Errorf("expected 384 dims, got %d", cp.Dimensions())
	}
}

func TestCompressedEmbed(t *testing.T) {
	inner := &stubProvider{dims: 384}
	cp, _ := NewCompressedProvider(inner, 4)

	full, compressed, err := cp.CompressedEmbed(context.Background(), "hello world")
	if err != nil {
		t.Fatal(err)
	}
	if len(full) != 384 {
		t.Errorf("expected 384 full dims, got %d", len(full))
	}
	if len(compressed) == 0 {
		t.Error("expected non-empty compressed bytes")
	}
	if len(compressed) >= len(full)*4 {
		t.Errorf("compressed (%d bytes) should be smaller than full (%d bytes)", len(compressed), len(full)*4)
	}
}

func TestCompressDecompressRoundtrip(t *testing.T) {
	inner := &stubProvider{dims: 384}
	cp, _ := NewCompressedProvider(inner, 8)

	full, compressed, err := cp.CompressedEmbed(context.Background(), "test roundtrip")
	if err != nil {
		t.Fatal(err)
	}

	reconstructed, err := cp.Decompress(compressed)
	if err != nil {
		t.Fatal(err)
	}
	if len(reconstructed) != len(full) {
		t.Fatalf("expected %d dims, got %d", len(full), len(reconstructed))
	}

	var dot, normA, normB float32
	for i := range full {
		dot += full[i] * reconstructed[i]
		normA += full[i] * full[i]
		normB += reconstructed[i] * reconstructed[i]
	}
	cosine := float64(dot) / (math.Sqrt(float64(normA)) * math.Sqrt(float64(normB)))
	if cosine < 0.95 {
		t.Errorf("cosine similarity too low: %f (expected > 0.95)", cosine)
	}
}

func TestCompressedProvider_RegularEmbed(t *testing.T) {
	inner := &stubProvider{dims: 384}
	cp, _ := NewCompressedProvider(inner, 4)

	emb, err := cp.Embed(context.Background(), "test")
	if err != nil {
		t.Fatal(err)
	}
	if len(emb) != 384 {
		t.Errorf("expected 384, got %d", len(emb))
	}
}

func TestCompressedProvider_EmbedBatch(t *testing.T) {
	inner := &stubProvider{dims: 384}
	cp, _ := NewCompressedProvider(inner, 4)

	results, err := cp.EmbedBatch(context.Background(), []string{"hello", "world", "test"})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 3 {
		t.Errorf("expected 3 results, got %d", len(results))
	}
}

func TestCompressionStats(t *testing.T) {
	inner := &stubProvider{dims: 384}
	cp, _ := NewCompressedProvider(inner, 4)

	stats := cp.CompressionStats()
	if stats == nil {
		t.Fatal("expected non-nil stats")
	}
	if _, ok := stats["bits_per_angle"]; !ok {
		t.Error("expected bits_per_angle in stats")
	}
}

func TestCompressedProvider_DifferentBitWidths(t *testing.T) {
	inner := &stubProvider{dims: 384}

	for _, bits := range []int{2, 4, 8} {
		cp, err := NewCompressedProvider(inner, bits)
		if err != nil {
			t.Fatalf("bits=%d: %v", bits, err)
		}
		_, compressed, err := cp.CompressedEmbed(context.Background(), "test")
		if err != nil {
			t.Fatalf("bits=%d: %v", bits, err)
		}
		if len(compressed) == 0 {
			t.Errorf("bits=%d: expected non-empty compressed bytes", bits)
		}
	}
}
