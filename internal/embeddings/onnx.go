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
	"strconv"
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

// workerSession holds a single ONNX inference session and its pre-allocated tensors.
// Each worker is independent and can run inference concurrently with other workers.
type workerSession struct {
	session        *ort.AdvancedSession
	inputIDsTensor *ort.Tensor[int64]
	maskTensor     *ort.Tensor[int64]
	typeTensor     *ort.Tensor[int64]
	outputTensor   *ort.Tensor[float32]
}

// OnnxProvider generates embeddings using all-MiniLM-L6-v2 via ONNX runtime.
// Uses a pool of worker sessions for concurrent embedding generation.
// Each worker has its own ONNX session and pre-allocated tensors.
type OnnxProvider struct {
	tokenizer  *Tokenizer
	logger     *slog.Logger
	workers    chan *workerSession // buffered channel acts as a worker pool
	numWorkers int
	embPool    sync.Pool // pool for []float32 embedding output buffers
}

// newWorkerSession creates a single ONNX inference worker with its own session and tensors.
func newWorkerSession(modelPath string) (*workerSession, error) {
	batchSize := int64(1)
	seqLen := int64(maxTokenLen)

	sessionOptions, err := ort.NewSessionOptions()
	if err != nil {
		return nil, fmt.Errorf("creating session options: %w", err)
	}
	defer sessionOptions.Destroy()

	intraThreads := onnxThreadEnv("MAGI_ONNX_INTRA_THREADS", 1)
	interThreads := onnxThreadEnv("MAGI_ONNX_INTER_THREADS", 1)
	if err := sessionOptions.SetIntraOpNumThreads(intraThreads); err != nil {
		return nil, fmt.Errorf("setting intra-op threads: %w", err)
	}
	if err := sessionOptions.SetInterOpNumThreads(interThreads); err != nil {
		return nil, fmt.Errorf("setting inter-op threads: %w", err)
	}
	if os.Getenv("MAGI_ONNX_EXECUTION_MODE") == "parallel" {
		if err := sessionOptions.SetExecutionMode(ort.ExecutionModeParallel); err != nil {
			return nil, fmt.Errorf("setting execution mode parallel: %w", err)
		}
	} else {
		if err := sessionOptions.SetExecutionMode(ort.ExecutionModeSequential); err != nil {
			return nil, fmt.Errorf("setting execution mode sequential: %w", err)
		}
	}

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

	session, err := ort.NewAdvancedSession(
		modelPath,
		[]string{"input_ids", "attention_mask", "token_type_ids"},
		[]string{"last_hidden_state"},
		[]ort.Value{inputIDsTensor, maskTensor, typeTensor},
		[]ort.Value{outputTensor},
		sessionOptions,
	)
	if err != nil {
		inputIDsTensor.Destroy()
		maskTensor.Destroy()
		typeTensor.Destroy()
		outputTensor.Destroy()
		return nil, fmt.Errorf("creating ONNX session: %w", err)
	}

	return &workerSession{
		session:        session,
		inputIDsTensor: inputIDsTensor,
		maskTensor:     maskTensor,
		typeTensor:     typeTensor,
		outputTensor:   outputTensor,
	}, nil
}

func (w *workerSession) destroy() {
	if w.session != nil {
		w.session.Destroy()
	}
	if w.inputIDsTensor != nil {
		w.inputIDsTensor.Destroy()
	}
	if w.maskTensor != nil {
		w.maskTensor.Destroy()
	}
	if w.typeTensor != nil {
		w.typeTensor.Destroy()
	}
	if w.outputTensor != nil {
		w.outputTensor.Destroy()
	}
}

