package embeddings

import (
	"context"
	"errors"
	"math"
	"strings"
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

// errProvider is a stub that always returns an error from Embed/EmbedBatch.
type errProvider struct {
	dims int
	err  error
}

func (e *errProvider) Embed(_ context.Context, _ string) ([]float32, error) {
	return nil, e.err
}

func (e *errProvider) EmbedBatch(_ context.Context, _ []string) ([][]float32, error) {
	return nil, e.err
}

func (e *errProvider) Dimensions() int { return e.dims }

// wrongDimsProvider returns embeddings with a different dimension than declared.
type wrongDimsProvider struct {
	declaredDims int
	actualDims   int
}

func (w *wrongDimsProvider) Embed(_ context.Context, _ string) ([]float32, error) {
	emb := make([]float32, w.actualDims)
	for i := range emb {
		emb[i] = 1.0 / float32(w.actualDims)
	}
	return emb, nil
}

func (w *wrongDimsProvider) EmbedBatch(_ context.Context, texts []string) ([][]float32, error) {
	var results [][]float32
	for _, t := range texts {
		e, _ := w.Embed(context.Background(), t)
		results = append(results, e)
	}
	return results, nil
}

func (w *wrongDimsProvider) Dimensions() int { return w.declaredDims }

// --- tests ---

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

// --- new coverage tests ---

func TestCompressedEmbed_InnerEmbedError(t *testing.T) {
	embedErr := errors.New("model unavailable")
	inner := &errProvider{dims: 384, err: embedErr}
	cp, err := NewCompressedProvider(inner, 4)
	if err != nil {
		t.Fatal(err)
	}

	full, compressed, err := cp.CompressedEmbed(context.Background(), "hello")
	if err == nil {
		t.Fatal("expected error from CompressedEmbed when inner.Embed fails")
	}
	if !errors.Is(err, embedErr) {
		t.Errorf("expected wrapped embed error, got: %v", err)
	}
	if full != nil {
		t.Errorf("expected nil full, got %v", full)
	}
	if compressed != nil {
		t.Errorf("expected nil compressed, got %v", compressed)
	}
}

func TestCompressedEmbed_CompressionError(t *testing.T) {
	// wrongDimsProvider declares 384 dims (so the store is built for 384)
	// but returns 128-dim vectors, causing CompressEmbedding to fail.
	inner := &wrongDimsProvider{declaredDims: 384, actualDims: 128}
	cp, err := NewCompressedProvider(inner, 4)
	if err != nil {
		t.Fatal(err)
	}

	full, compressed, err := cp.CompressedEmbed(context.Background(), "hello")
	if err == nil {
		t.Fatal("expected error from CompressedEmbed when compression fails due to dim mismatch")
	}
	if !strings.Contains(err.Error(), "compressing") {
		t.Errorf("expected error to mention 'compressing', got: %v", err)
	}
	// full is still returned even on compression error
	if full == nil {
		t.Error("expected non-nil full vector even when compression fails")
	}
	if compressed != nil {
		t.Errorf("expected nil compressed, got %v", compressed)
	}
}

func TestDecompress_InvalidData(t *testing.T) {
	inner := &stubProvider{dims: 384}
	cp, err := NewCompressedProvider(inner, 4)
	if err != nil {
		t.Fatal(err)
	}

	// Pass garbage bytes that cannot be unmarshalled.
	_, err = cp.Decompress([]byte("not-valid-compressed-data"))
	if err == nil {
		t.Fatal("expected error when decompressing invalid data")
	}
}

func TestDecompress_NilData(t *testing.T) {
	inner := &stubProvider{dims: 384}
	cp, err := NewCompressedProvider(inner, 4)
	if err != nil {
		t.Fatal(err)
	}

	_, err = cp.Decompress(nil)
	if err == nil {
		t.Fatal("expected error when decompressing nil data")
	}
}

func TestDecompress_EmptyData(t *testing.T) {
	inner := &stubProvider{dims: 384}
	cp, err := NewCompressedProvider(inner, 4)
	if err != nil {
		t.Fatal(err)
	}

	_, err = cp.Decompress([]byte{})
	if err == nil {
		t.Fatal("expected error when decompressing empty data")
	}
}

func TestCompressionStats_AllFields(t *testing.T) {
	inner := &stubProvider{dims: 384}
	cp, err := NewCompressedProvider(inner, 4)
	if err != nil {
		t.Fatal(err)
	}

	stats := cp.CompressionStats()
	expectedKeys := []string{"dims", "bits_per_angle", "original_bytes", "compressed_bytes", "ratio"}
	for _, key := range expectedKeys {
		if _, ok := stats[key]; !ok {
			t.Errorf("expected key %q in stats", key)
		}
	}

	if dims, ok := stats["dims"].(int); !ok || dims != 384 {
		t.Errorf("expected dims=384, got %v", stats["dims"])
	}
	if bpa, ok := stats["bits_per_angle"].(int); !ok || bpa != 4 {
		t.Errorf("expected bits_per_angle=4, got %v", stats["bits_per_angle"])
	}
	ratio, ok := stats["ratio"].(float64)
	if !ok || ratio <= 1.0 {
		t.Errorf("expected compression ratio > 1.0, got %v", stats["ratio"])
	}
}

func TestCompressionStats_DifferentBitWidths(t *testing.T) {
	inner := &stubProvider{dims: 384}

	// Higher bits_per_angle should result in lower compression ratio.
	var prevRatio float64
	for _, bits := range []int{2, 4, 8} {
		cp, err := NewCompressedProvider(inner, bits)
		if err != nil {
			t.Fatalf("bits=%d: %v", bits, err)
		}
		stats := cp.CompressionStats()
		ratio := stats["ratio"].(float64)
		if prevRatio > 0 && ratio >= prevRatio {
			t.Errorf("bits=%d: ratio %f should be less than previous %f (higher bits = less compression)",
				bits, ratio, prevRatio)
		}
		prevRatio = ratio
	}
}
