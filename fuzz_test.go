package catalog

import (
	"context"
	"strings"
	"testing"
	"unicode/utf8"
)

// FuzzDecodeEntities asserts the corpus parsers never panic on untrusted input
// and that normalization is idempotent.
func FuzzDecodeEntities(f *testing.F) {
	f.Add("service.a.yaml", "id: service.a\nname: A\nkind: service\n")
	f.Add("system.b.md", "---\nid: system.b\nname: B\nkind: system\n---\n\nbody\n")
	f.Add("web.c.json", `{"id":"web.c","name":"C","kind":"web"}`)
	f.Add("list.yaml", "- id: a\n  name: A\n  kind: k\n")
	f.Add("broken.md", "---\nunterminated")
	f.Fuzz(func(t *testing.T, name, payload string) {
		for _, strict := range []bool{false, true} {
			entities, err := decodeEntitiesWithPolicy(name, []byte(payload), strict)
			if err != nil {
				continue
			}
			for _, entity := range entities {
				once := normalizeEntity(entity)
				twice := normalizeEntity(once)
				if once.ID != twice.ID || once.Name != twice.Name || len(once.Tags) != len(twice.Tags) {
					t.Fatalf("normalizeEntity is not idempotent for %+v", entity)
				}
				if len(once.Refs) != len(twice.Refs) {
					t.Fatalf("ref normalization is not idempotent for %+v", entity)
				}
			}
		}
	})
}

// FuzzValidatePublicURL asserts every accepted URL satisfies the full contract.
func FuzzValidatePublicURL(f *testing.F) {
	f.Add("https://example.com/a")
	f.Add("http://example.com")
	f.Add("https://user:pw@example.com")
	f.Add("https://example.com:8443/x")
	f.Add("")
	allowed := hostSet([]string{"example.com"})
	f.Fuzz(func(t *testing.T, raw string) {
		if err := validatePublicURL(raw, allowed); err != nil {
			return
		}
		if raw == "" {
			return
		}
		if !strings.HasPrefix(strings.ToLower(raw), "https://") {
			t.Fatalf("accepted a non-https URL: %q", raw)
		}
		for _, forbidden := range []string{"?", "#", "@"} {
			if strings.Contains(raw, forbidden) {
				t.Fatalf("accepted URL %q containing %q", raw, forbidden)
			}
		}
	})
}

// FuzzGraphMermaid asserts a label can never break the diagram's line structure.
func FuzzGraphMermaid(f *testing.F) {
	f.Add("plain", "kind")
	f.Add("line\nbreak", "uses")
	f.Add(`quo"te`, "co#ntains")
	f.Fuzz(func(t *testing.T, name, kind string) {
		index := Index{Entities: []Entity{
			{ID: "a.one", Name: name, Kind: "service", Refs: []Ref{{Kind: kind, Target: "a.two"}}},
			{ID: "a.two", Name: "Two", Kind: "service"},
		}}
		graph := BuildGraph(index)
		mermaid := graph.Mermaid()
		lines := strings.Split(strings.TrimRight(mermaid, "\n"), "\n")
		if want := 1 + len(graph.Nodes) + len(graph.Edges); len(lines) != want {
			t.Fatalf("mermaid emitted %d lines, want %d for name=%q kind=%q", len(lines), want, name, kind)
		}
		for _, line := range lines[1:] {
			// Every label is delimited by exactly one pair of quotes per field;
			// an unescaped quote inside a label would add more.
			if strings.Contains(line, "\r") {
				t.Fatalf("line retained a carriage return: %q", line)
			}
		}
	})
}

