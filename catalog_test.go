package catalog

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testLayout(t *testing.T) Layout {
	t.Helper()
	root := t.TempDir()
	layout, err := DefaultLayout(root).Resolve(root)
	if err != nil {
		t.Fatal(err)
	}
	return layout
}

func testEntities() []Entity {
	return []Entity{
		{ID: "system.alpha", Name: "Alpha Platform", Kind: "system", Description: "Developer ownership graph and service catalog.", Tags: []string{"platform", "public"}, Visibility: "public", PublicURL: "https://example.com/alpha", Entrypoint: "/private/public-entity-path", Annotations: map[string]string{"query": "private public-entity query"}, Refs: []Ref{{Kind: "contains", Target: "service.beta"}}},
		{ID: "service.beta", Name: "Beta API", Kind: "service", Description: "Go API for inventory and dependency search.", Tags: []string{"go", "public"}, Visibility: "public", PublicURL: "https://example.com/beta"},
		{ID: "telemetry.secret", Name: "Host Telemetry", Kind: "telemetry", Description: "Host-only query sample.", Visibility: "private", Entrypoint: "/private/path", Annotations: map[string]string{"query": "private query"}},
	}
}

func TestReindexSearchDriftAndDeterminism(t *testing.T) {
	ctx := context.Background()
	layout := testLayout(t)
	provider := &StaticProvider{ProviderName: "fixture", Entities: testEntities()}
	engine, err := New(layout, provider)
	if err != nil {
		t.Fatal(err)
	}
	first, err := engine.Reindex(ctx)
	if err != nil {
		t.Fatal(err)
	}
	firstBytes, err := os.ReadFile(first.IndexPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := engine.Reindex(ctx); err != nil {
		t.Fatal(err)
	}
	secondBytes, err := os.ReadFile(first.IndexPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(firstBytes) != string(secondBytes) {
		t.Fatal("identical provider output produced different index bytes")
	}
	results, err := engine.Search("ownership graph", SearchOptions{Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) == 0 || results[0].Entity.ID != "system.alpha" {
		t.Fatalf("unexpected search results: %#v", results)
	}
	drift, err := engine.Drift(ctx)
	if err != nil || drift.Changed || !drift.OK {
		t.Fatalf("expected clean drift, got %#v err=%v", drift, err)
	}
	provider.Entities[0].Description += " changed"
	drift, err = engine.Drift(ctx)
	if err != nil || !drift.Changed || drift.OK {
		t.Fatalf("expected source drift, got %#v err=%v", drift, err)
	}
	reconciled, err := engine.Reconcile(ctx, true)
	if err != nil || !reconciled.Applied {
		t.Fatalf("expected applied reconcile, got %#v err=%v", reconciled, err)
	}
}

func TestPublicProjectionIsClosed(t *testing.T) {
	engine, err := New(testLayout(t), StaticProvider{Entities: testEntities()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := engine.Reindex(context.Background()); err != nil {
		t.Fatal(err)
	}
	index, err := engine.LoadIndex()
	if err != nil {
		t.Fatal(err)
	}
	projection, err := ProjectPublic(index, ProjectionPolicy{RequireVisibility: "public", AllowHosts: []string{"example.com"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(projection.Items) != 2 {
		t.Fatalf("expected 2 public entities, got %d", len(projection.Items))
	}
	payload, err := json.Marshal(projection)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"private query", "/private/path", "/private/public-entity-path", "private public-entity query", "annotations", "entrypoint", "telemetry.secret"} {
		if strings.Contains(string(payload), forbidden) {
			t.Fatalf("public projection leaked %q: %s", forbidden, payload)
		}
	}
}

func TestPublicProjectionTruncatesOnUTF8Boundary(t *testing.T) {
	entities := []Entity{{
		ID: "service.unicode", Name: "Unicode", Kind: "service", Visibility: "public",
		Description: "ééé", PublicURL: "https://example.com/unicode",
	}}
	engine, err := New(testLayout(t), StaticProvider{Entities: entities})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := engine.Reindex(context.Background()); err != nil {
		t.Fatal(err)
	}
	index, err := engine.LoadIndex()
	if err != nil {
		t.Fatal(err)
	}
	projection, err := ProjectPublic(index, ProjectionPolicy{RequireVisibility: "public", AllowHosts: []string{"EXAMPLE.COM"}, MaxSummaryBytes: 5})
	if err != nil {
		t.Fatal(err)
	}
	if got := projection.Items[0].Summary; got != "éé…" || !strings.Contains(got, "…") {
		t.Fatalf("summary = %q, want valid rune-safe truncation", got)
	}
}

func TestPublicProjectionZeroPolicyIsFailClosed(t *testing.T) {
	index := Index{Entities: testEntities()}
	projection, err := ProjectPublic(index, ProjectionPolicy{})
	if err == nil || !strings.Contains(err.Error(), "explicit hostname allowlist") {
		t.Fatalf("expected URL allowlist failure, got projection=%#v err=%v", projection, err)
	}

	index.Entities[0].PublicURL = ""
	index.Entities[1].PublicURL = ""
	projection, err = ProjectPublic(index, ProjectionPolicy{})
	if err != nil {
		t.Fatal(err)
	}
	if len(projection.Items) != 2 {
		t.Fatalf("zero policy projected %d items, want two public items", len(projection.Items))
	}
	for _, item := range projection.Items {
		if item.ID == "telemetry.secret" {
			t.Fatalf("zero policy leaked private item: %#v", item)
		}
	}
}

func TestPublicProjectionRejectsUnsafeURLsAndContent(t *testing.T) {
	tests := []struct {
		name string
		edit func(*Entity)
		want string
	}{
		{"credentials", func(e *Entity) { e.PublicURL = "https://user:pass@example.com/item" }, "credentials"},
		{"query", func(e *Entity) { e.PublicURL = "https://example.com/item?token=secret" }, "query or fragment"},
		{"fragment", func(e *Entity) { e.PublicURL = "https://example.com/item#private" }, "query or fragment"},
		{"port", func(e *Entity) { e.PublicURL = "https://example.com:8443/item" }, "non-HTTPS port"},
		{"path", func(e *Entity) { e.Description = "Built from /Users/private/catalog.yaml" }, "prohibited content"},
		{"secret", func(e *Entity) { e.Tags = []string{"api_key=abcdefghijklmnop"} }, "prohibited content"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entity := Entity{ID: "system.public", Name: "Public", Kind: "system", Visibility: "public", PublicURL: "https://example.com/item"}
			tt.edit(&entity)
			_, err := ProjectPublic(Index{Entities: []Entity{entity}}, ProjectionPolicy{AllowHosts: []string{"example.com"}})
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("expected %q error, got %v", tt.want, err)
			}
		})
	}
}

func TestPublicProjectionRejectsNonPublicVisibilityPolicy(t *testing.T) {
	_, err := ProjectPublic(Index{}, ProjectionPolicy{RequireVisibility: "private"})
	if err == nil || !strings.Contains(err.Error(), "requires visibility") {
		t.Fatalf("expected non-public policy rejection, got %v", err)
	}
}

func TestDuplicateIDFailsAcrossProviders(t *testing.T) {
	entity := Entity{ID: "service.duplicate", Name: "Duplicate", Kind: "service"}
	engine, err := New(testLayout(t), StaticProvider{ProviderName: "a", Entities: []Entity{entity}}, StaticProvider{ProviderName: "b", Entities: []Entity{entity}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := engine.Discover(context.Background()); err == nil || !strings.Contains(err.Error(), "duplicate entity id") {
		t.Fatalf("expected duplicate-id error, got %v", err)
	}
}

func TestFilesystemProviderReadsSupportedFormats(t *testing.T) {
	root := t.TempDir()
	corpus := filepath.Join(root, "catalog")
	if err := os.MkdirAll(corpus, 0o755); err != nil {
		t.Fatal(err)
	}
	markdown := "---\nid: service.frontmatter\nname: Frontmatter Service\nkind: service\n---\n\n# Frontmatter\n\nPortable description from the Markdown body.\n"
	if err := os.WriteFile(filepath.Join(corpus, "service.md"), []byte(markdown), 0o644); err != nil {
		t.Fatal(err)
	}
	layout, err := (Layout{CorpusDir: "catalog", ConfigDir: ".state", CacheDir: ".state/cache", SidecarDataDir: ".state/sidecars"}).Resolve(root)
	if err != nil {
		t.Fatal(err)
	}
	engine, err := New(layout, FilesystemProvider{})
	if err != nil {
		t.Fatal(err)
	}
	records, err := engine.Discover(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || !strings.Contains(records[0].Entity.Description, "Portable description") {
		t.Fatalf("unexpected records: %#v", records)
	}
}

func TestFilesystemProviderStrictRejectsUnknownFieldsAndMalformedFrontmatter(t *testing.T) {
	tests := map[string]string{
		"unknown.yaml": "id: service.strict\nname: Strict\nkind: service\nunknown_field: nope\n",
		"broken.md":    "---\nid: service.broken\nname: Broken\nkind: service\n",
	}
	for name, payload := range tests {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			corpus := filepath.Join(root, "catalog")
			if err := os.MkdirAll(corpus, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(corpus, name), []byte(payload), 0o644); err != nil {
				t.Fatal(err)
			}
			layout, err := (Layout{CorpusDir: "catalog", ConfigDir: ".state", CacheDir: ".state/cache", SidecarDataDir: ".state/sidecars"}).Resolve(root)
			if err != nil {
				t.Fatal(err)
			}
			engine, err := New(layout, FilesystemProvider{Strict: true})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := engine.Discover(context.Background()); err == nil {
				t.Fatal("expected strict provider failure")
			}
		})
	}
}

func TestMermaidIDsRemainUniqueAfterSanitization(t *testing.T) {
	left := mermaidID("service.a-b")
	right := mermaidID("service.a_b")
	if left == right {
		t.Fatalf("colliding Mermaid IDs: %q", left)
	}
	if left != mermaidID("service.a-b") {
		t.Fatal("Mermaid ID is not deterministic")
	}
}
