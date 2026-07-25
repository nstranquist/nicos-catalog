package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestDemoCorpusIsFullyDecodable walks the corpus shipped in the repository and
// asserts every file decodes. The JSON reader had no test at all despite JSON
// being an advertised format and demo/catalog containing a .json fixture.
func TestDemoCorpusIsFullyDecodable(t *testing.T) {
	root, err := filepath.Abs("demo")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "catalog")); err != nil {
		t.Skipf("demo corpus unavailable: %v", err)
	}
	layout, err := (Layout{
		CorpusDir: filepath.Join(root, "catalog"),
		ConfigDir: filepath.Join(t.TempDir(), "config"),
		CacheDir:  filepath.Join(t.TempDir(), "cache"),

		SidecarDataDir: filepath.Join(t.TempDir(), "sidecars"),
	}).Resolve(root)
	if err != nil {
		t.Fatal(err)
	}
	records, err := FilesystemProvider{Strict: true}.Provide(context.Background(), layout)
	if err != nil {
		t.Fatalf("shipped demo corpus does not decode under strict mode: %v", err)
	}
	if len(records) == 0 {
		t.Fatal("demo corpus produced no records")
	}
	byExt := map[string]int{}
	for _, record := range records {
		byExt[strings.ToLower(filepath.Ext(record.Source))]++
	}
	// The JSON fixture exists precisely so the JSON reader has a real exercise.
	if byExt[".json"] == 0 {
		t.Fatalf("no JSON entity was decoded from the demo corpus: %v", byExt)
	}
}

func TestFilesystemProviderReadsEveryFormat(t *testing.T) {
	layout := testLayout(t)
	writeCorpusFile(t, layout, "system.md.md",
		"---\nid: system.md\nname: From Markdown\nkind: system\n---\n\nBody paragraph becomes the description.\n")
	writeCorpusFile(t, layout, "service.yaml.yaml",
		"id: service.yaml\nname: From YAML\nkind: service\n")
	writeCorpusFile(t, layout, "service.json.json",
		`{"id":"service.json","name":"From JSON","kind":"service"}`)
	writeCorpusFile(t, layout, "list.yaml",
		"- id: service.list-one\n  name: List One\n  kind: service\n- id: service.list-two\n  name: List Two\n  kind: service\n")
	writeCorpusFile(t, layout, "list.json",
		`[{"id":"service.jlist-one","name":"JSON List One","kind":"service"}]`)

	records, err := FilesystemProvider{Strict: true}.Provide(context.Background(), layout)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, record := range records {
		got[record.Entity.ID] = record.Entity.Name
	}
	for id, name := range map[string]string{
		"system.md": "From Markdown", "service.yaml": "From YAML", "service.json": "From JSON",
		"service.list-one": "List One", "service.list-two": "List Two", "service.jlist-one": "JSON List One",
	} {
		if got[id] != name {
			t.Fatalf("entity %s = %q, want %q (all of %v)", id, got[id], name, got)
		}
	}
	// The markdown body supplies the description when frontmatter omits it.
	for _, record := range records {
		if record.Entity.ID == "system.md" && !strings.Contains(record.Entity.Description, "Body paragraph") {
			t.Fatalf("markdown body was not used as the description: %q", record.Entity.Description)
		}
	}
}

func TestFilesystemProviderStrictRejections(t *testing.T) {
	tests := []struct {
		name string
		file string
		body string
	}{
		{"unknown yaml field", "bad.yaml", "id: service.bad\nname: Bad\nkind: service\nnot_a_field: 1\n"},
		{"missing yaml id", "noid.yaml", "name: No ID\nkind: service\n"},
		{"trailing yaml document", "multi.yaml", "id: service.a\nname: A\nkind: service\n---\nid: service.b\n"},
		{"unknown json field", "bad.json", `{"id":"service.j","name":"J","kind":"service","nope":1}`},
		{"missing json id", "noid.json", `{"name":"No ID","kind":"service"}`},
		{"trailing json value", "multi.json", `{"id":"service.a","name":"A","kind":"service"} {"id":"service.b"}`},
		{"unterminated frontmatter", "open.md", "---\nid: service.open\nname: Open\nkind: service\n"},
		{"missing md id", "noid.md", "---\nname: No ID\nkind: service\n---\n\nbody\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			layout := testLayout(t)
			writeCorpusFile(t, layout, tt.file, tt.body)
			if _, err := (FilesystemProvider{Strict: true}).Provide(context.Background(), layout); err == nil {
				t.Fatal("strict mode accepted a malformed entity file")
			}
		})
	}
}