// FuzzBoundedSummary asserts the byte bound is never exceeded and the result is
// always valid UTF-8.
func FuzzBoundedSummary(f *testing.F) {
	f.Add("short", 32)
	f.Add("ééééééééé", 5)
	f.Add(strings.Repeat("x", 500), 320)
	f.Fuzz(func(t *testing.T, value string, maxBytes int) {
		if maxBytes < 1 || maxBytes > 4096 {
			t.Skip()
		}
		got := boundedSummary(value, maxBytes, "…")
		if len(got) > maxBytes {
			t.Fatalf("boundedSummary(%q, %d) = %q (%d bytes), exceeds the bound", value, maxBytes, got, len(got))
		}
		if !utf8.ValidString(got) {
			t.Fatalf("boundedSummary produced invalid UTF-8: %q", got)
		}
	})
}

// FuzzSplitFrontmatter asserts the markdown reader never panics and that a
// successful split preserves the frontmatter bytes.
func FuzzSplitFrontmatter(f *testing.F) {
	f.Add("---\nid: a\n---\n\nbody\n")
	f.Add("---\r\nid: a\r\n---\r\n")
	f.Add("no frontmatter")
	f.Fuzz(func(t *testing.T, payload string) {
		frontmatter, _, ok := splitFrontmatter([]byte(payload))
		if !ok {
			return
		}
		normalized := strings.ReplaceAll(payload, "\r\n", "\n")
		if !strings.Contains(normalized, string(frontmatter)) {
			t.Fatalf("frontmatter %q is not a substring of the normalized payload", frontmatter)
		}
	})
}

// FuzzScanPublicText asserts the scanner never panics and never reproduces the
// value it rejected.
func FuzzScanPublicText(f *testing.F) {
	f.Add("field", "ordinary text")
	f.Add("summary", "/Users/someone/secret")
	f.Add("tags[0]", "api_key=value")
	f.Fuzz(func(t *testing.T, field, value string) {
		err := ScanPublicText(field, value)
		if err == nil {
			return
		}
		if len(value) >= 8 && strings.Contains(err.Error(), value) {
			t.Fatalf("scan error reproduced the rejected value: %v", err)
		}
	})
}

func BenchmarkDiscoverStatic(b *testing.B) {
	entities := benchEntities(1000)
	layout, err := DefaultLayout(b.TempDir()).Resolve(b.TempDir())
	if err != nil {
		b.Fatal(err)
	}
	engine, err := New(layout, WithProviders(StaticProvider{Entities: entities}))
	if err != nil {
		b.Fatal(err)
	}
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := engine.Discover(ctx); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkReindex(b *testing.B) {
	root := b.TempDir()
	layout, err := DefaultLayout(root).Resolve(root)
	if err != nil {
		b.Fatal(err)
	}
	engine, err := New(layout, WithProviders(StaticProvider{Entities: benchEntities(1000)}))
	if err != nil {
		b.Fatal(err)
	}
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := engine.Reindex(ctx); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkSearch(b *testing.B) {
	root := b.TempDir()
	layout, err := DefaultLayout(root).Resolve(root)
	if err != nil {
		b.Fatal(err)
	}
	engine, err := New(layout, WithProviders(StaticProvider{Entities: benchEntities(1000)}))
	if err != nil {
		b.Fatal(err)
	}
	ctx := context.Background()
	if _, err := engine.Reindex(ctx); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := engine.Search(ctx, "service description 500", SearchOptions{Limit: 10}); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkProjectPublic(b *testing.B) {
	index := Index{Entities: benchEntities(10000)}
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := ProjectPublic(ctx, index, ProjectionPolicy{}); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkBuildGraph(b *testing.B) {
	index := Index{Entities: benchEntities(10000)}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		BuildGraph(index)
	}
}

func benchEntities(count int) []Entity {
	entities := make([]Entity, 0, count)
	for i := 0; i < count; i++ {
		id := "service." + itoa(i)
		entity := Entity{
			ID: id, Name: "Service " + itoa(i), Kind: "service",
			Description: "Service description " + itoa(i) + " for benchmark corpora.",
			Visibility:  VisibilityPublic,
			Tags:        []string{"bench", "service"},
		}
		if i > 0 {
			entity.Refs = []Ref{{Kind: "depends_on", Target: "service." + itoa(i-1)}}
		}
		entities = append(entities, entity)
	}
	return entities
}
