package polarquant

import (
	"math"
	"testing"
)

func TestCartesianToPolarZeroVector(t *testing.T) {
	vec := []float32{0, 0, 0, 0}
	radius, angles := CartesianToPolar(vec)
	if radius != 0 {
		t.Errorf("expected radius 0, got %f", radius)
	}
	if len(angles) != 3 {
		t.Fatalf("expected 3 angles, got %d", len(angles))
	}
	for i, a := range angles {
		if a != 0 {
			t.Errorf("angle[%d] = %f, want 0", i, a)
		}
	}
}

func TestCartesianToPolarEmptyVector(t *testing.T) {
	radius, angles := CartesianToPolar([]float32{})
	if radius != 0 {
		t.Errorf("expected radius 0, got %f", radius)
	}
	if angles != nil {
		t.Errorf("expected nil angles, got %v", angles)
	}
}

func TestCartesianToPolarSingleDim(t *testing.T) {
	radius, angles := CartesianToPolar([]float32{5.0})
	if math.Abs(float64(radius)-5.0) > 1e-5 {
		t.Errorf("expected radius 5.0, got %f", radius)
	}
	if len(angles) != 0 {
		t.Errorf("expected 0 angles, got %d", len(angles))
	}
}

func TestCartesianToPolarTwoDim(t *testing.T) {
	// (3, 4) -> radius=5, angle=arccos(3/5)
	radius, angles := CartesianToPolar([]float32{3, 4})
	if math.Abs(float64(radius)-5.0) > 1e-4 {
		t.Errorf("expected radius ~5.0, got %f", radius)
	}
	if len(angles) != 1 {
		t.Fatalf("expected 1 angle, got %d", len(angles))
	}
	expected := math.Acos(3.0 / 5.0)
	if math.Abs(angles[0]-expected) > 1e-10 {
		t.Errorf("angle = %f, want %f", angles[0], expected)
	}
}

func TestCartesianToPolarNegativeLastDim(t *testing.T) {
	// Test the branch where vec[n-1] < 0 (angle wraps to [pi, 2*pi])
	radius, angles := CartesianToPolar([]float32{1, -1})
	if radius == 0 {
		t.Error("expected non-zero radius")
	}
	// Last angle should be > pi since last component is negative
	if angles[0] <= math.Pi {
		t.Errorf("expected angle > pi for negative last dim, got %f", angles[0])
	}
}

func TestPolarRoundTrip(t *testing.T) {
	vec := []float32{1.0, 2.0, 3.0, 4.0}
	radius, angles := CartesianToPolar(vec)
	recovered := PolarToCartesian(radius, angles)
	if len(recovered) != len(vec) {
		t.Fatalf("length mismatch: %d vs %d", len(recovered), len(vec))
	}
	for i := range vec {
		if math.Abs(float64(vec[i]-recovered[i])) > 1e-4 {
			t.Errorf("dim %d: got %f, want %f", i, recovered[i], vec[i])
		}
	}
}

func TestCosineSimilarityZeroVectors(t *testing.T) {
	zero := []float32{0, 0, 0}
	nonzero := []float32{1, 2, 3}

	if sim := CosineSimilarity(zero, nonzero); sim != 0 {
		t.Errorf("expected 0 for zero first vector, got %f", sim)
	}
	if sim := CosineSimilarity(nonzero, zero); sim != 0 {
		t.Errorf("expected 0 for zero second vector, got %f", sim)
	}
	if sim := CosineSimilarity(zero, zero); sim != 0 {
		t.Errorf("expected 0 for both zero vectors, got %f", sim)
	}
}

func TestCosineSimilarityIdentical(t *testing.T) {
	v := []float32{1, 2, 3, 4, 5}
	sim := CosineSimilarity(v, v)
	if math.Abs(sim-1.0) > 1e-10 {
		t.Errorf("expected ~1.0 for identical vectors, got %f", sim)
	}
}