func TestFilesystemProviderLaxModeSkipsUndecodableFiles(t *testing.T) {
	layout := testLayout(t)
	writeCorpusFile(t, layout, "notes.md", "# Just a heading\n\nNo frontmatter here.\n")
	writeCorpusFile(t, layout, "noid.yaml", "name: No ID\nkind: service\n")
	writeCorpusFile(t, layout, "ignored.txt", "id: service.txt\n")
	writeCorpusFile(t, layout, "real.yaml", "id: service.real\nname: Real\nkind: service\n")
	writeCorpusFile(t, layout, "empty.yaml", "   \n")

	records, err := FilesystemProvider{}.Provide(context.Background(), layout)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].Entity.ID != "service.real" {
		t.Fatalf("lax provider returned %+v, want only service.real", records)
	}
}

// TestFilesystemProviderExcludesDirectories covers the discovery-side privacy
// boundary: a directory that is excluded must contribute nothing, even when it
// contains perfectly valid entity files.
func TestFilesystemProviderExcludesDirectories(t *testing.T) {
	layout := testLayout(t)
	writeCorpusFile(t, layout, "keep.yaml", "id: service.keep\nname: Keep\nkind: service\n")
	for _, dir := range []string{".hidden", "node_modules", "vendor", "_archive", "drafts"} {
		writeCorpusFile(t, layout, dir+"/leak.yaml",
			"id: service."+strings.TrimLeft(dir, "._")+"\nname: Leak\nkind: service\n")
	}
	records, err := FilesystemProvider{ExcludeDirs: []string{"drafts"}}.Provide(context.Background(), layout)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].Entity.ID != "service.keep" {
		var ids []string
		for _, record := range records {
			ids = append(ids, record.Entity.ID)
		}
		t.Fatalf("excluded directories leaked entities: %v", ids)
	}
}

// The corpus root itself may be dot-prefixed; only nested dot directories are
// excluded, otherwise a corpus at .catalog would discover nothing.
func TestFilesystemProviderScansDotPrefixedCorpusRoot(t *testing.T) {
	root := t.TempDir()
	layout, err := (Layout{
		CorpusDir: ".catalog", ConfigDir: "conf", CacheDir: "cache", SidecarDataDir: "side",
	}).Resolve(root)
	if err != nil {
		t.Fatal(err)
	}
	writeCorpusFile(t, layout, "service.a.yaml", "id: service.a\nname: A\nkind: service\n")
	records, err := FilesystemProvider{}.Provide(context.Background(), layout)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 {
		t.Fatalf("dot-prefixed corpus root produced %d records, want 1", len(records))
	}
}

// splitFrontmatter must accept CRLF, which is what a Windows checkout produces.
func TestSplitFrontmatterHandlesCRLF(t *testing.T) {
	frontmatter, body, ok := splitFrontmatter([]byte("---\r\nid: service.crlf\r\nname: CRLF\r\n---\r\n\r\nBody text\r\n"))
	if !ok {
		t.Fatal("CRLF frontmatter was not recognized")
	}
	if strings.Contains(string(frontmatter), "\r") || strings.Contains(string(body), "\r") {
		t.Fatalf("carriage returns survived normalization: %q / %q", frontmatter, body)
	}
	if !strings.Contains(string(frontmatter), "id: service.crlf") {
		t.Fatalf("frontmatter = %q", frontmatter)
	}
	if !strings.Contains(string(body), "Body text") {
		t.Fatalf("body = %q", body)
	}
}

func TestSplitFrontmatterRejections(t *testing.T) {
	for _, payload := range []string{"no delimiter at all", "---\nunterminated\n", "text\n---\nid: x\n---\n"} {
		if _, _, ok := splitFrontmatter([]byte(payload)); ok {
			t.Fatalf("splitFrontmatter accepted %q", payload)
		}
	}
}

func TestFirstParagraphSkipsHeadings(t *testing.T) {
	got := firstParagraph("# Heading\n\n## Another\n\nThe real   paragraph\nwrapped.\n\nSecond.")
	if got != "The real paragraph wrapped." {
		t.Fatalf("firstParagraph = %q", got)
	}
	if firstParagraph("# Only a heading") != "" {
		t.Fatal("a heading-only body should produce no description")
	}
}

func TestStaticProviderNameFallbackAndSourceDigest(t *testing.T) {
	records, err := StaticProvider{Entities: []Entity{{ID: "service.a", Name: "A", Kind: "service"}}}.
		Provide(context.Background(), Layout{})
	if err != nil {
		t.Fatal(err)
	}
	if records[0].Provider != "static" {
		t.Fatalf("provider name = %q, want the static fallback", records[0].Provider)
	}
	if !strings.HasPrefix(records[0].SourceDigest, "sha256:") {
		t.Fatalf("SourceDigest = %q, want a sha256 digest", records[0].SourceDigest)
	}
	// Digest is assigned by the engine during Discover, not by the provider.
	if records[0].Digest != "" {
		t.Fatalf("provider set Digest = %q; that is the engine's responsibility", records[0].Digest)
	}
}

func TestFilesystemProviderNameFallback(t *testing.T) {
	if got := (FilesystemProvider{}).Name(); got != "filesystem" {
		t.Fatalf("Name() = %q, want filesystem", got)
	}
	if got := (FilesystemProvider{ProviderName: "  custom  "}).Name(); got != "custom" {
		t.Fatalf("Name() = %q, want custom", got)
	}
}

