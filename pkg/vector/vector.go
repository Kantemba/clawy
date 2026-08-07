package vector

import (
	"fmt"
	"math"
	"sort"
	"sync"
)

// Vec is a dense embedding vector.
type Vec []float64

// CosineSimilarity returns the cosine similarity between two vectors.
// Returns 0 if either vector is empty or dimensions mismatch.
func CosineSimilarity(a, b Vec) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot, normA, normB float64
	for i := range a {
		dot += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}

// Document represents a single indexed document with its embedding.
type Document struct {
	ID        string         `json:"id"`
	Content   string         `json:"content"`
	Metadata  map[string]any `json:"metadata,omitempty"`
	Embedding Vec            `json:"embedding,omitempty"`
}

// Match is a search result with similarity score.
type Match struct {
	Document   Document `json:"document"`
	Similarity float64  `json:"similarity"`
}

// EmbeddingFunc converts text into a dense vector.
type EmbeddingFunc func(text string) (Vec, error)

// Store is an in-memory vector index with optional persistence.
type Store struct {
	docs  []Document
	mu    sync.RWMutex
	dim   int
	embed EmbeddingFunc
}

// NewStore creates a vector store with the given embedding function and dimension.
func NewStore(embed EmbeddingFunc, dim int) *Store {
	return &Store{
		docs:  make([]Document, 0),
		dim:   dim,
		embed: embed,
	}
}

// Add inserts a document and computes its embedding if not provided.
func (s *Store) Add(doc Document) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(doc.Embedding) == 0 && s.embed != nil {
		vec, err := s.embed(doc.Content)
		if err != nil {
			return fmt.Errorf("embed document: %w", err)
		}
		doc.Embedding = vec
	}
	s.docs = append(s.docs, doc)
	return nil
}

// AddBatch inserts multiple documents efficiently.
func (s *Store) AddBatch(docs []Document) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, doc := range docs {
		if len(doc.Embedding) == 0 && s.embed != nil {
			vec, err := s.embed(doc.Content)
			if err != nil {
				return fmt.Errorf("embed document: %w", err)
			}
			doc.Embedding = vec
		}
		s.docs = append(s.docs, doc)
	}
	return nil
}

// Search returns the top-k most similar documents to the query vector.
func (s *Store) Search(query Vec, topK int) []Match {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if len(s.docs) == 0 || len(query) == 0 {
		return nil
	}

	matches := make([]Match, 0, len(s.docs))
	for _, doc := range s.docs {
		if len(doc.Embedding) != len(query) {
			continue
		}
		sim := CosineSimilarity(query, doc.Embedding)
		matches = append(matches, Match{Document: doc, Similarity: sim})
	}

	return topKMatches(matches, topK)
}

// SearchText embeds the query text and searches for similar documents.
func (s *Store) SearchText(query string, topK int) ([]Match, error) {
	if s.embed == nil {
		return nil, fmt.Errorf("no embedding function configured")
	}
	vec, err := s.embed(query)
	if err != nil {
		return nil, fmt.Errorf("embed query: %w", err)
	}
	return s.Search(vec, topK), nil
}

// Delete removes a document by ID.
func (s *Store) Delete(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	filtered := s.docs[:0]
	for _, doc := range s.docs {
		if doc.ID != id {
			filtered = append(filtered, doc)
		}
	}
	s.docs = filtered
}

// Count returns the number of documents in the store.
func (s *Store) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.docs)
}

// Documents returns a copy of all documents.
func (s *Store) Documents() []Document {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Document, len(s.docs))
	copy(out, s.docs)
	return out
}

// topKMatches returns the top-k matches by similarity using a min-heap approach.
func topKMatches(matches []Match, topK int) []Match {
	if topK <= 0 || len(matches) == 0 {
		return nil
	}
	if len(matches) <= topK {
		sortMatchesDesc(matches)
		return matches
	}

	heap := make([]Match, 0, topK)
	for _, m := range matches {
		if len(heap) < topK {
			heap = append(heap, m)
			if len(heap) == topK {
				buildMinHeap(heap)
			}
		} else if m.Similarity > heap[0].Similarity {
			heap[0] = m
			siftDown(heap, 0)
		}
	}

	sortMatchesDesc(heap)
	return heap
}

func sortMatchesDesc(matches []Match) {
	sort.Slice(matches, func(i, j int) bool {
		return matches[i].Similarity > matches[j].Similarity
	})
}

func buildMinHeap(h []Match) {
	for i := len(h)/2 - 1; i >= 0; i-- {
		siftDown(h, i)
	}
}

func siftDown(h []Match, i int) {
	n := len(h)
	for {
		smallest := i
		l, r := 2*i+1, 2*i+2
		if l < n && h[l].Similarity < h[smallest].Similarity {
			smallest = l
		}
		if r < n && h[r].Similarity < h[smallest].Similarity {
			smallest = r
		}
		if smallest == i {
			break
		}
		h[i], h[smallest] = h[smallest], h[i]
		i = smallest
	}
}
