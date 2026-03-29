package polarquant

import (
	"math"
	"math/rand"
	"testing"
)

func TestStoreRoundTrip(t *testing.T) {
	store, err := NewStore(384, 4)
	if err != nil {
		t.Fatal(err)
	}

	// Generate random embedding
	rng := rand.New(rand.NewSource(42))
	vec := make([]float32, 384)
	var norm float64
	for i := range vec {
		vec[i] = float32(rng.NormFloat64())
		norm += float64(vec[i]) * float64(vec[i])
	}
	norm = math.Sqrt(norm)
	for i := range vec {
		vec[i] /= float32(norm)
	}

	// Compress
	data, err := store.CompressEmbedding(vec)
	if err != nil {
		t.Fatal(err)
	}

	// Decompress
	recovered, err := store.DecompressEmbedding(data)
	if err != nil {
		t.Fatal(err)
	}

	if len(recovered) != 384 {
		t.Fatalf("expected 384 dims, got %d", len(recovered))
	}

	// Check similarity preserved
	sim := CosineSimilarity(vec, recovered)
	t.Logf("Compressed: %d bytes (%.1fx), cosine similarity: %.4f", len(data), float64(384*4)/float64(len(data)), sim)

	if sim < 0.5 {
		t.Errorf("cosine similarity too low: %.4f", sim)
	}
}

func TestStoreWrongDims(t *testing.T) {
	store, err := NewStore(384, 4)
	if err != nil {
		t.Fatal(err)
	}

	_, err = store.CompressEmbedding(make([]float32, 100))
	if err == nil {
		t.Error("expected error for wrong dims")
	}
}

func TestStoreStats(t *testing.T) {
	store, err := NewStore(384, 4)
	if err != nil {
		t.Fatal(err)
	}

	stats := store.CompressionStats()
	if stats["dims"] != 384 {
		t.Errorf("expected dims=384, got %v", stats["dims"])
	}
	ratio := stats["ratio"].(float64)
	if ratio < 5.0 {
		t.Errorf("expected ratio > 5x, got %.1f", ratio)
	}
	t.Logf("Stats: %v", stats)
}

func TestApproximateSimilarity(t *testing.T) {
	store, err := NewStore(384, 4)
	if err != nil {
		t.Fatal(err)
	}

	rng := rand.New(rand.NewSource(99))

	// Two similar vectors
	vecA := make([]float32, 384)
	vecB := make([]float32, 384)
	for i := range vecA {
		vecA[i] = float32(rng.NormFloat64())
		vecB[i] = vecA[i] + float32(rng.NormFloat64()*0.1) // small noise
	}

	dataA, _ := store.CompressEmbedding(vecA)
	dataB, _ := store.CompressEmbedding(vecB)

	sim, err := store.ApproximateCosineSimilarity(dataA, dataB)
	if err != nil {
		t.Fatal(err)
	}

	trueSim := CosineSimilarity(vecA, vecB)
	t.Logf("True similarity: %.4f, Approximate: %.4f", trueSim, sim)

	// Both should indicate high similarity
	if trueSim < 0.8 {
		t.Errorf("true similarity unexpectedly low: %.4f", trueSim)
	}
}

func BenchmarkStoreCompress(b *testing.B) {
	store, _ := NewStore(384, 4)
	rng := rand.New(rand.NewSource(42))
	vec := make([]float32, 384)
	for i := range vec {
		vec[i] = float32(rng.NormFloat64())
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		store.CompressEmbedding(vec)
	}
}

func BenchmarkStoreDecompress(b *testing.B) {
	store, _ := NewStore(384, 4)
	rng := rand.New(rand.NewSource(42))
	vec := make([]float32, 384)
	for i := range vec {
		vec[i] = float32(rng.NormFloat64())
	}
	data, _ := store.CompressEmbedding(vec)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		store.DecompressEmbedding(data)
	}
}
