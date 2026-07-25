package catalog

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// writeCorpusFile creates a corpus file relative to layout.CorpusDir.
func writeCorpusFile(t *testing.T, layout Layout, rel, contents string) string {
	t.Helper()
	path := filepath.Join(layout.CorpusDir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestFilesystemProviderDoesNotFollowSymlinkedFiles locks the corpus boundary.
// A symlinked corpus file resolves to content outside the corpus root, and that
// content decodes into perfectly valid entities, so the link must be refused
// rather than followed.
func TestFilesystemProviderDoesNotFollowSymlinkedFiles(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires elevation on Windows")
	}
	layout := testLayout(t)
	writeCorpusFile(t, layout, "service.real.yaml", "id: service.real\nname: Real\nkind: service\n")

	outside := filepath.Join(t.TempDir(), "secrets.yaml")
	if err := os.WriteFile(outside, []byte("id: service.leaked\nname: Leaked\nkind: service\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(layout.CorpusDir, "service.leaked.yaml")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	lax, err := FilesystemProvider{}.Provide(context.Background(), layout)
	if err != nil {
		t.Fatal(err)
	}
	for _, record := range lax {
		if record.Entity.ID == "service.leaked" {
			t.Fatalf("lax provider followed a symlink out of the corpus: %+v", record)
		}
	}
	if len(lax) != 1 {
		t.Fatalf("lax provider returned %d records, want only the real file", len(lax))
	}

	_, err = FilesystemProvider{Strict: true}.Provide(context.Background(), layout)
	if !errors.Is(err, ErrCorpusEscape) {
		t.Fatalf("strict provider error = %v, want ErrCorpusEscape", err)
	}
}

// TestLoadIndexMissingWrapsErrNotExist locks both sentinels. A previous version
// formatted the missing-index error with %s, which made every errors.Is check
// against it silently false and left Drift's index_missing branch unreachable.
func TestLoadIndexMissingWrapsErrNotExist(t *testing.T) {
	engine, err := New(testLayout(t), WithProviders(StaticProvider{Entities: testEntities()}))
	if err != nil {
		t.Fatal(err)
	}
	_, err = engine.LoadIndex(context.Background())
	if err == nil {
		t.Fatal("expected a missing-index error")
	}
	if !errors.Is(err, ErrIndexMissing) {
		t.Fatalf("errors.Is(err, ErrIndexMissing) = false for %v", err)
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("errors.Is(err, os.ErrNotExist) = false for %v", err)
	}
	var indexErr *IndexError
	if !errors.As(err, &indexErr) {
		t.Fatalf("errors.As(err, *IndexError) = false for %v", err)
	}
}

// TestDriftReportsIndexMissingWithoutStatFallback proves the branch is reached
// through the wrapped sentinel rather than a filesystem re-check.
func TestDriftReportsIndexMissingWithoutStatFallback(t *testing.T) {
	engine, err := New(testLayout(t), WithProviders(StaticProvider{Entities: testEntities()}))
	if err != nil {
		t.Fatal(err)
	}
	report, err := engine.Drift(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !report.Changed || report.Reason != "index_missing" {
		t.Fatalf("drift report = %+v, want changed with reason index_missing", report)
	}
}

// TestDriftTreatsSchemaMismatchAsChanged keeps an engine upgrade a reindex
// prompt rather than a hard failure on first run.
func TestDriftTreatsSchemaMismatchAsChanged(t *testing.T) {
	layout := testLayout(t)
	engine, err := New(layout, WithProviders(StaticProvider{Entities: testEntities()}))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := engine.Reindex(context.Background()); err != nil {
		t.Fatal(err)
	}
	path := layout.indexPath()
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	stale := strings.Replace(string(payload),
		`"schema_version": 2`, `"schema_version": 999`, 1)
	if stale == string(payload) {
		t.Fatal("failed to rewrite the index schema version")
	}
	if err := os.WriteFile(path, []byte(stale), 0o644); err != nil {
		t.Fatal(err)
	}
	report, err := engine.Drift(context.Background())
	if err != nil {
		t.Fatalf("schema mismatch returned a hard error: %v", err)
	}
	if !report.Changed || report.Reason != "index_schema_mismatch" {
		t.Fatalf("drift report = %+v, want changed with reason index_schema_mismatch", report)
	}
}

// TestDiscoverDoesNotMutateProviderEntities locks the copy semantics. Refs and
// Annotations are reference types, so normalization used to trim and re-sort
// the caller's own slice through the shared backing array.
func TestDiscoverDoesNotMutateProviderEntities(t *testing.T) {
	entities := []Entity{{
		ID: "service.alpha", Name: "Alpha", Kind: "service",
		Refs: []Ref{
			{Kind: " zeta ", Target: " service.omega "},
			{Kind: "alpha", Target: "service.beta"},
		},
		Annotations: map[string]string{"parent": " system.root "},
	}}
	provider := StaticProvider{Entities: entities}
	engine, err := New(testLayout(t), WithProviders(provider))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := engine.Discover(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := provider.Entities[0].Refs[0]; got.Kind != " zeta " || got.Target != " service.omega " {
		t.Fatalf("Discover rewrote the caller's refs: %+v", provider.Entities[0].Refs)
	}
	if got := provider.Entities[0].Annotations["parent"]; got != " system.root " {
		t.Fatalf("Discover rewrote the caller's annotations: %q", got)
	}
}

// TestMermaidLineCountInvariant proves a label can never break the diagram's
// line structure. Backslash escaping alone left raw newlines intact, which
// terminated the statement and corrupted every following line.
func TestMermaidLineCountInvariant(t *testing.T) {
	index := Index{Entities: []Entity{
		{ID: "a.one", Name: "Line\nBreak\r\nHere \"quoted\" #hash \\slash\t", Kind: "service",
			Refs: []Ref{{Kind: "uses\nbroken", Target: "a.two"}}},
		{ID: "a.two", Name: "Plain", Kind: "service"},
	}}
	graph := BuildGraph(index)
	mermaid := graph.Mermaid()
	lines := strings.Split(strings.TrimRight(mermaid, "\n"), "\n")
	want := 1 + len(graph.Nodes) + len(graph.Edges)
	if len(lines) != want {
		t.Fatalf("mermaid emitted %d lines, want %d:\n%s", len(lines), want, mermaid)
	}
	for _, line := range lines[1:] {
		if strings.Contains(line, "\r") {
			t.Fatalf("mermaid line retained a carriage return: %q", line)
		}
	}
}

// TestProjectPublicLargeCorpusCompletesInBudget guards the connection-resolution
// loop against reverting to a linear scan per item. At this size a quadratic
// implementation performs billions of comparisons and cannot finish inside the
// package test timeout, while the indexed form completes in milliseconds.
func TestProjectPublicLargeCorpusCompletesInBudget(t *testing.T) {
	const size = 50000
	entities := make([]Entity, 0, size)
	for i := 0; i < size; i++ {
		id := "service." + itoa(i)
		entity := Entity{ID: id, Name: "Service " + itoa(i), Kind: "service", Visibility: VisibilityPublic}
		if i > 0 {
			entity.Refs = []Ref{{Kind: "depends_on", Target: "service." + itoa(i-1)}}
		}
		entities = append(entities, entity)
	}
	projection, err := ProjectPublic(context.Background(), Index{Entities: entities}, ProjectionPolicy{})
	if err != nil {
		t.Fatal(err)
	}
	if len(projection.Items) != size {
		t.Fatalf("projected %d items, want %d", len(projection.Items), size)
	}
	connections := 0
	for _, item := range projection.Items {
		connections += len(item.Connections)
	}
	if connections != size-1 {
		t.Fatalf("projected %d connections, want %d", connections, size-1)
	}
}

// TestWriteJSONAtomicLeavesNoTempFiles proves the install path cleans up after
// itself on both the success and marshal-failure routes.
func TestWriteJSONAtomicLeavesNoTempFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "index.json")
	if err := writeJSONAtomic(path, map[string]string{"ok": "yes"}); err != nil {
		t.Fatal(err)
	}
	if err := writeJSONAtomic(path, make(chan int)); err == nil {
		t.Fatal("expected a marshal failure for an unencodable value")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".index-") {
			t.Fatalf("temporary file survived: %s", entry.Name())
		}
	}
	// The prior good content must still be readable after the failed write.
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(payload), `"ok"`) {
		t.Fatalf("failed write damaged the existing file: %s", payload)
	}
}

// TestKindFilteringIsCaseInsensitiveEverywhere drives one table through both
// filters. ProjectPublic used an exact-match set while Search lowercased, so
// the same policy string selected different entities depending on the call.
func TestKindFilteringIsCaseInsensitiveEverywhere(t *testing.T) {
	layout := testLayout(t)
	entities := []Entity{
		{ID: "service.alpha", Name: "Alpha", Kind: "Service", Visibility: VisibilityPublic, Description: "alpha"},
		{ID: "system.beta", Name: "Beta", Kind: "system", Visibility: VisibilityPublic, Description: "beta"},
	}
	engine, err := New(layout, WithProviders(StaticProvider{Entities: entities}))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := engine.Reindex(context.Background()); err != nil {
		t.Fatal(err)
	}
	index, err := engine.LoadIndex(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"service", "Service", "SERVICE"} {
		projection, err := ProjectPublic(context.Background(), index, ProjectionPolicy{IncludeKinds: []string{kind}})
		if err != nil {
			t.Fatal(err)
		}
		if len(projection.Items) != 1 || projection.Items[0].ID != "service.alpha" {
			t.Fatalf("ProjectPublic(kind=%q) = %+v, want only service.alpha", kind, projection.Items)
		}
		results, err := engine.Search(context.Background(), "alpha", SearchOptions{Kinds: []string{kind}})
		if err != nil {
			t.Fatal(err)
		}
		if len(results) != 1 || results[0].Entity.ID != "service.alpha" {
			t.Fatalf("Search(kind=%q) = %+v, want only service.alpha", kind, results)
		}
	}
}
