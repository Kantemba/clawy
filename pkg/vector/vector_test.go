package vector

import (
	"testing"
)

func TestCosineSimilarity(t *testing.T) {
	tests := []struct {
		name string
		a    Vec
		b    Vec
		want float64
	}{
		{"identical vectors", Vec{1, 0, 0}, Vec{1, 0, 0}, 1.0},
		{"opposite vectors", Vec{1, 0, 0}, Vec{-1, 0, 0}, -1.0},
		{"orthogonal vectors", Vec{1, 0, 0}, Vec{0, 1, 0}, 0.0},
		{"empty vectors", nil, nil, 0},
		{"dimension mismatch", Vec{1, 2}, Vec{1, 2, 3}, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CosineSimilarity(tt.a, tt.b)
			if diff := got - tt.want; diff > 1e-9 || diff < -1e-9 {
				t.Errorf("CosineSimilarity() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestStoreSearch(t *testing.T) {
	embed := NewHashEmbedder(64)
	store := NewStore(embed.EmbedFunc(), 64)

	docs := []Document{
		{ID: "1", Content: "The weather in Tokyo is sunny today"},
		{ID: "2", Content: "Python programming language tutorial"},
		{ID: "3", Content: "Machine learning and artificial intelligence"},
	}

	if err := store.AddBatch(docs); err != nil {
		t.Fatalf("AddBatch failed: %v", err)
	}

	if store.Count() != 3 {
		t.Errorf("Count() = %d, want 3", store.Count())
	}

	matches, err := store.SearchText("Tokyo weather forecast", 2)
	if err != nil {
		t.Fatalf("SearchText failed: %v", err)
	}

	if len(matches) == 0 {
		t.Fatal("expected at least one match")
	}

	if matches[0].Document.ID != "1" {
		t.Errorf("expected doc 1 to be top result, got %s", matches[0].Document.ID)
	}
}

func TestStoreDelete(t *testing.T) {
	embed := NewHashEmbedder(64)
	store := NewStore(embed.EmbedFunc(), 64)

	docs := []Document{
		{ID: "a", Content: "apple fruit"},
		{ID: "b", Content: "banana fruit"},
	}
	_ = store.AddBatch(docs)

	store.Delete("a")
	if store.Count() != 1 {
		t.Errorf("Count() after delete = %d, want 1", store.Count())
	}

	remaining := store.Documents()
	if remaining[0].ID != "b" {
		t.Errorf("remaining doc ID = %s, want b", remaining[0].ID)
	}
}

func TestTopKMatches(t *testing.T) {
	matches := []Match{
		{Document: Document{ID: "1"}, Similarity: 0.1},
		{Document: Document{ID: "2"}, Similarity: 0.9},
		{Document: Document{ID: "3"}, Similarity: 0.5},
		{Document: Document{ID: "4"}, Similarity: 0.7},
		{Document: Document{ID: "5"}, Similarity: 0.3},
	}

	top3 := topKMatches(matches, 3)
	if len(top3) != 3 {
		t.Fatalf("topKMatches() returned %d matches, want 3", len(top3))
	}

	expected := []string{"2", "4", "3"}
	for i, id := range expected {
		if top3[i].Document.ID != id {
			t.Errorf("match[%d].ID = %s, want %s", i, top3[i].Document.ID, id)
		}
	}
}
