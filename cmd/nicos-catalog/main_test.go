package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	catalog "github.com/nstranquist/nicos-catalog"
)

func TestVersionExpectation(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run(context.Background(), []string{"--json", "version", "--expect", catalog.Version()}, &stdout, &stderr); code != 0 {
		t.Fatalf("run returned %d: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"version": "`+catalog.Version()+`"`) {
		t.Fatalf("unexpected output: %s", stdout.String())
	}
}

func TestVersionExpectMismatchExitsOne(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run(context.Background(), []string{"version", "--expect", "v0.0.0-not-this"}, &stdout, &stderr); code != 1 {
		t.Fatalf("run returned %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "version mismatch") {
		t.Fatalf("unexpected stderr: %s", stderr.String())
	}
}

func TestSyntheticDemoExcludesPrivateEntity(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run(context.Background(), []string{"--json", "demo"}, &stdout, &stderr); code != 0 {
		t.Fatalf("run returned %d: %s", code, stderr.String())
	}
	if strings.Contains(stdout.String(), "telemetry.private-sample") || strings.Contains(stdout.String(), "never projected") {
		t.Fatalf("demo public output leaked private entity: %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "system.orchard") {
		t.Fatalf("demo output missing public entity: %s", stdout.String())
	}
}

// writeDemoCorpus materializes a two-entity corpus and returns the host root.
func writeDemoCorpus(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	corpus := filepath.Join(root, "catalog")
	if err := os.MkdirAll(corpus, 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"system.alpha.md":   "---\nid: system.alpha\nname: Alpha\nkind: system\nvisibility: public\nrefs:\n  - kind: contains\n    target: service.beta\n---\n\nAlpha body paragraph.\n",
		"service.beta.yaml": "id: service.beta\nname: Beta\nkind: service\nvisibility: public\n",
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(corpus, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// TestDriftExitsThreeWhenChanged locks the documented CI contract: drift is
// signalled by exit code 3, distinct from an operational failure (1).
func TestDriftExitsThreeWhenChanged(t *testing.T) {
	root := writeDemoCorpus(t)
	var stdout, stderr bytes.Buffer
	if code := run(context.Background(), []string{"--root", root, "reindex"}, &stdout, &stderr); code != 0 {
		t.Fatalf("reindex returned %d: %s", code, stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	if code := run(context.Background(), []string{"--root", root, "drift"}, &stdout, &stderr); code != 0 {
		t.Fatalf("clean drift returned %d: %s", code, stderr.String())
	}

	if err := os.WriteFile(filepath.Join(root, "catalog", "service.beta.yaml"),
		[]byte("id: service.beta\nname: Beta Renamed\nkind: service\nvisibility: public\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	stderr.Reset()
	code := run(context.Background(), []string{"--json", "--root", root, "drift"}, &stdout, &stderr)
	if code != 3 {
		t.Fatalf("changed drift returned %d, want 3: %s%s", code, stdout.String(), stderr.String())
	}
	var report catalog.DriftReport
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("drift output is not a DriftReport: %v (%s)", err, stdout.String())
	}
	if !report.Changed {
		t.Fatalf("drift report = %+v, want changed", report)
	}
}

func TestUnknownCommandExitsTwo(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run(context.Background(), []string{"no-such-command"}, &stdout, &stderr); code != 2 {
		t.Fatalf("run returned %d, want 2", code)
	}
}

func TestReconcileDryRunDoesNotWrite(t *testing.T) {
	root := writeDemoCorpus(t)
	var stdout, stderr bytes.Buffer
	if code := run(context.Background(), []string{"--root", root, "reindex"}, &stdout, &stderr); code != 0 {
		t.Fatalf("reindex returned %d: %s", code, stderr.String())
	}
	if err := os.WriteFile(filepath.Join(root, "catalog", "service.beta.yaml"),
		[]byte("id: service.beta\nname: Beta Renamed\nkind: service\nvisibility: public\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	stderr.Reset()
	if code := run(context.Background(), []string{"--json", "--root", root, "reconcile"}, &stdout, &stderr); code != 0 {
		t.Fatalf("reconcile returned %d: %s", code, stderr.String())
	}
	var report catalog.ReconcileReport
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("reconcile output is not a ReconcileReport: %v (%s)", err, stdout.String())
	}
	if report.Applied {
		t.Fatal("reconcile without --apply wrote the index")
	}
	if report.Mode != catalog.ReconcileDryRun {
		t.Fatalf("reconcile mode = %v, want dry-run", report.Mode)
	}

	stdout.Reset()
	stderr.Reset()
	if code := run(context.Background(), []string{"--json", "--root", root, "reconcile", "--apply"}, &stdout, &stderr); code != 0 {
		t.Fatalf("reconcile --apply returned %d: %s", code, stderr.String())
	}
	report = catalog.ReconcileReport{}
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if !report.Applied || report.Mode != catalog.ReconcileApply {
		t.Fatalf("reconcile --apply report = %+v, want applied in apply mode", report)
	}
}
