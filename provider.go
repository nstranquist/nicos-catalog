package catalog

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Provider is the host extension boundary. Providers discover authored facts;
// the engine owns normalization, validation, indexing, graph, and drift.
type Provider interface {
	Name() string
	Provide(context.Context, Layout) ([]Record, error)
}

// StaticProvider is useful for tests, embedded demos, and API-backed hosts.
type StaticProvider struct {
	ProviderName string
	Entities     []Entity
}

func (p StaticProvider) Name() string {
	if strings.TrimSpace(p.ProviderName) == "" {
		return "static"
	}
	return strings.TrimSpace(p.ProviderName)
}

func (p StaticProvider) Provide(_ context.Context, _ Layout) ([]Record, error) {
	records := make([]Record, 0, len(p.Entities))
	for i, entity := range p.Entities {
		payload, err := json.Marshal(entity)
		if err != nil {
			return nil, err
		}
		records = append(records, Record{
			Entity: entity, Provider: p.Name(), Source: fmt.Sprintf("static:%04d", i), Digest: digestBytes(payload),
		})
	}
	return records, nil
}

// FilesystemProvider reads .md YAML frontmatter, .yaml/.yml, and .json entity
// records recursively from Layout.CorpusDir.
type FilesystemProvider struct {
	ProviderName string
	ExcludeDirs  []string
	// Strict rejects unknown fields, malformed frontmatter, trailing documents,
	// and entity files without IDs. Public and release paths should enable it.
	Strict bool
}

func (p FilesystemProvider) Name() string {
	if strings.TrimSpace(p.ProviderName) == "" {
		return "filesystem"
	}
	return strings.TrimSpace(p.ProviderName)
}

func (p FilesystemProvider) Provide(ctx context.Context, layout Layout) ([]Record, error) {
	var paths []string
	err := filepath.WalkDir(layout.CorpusDir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if entry.IsDir() {
			name := entry.Name()
			if path != layout.CorpusDir && p.excludeDir(name) {
				return filepath.SkipDir
			}
			return nil
		}
		switch strings.ToLower(filepath.Ext(path)) {
		case ".md", ".yaml", ".yml", ".json":
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk corpus %s: %w", layout.CorpusDir, err)
	}
	sort.Strings(paths)
	var records []Record
	for _, path := range paths {
		payload, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", path, err)
		}
		entities, err := decodeEntitiesWithPolicy(path, payload, p.Strict)
		if err != nil {
			return nil, err
		}
		rel, err := filepath.Rel(layout.CorpusDir, path)
		if err != nil {
			rel = path
		}
		for i, entity := range entities {
			source := filepath.ToSlash(rel)
			if len(entities) > 1 {
				source = fmt.Sprintf("%s#%d", source, i)
			}
			records = append(records, Record{Entity: entity, Provider: p.Name(), Source: source, Digest: digestBytes(payload)})
		}
	}
	return records, nil
}

func (p FilesystemProvider) excludeDir(name string) bool {
	if strings.HasPrefix(name, ".") || name == "node_modules" || name == "vendor" || name == "_archive" {
		return true
	}
	for _, excluded := range p.ExcludeDirs {
		if name == strings.TrimSpace(excluded) {
			return true
		}
	}
	return false
}

func decodeEntities(path string, payload []byte) ([]Entity, error) {
	return decodeEntitiesWithPolicy(path, payload, false)
}

func decodeEntitiesWithPolicy(path string, payload []byte, strict bool) ([]Entity, error) {
	text := bytes.TrimSpace(payload)
	if len(text) == 0 {
		return nil, nil
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".md":
		frontmatter, body, ok := splitFrontmatter(text)
		if !ok {
			if strict && bytes.HasPrefix(text, []byte("---")) {
				return nil, fmt.Errorf("decode %s frontmatter: missing closing delimiter", path)
			}
			return nil, nil
		}
		var entity Entity
		if err := decodeYAML(frontmatter, &entity, strict); err != nil {
			return nil, fmt.Errorf("decode %s frontmatter: %w", path, err)
		}
		if entity.Description == "" {
			entity.Description = firstParagraph(string(body))
		}
		if entity.ID == "" {
			if strict {
				return nil, fmt.Errorf("decode %s frontmatter: id is required", path)
			}
			return nil, nil
		}
		return []Entity{entity}, nil
	case ".yaml", ".yml":
		var list []Entity
		if err := decodeYAML(text, &list, strict); err == nil && len(list) > 0 {
			if strict {
				for i, entity := range list {
					if strings.TrimSpace(entity.ID) == "" {
						return nil, fmt.Errorf("decode %s: entity %d id is required", path, i)
					}
				}
			}
			return list, nil
		}
		var entity Entity
		if err := decodeYAML(text, &entity, strict); err != nil {
			return nil, fmt.Errorf("decode %s: %w", path, err)
		}
		if entity.ID == "" {
			if strict {
				return nil, fmt.Errorf("decode %s: id is required", path)
			}
			return nil, nil
		}
		return []Entity{entity}, nil
	case ".json":
		var list []Entity
		if err := decodeJSON(text, &list, strict); err == nil {
			if strict {
				for i, entity := range list {
					if strings.TrimSpace(entity.ID) == "" {
						return nil, fmt.Errorf("decode %s: entity %d id is required", path, i)
					}
				}
			}
			return list, nil
		}
		var entity Entity
		if err := decodeJSON(text, &entity, strict); err != nil {
			return nil, fmt.Errorf("decode %s: %w", path, err)
		}
		if entity.ID == "" {
			if strict {
				return nil, fmt.Errorf("decode %s: id is required", path)
			}
			return nil, nil
		}
		return []Entity{entity}, nil
	default:
		return nil, nil
	}
}

func decodeYAML(payload []byte, target any, strict bool) error {
	if !strict {
		return yaml.Unmarshal(payload, target)
	}
	decoder := yaml.NewDecoder(bytes.NewReader(payload))
	decoder.KnownFields(true)
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return fmt.Errorf("multiple YAML documents are not supported")
		}
		return err
	}
	return nil
}

func decodeJSON(payload []byte, target any, strict bool) error {
	if !strict {
		return json.Unmarshal(payload, target)
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return fmt.Errorf("multiple JSON values are not supported")
		}
		return err
	}
	return nil
}

func splitFrontmatter(payload []byte) ([]byte, []byte, bool) {
	if !bytes.HasPrefix(payload, []byte("---\n")) && !bytes.HasPrefix(payload, []byte("---\r\n")) {
		return nil, nil, false
	}
	normalized := bytes.ReplaceAll(payload, []byte("\r\n"), []byte("\n"))
	rest := normalized[4:]
	end := bytes.Index(rest, []byte("\n---\n"))
	if end < 0 {
		return nil, nil, false
	}
	return rest[:end], rest[end+5:], true
}

func firstParagraph(body string) string {
	for _, block := range strings.Split(strings.TrimSpace(body), "\n\n") {
		block = strings.TrimSpace(block)
		if block == "" || strings.HasPrefix(block, "#") {
			continue
		}
		return strings.Join(strings.Fields(block), " ")
	}
	return ""
}

func digestBytes(payload []byte) string {
	digest := sha256.Sum256(payload)
	return "sha256:" + hex.EncodeToString(digest[:])
}
