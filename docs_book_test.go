package catalog

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestUserDocsBookCoversEngineTopicsAndExcludesHostTimeline asserts the
// Starlight content tree (the shipped user book) has the five user-doc
// outcomes and does not teach host-only ndev catalog timeline as engine API.
func TestUserDocsBookCoversEngineTopicsAndExcludesHostTimeline(t *testing.T) {
	root := filepath.Join("site", "src", "content", "docs")
	required := map[string][]string{
		"install.md":      {"go install", "nicos-catalog version", "demo"},
		"host.md":         {"Layout", "Provider", "FilesystemProvider"},
		"cli.md":          {"validate", "reindex", "search", "graph", "drift", "collate", "insteadOf"},
		"architecture.md": {"Discover", "Reindex", "public DTO"},
		"privacy.md":      {"PublicEntity", "ProjectPublic"},
		"search.md":       {"BM25", "ErrEmptyQuery", "MatchedTerms"},
		"graph.md":        {"BuildGraph", "Mermaid", "dangling"},
		"drift.md":        {"index_schema_mismatch", "ReconcileDryRun", "ReconcileApply"},
		"stability.md":    {"SchemaVersion", "errors.Is", "PublicEntity"},
		"migrate.md":      {"WithProviders", "SchemaVersion", "ReconcileApply"},
		"contribute.md":   {"go test", "host-independent"},
		"docs.mdx":        {"/install/", "/search/", "/drift/"},
	}
	for name, needles := range required {
		body, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		text := string(body)
		for _, n := range needles {
			if !strings.Contains(text, n) {
				t.Errorf("%s missing %q", name, n)
			}
		}
		if hasHostTimeline(text) {
			t.Errorf("%s documents host ndev catalog timeline; that belongs in the operator page", name)
		}
	}
	index, err := os.ReadFile(filepath.Join(root, "index.mdx"))
	if err != nil {
		t.Fatal(err)
	}
	if hasHostTimeline(string(index)) {
		t.Error("index.mdx documents ndev catalog timeline as engine API")
	}
	if !strings.Contains(string(index), "text: Docs") || !strings.Contains(string(index), "link: /docs/") {
		t.Error("index.mdx must expose a Docs hero action to /docs/")
	}
	cfg, err := os.ReadFile(filepath.Join("site", "astro.config.mjs"))
	if err != nil {
		t.Fatal(err)
	}
	cfgText := string(cfg)
	if !strings.Contains(cfgText, "@astrojs/starlight") {
		t.Fatal("astro.config.mjs is not a Starlight site")
	}
	if strings.Contains(cfgText, "vitepress") || strings.Contains(cfgText, "vocs") || strings.Contains(cfgText, "docusaurus") {
		t.Fatal("engine book must stay Starlight, not a second viewer")
	}
	for _, slug := range []string{
		"docs", "install", "host", "search", "graph", "drift", "cli",
		"architecture", "stability", "privacy", "migrate", "contribute",
	} {
		if !strings.Contains(cfgText, "slug: '"+slug+"'") {
			t.Errorf("sidebar missing slug %s", slug)
		}
	}
}

func hasHostTimeline(text string) bool {
	lower := strings.ToLower(text)
	return strings.Contains(lower, "ndev catalog timeline") ||
		strings.Contains(lower, "timeline digest") ||
		strings.Contains(lower, "ndev.catalog.timeline")
}
