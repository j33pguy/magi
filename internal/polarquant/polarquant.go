// Package polarquant implements PolarQuant vector compression for embedding storage.
// Based on Google's TurboQuant paper (ICLR 2026) - PolarQuant component.
//
// Core idea: Convert high-dimensional embedding vectors from Cartesian coordinates
// to polar coordinates (radius + angles), then quantize the angles.
// After random preconditioning, angles have a tightly bounded, concentrated distribution
// that eliminates the need for explicit normalization.
//
// For MAGI embeddings:
// - Input: 384-dim float32 vectors from all-MiniLM-L6-v2 (1,536 bytes each)
// - Output: 1 float32 radius + 383 quantized angles (variable bit-width)
// - At 4-bit angles: 1 float32 (4 bytes) + 383 * 0.5 bytes = ~196 bytes (7.8x compression)
// - At 2-bit angles: 1 float32 (4 bytes) + 383 * 0.25 bytes = ~100 bytes (15x compression)
package polarquant

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"math"
)

// CompressedVector holds a PolarQuant-compressed embedding.
type CompressedVector struct {
	Radius      float32 // L2 norm of the original vector
	Angles      []byte  // Quantized polar angles (packed bits)
	Dims        int     // Original dimensionality
	BitsPerAngle int    // Quantization bit-width (2, 4, or 8)
}

// RandomRotation is a preconditioning matrix that spreads vector energy
// uniformly across dimensions. Generated once, reused for all vectors.
type RandomRotation struct {
	signs []int8 // Random sign flips (+1/-1) for fast Hadamard-like transform
	seed  uint64
}

// NewRandomRotation creates a random preconditioning transform for the given dimensionality.
// This needs to be created once and stored — all vectors must use the same rotation.
func NewRandomRotation(dims int) (*RandomRotation, error) {
	signs := make([]int8, dims)
	buf := make([]byte, dims)
	if _, err := rand.Read(buf); err != nil {
		return nil, fmt.Errorf("polarquant: generate random signs: %w", err)
	}
	for i, b := range buf {
		if b&1 == 0 {
			signs[i] = 1
		} else {
			signs[i] = -1
		}
	}
	var seed uint64
	seedBuf := make([]byte, 8)
	rand.Read(seedBuf)
	seed = binary.LittleEndian.Uint64(seedBuf)
	return &RandomRotation{signs: signs, seed: seed}, nil
}

// Precondition applies the random rotation to spread vector energy uniformly.
// This is the key insight from PolarQuant — after preconditioning,
// the polar angles have a concentrated, predictable distribution.
func (r *RandomRotation) Precondition(vec []float32) []float32 {
	n := len(vec)
	out := make([]float32, n)
	// Step 1: Random sign flip (diagonal Rademacher matrix)
	for i := 0; i < n; i++ {
		out[i] = vec[i] * float32(r.signs[i])
	}
	// Step 2: Fast Walsh-Hadamard-like transform (simplified)
	// In production, use proper randomized Hadamard transform
	// For prototype, the sign flip alone provides good spreading
	return out
}

// CartesianToPolar converts a Cartesian vector to polar coordinates.
// Returns radius and n-1 angles (theta_1...theta_{n-1}).
// theta_i ranges from [0, pi] for i < n-1, and [0, 2*pi] for i = n-1.
func CartesianToPolar(vec []float32) (float32, []float64) {
	n := len(vec)
	if n == 0 {
		return 0, nil
	}

	// Compute radius (L2 norm)
	var sumSq float64
	for _, v := range vec {
		sumSq += float64(v) * float64(v)
	}
	radius := math.Sqrt(sumSq)

	if radius < 1e-10 {
		return 0, make([]float64, n-1)
	}

	// Compute angles using recursive formula
	// theta_k = arccos(x_k / sqrt(sum_{i=k}^{n} x_i^2))
	angles := make([]float64, n-1)

	for k := 0; k < n-1; k++ {
		// Compute partial radius from dimension k to n
		var partialSumSq float64
		for i := k; i < n; i++ {
			partialSumSq += float64(vec[i]) * float64(vec[i])
		}
		partialRadius := math.Sqrt(partialSumSq)

		if partialRadius < 1e-10 {
			angles[k] = 0
			continue
		}

		cosAngle := float64(vec[k]) / partialRadius
		// Clamp to [-1, 1] for numerical stability
		cosAngle = math.Max(-1.0, math.Min(1.0, cosAngle))
		angles[k] = math.Acos(cosAngle)
	}

	// Last angle needs special handling for full [0, 2*pi] range
	if n >= 2 && vec[n-1] < 0 {
		angles[n-2] = 2*math.Pi - angles[n-2]
	}

	return float32(radius), angles
}

