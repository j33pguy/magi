package embeddings

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sync"

	ort "github.com/yalue/onnxruntime_go"
)

const (
	modelName   = "all-MiniLM-L6-v2"
	dimensions  = 384
	maxTokenLen = 128

	modelURL = "https://huggingface.co/sentence-transformers/all-MiniLM-L6-v2/resolve/main/onnx/model.onnx"
	vocabURL = "https://huggingface.co/sentence-transformers/all-MiniLM-L6-v2/resolve/main/vocab.txt"
)

// OnnxProvider generates embeddings using all-MiniLM-L6-v2 via ONNX runtime.
// A single persistent session is reused across calls (sessions are not thread-safe;
// access is serialized via mu).
type OnnxProvider struct {
	tokenizer *Tokenizer
	session   *ort.AdvancedSession
	logger    *slog.Logger
	mu        sync.Mutex

	// pre-allocated tensors reused across calls to avoid repeated alloc/free
	inputIDsTensor *ort.Tensor[int64]
	maskTensor     *ort.Tensor[int64]
	typeTensor     *ort.Tensor[int64]
	outputTensor   *ort.Tensor[float32]
}

// NewOnnxProvider creates a new ONNX embedding provider. Downloads the model
// and vocab on first use if not already cached.
func NewOnnxProvider(logger *slog.Logger) (*OnnxProvider, error) {
	modelDir := os.Getenv("CLAUDE_MEMORY_MODEL_DIR")
	if modelDir == "" {
		home, _ := os.UserHomeDir()
		modelDir = filepath.Join(home, ".claude", "models")
	}

	dir := filepath.Join(modelDir, modelName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("creating model directory: %w", err)
	}

	modelPath := filepath.Join(dir, "model.onnx")
	vocabPath := filepath.Join(dir, "vocab.txt")

	// Download model files if not cached
	if err := downloadIfMissing(modelPath, modelURL, logger); err != nil {
		return nil, fmt.Errorf("downloading model: %w", err)
	}
	if err := downloadIfMissing(vocabPath, vocabURL, logger); err != nil {
		return nil, fmt.Errorf("downloading vocab: %w", err)
	}

	// Initialize ONNX runtime
	libPath := findOnnxRuntimeLib()
	if libPath == "" {
		return nil, fmt.Errorf("onnxruntime shared library not found; set ONNXRUNTIME_LIB or install via your package manager")
	}

	ort.SetSharedLibraryPath(libPath)
	if err := ort.InitializeEnvironment(); err != nil {
		if !ort.IsInitialized() {
			return nil, fmt.Errorf("initializing ONNX runtime: %w", err)
		}
	}

	// Load tokenizer
	tokenizer, err := NewTokenizer(vocabPath, maxTokenLen)
	if err != nil {
		return nil, fmt.Errorf("loading tokenizer: %w", err)
	}

	batchSize := int64(1)
	seqLen := int64(maxTokenLen)

	// Pre-allocate tensors once — reused across all inference calls.
	inputIDsTensor, err := ort.NewTensor(ort.NewShape(batchSize, seqLen), make([]int64, batchSize*seqLen))
	if err != nil {
		return nil, fmt.Errorf("creating input_ids tensor: %w", err)
	}
	maskTensor, err := ort.NewTensor(ort.NewShape(batchSize, seqLen), make([]int64, batchSize*seqLen))
	if err != nil {
		inputIDsTensor.Destroy()
		return nil, fmt.Errorf("creating attention_mask tensor: %w", err)
	}
	typeTensor, err := ort.NewTensor(ort.NewShape(batchSize, seqLen), make([]int64, batchSize*seqLen))
	if err != nil {
		inputIDsTensor.Destroy()
		maskTensor.Destroy()
		return nil, fmt.Errorf("creating token_type_ids tensor: %w", err)
	}
	outputTensor, err := ort.NewEmptyTensor[float32](ort.NewShape(batchSize, seqLen, int64(dimensions)))
	if err != nil {
		inputIDsTensor.Destroy()
		maskTensor.Destroy()
		typeTensor.Destroy()
		return nil, fmt.Errorf("creating output tensor: %w", err)
	}

	// Create a single persistent session.
	session, err := ort.NewAdvancedSession(
		modelPath,
		[]string{"input_ids", "attention_mask", "token_type_ids"},
		[]string{"last_hidden_state"},
		[]ort.Value{inputIDsTensor, maskTensor, typeTensor},
		[]ort.Value{outputTensor},
		nil,
	)
	if err != nil {
		inputIDsTensor.Destroy()
		maskTensor.Destroy()
		typeTensor.Destroy()
		outputTensor.Destroy()
		return nil, fmt.Errorf("creating ONNX session: %w", err)
	}

	return &OnnxProvider{
		tokenizer:      tokenizer,
		session:        session,
		logger:         logger,
		inputIDsTensor: inputIDsTensor,
		maskTensor:     maskTensor,
		typeTensor:     typeTensor,
		outputTensor:   outputTensor,
	}, nil
}