// NewOnnxProvider creates a new ONNX embedding provider with a pool of worker
// sessions for concurrent embedding generation. Downloads the model and vocab
// on first use if not already cached.
func NewOnnxProvider(logger *slog.Logger) (*OnnxProvider, error) {
	modelDir := os.Getenv("MAGI_MODEL_DIR")
	if modelDir == "" {
		home, _ := os.UserHomeDir()
		modelDir = filepath.Join(home, ".magi", "models")
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

	// Determine worker count: default to NumCPU, capped at 8, overridable via env.
	numWorkers := runtime.NumCPU()
	if numWorkers > 8 {
		numWorkers = 8
	}
	if numWorkers < 1 {
		numWorkers = 1
	}
	if v := os.Getenv("MAGI_EMBED_WORKERS"); v != "" {
		var n int
		if _, err := fmt.Sscanf(v, "%d", &n); err == nil && n > 0 {
			numWorkers = n
		}
	}

	workers := make(chan *workerSession, numWorkers)
	for i := 0; i < numWorkers; i++ {
		w, err := newWorkerSession(modelPath)
		if err != nil {
			// Clean up already-created workers.
			close(workers)
			for w := range workers {
				w.destroy()
			}
			return nil, fmt.Errorf("creating worker %d: %w", i, err)
		}
		workers <- w
	}

	logger.Info("ONNX embedding pool ready", "workers", numWorkers)

	return &OnnxProvider{
		tokenizer:  tokenizer,
		logger:     logger,
		workers:    workers,
		numWorkers: numWorkers,
		embPool: sync.Pool{
			New: func() any {
				buf := make([]float32, dimensions)
				return &buf
			},
		},
	}, nil
}

// Embed generates an embedding for a single text.
func (p *OnnxProvider) Embed(_ context.Context, text string) ([]float32, error) {
	w := <-p.workers
	defer func() { p.workers <- w }()
	return p.embedWithWorker(w, text)
}

// EmbedBatch generates embeddings for multiple texts concurrently using the
// worker pool. Each worker runs inference independently, bounded by the pool size.
func (p *OnnxProvider) EmbedBatch(_ context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}
	if len(texts) == 1 {
		w := <-p.workers
		defer func() { p.workers <- w }()
		emb, err := p.embedWithWorker(w, texts[0])
		if err != nil {
			return nil, err
		}
		return [][]float32{emb}, nil
	}

	results := make([][]float32, len(texts))
	errs := make([]error, len(texts))

	var wg sync.WaitGroup
	wg.Add(len(texts))

	for i, text := range texts {
		go func(idx int, t string) {
			defer wg.Done()
			w := <-p.workers // borrow worker (blocks if all busy)
			emb, err := p.embedWithWorker(w, t)
			p.workers <- w // return worker
			results[idx] = emb
			errs[idx] = err
		}(i, text)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			return nil, fmt.Errorf("embedding text %d: %w", i, err)
		}
	}
	return results, nil
}

// embedWithWorker runs inference for one text using the given worker session.
func (p *OnnxProvider) embedWithWorker(w *workerSession, text string) ([]float32, error) {
	tokens := p.tokenizer.Tokenize(text)

	// Copy token data into the worker's pre-allocated tensor buffers.
	copy(w.inputIDsTensor.GetData(), tokens.InputIDs)
	copy(w.maskTensor.GetData(), tokens.AttentionMask)
	copy(w.typeTensor.GetData(), tokens.TokenTypeIDs)

	if err := w.session.Run(); err != nil {
		return nil, fmt.Errorf("running inference: %w", err)
	}

	raw := w.outputTensor.GetData()
	seqLen := int(maxTokenLen)
	embedding := meanPool(raw, tokens.AttentionMask, seqLen, dimensions)
	return embedding, nil
}

// Dimensions returns the embedding vector dimensions.
func (p *OnnxProvider) Dimensions() int {
	return dimensions
}

// Destroy cleans up all worker sessions, tensors, and the runtime environment.
func (p *OnnxProvider) Destroy() {
	// Drain all workers from the pool and destroy them.
	for i := 0; i < p.numWorkers; i++ {
		w := <-p.workers
		w.destroy()
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

func onnxThreadEnv(name string, def int) int {
	v := os.Getenv(name)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 1 {
		return def
	}
	return n
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
