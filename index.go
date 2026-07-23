package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

type Document struct {
	EntityID      string         `json:"entity_id"`
	Length        int            `json:"length"`
	TermFrequency map[string]int `json:"term_frequency"`
}

// Index is the deterministic, portable catalog cache. It intentionally omits
// wall-clock timestamps so identical inputs produce byte-identical output.
type Index struct {
	SchemaVersion         int        `json:"schema_version"`
	SourceDigest          string     `json:"source_digest"`
	Entities              []Entity   `json:"entities"`
	Documents             []Document `json:"documents"`
	AverageDocumentLength float64    `json:"average_document_length"`
}

type ReindexReport struct {
	OK            bool   `json:"ok"`
	EntityCount   int    `json:"entity_count"`
	DocumentCount int    `json:"document_count"`
	SourceDigest  string `json:"source_digest"`
	IndexPath     string `json:"index_path"`
}

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

func (e *Engine) LoadIndex() (Index, error) {
	payload, err := os.ReadFile(e.layout.indexPath())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Index{}, fmt.Errorf("index missing at %s; run reindex", e.layout.indexPath())
		}
		return Index{}, err
	}
	var index Index
	if err := json.Unmarshal(payload, &index); err != nil {
		return Index{}, fmt.Errorf("decode index: %w", err)
	}
	if index.SchemaVersion != SchemaVersion {
		return Index{}, fmt.Errorf("unsupported index schema %d; expected %d", index.SchemaVersion, SchemaVersion)
	}
	if len(index.Entities) != len(index.Documents) {
		return Index{}, fmt.Errorf("invalid index: %d entities but %d documents", len(index.Entities), len(index.Documents))
	}
	return index, nil
}

func writeJSONAtomic(path string, value any) error {
	payload, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	payload = append(payload, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create index directory: %w", err)
	}
	temp, err := os.CreateTemp(filepath.Dir(path), ".index-*.tmp")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if err := temp.Chmod(0o644); err != nil {
		temp.Close()
		return err
	}
	if _, err := temp.Write(payload); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tempPath, path); err != nil {
		return fmt.Errorf("install index: %w", err)
	}
	return nil
}
