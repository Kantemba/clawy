package vector

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/Kantemba/clawy/pkg/fileutil"
)

// PersistentStore extends Store with JSONL-backed persistence.
//
// Documents are appended to a JSONL file. Embeddings are stored as
// base64-free float arrays for compactness. The file is crash-safe:
// partial writes at EOF are ignored on reload.
type PersistentStore struct {
	Store
	path string
}

// NewPersistentStore creates a store backed by a JSONL file.
func NewPersistentStore(path string, embed EmbeddingFunc, dim int) (*PersistentStore, error) {
	ps := &PersistentStore{
		Store: *NewStore(embed, dim),
		path:  path,
	}
	if err := ps.load(); err != nil {
		return nil, err
	}
	return ps, nil
}

// load reads all documents from the JSONL file.
func (ps *PersistentStore) load() error {
	f, err := os.Open(ps.path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("open vector store: %w", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var doc Document
		if err := json.Unmarshal(line, &doc); err != nil {
			continue
		}
		ps.docs = append(ps.docs, doc)
	}
	return scanner.Err()
}

// Add inserts a document and appends it to the persistence file.
func (ps *PersistentStore) Add(doc Document) error {
	if err := ps.Store.Add(doc); err != nil {
		return err
	}
	return ps.appendDoc(doc)
}

// AddBatch inserts multiple documents efficiently with a single write.
func (ps *PersistentStore) AddBatch(docs []Document) error {
	if err := ps.Store.AddBatch(docs); err != nil {
		return err
	}
	return ps.appendDocs(docs)
}

func (ps *PersistentStore) appendDoc(doc Document) error {
	return ps.appendDocs([]Document{doc})
}

func (ps *PersistentStore) appendDocs(docs []Document) error {
	dir := filepath.Dir(ps.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create vector store dir: %w", err)
	}
	f, err := os.OpenFile(ps.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("open vector store for append: %w", err)
	}
	defer f.Close()

	for _, doc := range docs {
		line, err := json.Marshal(doc)
		if err != nil {
			return fmt.Errorf("marshal document: %w", err)
		}
		line = append(line, '\n')
		if _, err := f.Write(line); err != nil {
			return fmt.Errorf("write document: %w", err)
		}
	}
	return f.Sync()
}

// Delete removes a document and rewrites the persistence file.
func (ps *PersistentStore) Delete(id string) {
	ps.Store.Delete(id)
	ps.rewrite()
}

// rewrite atomically replaces the JSONL file with current documents.
func (ps *PersistentStore) rewrite() error {
	ps.mu.RLock()
	docs := make([]Document, len(ps.docs))
	copy(docs, ps.docs)
	ps.mu.RUnlock()

	dir := filepath.Dir(ps.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create vector store dir: %w", err)
	}

	var buf []byte
	for _, doc := range docs {
		line, err := json.Marshal(doc)
		if err != nil {
			return fmt.Errorf("marshal document: %w", err)
		}
		buf = append(line, '\n')
	}
	return fileutil.WriteFileAtomic(ps.path, buf, 0o644)
}

// ImportFromReader reads documents from a reader and adds them.
// Each line should be a JSON-encoded Document.
func (ps *PersistentStore) ImportFromReader(r io.Reader) (int, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)

	batch := make([]Document, 0, 256)
	count := 0
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var doc Document
		if err := json.Unmarshal(line, &doc); err != nil {
			continue
		}
		batch = append(batch, doc)
		count++
		if len(batch) >= 256 {
			if err := ps.AddBatch(batch); err != nil {
				return count, err
			}
			batch = batch[:0]
		}
	}
	if len(batch) > 0 {
		if err := ps.AddBatch(batch); err != nil {
			return count, err
		}
	}
	return count, scanner.Err()
}