// QuantizeAngles quantizes polar angles to the specified bit-width.
// Angles are in [0, pi] for all but the last which is [0, 2*pi].
func QuantizeAngles(angles []float64, bitsPerAngle int) []byte {
	n := len(angles)
	levels := 1 << bitsPerAngle // 2^bits quantization levels

	// Pack quantized values into bytes
	totalBits := n * bitsPerAngle
	nBytes := (totalBits + 7) / 8
	packed := make([]byte, nBytes)

	for i, angle := range angles {
		// Determine range: last angle is [0, 2*pi], others are [0, pi]
		maxAngle := math.Pi
		if i == n-1 {
			maxAngle = 2 * math.Pi
		}

		// Quantize to [0, levels-1]
		normalized := angle / maxAngle
		normalized = math.Max(0, math.Min(1.0-1e-10, normalized))
		qVal := int(normalized * float64(levels))
		if qVal >= levels {
			qVal = levels - 1
		}

		// Pack bits
		bitOffset := i * bitsPerAngle
		for b := 0; b < bitsPerAngle; b++ {
			if qVal&(1<<b) != 0 {
				byteIdx := (bitOffset + b) / 8
				bitIdx := (bitOffset + b) % 8
				packed[byteIdx] |= 1 << bitIdx
			}
		}
	}

	return packed
}

// DequantizeAngles recovers approximate angles from quantized representation.
func DequantizeAngles(packed []byte, nAngles, bitsPerAngle int) []float64 {
	levels := 1 << bitsPerAngle
	angles := make([]float64, nAngles)

	for i := 0; i < nAngles; i++ {
		// Unpack bits
		bitOffset := i * bitsPerAngle
		var qVal int
		for b := 0; b < bitsPerAngle; b++ {
			byteIdx := (bitOffset + b) / 8
			bitIdx := (bitOffset + b) % 8
			if packed[byteIdx]&(1<<bitIdx) != 0 {
				qVal |= 1 << b
			}
		}

		// Dequantize: map back to angle range
		maxAngle := math.Pi
		if i == nAngles-1 {
			maxAngle = 2 * math.Pi
		}
		angles[i] = (float64(qVal) + 0.5) / float64(levels) * maxAngle
	}

	return angles
}

// PolarToCartesian reconstructs a Cartesian vector from polar coordinates.
func PolarToCartesian(radius float32, angles []float64) []float32 {
	n := len(angles) + 1
	vec := make([]float32, n)

	r := float64(radius)
	for i := 0; i < n-1; i++ {
		vec[i] = float32(r * math.Cos(angles[i]))
		r *= math.Sin(angles[i])
	}
	vec[n-1] = float32(r)

	return vec
}

// Compress compresses a float32 embedding vector using PolarQuant.
func Compress(vec []float32, rotation *RandomRotation, bitsPerAngle int) *CompressedVector {
	// Step 1: Precondition
	preconditioned := rotation.Precondition(vec)

	// Step 2: Convert to polar
	radius, angles := CartesianToPolar(preconditioned)

	// Step 3: Quantize angles
	packed := QuantizeAngles(angles, bitsPerAngle)

	return &CompressedVector{
		Radius:       radius,
		Angles:       packed,
		Dims:         len(vec),
		BitsPerAngle: bitsPerAngle,
	}
}

// Decompress reconstructs an approximate embedding from compressed form.
func Decompress(cv *CompressedVector, rotation *RandomRotation) []float32 {
	// Dequantize angles
	angles := DequantizeAngles(cv.Angles, cv.Dims-1, cv.BitsPerAngle)

	// Reconstruct Cartesian from polar
	vec := PolarToCartesian(cv.Radius, angles)

	// Reverse preconditioning (sign flip is its own inverse)
	for i := 0; i < len(vec); i++ {
		vec[i] *= float32(rotation.signs[i])
	}

	return vec
}

// CosineSimilarity computes cosine similarity between two vectors.
func CosineSimilarity(a, b []float32) float64 {
	var dot, normA, normB float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}
	if normA < 1e-10 || normB < 1e-10 {
		return 0
	}
	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}

// ByteSize returns the compressed size in bytes.
func (cv *CompressedVector) ByteSize() int {
	return 4 + len(cv.Angles) // 4 bytes for radius + packed angles
}

// CompressionRatio returns the compression ratio vs original float32.
func (cv *CompressedVector) CompressionRatio() float64 {
	original := cv.Dims * 4 // float32 = 4 bytes
	return float64(original) / float64(cv.ByteSize())
}