// Embed generates an embedding for a single text.
func (p *OnnxProvider) Embed(_ context.Context, text string) ([]float32, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.embedLocked(text)
}

// EmbedBatch generates embeddings for multiple texts.
func (p *OnnxProvider) EmbedBatch(_ context.Context, texts []string) ([][]float32, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	results := make([][]float32, len(texts))
	for i, text := range texts {
		emb, err := p.embedLocked(text)
		if err != nil {
			return nil, fmt.Errorf("embedding text %d: %w", i, err)
		}
		results[i] = emb
	}
	return results, nil
}

// embedLocked runs inference for one text. Caller must hold p.mu.
func (p *OnnxProvider) embedLocked(text string) ([]float32, error) {
	tokens := p.tokenizer.Tokenize(text)

	// Copy token data into the pre-allocated tensor buffers.
	copy(p.inputIDsTensor.GetData(), tokens.InputIDs)
	copy(p.maskTensor.GetData(), tokens.AttentionMask)
	copy(p.typeTensor.GetData(), tokens.TokenTypeIDs)

	if err := p.session.Run(); err != nil {
		return nil, fmt.Errorf("running inference: %w", err)
	}

	raw := p.outputTensor.GetData()
	seqLen := int(maxTokenLen)
	embedding := meanPool(raw, tokens.AttentionMask, seqLen, dimensions)
	return embedding, nil
}

// Dimensions returns the embedding vector dimensions.
func (p *OnnxProvider) Dimensions() int {
	return dimensions
}

// Destroy cleans up the ONNX session, tensors, and runtime environment.
func (p *OnnxProvider) Destroy() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.session != nil {
		p.session.Destroy()
	}
	if p.inputIDsTensor != nil {
		p.inputIDsTensor.Destroy()
	}
	if p.maskTensor != nil {
		p.maskTensor.Destroy()
	}
	if p.typeTensor != nil {
		p.typeTensor.Destroy()
	}
	if p.outputTensor != nil {
		p.outputTensor.Destroy()
	}
	ort.DestroyEnvironment()
}

// meanPool computes the mean of token embeddings, weighted by attention mask.
func meanPool(data []float32, mask []int64, seqLen, embDim int) []float32 {
	embedding := make([]float32, embDim)
	var maskSum float32

	for i := 0; i < seqLen; i++ {
		if mask[i] == 0 {
			continue
		}
		maskSum++
		for j := 0; j < embDim; j++ {
			embedding[j] += data[i*embDim+j]
		}
	}

	if maskSum > 0 {
		for j := range embedding {
			embedding[j] /= maskSum
		}
	}

	// L2 normalize
	var norm float32
	for _, v := range embedding {
		norm += v * v
	}
	norm = float32(math.Sqrt(float64(norm)))
	if norm > 0 {
		for j := range embedding {
			embedding[j] /= norm
		}
	}

	return embedding
}

// findOnnxRuntimeLib searches common locations for the ONNX Runtime shared library.
func findOnnxRuntimeLib() string {
	// Env override takes priority
	if v := os.Getenv("ONNXRUNTIME_LIB"); v != "" {
		return v
	}

	var candidates []string

	switch runtime.GOOS {
	case "darwin":
		candidates = []string{
			"/opt/homebrew/lib/libonnxruntime.dylib",
			"/usr/local/lib/libonnxruntime.dylib",
		}
	case "linux":
		candidates = []string{
			// Fedora (dnf install onnxruntime)
			"/usr/lib64/libonnxruntime.so",
			"/usr/lib64/libonnxruntime.so.1",
			// Ubuntu / Debian
			"/usr/lib/libonnxruntime.so",
			"/usr/local/lib/libonnxruntime.so",
			"/usr/lib/x86_64-linux-gnu/libonnxruntime.so",
		}
	}

	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return ""
}

// downloadIfMissing downloads a file from url to path if it doesn't exist.
func downloadIfMissing(path, url string, logger *slog.Logger) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	}

	logger.Info("Downloading model file", slog.String("path", path))

	resp, err := http.Get(url) //nolint:gosec // URL is a hardcoded constant
	if err != nil {
		return fmt.Errorf("fetching %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("fetching %s: status %d", url, resp.StatusCode)
	}

	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("creating %s: %w", path, err)
	}
	defer f.Close()

	if _, err := io.Copy(f, resp.Body); err != nil {
		os.Remove(path)
		return fmt.Errorf("writing %s: %w", path, err)
	}

	return nil
}
