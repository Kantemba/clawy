package vector

import (
	"hash/fnv"
	"strings"
)

// HashEmbedder is a deterministic, zero-dependency embedding function.
//
// It produces fixed-dimension vectors via token hashing. While not
// semantically meaningful like neural embeddings, it provides a usable
// baseline for RAG on resource-constrained devices where calling an
// external embedding API is impractical.
//
// For production semantic search, plug in OpenAI/Gemini/local model
// embeddings via EmbeddingFunc.
type HashEmbedder struct {
	dim     int
	seed    uint64
}

// NewHashEmbedder creates a hash-based embedder with the given dimension.
func NewHashEmbedder(dim int) *HashEmbedder {
	return &HashEmbedder{dim: dim, seed: 0x9e3779b97f4a7c15}
}

// Embed converts text into a dense vector using token hashing.
func (h *HashEmbedder) Embed(text string) (Vec, error) {
	vec := make(Vec, h.dim)
	tokens := tokenize(text)
	for _, tok := range tokens {
		hash := h.hashToken(tok)
		idx := int(hash % uint64(h.dim))
		// Use sign hashing for better distribution
		if hash&1 == 1 {
			vec[idx] += 1.0
		} else {
			vec[idx] -= 1.0
		}
	}
	return normalize(vec), nil
}

// EmbedFunc returns the EmbeddingFunc for this embedder.
func (h *HashEmbedder) EmbedFunc() EmbeddingFunc {
	return h.Embed
}

func (h *HashEmbedder) hashToken(token string) uint64 {
	hasher := fnv.New64a()
	hasher.Write([]byte(token))
	return hasher.Sum64() ^ h.seed
}

func tokenize(text string) []string {
	text = strings.ToLower(text)
	var tokens []string
	for _, field := range strings.Fields(text) {
		field = strings.Trim(field, ".,;:!?\"'()/\\-_")
		if field != "" {
			tokens = append(tokens, field)
		}
	}
	return tokens
}

func normalize(v Vec) Vec {
	var sumSquares float64
	for _, x := range v {
		sumSquares += x * x
	}
	if sumSquares == 0 {
		return v
	}
	norm := sqrt(sumSquares)
	for i := range v {
		v[i] /= norm
	}
	return v
}

func sqrt(x float64) float64 {
	if x <= 0 {
		return 0
	}
	z := x
	for i := 0; i < 20; i++ {
		prev := z
		z = (z + x/z) / 2
		if abs(z-prev) < 1e-15 {
			break
		}
	}
	return z
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
