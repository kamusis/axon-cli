package index

import "math"

// Cosine computes cosine similarity between two vectors of equal length.
func Cosine(a, b []float32) (float64, error) {
	if len(a) != len(b) {
		return 0, ErrVectorLengthMismatch
	}
	var dot float64
	var na float64
	var nb float64
	for i := 0; i < len(a); i++ {
		x := float64(a[i])
		y := float64(b[i])
		dot += x * y
		na += x * x
		nb += y * y
	}
	den := math.Sqrt(na) * math.Sqrt(nb)
	if den == 0 {
		return 0, nil
	}
	return dot / den, nil
}

// NormalizeL2 returns a new vector normalized to unit L2 norm.
func NormalizeL2(v []float32) []float32 {
	var sum float64
	for _, x := range v {
		sum += float64(x) * float64(x)
	}
	n := math.Sqrt(sum)
	if n == 0 {
		out := make([]float32, len(v))
		copy(out, v)
		return out
	}
	out := make([]float32, len(v))
	inv := float32(1.0 / n)
	for i := range v {
		out[i] = v[i] * inv
	}
	return out
}
