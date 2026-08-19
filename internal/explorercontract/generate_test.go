package explorercontract

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGeneratedContractIsStableAndValid(t *testing.T) {
	schemaA, typesA, err := Generated()
	if err != nil {
		t.Fatal(err)
	}
	schemaB, typesB, err := Generated()
	if err != nil {
		t.Fatal(err)
	}
	if string(schemaA) != string(schemaB) || string(typesA) != string(typesB) {
		t.Fatal("contract generation is not deterministic")
	}
	var decoded map[string]any
	if err := json.Unmarshal(schemaA, &decoded); err != nil {
		t.Fatal(err)
	}
	for _, needle := range []string{"Explorer v1", "ProjectionMode", "source_digest", "GraphPage"} {
		if !strings.Contains(string(schemaA)+string(typesA), needle) {
			t.Fatalf("generated contract missing %q", needle)
		}
	}
}

func TestWriteGeneratedCreatesAndChecksArtifacts(t *testing.T) {
	root := t.TempDir()
	if err := WriteGenerated(root, false); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{filepath.Join(root, "schema", "explorer-v1.schema.json"), filepath.Join(root, "explorer", "src", "generated", "contract.ts")} {
		if info, err := os.Stat(path); err != nil || info.Size() == 0 {
			t.Fatalf("generated %s: %v %+v", path, err, info)
		}
	}
	if err := WriteGenerated(root, true); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "schema", "explorer-v1.schema.json"), []byte("drift"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := WriteGenerated(root, true); err == nil {
		t.Fatal("generated drift passed")
	}
	blocked := filepath.Join(t.TempDir(), "blocked")
	if err := os.WriteFile(blocked, []byte("file"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := WriteGenerated(blocked, false); err == nil {
		t.Fatal("generation through a file succeeded")
	}
}

func TestProjectionModeVocabulary(t *testing.T) {
	if !ProjectionLocal.Valid() || !ProjectionPublic.Valid() || ProjectionMode("private").Valid() {
		t.Fatal("projection mode vocabulary drifted")
	}
}
