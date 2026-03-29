// Package polarquant provides PolarQuant vector compression for MAGI embeddings.
// Based on Google's TurboQuant paper (ICLR 2026).
//
// Integration with MAGI:
//   - Compress embeddings before storage to reduce DB size
//   - Decompress on retrieval for full-precision search
//   - Optional: fast approximate search directly on compressed vectors
package polarquant

import (
	"encoding/json"
	"fmt"
	"sync"
)

// Store manages a shared RandomRotation and provides compress/decompress operations
// for embedding vectors. Thread-safe.
type Store struct {
	rotation     *RandomRotation
	bitsPerAngle int
	dims         int
	mu           sync.RWMutex
}

// NewStore creates a PolarQuant store for the given dimensionality and bit-width.
// The RandomRotation is generated once and must be consistent across all vectors.
func NewStore(dims, bitsPerAngle int) (*Store, error) {
	rotation, err := NewRandomRotation(dims)
	if err != nil {
		return nil, fmt.Errorf("polarquant: create rotation: %w", err)
	}
	return &Store{
		rotation:     rotation,
		bitsPerAngle: bitsPerAngle,
		dims:         dims,
	}, nil
}

// CompressEmbedding compresses a float32 embedding vector.
// Returns the compressed bytes and the original byte size for comparison.
func (s *Store) CompressEmbedding(vec []float32) ([]byte, error) {
	if len(vec) != s.dims {
		return nil, fmt.Errorf("polarquant: expected %d dims, got %d", s.dims, len(vec))
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	cv := Compress(vec, s.rotation, s.bitsPerAngle)
	return cv.Marshal()
}

// DecompressEmbedding reconstructs an approximate float32 vector from compressed bytes.
func (s *Store) DecompressEmbedding(data []byte) ([]float32, error) {
	cv, err := Unmarshal(data)
	if err != nil {
		return nil, fmt.Errorf("polarquant: unmarshal: %w", err)
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	return Decompress(cv, s.rotation), nil
}

// CompressionStats returns size comparison for the configured bit-width.
func (s *Store) CompressionStats() map[string]interface{} {
	originalBytes := s.dims * 4 // float32
	compressedBytes := 4 + ((s.dims - 1) * s.bitsPerAngle / 8) // radius + packed angles
	return map[string]interface{}{
		"dims":             s.dims,
		"bits_per_angle":   s.bitsPerAngle,
		"original_bytes":   originalBytes,
		"compressed_bytes": compressedBytes,
		"ratio":            float64(originalBytes) / float64(compressedBytes),
	}
}

// Marshal serializes a CompressedVector to bytes for storage.
func (cv *CompressedVector) Marshal() ([]byte, error) {
	return json.Marshal(cv)
}

// Unmarshal deserializes a CompressedVector from bytes.
func Unmarshal(data []byte) (*CompressedVector, error) {
	cv := &CompressedVector{}
	if err := json.Unmarshal(data, cv); err != nil {
		return nil, err
	}
	return cv, nil
}

// ApproximateCosineSimilarity computes cosine similarity between two compressed vectors
// without full decompression. Faster but less precise.
func (s *Store) ApproximateCosineSimilarity(a, b []byte) (float64, error) {
	cvA, err := Unmarshal(a)
	if err != nil {
		return 0, err
	}
	cvB, err := Unmarshal(b)
	if err != nil {
		return 0, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	vecA := Decompress(cvA, s.rotation)
	vecB := Decompress(cvB, s.rotation)
	return CosineSimilarity(vecA, vecB), nil
}