func TestQuantizeDequantizeRoundTrip(t *testing.T) {
	for _, bits := range []int{2, 4, 8} {
		angles := []float64{0.5, 1.0, 2.5} // last angle can be up to 2*pi
		packed := QuantizeAngles(angles, bits)
		recovered := DequantizeAngles(packed, len(angles), bits)
		if len(recovered) != len(angles) {
			t.Fatalf("bits=%d: length mismatch", bits)
		}
		// Higher bits => better accuracy
		levels := 1 << bits
		maxErr := math.Pi / float64(levels) // rough max quantization error
		for i := range angles {
			diff := math.Abs(recovered[i] - angles[i])
			if diff > maxErr+0.1 { // some tolerance
				t.Errorf("bits=%d, angle[%d]: got %f, want ~%f (diff=%f)", bits, i, recovered[i], angles[i], diff)
			}
		}
	}
}

func TestMarshalUnmarshalRoundTrip(t *testing.T) {
	cv := &CompressedVector{
		Radius:       3.14,
		Angles:       []byte{0xAB, 0xCD},
		Dims:         10,
		BitsPerAngle: 4,
	}
	data, err := cv.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	recovered, err := Unmarshal(data)
	if err != nil {
		t.Fatal(err)
	}
	if recovered.Radius != cv.Radius {
		t.Errorf("radius: got %f, want %f", recovered.Radius, cv.Radius)
	}
	if recovered.Dims != cv.Dims {
		t.Errorf("dims: got %d, want %d", recovered.Dims, cv.Dims)
	}
	if recovered.BitsPerAngle != cv.BitsPerAngle {
		t.Errorf("bitsPerAngle: got %d, want %d", recovered.BitsPerAngle, cv.BitsPerAngle)
	}
}

func TestUnmarshalInvalidJSON(t *testing.T) {
	_, err := Unmarshal([]byte("not valid json"))
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestDecompressEmbeddingInvalidData(t *testing.T) {
	store, err := NewStore(8, 4)
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.DecompressEmbedding([]byte("bad data"))
	if err == nil {
		t.Error("expected error for invalid compressed data")
	}
}

func TestApproximateCosineSimilarityInvalidA(t *testing.T) {
	store, err := NewStore(8, 4)
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.ApproximateCosineSimilarity([]byte("bad"), []byte("bad"))
	if err == nil {
		t.Error("expected error for invalid first argument")
	}
}

func TestApproximateCosineSimilarityInvalidB(t *testing.T) {
	store, err := NewStore(8, 4)
	if err != nil {
		t.Fatal(err)
	}
	// Compress a valid vector for A
	vec := make([]float32, 8)
	for i := range vec {
		vec[i] = float32(i + 1)
	}
	dataA, err := store.CompressEmbedding(vec)
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.ApproximateCosineSimilarity(dataA, []byte("bad"))
	if err == nil {
		t.Error("expected error for invalid second argument")
	}
}

func TestByteSizeAndCompressionRatio(t *testing.T) {
	cv := &CompressedVector{
		Radius:       1.0,
		Angles:       make([]byte, 192), // simulating 384-dim at 4-bit
		Dims:         384,
		BitsPerAngle: 4,
	}
	size := cv.ByteSize()
	if size != 4+192 {
		t.Errorf("ByteSize: got %d, want %d", size, 196)
	}
	ratio := cv.CompressionRatio()
	expected := float64(384*4) / float64(196)
	if math.Abs(ratio-expected) > 1e-10 {
		t.Errorf("CompressionRatio: got %f, want %f", ratio, expected)
	}
}

func TestCartesianToPolarPartialRadiusZero(t *testing.T) {
	// Vector where trailing components are zero, triggering partialRadius < 1e-10
	vec := []float32{1.0, 0, 0, 0}
	radius, angles := CartesianToPolar(vec)
	if math.Abs(float64(radius)-1.0) > 1e-5 {
		t.Errorf("expected radius ~1.0, got %f", radius)
	}
	// The partial radius from dim 1 onward is 0, so those angles should be 0
	for i := 1; i < len(angles); i++ {
		if angles[i] != 0 {
			t.Errorf("angle[%d] = %f, want 0 (partial radius is zero)", i, angles[i])
		}
	}
}
