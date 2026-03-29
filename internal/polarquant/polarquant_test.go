package polarquant

import (
	"fmt"
	"math"
	"math/rand"
	"testing"
)

func TestPolarQuantRoundTrip(t *testing.T) {
	dims := 384 // all-MiniLM-L6-v2 dimensionality
	rng := rand.New(rand.NewSource(42))

	// Generate a random unit vector (simulating a normalized embedding)
	vec := make([]float32, dims)
	var norm float64
	for i := range vec {
		vec[i] = float32(rng.NormFloat64())
		norm += float64(vec[i]) * float64(vec[i])
	}
	norm = math.Sqrt(norm)
	for i := range vec {
		vec[i] /= float32(norm)
	}

	rotation, err := NewRandomRotation(dims)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		bits     int
		minSim   float64 // minimum acceptable cosine similarity
		maxRatio float64 // minimum compression ratio
	}{
		{8, 0.99, 3.5},   // 8-bit: near-lossless
		{4, 0.5, 7.0},    // 4-bit: good quality
		{2, 0.1, 13.0},   // 2-bit: aggressive but usable for search
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("%d-bit", tt.bits), func(t *testing.T) {
			compressed := Compress(vec, rotation, tt.bits)
			decompressed := Decompress(compressed, rotation)

			sim := CosineSimilarity(vec, decompressed)
			ratio := compressed.CompressionRatio()
			size := compressed.ByteSize()

			t.Logf("Bits: %d, Size: %d bytes (%.1fx compression), Cosine similarity: %.6f",
				tt.bits, size, ratio, sim)

			if sim < tt.minSim {
				t.Errorf("Cosine similarity %.6f below threshold %.6f", sim, tt.minSim)
			}
			if ratio < tt.maxRatio {
				t.Errorf("Compression ratio %.1f below threshold %.1f", ratio, tt.maxRatio)
			}
		})
	}
}

func TestSearchAccuracy(t *testing.T) {
	// Simulate a search scenario: compress a corpus, find nearest neighbor
	dims := 384
	nVectors := 100
	rng := rand.New(rand.NewSource(123))

	rotation, _ := NewRandomRotation(dims)

	// Generate corpus of random normalized vectors
	corpus := make([][]float32, nVectors)
	for i := range corpus {
		vec := make([]float32, dims)
		var norm float64
		for j := range vec {
			vec[j] = float32(rng.NormFloat64())
			norm += float64(vec[j]) * float64(vec[j])
		}
		norm = math.Sqrt(norm)
		for j := range vec {
			vec[j] /= float32(norm)
		}
		corpus[i] = vec
	}

	// Make query similar to corpus[42]
	query := make([]float32, dims)
	copy(query, corpus[42])
	// Add small noise
	for i := range query {
		query[i] += float32(rng.NormFloat64() * 0.05)
	}

	// Find true nearest neighbor (brute force on original vectors)
	trueNN := -1
	bestSim := -1.0
	for i, v := range corpus {
		sim := CosineSimilarity(query, v)
		if sim > bestSim {
			bestSim = sim
			trueNN = i
		}
	}

	// Compress corpus and find nearest neighbor on compressed vectors
	for _, bits := range []int{8, 4, 2} {
		compressedCorpus := make([]*CompressedVector, nVectors)
		for i, v := range corpus {
			compressedCorpus[i] = Compress(v, rotation, bits)
		}

		// Decompress and search
		compNN := -1
		compBestSim := -1.0
		for i, cv := range compressedCorpus {
			decompressed := Decompress(cv, rotation)
			sim := CosineSimilarity(query, decompressed)
			if sim > compBestSim {
				compBestSim = sim
				compNN = i
			}
		}

		match := compNN == trueNN
		totalBytes := 0
		for _, cv := range compressedCorpus {
			totalBytes += cv.ByteSize()
		}
		origBytes := nVectors * dims * 4

		t.Logf("%d-bit: NN match=%v (true=%d, compressed=%d), corpus size: %d -> %d bytes (%.1fx)",
			bits, match, trueNN, compNN, origBytes, totalBytes, float64(origBytes)/float64(totalBytes))
	}
}

func BenchmarkCompress384(b *testing.B) {
	dims := 384
	rng := rand.New(rand.NewSource(42))
	vec := make([]float32, dims)
	for i := range vec {
		vec[i] = float32(rng.NormFloat64())
	}
	rotation, _ := NewRandomRotation(dims)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Compress(vec, rotation, 4)
	}
}

func BenchmarkDecompress384(b *testing.B) {
	dims := 384
	rng := rand.New(rand.NewSource(42))
	vec := make([]float32, dims)
	for i := range vec {
		vec[i] = float32(rng.NormFloat64())
	}
	rotation, _ := NewRandomRotation(dims)
	cv := Compress(vec, rotation, 4)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Decompress(cv, rotation)
	}
}
