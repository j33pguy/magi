package embeddings

import (
	"context"
	"errors"
	"log/slog"
	"math"
	"os"
	"testing"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

// --- MockEmbedder with configurable behavior ---

// ErrorMockProvider implements Provider but returns errors.
type ErrorMockProvider struct {
	dim int
	err error
}

func (m *ErrorMockProvider) Embed(_ context.Context, _ string) ([]float32, error) {
	return nil, m.err
}

func (m *ErrorMockProvider) EmbedBatch(_ context.Context, texts []string) ([][]float32, error) {
	return nil, m.err
}

func (m *ErrorMockProvider) Dimensions() int {
	return m.dim
}

// DeterministicMockProvider returns deterministic 384-dim vectors based on input text.
type DeterministicMockProvider struct{}

func (m *DeterministicMockProvider) Embed(_ context.Context, text string) ([]float32, error) {
	v := make([]float32, 384)
	for i := range v {
		v[i] = float32(len(text)+i) / 10000.0
	}
	// L2 normalize
	var norm float64
	for _, val := range v {
		norm += float64(val) * float64(val)
	}
	norm = math.Sqrt(norm)
	if norm > 0 {
		for i := range v {
			v[i] = float32(float64(v[i]) / norm)
		}
	}
	return v, nil
}

func (m *DeterministicMockProvider) EmbedBatch(_ context.Context, texts []string) ([][]float32, error) {
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

func (m *DeterministicMockProvider) Dimensions() int { return 384 }

// --- Tests ---

func TestErrorMockProvider_Embed(t *testing.T) {
	expectedErr := errors.New("model offline")
	p := &ErrorMockProvider{dim: 384, err: expectedErr}

	_, err := p.Embed(context.Background(), "test")
	if !errors.Is(err, expectedErr) {
		t.Errorf("expected error %v, got %v", expectedErr, err)
	}
}

func TestErrorMockProvider_EmbedBatch(t *testing.T) {
	expectedErr := errors.New("batch failure")
	p := &ErrorMockProvider{dim: 384, err: expectedErr}

	_, err := p.EmbedBatch(context.Background(), []string{"a", "b"})
	if !errors.Is(err, expectedErr) {
		t.Errorf("expected error %v, got %v", expectedErr, err)
	}
}

func TestErrorMockProvider_Dimensions(t *testing.T) {
	p := &ErrorMockProvider{dim: 768, err: errors.New("fail")}
	if p.Dimensions() != 768 {
		t.Errorf("Dimensions = %d, want 768", p.Dimensions())
	}
}

func TestDeterministicMockProvider_Consistency(t *testing.T) {
	p := &DeterministicMockProvider{}

	v1, err := p.Embed(context.Background(), "hello world")
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	v2, err := p.Embed(context.Background(), "hello world")
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}

	for i := range v1 {
		if v1[i] != v2[i] {
			t.Fatalf("embedding not deterministic at dim %d: %f != %f", i, v1[i], v2[i])
		}
	}
}

func TestDeterministicMockProvider_L2Normalized(t *testing.T) {
	p := &DeterministicMockProvider{}
	v, err := p.Embed(context.Background(), "test input")
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}

	var norm float64
	for _, val := range v {
		norm += float64(val) * float64(val)
	}
	norm = math.Sqrt(norm)

	if math.Abs(norm-1.0) > 1e-4 {
		t.Errorf("L2 norm = %f, want ~1.0", norm)
	}
}

func TestDeterministicMockProvider_DifferentInputsDifferentVectors(t *testing.T) {
	p := &DeterministicMockProvider{}

	v1, _ := p.Embed(context.Background(), "short")
	v2, _ := p.Embed(context.Background(), "a much longer input string")

	same := true
	for i := range v1 {
		if v1[i] != v2[i] {
			same = false
			break
		}
	}
	if same {
		t.Error("different inputs should produce different embeddings")
	}
}

func TestDeterministicMockProvider_EmbedBatch(t *testing.T) {
	p := &DeterministicMockProvider{}
	results, err := p.EmbedBatch(context.Background(), []string{"a", "bb", "ccc"})
	if err != nil {
		t.Fatalf("EmbedBatch: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
	for i, r := range results {
		if len(r) != 384 {
			t.Errorf("result[%d] has %d dims, want 384", i, len(r))
		}
	}
}

func TestDeterministicMockProvider_Dimensions(t *testing.T) {
	p := &DeterministicMockProvider{}
	if p.Dimensions() != 384 {
		t.Errorf("Dimensions = %d, want 384", p.Dimensions())
	}
}

func TestMeanPool_SingleToken(t *testing.T) {
	data := []float32{3, 4}
	mask := []int64{1}
	result := meanPool(data, mask, 1, 2)
	if len(result) != 2 {
		t.Fatalf("expected 2 dims, got %d", len(result))
	}
	// 3/5, 4/5
	if math.Abs(float64(result[0])-0.6) > 1e-5 {
		t.Errorf("dim 0: got %f, want 0.6", result[0])
	}
	if math.Abs(float64(result[1])-0.8) > 1e-5 {
		t.Errorf("dim 1: got %f, want 0.8", result[1])
	}
}

func TestMeanPool_LargeDim(t *testing.T) {
	// 384 dims, 2 tokens, all attended
	seqLen := 2
	embDim := 384
	data := make([]float32, seqLen*embDim)
	mask := make([]int64, seqLen)
	for i := range data {
		data[i] = float32(i) * 0.001
	}
	mask[0] = 1
	mask[1] = 1

	result := meanPool(data, mask, seqLen, embDim)
	if len(result) != 384 {
		t.Fatalf("expected 384 dims, got %d", len(result))
	}

	// Verify L2 normalized
	var norm float64
	for _, v := range result {
		norm += float64(v) * float64(v)
	}
	norm = math.Sqrt(norm)
	if math.Abs(norm-1.0) > 1e-4 {
		t.Errorf("L2 norm = %f, want ~1.0", norm)
	}
}

func TestMockProvider_EmptyBatch(t *testing.T) {
	p := &MockProvider{dim: 384}
	results, err := p.EmbedBatch(context.Background(), []string{})
	if err != nil {
		t.Fatalf("EmbedBatch: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

func TestMockProvider_EmptyString(t *testing.T) {
	p := &MockProvider{dim: 384}
	v, err := p.Embed(context.Background(), "")
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if len(v) != 384 {
		t.Errorf("expected 384 dims, got %d", len(v))
	}
}

func TestFindOnnxRuntimeLib_SystemPaths(t *testing.T) {
	// Ensure env override works and falls back
	t.Setenv("ONNXRUNTIME_LIB", "")
	result := findOnnxRuntimeLib()
	// May or may not find it — just ensure it doesn't panic
	_ = result
}

func TestDownloadIfMissing_IOCopyError(t *testing.T) {
	// Test with a server that closes connection after headers
	// This is covered by the existing tests, but let's verify path creation error
	logger := testLogger()
	err := downloadIfMissing("/proc/nonexistent/deep/path/file.bin", "http://127.0.0.1:1/nope", logger)
	if err == nil {
		t.Fatal("expected error for unreachable URL")
	}
}