func TestFilesystemProviderReportsMissingCorpus(t *testing.T) {
	layout := testLayout(t)
	if _, err := (FilesystemProvider{}).Provide(context.Background(), layout); err == nil {
		t.Fatal("a missing corpus directory should be reported rather than silently empty")
	}
}

func TestDecodeJSONStrictAndLax(t *testing.T) {
	var entity Entity
	if err := decodeJSON([]byte(`{"id":"a","name":"A","kind":"k","nope":1}`), &entity, false); err != nil {
		t.Fatalf("lax decode rejected an unknown field: %v", err)
	}
	if err := decodeJSON([]byte(`{"id":"a","name":"A","kind":"k","nope":1}`), &entity, true); err == nil {
		t.Fatal("strict decode accepted an unknown field")
	}
	if err := decodeJSON([]byte(`{"id":"a"} {"id":"b"}`), &entity, true); err == nil {
		t.Fatal("strict decode accepted a trailing value")
	}
	if err := decodeJSON([]byte(`not json`), &entity, true); err == nil {
		t.Fatal("strict decode accepted malformed JSON")
	}
}

func TestDecodeYAMLStrictAndLax(t *testing.T) {
	var entity Entity
	if err := decodeYAML([]byte("id: a\nname: A\nkind: k\nnope: 1\n"), &entity, false); err != nil {
		t.Fatalf("lax decode rejected an unknown field: %v", err)
	}
	if err := decodeYAML([]byte("id: a\nname: A\nkind: k\nnope: 1\n"), &entity, true); err == nil {
		t.Fatal("strict decode accepted an unknown field")
	}
	if err := decodeYAML([]byte("id: a\n---\nid: b\n"), &entity, true); err == nil {
		t.Fatal("strict decode accepted multiple documents")
	}
}

func TestRelativeToFallsBackToAbsolute(t *testing.T) {
	if got := relativeTo("/a/b", "/a/b/c"); got != "c" {
		t.Fatalf("relativeTo = %q, want c", got)
	}
}

// A record whose entity JSON cannot be marshaled must be attributed, not panic.
func TestDigestBytesIsStable(t *testing.T) {
	first := digestBytes([]byte("payload"))
	if first != digestBytes([]byte("payload")) {
		t.Fatal("digestBytes is not deterministic")
	}
	if first == digestBytes([]byte("payload2")) {
		t.Fatal("digestBytes collided on distinct payloads")
	}
}

func TestReindexReportsAndRoundTrips(t *testing.T) {
	layout := testLayout(t)
	engine, err := New(layout, WithProviders(StaticProvider{Entities: testEntities()}))
	if err != nil {
		t.Fatal(err)
	}
	report, err := engine.Reindex(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !report.OK || report.EntityCount != 3 || report.DocumentCount != 3 {
		t.Fatalf("reindex report = %+v", report)
	}
	if report.IndexPath != layout.indexPath() {
		t.Fatalf("index path = %q, want %q", report.IndexPath, layout.indexPath())
	}
	payload, err := os.ReadFile(report.IndexPath)
	if err != nil {
		t.Fatal(err)
	}
	var index Index
	if err := json.Unmarshal(payload, &index); err != nil {
		t.Fatal(err)
	}
	if index.SchemaVersion != SchemaVersion || index.AverageDocumentLength <= 0 {
		t.Fatalf("index = %+v", index)
	}
}

func TestLoadIndexRejectsCorruptPayloads(t *testing.T) {
	layout := testLayout(t)
	engine, err := New(layout, WithProviders(StaticProvider{Entities: testEntities()}))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := engine.Reindex(context.Background()); err != nil {
		t.Fatal(err)
	}
	path := layout.indexPath()

	t.Run("malformed json", func(t *testing.T) {
		if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := engine.LoadIndex(context.Background()); !errors.Is(err, ErrIndexCorrupt) {
			t.Fatalf("LoadIndex error = %v, want ErrIndexCorrupt", err)
		}
	})
	t.Run("entity document mismatch", func(t *testing.T) {
		index := Index{SchemaVersion: SchemaVersion, Entities: []Entity{{ID: "a"}}}
		payload, err := json.Marshal(index)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, payload, 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := engine.LoadIndex(context.Background()); !errors.Is(err, ErrIndexCorrupt) {
			t.Fatalf("LoadIndex error = %v, want ErrIndexCorrupt", err)
		}
	})
	t.Run("cancelled context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if _, err := engine.LoadIndex(ctx); !errors.Is(err, context.Canceled) {
			t.Fatalf("LoadIndex error = %v, want context.Canceled", err)
		}
	})
}

func TestWriteJSONAtomicCreatesParentDirectories(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "deeper", "index.json")
	if err := writeJSONAtomic(path, map[string]int{"n": 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
}
