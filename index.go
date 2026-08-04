package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Document is the per-entity term-frequency record backing BM25 retrieval.
type Document struct {
	EntityID      string         `json:"entity_id"`
	Length        int            `json:"length"`
	TermFrequency map[string]int `json:"term_frequency"`
	_             struct{}
}

// Index is the deterministic, portable catalog cache. It intentionally omits
// wall-clock timestamps so identical inputs produce byte-identical output.
type Index struct {
	SchemaVersion         int        `json:"schema_version"`
	SourceDigest          string     `json:"source_digest"`
	Entities              []Entity   `json:"entities"`
	Documents             []Document `json:"documents"`
	AverageDocumentLength float64    `json:"average_document_length"`
	_                     struct{}
}

// ReindexReport summarizes a completed reindex.
type ReindexReport struct {
	OK            bool   `json:"ok"`
	EntityCount   int    `json:"entity_count"`
	DocumentCount int    `json:"document_count"`
	SourceDigest  string `json:"source_digest"`
	IndexPath     string `json:"index_path"`
	_             struct{}
}

// Reindex discovers, indexes, and atomically installs the derived index.
// Identical inputs produce byte-identical output.
func (e *Engine) Reindex(ctx context.Context) (ReindexReport, error) {
	records, err := e.Discover(ctx)
	if err != nil {
		return ReindexReport{}, err
	}
	digest, err := recordsDigest(records)
	if err != nil {
		return ReindexReport{}, fmt.Errorf("digest records: %w", err)
	}
	index := buildIndex(records, digest)
	if err := writeJSONAtomic(e.layout.indexPath(), index); err != nil {
		return ReindexReport{}, err
	}
	return ReindexReport{
		OK: true, EntityCount: len(index.Entities), DocumentCount: len(index.Documents),
		SourceDigest: digest, IndexPath: e.layout.indexPath(),
	}, nil
}

func buildIndex(records []Record, digest string) Index {
	index := Index{SchemaVersion: SchemaVersion, SourceDigest: digest}
	var totalLength int
	for _, record := range records {
		entity := record.Entity
		index.Entities = append(index.Entities, entity)
		terms := tokenize(searchableText(entity))
		frequency := make(map[string]int, len(terms))
		for _, term := range terms {
			frequency[term]++
		}
		index.Documents = append(index.Documents, Document{EntityID: entity.ID, Length: len(terms), TermFrequency: frequency})
		totalLength += len(terms)
	}
	if len(index.Documents) > 0 {
		index.AverageDocumentLength = float64(totalLength) / float64(len(index.Documents))
	}
	return index
}

// LoadIndex reads the derived index written by Reindex.
func (e *Engine) LoadIndex(ctx context.Context) (Index, error) {
	if err := ctx.Err(); err != nil {
		return Index{}, err
	}
	path := e.layout.indexPath()
	// #nosec G304 -- path is always layout.indexPath() under the engine root, never caller-supplied.
	payload, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			// Both sentinels are preserved: callers testing for the domain
			// condition (ErrIndexMissing) and callers testing for the I/O
			// condition (os.ErrNotExist) must each match. An earlier version
			// formatted this with %s, which silently broke every errors.Is
			// check against it.
			return Index{}, &IndexError{Path: path, Err: fmt.Errorf("%w: %w", ErrIndexMissing, err)}
		}
		return Index{}, &IndexError{Path: path, Err: err}
	}
	var index Index
	if err := json.Unmarshal(payload, &index); err != nil {
		return Index{}, &IndexError{Path: path, Err: fmt.Errorf("%w: %w", ErrIndexCorrupt, err)}
	}
	if index.SchemaVersion != SchemaVersion {
		return Index{}, &IndexError{Path: path, Err: fmt.Errorf("%w: found %d, expected %d",
			ErrIndexSchema, index.SchemaVersion, SchemaVersion)}
	}
	if len(index.Entities) != len(index.Documents) {
		return Index{}, &IndexError{Path: path, Err: fmt.Errorf("%w: %d entities but %d documents",
			ErrIndexCorrupt, len(index.Entities), len(index.Documents))}
	}
	return index, nil
}

func writeJSONAtomic(path string, value any) error {
	payload, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	payload = append(payload, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return fmt.Errorf("create index directory: %w", err)
	}
	temp, err := os.CreateTemp(filepath.Dir(path), ".index-*.tmp")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	defer func() { _ = os.Remove(tempPath) }()
	if err := temp.Chmod(0o644); err != nil {
		_ = temp.Close()
		return err
	}
	if _, err := temp.Write(payload); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tempPath, path); err != nil {
		return fmt.Errorf("install index: %w", err)
	}
	// Syncing the file alone does not durably record the rename; the directory
	// entry itself must be flushed. Failures here are tolerated because some
	// filesystems and platforms refuse directory sync, and the rename has
	// already made the new content visible.
	if dir, err := os.Open(filepath.Dir(path)); err == nil {
		_ = dir.Sync()
		_ = dir.Close()
	}
	return nil
}
