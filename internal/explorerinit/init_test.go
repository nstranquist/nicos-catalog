package explorerinit

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func canonicalTemp(t *testing.T) string {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func TestRunDryWritePresentAndRefuse(t *testing.T) {
	root := canonicalTemp(t)
	dry, err := Run(Options{Root: root, Template: "minimal", DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if !dry.DryRun || len(dry.Planned) != 2 || len(dry.Written) != 0 {
		t.Fatalf("dry = %+v", dry)
	}
	written, err := Run(Options{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	if len(written.Written) != 2 || len(written.Present) != 0 {
		t.Fatalf("written = %+v", written)
	}
	present, err := Run(Options{Root: root, Template: "minimal"})
	if err != nil {
		t.Fatal(err)
	}
	if len(present.Present) != 2 || len(present.Written) != 0 {
		t.Fatalf("present = %+v", present)
	}
	changed := filepath.Join(root, "catalog", "example.md")
	if err := os.WriteFile(changed, []byte("caller data"), 0o644); err != nil {
		t.Fatal(err)
	}
	blocked, err := Run(Options{Root: root, Template: "minimal"})
	if err == nil || len(blocked.Blocked) != 1 || blocked.Blocked[0] != "catalog/example.md" {
		t.Fatalf("blocked = %+v err=%v", blocked, err)
	}
	payload, _ := os.ReadFile(changed)
	if string(payload) != "caller data" {
		t.Fatalf("changed file overwritten: %q", payload)
	}
}

func TestRunDefaultsToCurrentDirectory(t *testing.T) {
	root := canonicalTemp(t)
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(previous) }()
	receipt, err := Run(Options{DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Template != "minimal" || len(receipt.Planned) != 2 {
		t.Fatalf("defaults = %+v", receipt)
	}
}

func TestSampleTemplateIsRelativeAndCoherent(t *testing.T) {
	root := canonicalTemp(t)
	receipt, err := Run(Options{Root: root, Template: "sample"})
	if err != nil {
		t.Fatal(err)
	}
	if len(receipt.Written) < 8 {
		t.Fatalf("sample = %+v", receipt)
	}
	payload, err := json.Marshal(receipt)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(payload), root) || strings.Contains(string(payload), "/Users/") {
		t.Fatalf("receipt leaked root: %s", payload)
	}
	for _, path := range receipt.Written {
		if filepath.IsAbs(path) || strings.Contains(path, "..") {
			t.Fatalf("unsafe receipt path %q", path)
		}
	}
	private, err := os.ReadFile(filepath.Join(root, "catalog", "telemetry.private-sample.yaml"))
	if err != nil || !strings.Contains(string(private), "visibility: private") {
		t.Fatalf("private fixture = %q %v", private, err)
	}
}

func TestRunRejectsUnsafeRootsTemplatesAndComponents(t *testing.T) {
	root := canonicalTemp(t)
	if _, err := Run(Options{Root: root, Template: "unknown"}); err == nil {
		t.Fatal("unknown template succeeded")
	}
	if _, err := Run(Options{Root: filepath.Join(root, "missing")}); err == nil {
		t.Fatal("missing root succeeded")
	}
	file := filepath.Join(root, "file")
	_ = os.WriteFile(file, []byte("x"), 0o644)
	if _, err := Run(Options{Root: file}); err == nil {
		t.Fatal("file root succeeded")
	}
	real := filepath.Join(root, "real")
	_ = os.Mkdir(real, 0o755)
	link := filepath.Join(root, "link")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	if _, err := Run(Options{Root: link}); err == nil {
		t.Fatal("symlink root succeeded")
	}
	unsafe := canonicalTemp(t)
	outside := canonicalTemp(t)
	if err := os.Symlink(outside, filepath.Join(unsafe, "catalog")); err != nil {
		t.Fatal(err)
	}
	if _, err := Run(Options{Root: unsafe}); err == nil {
		t.Fatal("symlink corpus succeeded")
	}
}

func TestSafeTargetAndContains(t *testing.T) {
	root := canonicalTemp(t)
	if _, err := safeTarget(root, "../outside"); err == nil {
		t.Fatal("traversal succeeded")
	}
	if _, err := safeTarget(root, filepath.Join(string(filepath.Separator), "absolute")); err == nil {
		t.Fatal("absolute target succeeded")
	}
	if !contains([]string{"a", "b"}, "b") || contains([]string{"a"}, "z") {
		t.Fatal("contains")
	}
}

func TestRunFilesystemFailuresDoNotOverwrite(t *testing.T) {
	directoryTarget := canonicalTemp(t)
	if err := os.MkdirAll(filepath.Join(directoryTarget, "catalog", "example.md"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := Run(Options{Root: directoryTarget}); err == nil {
		t.Fatal("directory target succeeded")
	}

	failedWrite := canonicalTemp(t)
	owner := filepath.Join(failedWrite, "owner.txt")
	if err := os.WriteFile(owner, []byte("caller data"), 0o644); err != nil {
		t.Fatal(err)
	}
	filesys := systemFileOps()
	filesys.mkdirAll = func(string, os.FileMode) error { return os.ErrPermission }
	receipt, err := run(Options{Root: failedWrite}, filesys)
	if err == nil || len(receipt.Written) != 0 {
		t.Fatalf("injected write failure = %+v %v", receipt, err)
	}
	payload, readErr := os.ReadFile(owner)
	if readErr != nil || string(payload) != "caller data" {
		t.Fatalf("caller file changed = %q %v", payload, readErr)
	}
	for _, path := range []string{".nicos-catalog", "catalog"} {
		if _, statErr := os.Stat(filepath.Join(failedWrite, path)); !os.IsNotExist(statErr) {
			t.Fatalf("partial output %q remains: %v", path, statErr)
		}
	}
}
