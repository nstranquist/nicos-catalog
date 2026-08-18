package hostcollate

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	catalog "github.com/nstranquist/nicos-catalog"
)

func TestEmitLayoutRecords(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".nicos-catalog"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "catalog"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "catalog", "system.layout-seed.yaml"), []byte("id: system.layout-seed\nname: Layout Seed\nkind: system\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	item := Item{Bucket: BucketCollated, Path: root, Repo: "nstranquist/layout-seed"}
	clone := Clone{Path: root, Registrations: detectRegistrations(root)}
	if len(clone.Registrations) != 1 || clone.Registrations[0].Kind != RegistrationLayout {
		t.Fatalf("registrations = %#v", clone.Registrations)
	}
	records, err := EmitRecords(context.Background(), item, clone)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].Entity.ID != "system.layout-seed" {
		t.Fatalf("records = %#v", records)
	}
	if !strings.Contains(records[0].Source, "nstranquist/layout-seed") {
		t.Fatalf("provenance = %q", records[0].Source)
	}
}

func TestEmitProductYAMLRequiresID(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "product.yaml")
	if err := os.WriteFile(path, []byte("name: Missing ID\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	record, err := emitProductYAML(Item{Path: root, Repo: "nstranquist/x"}, path)
	if err != nil {
		t.Fatal(err)
	}
	if record != nil {
		t.Fatal("missing id must not emit")
	}
}

func TestRecordsProviderDefaultName(t *testing.T) {
	provider := RecordsProvider{}
	if provider.Name() != ProviderName {
		t.Fatalf("name = %q", provider.Name())
	}
	got, err := provider.Provide(context.Background(), catalog.Layout{})
	if err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Fatal("nil records")
	}
}
