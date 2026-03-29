package embeddings

import (
	"log/slog"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestMeanPool_Basic(t *testing.T) {
	// 2 tokens, 3 dims, both attended
	data := []float32{1, 2, 3, 4, 5, 6}
	mask := []int64{1, 1}
	result := meanPool(data, mask, 2, 3)

	if len(result) != 3 {
		t.Fatalf("expected 3 dims, got %d", len(result))
	}
	// Mean: (1+4)/2=2.5, (2+5)/2=3.5, (3+6)/2=4.5
	// Then L2-normalized
	norm := float32(math.Sqrt(2.5*2.5 + 3.5*3.5 + 4.5*4.5))
	expected := []float32{2.5 / norm, 3.5 / norm, 4.5 / norm}
	for i, v := range result {
		if math.Abs(float64(v-expected[i])) > 1e-5 {
			t.Errorf("dim %d: got %f, want %f", i, v, expected[i])
		}
	}
}

func TestMeanPool_WithMask(t *testing.T) {
	// 3 tokens, 2 dims; only first and third are attended
	data := []float32{1, 2, 99, 99, 3, 4}
	mask := []int64{1, 0, 1}
	result := meanPool(data, mask, 3, 2)

	if len(result) != 2 {
		t.Fatalf("expected 2 dims, got %d", len(result))
	}
	// Mean: (1+3)/2=2, (2+4)/2=3
	norm := float32(math.Sqrt(4 + 9))
	expected := []float32{2 / norm, 3 / norm}
	for i, v := range result {
		if math.Abs(float64(v-expected[i])) > 1e-5 {
			t.Errorf("dim %d: got %f, want %f", i, v, expected[i])
		}
	}
}

func TestMeanPool_AllMasked(t *testing.T) {
	data := []float32{1, 2, 3, 4}
	mask := []int64{0, 0}
	result := meanPool(data, mask, 2, 2)
	// All zeros when no tokens attended
	for i, v := range result {
		if v != 0 {
			t.Errorf("dim %d: expected 0, got %f", i, v)
		}
	}
}

func TestMeanPool_L2Normalized(t *testing.T) {
	data := []float32{3, 4}
	mask := []int64{1}
	result := meanPool(data, mask, 1, 2)
	// Should be L2-normalized: 3/5, 4/5
	var norm float32
	for _, v := range result {
		norm += v * v
	}
	if math.Abs(float64(norm)-1.0) > 1e-5 {
		t.Errorf("L2 norm = %f, want 1.0", norm)
	}
}

func TestFindOnnxRuntimeLib_EnvOverride(t *testing.T) {
	t.Setenv("ONNXRUNTIME_LIB", "/custom/path/libonnxruntime.so")
	result := findOnnxRuntimeLib()
	if result != "/custom/path/libonnxruntime.so" {
		t.Errorf("got %q, want /custom/path/libonnxruntime.so", result)
	}
}

func TestFindOnnxRuntimeLib_NoEnv(t *testing.T) {
	t.Setenv("ONNXRUNTIME_LIB", "")
	// Just make sure it doesn't panic — result depends on system
	_ = findOnnxRuntimeLib()
}

func TestDownloadIfMissing_AlreadyExists(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "existing.txt")
	os.WriteFile(path, []byte("data"), 0644)

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	err := downloadIfMissing(path, "http://should-not-be-called.invalid", logger)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDownloadIfMissing_Downloads(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("model data"))
	}))
	defer srv.Close()

	dir := t.TempDir()
	path := filepath.Join(dir, "model.bin")

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	err := downloadIfMissing(path, srv.URL+"/model.bin", logger)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading downloaded file: %v", err)
	}
	if string(data) != "model data" {
		t.Errorf("content = %q, want %q", data, "model data")
	}
}

func TestDownloadIfMissing_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	dir := t.TempDir()
	path := filepath.Join(dir, "model.bin")

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	err := downloadIfMissing(path, srv.URL+"/model.bin", logger)
	if err == nil {
		t.Fatal("expected error on server error")
	}
}

func TestOnnxProviderDimensions(t *testing.T) {
	// Test the constant without needing an actual ONNX session
	if dimensions != 384 {
		t.Errorf("dimensions = %d, want 384", dimensions)
	}
}

func TestDownloadIfMissing_BadURL(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "model.bin")
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	err := downloadIfMissing(path, "http://127.0.0.1:1/nonexistent", logger)
	if err == nil {
		t.Fatal("expected error for unreachable URL")
	}
}

func TestDownloadIfMissing_BadDir(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("data"))
	}))
	defer srv.Close()

	// Path in non-existent directory
	err := downloadIfMissing("/nonexistent/dir/file.bin", srv.URL+"/file", logger)
	if err == nil {
		t.Fatal("expected error for bad directory")
	}
}
