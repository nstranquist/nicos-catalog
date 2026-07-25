package main

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// reindexed prepares a corpus with a built index and returns its host root.
func reindexed(t *testing.T) string {
	t.Helper()
	root := writeDemoCorpus(t)
	var stdout, stderr bytes.Buffer
	if code := run(context.Background(), []string{"--root", root, "reindex"}, &stdout, &stderr); code != 0 {
		t.Fatalf("reindex returned %d: %s", code, stderr.String())
	}
	return root
}

func exec(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := run(context.Background(), args, &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

func TestSearchCommand(t *testing.T) {
	root := reindexed(t)
	t.Run("human", func(t *testing.T) {
		code, stdout, stderr := exec(t, "--root", root, "search", "alpha")
		if code != 0 {
			t.Fatalf("search returned %d: %s", code, stderr)
		}
		if !strings.Contains(stdout, "system.alpha") {
			t.Fatalf("search output = %q", stdout)
		}
	})
	t.Run("json", func(t *testing.T) {
		code, stdout, stderr := exec(t, "--json", "--root", root, "search", "--limit", "1", "alpha")
		if code != 0 {
			t.Fatalf("search returned %d: %s", code, stderr)
		}
		var results []map[string]any
		if err := json.Unmarshal([]byte(stdout), &results); err != nil {
			t.Fatalf("search json = %q: %v", stdout, err)
		}
		if len(results) != 1 {
			t.Fatalf("search returned %d results, want 1", len(results))
		}
	})
	t.Run("kind filter", func(t *testing.T) {
		code, stdout, _ := exec(t, "--root", root, "search", "--kinds", "service", "beta")
		if code != 0 {
			t.Fatalf("search returned %d", code)
		}
		if strings.Contains(stdout, "system.alpha") {
			t.Fatalf("kind filter leaked a system: %q", stdout)
		}
	})
	t.Run("bad flag exits two", func(t *testing.T) {
		if code, _, _ := exec(t, "--root", root, "search", "--nope"); code != 2 {
			t.Fatalf("bad flag returned %d, want 2", code)
		}
	})
	t.Run("empty query exits one", func(t *testing.T) {
		if code, _, _ := exec(t, "--root", root, "search"); code != 1 {
			t.Fatalf("empty query returned %d, want 1", code)
		}
	})
}

func TestGraphCommand(t *testing.T) {
	root := reindexed(t)
	t.Run("mermaid", func(t *testing.T) {
		code, stdout, stderr := exec(t, "--root", root, "graph")
		if code != 0 {
			t.Fatalf("graph returned %d: %s", code, stderr)
		}
		if !strings.HasPrefix(stdout, "graph LR") {
			t.Fatalf("graph output = %q", stdout)
		}
	})
	t.Run("json", func(t *testing.T) {
		code, stdout, _ := exec(t, "--json", "--root", root, "graph")
		if code != 0 {
			t.Fatalf("graph returned %d", code)
		}
		var graph struct {
			Nodes []map[string]any `json:"nodes"`
			Edges []map[string]any `json:"edges"`
		}
		if err := json.Unmarshal([]byte(stdout), &graph); err != nil {
			t.Fatalf("graph json = %q: %v", stdout, err)
		}
		if len(graph.Nodes) != 2 || len(graph.Edges) != 1 {
			t.Fatalf("graph = %d nodes / %d edges, want 2/1", len(graph.Nodes), len(graph.Edges))
		}
	})
	t.Run("format json flag", func(t *testing.T) {
		code, stdout, _ := exec(t, "--root", root, "graph", "--format", "json")
		if code != 0 {
			t.Fatalf("graph returned %d", code)
		}
		if !strings.HasPrefix(strings.TrimSpace(stdout), "{") {
			t.Fatalf("graph --format json output = %q", stdout)
		}
	})
	t.Run("bad flag exits two", func(t *testing.T) {
		if code, _, _ := exec(t, "--root", root, "graph", "--nope"); code != 2 {
			t.Fatalf("bad flag returned %d, want 2", code)
		}
	})
	t.Run("no index exits one", func(t *testing.T) {
		if code, _, _ := exec(t, "--root", writeDemoCorpus(t), "graph"); code != 1 {
			t.Fatalf("missing index returned %d, want 1", code)
		}
	})
}

func TestProjectCommand(t *testing.T) {
	root := reindexed(t)
	t.Run("json", func(t *testing.T) {
		code, stdout, stderr := exec(t, "--json", "--root", root, "project")
		if code != 0 {
			t.Fatalf("project returned %d: %s", code, stderr)
		}
		var projection struct {
			SchemaVersion int              `json:"schema_version"`
			Items         []map[string]any `json:"items"`
		}
		if err := json.Unmarshal([]byte(stdout), &projection); err != nil {
			t.Fatalf("project json = %q: %v", stdout, err)
		}
		if len(projection.Items) != 2 {
			t.Fatalf("projected %d items, want 2", len(projection.Items))
		}
		for _, item := range projection.Items {
			for _, forbidden := range []string{"entrypoint", "owner", "annotations", "visibility"} {
				if _, ok := item[forbidden]; ok {
					t.Fatalf("projection leaked %q: %v", forbidden, item)
				}
			}
		}
	})
	t.Run("human", func(t *testing.T) {
		code, stdout, _ := exec(t, "--root", root, "project")
		if code != 0 {
			t.Fatalf("project returned %d", code)
		}
		if !strings.Contains(stdout, "system.alpha") {
			t.Fatalf("project output = %q", stdout)
		}
	})
	t.Run("kind and tag filters", func(t *testing.T) {
		code, stdout, _ := exec(t, "--json", "--root", root, "project", "--kinds", "service")
		if code != 0 {
			t.Fatalf("project returned %d", code)
		}
		if strings.Contains(stdout, "system.alpha") {
			t.Fatalf("kind filter leaked a system: %q", stdout)
		}
	})
	t.Run("non-public visibility exits one", func(t *testing.T) {
		if code, _, _ := exec(t, "--root", root, "project", "--visibility", "internal"); code != 1 {
			t.Fatalf("non-public visibility returned %d, want 1", code)
		}
	})
	t.Run("bad flag exits two", func(t *testing.T) {
		if code, _, _ := exec(t, "--root", root, "project", "--nope"); code != 2 {
			t.Fatalf("bad flag returned %d, want 2", code)
		}
	})
}

func TestValidateCommand(t *testing.T) {
	root := writeDemoCorpus(t)
	code, stdout, stderr := exec(t, "--json", "--root", root, "validate")
	if code != 0 {
		t.Fatalf("validate returned %d: %s", code, stderr)
	}
	var report struct {
		OK          bool `json:"ok"`
		EntityCount int  `json:"entity_count"`
	}
	if err := json.Unmarshal([]byte(stdout), &report); err != nil {
		t.Fatalf("validate json = %q: %v", stdout, err)
	}
	if !report.OK || report.EntityCount != 2 {
		t.Fatalf("validate report = %+v", report)
	}
}

func TestUsageAndHelp(t *testing.T) {
	for _, args := range [][]string{{}, {"--help"}} {
		code, stdout, stderr := exec(t, args...)
		text := stdout + stderr
		if !strings.Contains(text, "Nicos Catalog") {
			t.Fatalf("args %v produced no usage banner: %q (code %d)", args, text, code)
		}
		for _, command := range []string{"validate", "reindex", "search", "graph", "drift", "reconcile", "project", "demo", "version"} {
			if !strings.Contains(text, command) {
				t.Fatalf("usage does not mention %q", command)
			}
		}
	}
}

func TestVersionCommandJSON(t *testing.T) {
	code, stdout, stderr := exec(t, "--json", "version")
	if code != 0 {
		t.Fatalf("version returned %d: %s", code, stderr)
	}
	var info map[string]any
	if err := json.Unmarshal([]byte(stdout), &info); err != nil {
		t.Fatalf("version json = %q: %v", stdout, err)
	}
	for _, key := range []string{"version", "schema_version", "capabilities"} {
		if _, ok := info[key]; !ok {
			t.Fatalf("version output missing %q: %v", key, info)
		}
	}
}

func TestDemoWithQuery(t *testing.T) {
	code, stdout, stderr := exec(t, "--json", "demo", "--query", "ownership")
	if code != 0 {
		t.Fatalf("demo returned %d: %s", code, stderr)
	}
	if !strings.Contains(stdout, "ownership") {
		t.Fatalf("demo output does not reflect the query: %q", stdout)
	}
}

func TestDemoBadFlagExitsTwo(t *testing.T) {
	if code, _, _ := exec(t, "demo", "--nope"); code != 2 {
		t.Fatalf("bad demo flag returned %d, want 2", code)
	}
}

func TestInvalidLayoutExitsOne(t *testing.T) {
	// cache_dir nested beneath corpus_dir is rejected by Layout.Validate.
	if code, _, _ := exec(t, "--root", t.TempDir(), "--corpus", "cat", "--cache", "cat/cache", "validate"); code != 1 {
		t.Fatalf("invalid layout returned %d, want 1", code)
	}
}

func TestSplitCSV(t *testing.T) {
	tests := []struct {
		in   string
		want []string
	}{
		{"", nil},
		{"   ", nil},
		{"a", []string{"a"}},
		{"a,b", []string{"a", "b"}},
		{" a , b ", []string{"a", "b"}},
		{"a,,b", []string{"a", "b"}},
		{"a,", []string{"a"}},
	}
	for _, tt := range tests {
		if got := splitCSV(tt.in); !reflect.DeepEqual(got, tt.want) {
			t.Fatalf("splitCSV(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}
