package catalog

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Layout injects every host-owned filesystem boundary used by the engine.
// The engine never assumes a repository name, home directory, or corpus shape.
type Layout struct {
	// CorpusDir holds authored entity files. It is the only directory the
	// engine reads as input.
	CorpusDir string `json:"corpus_dir" yaml:"corpus_dir"`
	// ConfigDir holds host configuration. The engine does not read it; it is
	// carried so a host has one place to describe all four boundaries.
	ConfigDir string `json:"config_dir" yaml:"config_dir"`
	// CacheDir holds derived state, including the index. It must not be nested
	// beneath CorpusDir, or generated output would become authored input on the
	// next run.
	CacheDir string `json:"cache_dir" yaml:"cache_dir"`
	// SidecarDataDir holds host-owned data adjacent to the catalog. The engine
	// does not read or write it.
	SidecarDataDir string `json:"sidecar_data_dir" yaml:"sidecar_data_dir"`
}

// DefaultLayout returns a portable layout rooted at root.
func DefaultLayout(root string) Layout {
	return Layout{
		CorpusDir:      filepath.Join(root, "catalog"),
		ConfigDir:      filepath.Join(root, ".nicos-catalog"),
		CacheDir:       filepath.Join(root, ".nicos-catalog", "cache"),
		SidecarDataDir: filepath.Join(root, ".nicos-catalog", "sidecars"),
	}
}

// Resolve converts relative paths to absolute paths under root and validates
// that all four host boundaries are explicit and distinct where required.
func (l Layout) Resolve(root string) (Layout, error) {
	root, err := filepath.Abs(filepath.Clean(root))
	if err != nil {
		return Layout{}, fmt.Errorf("%w: resolve host root: %w", ErrInvalidLayout, err)
	}
	resolve := func(raw string) string {
		raw = strings.TrimSpace(raw)
		if filepath.IsAbs(raw) {
			return filepath.Clean(raw)
		}
		return filepath.Clean(filepath.Join(root, raw))
	}
	resolved := Layout{
		CorpusDir:      resolve(l.CorpusDir),
		ConfigDir:      resolve(l.ConfigDir),
		CacheDir:       resolve(l.CacheDir),
		SidecarDataDir: resolve(l.SidecarDataDir),
	}
	if err := resolved.Validate(); err != nil {
		return Layout{}, err
	}
	return resolved, nil
}

// Validate rejects ambiguous layouts and unsafe cache placement.
func (l Layout) Validate() error {
	paths := map[string]string{
		"corpus_dir":       l.CorpusDir,
		"config_dir":       l.ConfigDir,
		"cache_dir":        l.CacheDir,
		"sidecar_data_dir": l.SidecarDataDir,
	}
	for name, path := range paths {
		if strings.TrimSpace(path) == "" {
			return fmt.Errorf("%w: %s is required", ErrInvalidLayout, name)
		}
		if !filepath.IsAbs(path) {
			return fmt.Errorf("%w: %s must be absolute after resolution: %q", ErrInvalidLayout, name, path)
		}
	}
	corpus := filepath.Clean(l.CorpusDir)
	cache := filepath.Clean(l.CacheDir)
	if corpus == cache {
		return fmt.Errorf("%w: cache_dir must not equal corpus_dir", ErrInvalidLayout)
	}
	if within(cache, corpus) {
		return fmt.Errorf("%w: cache_dir %q must not contain corpus_dir %q", ErrInvalidLayout, cache, corpus)
	}
	if within(corpus, cache) {
		return fmt.Errorf("%w: cache_dir %q must not be nested beneath corpus_dir %q", ErrInvalidLayout, cache, corpus)
	}
	return nil
}

func within(parent, child string) bool {
	rel, err := filepath.Rel(parent, child)
	return err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator))
}

func (l Layout) indexPath() string { return filepath.Join(l.CacheDir, "index.json") }
