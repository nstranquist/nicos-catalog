package explorerbundle

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nstranquist/nicos-catalog/internal/explorerapi"
	"github.com/nstranquist/nicos-catalog/internal/explorercontract"
)

func publicService(t *testing.T, suffix string) *explorerapi.Service {
	t.Helper()
	dataset := explorercontract.Dataset{
		SchemaVersion: 1, ProjectionMode: explorercontract.ProjectionPublic, SourceDigest: "sha256:bundle-" + suffix,
		Entities: []explorercontract.Entity{{ID: "service.seed-api", Name: "Seed API " + suffix, Kind: "service", Summary: "Synthetic public service."}, {ID: "web.console", Name: "Console", Kind: "web-app", Summary: "Synthetic public UI."}},
		Edges:    []explorercontract.Edge{{Source: "service.seed-api", Target: "web.console", Kind: "serves"}},
	}
	service, err := explorerapi.NewService(dataset)
	if err != nil {
		t.Fatal(err)
	}
	return service
}

func TestExportDeterministicOwnedReplacement(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(root, "public")
	first, err := Export(context.Background(), publicService(t, "one"), Options{OutDir: out, ProductVersion: "v0.3.0"})
	if err != nil {
		t.Fatal(err)
	}
	if !first.OutputChanged || first.EntityCount != 2 || len(first.Files) < 7 {
		t.Fatalf("first receipt = %+v", first)
	}
	for _, name := range []string{"_headers", "index.html", "data/manifest.json", "data/entities.json", "data/graph.json", "data/health.json", "data/search.json"} {
		if _, err := os.Stat(filepath.Join(out, name)); err != nil {
			t.Fatalf("missing %s: %v", name, err)
		}
	}
	headerBytes, err := os.ReadFile(filepath.Join(out, "_headers"))
	if err != nil {
		t.Fatal(err)
	}
	if string(headerBytes) != staticHeaders {
		t.Fatalf("static headers changed:\n%s", headerBytes)
	}
	for _, directive := range []string{
		"Content-Security-Policy:",
		"frame-ancestors 'none'",
		"Permissions-Policy:",
		"Referrer-Policy: no-referrer",
		"X-Content-Type-Options: nosniff",
		"X-Frame-Options: DENY",
	} {
		if !strings.Contains(string(headerBytes), directive) {
			t.Errorf("static headers do not contain %q", directive)
		}
	}
	indexBytes, err := os.ReadFile(filepath.Join(out, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(indexBytes), staticSourceMarker) || strings.Contains(string(indexBytes), liveSourceMarker) {
		t.Fatalf("static source marker missing from index: %s", indexBytes)
	}
	manifestBytes, err := os.ReadFile(filepath.Join(out, "data", "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest explorercontract.Manifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.Generator != explorercontract.StaticGenerator || manifest.SourceDigest != "sha256:bundle-one" || manifest.Content.Entities == "" {
		t.Fatalf("manifest = %+v", manifest)
	}
	second, err := Export(context.Background(), publicService(t, "one"), Options{OutDir: out, ProductVersion: "v0.3.0"})
	if err != nil {
		t.Fatal(err)
	}
	if second.OutputChanged {
		t.Fatal("equal export rewrote output")
	}
	third, err := Export(context.Background(), publicService(t, "two"), Options{OutDir: out, ProductVersion: "v0.3.0"})
	if err != nil {
		t.Fatal(err)
	}
	if !third.OutputChanged {
		t.Fatal("changed export did not replace output")
	}
	payload, _ := os.ReadFile(filepath.Join(out, "data", "entities.json"))
	if !strings.Contains(string(payload), "Seed API two") || strings.Contains(string(payload), "Seed API one") {
		t.Fatalf("replacement bytes = %s", payload)
	}
	if matches, _ := filepath.Glob(out + ".previous-*"); len(matches) != 0 {
		t.Fatalf("backup residue = %v", matches)
	}
}

func TestExportRejectsUnsafeTargetsAndModes(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	service := publicService(t, "safe")
	local, _ := explorerapi.NewService(explorercontract.Dataset{SchemaVersion: 1, ProjectionMode: explorercontract.ProjectionLocal, SourceDigest: "sha256:local"})
	if _, err := Export(context.Background(), local, Options{OutDir: filepath.Join(root, "local")}); err == nil {
		t.Fatal("local static export succeeded")
	}
	for _, options := range []Options{{}, {OutDir: string(filepath.Separator)}, {OutDir: filepath.Join(root, "catalog", "export"), ForbiddenRoots: []string{filepath.Join(root, "catalog")}}} {
		if _, err := Export(context.Background(), service, options); err == nil {
			t.Fatalf("unsafe target succeeded: %+v", options)
		}
	}
	unknown := filepath.Join(root, "unknown")
	if err := os.MkdirAll(unknown, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(unknown, "owner.txt"), []byte("caller"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Export(context.Background(), service, Options{OutDir: unknown}); err == nil {
		t.Fatal("unknown non-empty output succeeded")
	}
	file := filepath.Join(root, "file")
	_ = os.WriteFile(file, []byte("x"), 0o644)
	if _, err := Export(context.Background(), service, Options{OutDir: file}); err == nil {
		t.Fatal("file target succeeded")
	}
	if _, err := Export(context.Background(), service, Options{OutDir: filepath.Join(file, "child")}); err == nil {
		t.Fatal("output below file succeeded")
	}
	real := filepath.Join(root, "real")
	_ = os.Mkdir(real, 0o755)
	link := filepath.Join(root, "link")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	if _, err := Export(context.Background(), service, Options{OutDir: filepath.Join(link, "output")}); err == nil {
		t.Fatal("symlink target succeeded")
	}
	empty := filepath.Join(root, "empty")
	_ = os.Mkdir(empty, 0o755)
	if _, err := Export(context.Background(), service, Options{OutDir: empty, ProductVersion: "v0.3.0"}); err != nil {
		t.Fatalf("empty target: %v", err)
	}
	invalidManifest := filepath.Join(root, "invalid-manifest")
	_ = os.MkdirAll(filepath.Join(invalidManifest, "data"), 0o755)
	_ = os.WriteFile(filepath.Join(invalidManifest, "data", "manifest.json"), []byte("not-json"), 0o644)
	if exists, owned, err := targetState(invalidManifest); err != nil || !exists || owned {
		t.Fatalf("invalid manifest state = %v %v %v", exists, owned, err)
	}
}

func TestExportCancellationAndHelpers(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Export(ctx, publicService(t, "cancel"), Options{OutDir: filepath.Join(t.TempDir(), "out")}); err == nil {
		t.Fatal("canceled export succeeded")
	}
	if !sameOrWithin("/a", "/a/b") || sameOrWithin("/a", "/ab") {
		t.Fatal("path containment")
	}
	encoded, err := marshalStable(map[string]int{"b": 2, "a": 1})
	if err != nil || !strings.HasSuffix(string(encoded), "\n") {
		t.Fatalf("stable JSON = %q %v", encoded, err)
	}
	if digest(encoded) == "" {
		t.Fatal("empty digest")
	}
	root := t.TempDir()
	if files, err := relativeFiles(root); err != nil || len(files) != 0 {
		t.Fatalf("empty files = %v %v", files, err)
	}
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	if out, err := validateTarget(filepath.Join(resolvedRoot, "blank-forbidden"), []string{"  "}); err != nil || out == "" {
		t.Fatalf("blank forbidden root = %q %v", out, err)
	}
	if err := rejectSymlinkChain(string(filepath.Separator)); err != nil {
		t.Fatalf("filesystem root symlink check = %v", err)
	}
}

func TestDirectoryAndStateHelpers(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	temp := filepath.Join(root, ".out.tmp-first")
	target := filepath.Join(root, "out")
	if err := os.Mkdir(temp, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeFile(filepath.Join(temp, "nested", "a.txt"), []byte("a")); err != nil {
		t.Fatal(err)
	}
	if err := installDirectory(temp, target, false); err != nil {
		t.Fatal(err)
	}
	if exists, owned, err := targetState(filepath.Join(root, "missing")); err != nil || exists || owned {
		t.Fatalf("missing state = %v %v %v", exists, owned, err)
	}
	file := filepath.Join(root, "plain")
	_ = os.WriteFile(file, []byte("x"), 0o644)
	if exists, owned, err := targetState(file); err != nil || !exists || owned {
		t.Fatalf("file state = %v %v %v", exists, owned, err)
	}
	if _, owned, _ := targetState(filepath.Join(file, "child")); owned {
		t.Fatal("owned export below a file")
	}
	empty := filepath.Join(root, "empty")
	_ = os.Mkdir(empty, 0o755)
	if exists, owned, err := targetState(empty); err != nil || !exists || !owned {
		t.Fatalf("empty state = %v %v %v", exists, owned, err)
	}

	left := filepath.Join(root, "left")
	right := filepath.Join(root, "right")
	_ = os.Mkdir(left, 0o755)
	_ = os.Mkdir(right, 0o755)
	_ = writeFile(filepath.Join(left, "a"), []byte("same"))
	_ = writeFile(filepath.Join(right, "a"), []byte("same"))
	if equal, err := equalTrees(left, right); err != nil || !equal {
		t.Fatalf("equal trees = %v %v", equal, err)
	}
	if _, err := equalTrees(filepath.Join(root, "missing-left"), right); err == nil {
		t.Fatal("missing left tree succeeded")
	}
	if _, err := equalTrees(left, filepath.Join(root, "missing-right")); err == nil {
		t.Fatal("missing right tree succeeded")
	}
	_ = writeFile(filepath.Join(right, "a"), []byte("different"))
	if equal, _ := equalTrees(left, right); equal {
		t.Fatal("different content equal")
	}
	_ = writeFile(filepath.Join(right, "b"), []byte("extra"))
	if equal, _ := equalTrees(left, right); equal {
		t.Fatal("different lists equal")
	}

	replacement := filepath.Join(root, ".out.tmp-second")
	_ = os.Mkdir(replacement, 0o755)
	_ = writeFile(filepath.Join(replacement, "b"), []byte("new"))
	if err := installDirectory(replacement, target, true); err != nil {
		t.Fatal(err)
	}
	if payload, _ := os.ReadFile(filepath.Join(target, "b")); string(payload) != "new" {
		t.Fatalf("replacement = %q", payload)
	}
	collisionTemp := filepath.Join(root, ".out.tmp-collision")
	_ = os.Mkdir(collisionTemp, 0o755)
	collisionBackup := target + ".previous-collision"
	_ = os.Mkdir(collisionBackup, 0o755)
	if err := installDirectory(collisionTemp, target, true); err == nil {
		t.Fatal("backup collision succeeded")
	}
	_ = os.RemoveAll(collisionBackup)
	_ = os.RemoveAll(collisionTemp)
	missingTemp := filepath.Join(root, ".out.tmp-missing")
	if err := installDirectory(missingTemp, target, true); err == nil {
		t.Fatal("missing replacement succeeded")
	}
	if _, err := os.Stat(target); err != nil {
		t.Fatalf("rollback did not restore target: %v", err)
	}
	nameLeft := filepath.Join(root, "name-left")
	nameRight := filepath.Join(root, "name-right")
	_ = os.Mkdir(nameLeft, 0o755)
	_ = os.Mkdir(nameRight, 0o755)
	_ = writeFile(filepath.Join(nameLeft, "a"), []byte("x"))
	_ = writeFile(filepath.Join(nameRight, "b"), []byte("x"))
	if equal, _ := equalTrees(nameLeft, nameRight); equal {
		t.Fatal("different file names equal")
	}
	blockedParent := filepath.Join(root, "blocked")
	_ = os.WriteFile(blockedParent, []byte("file"), 0o644)
	if err := writeFile(filepath.Join(blockedParent, "child"), []byte("x")); err == nil {
		t.Fatal("write through file parent succeeded")
	}
	if err := writeFile(filepath.Join(target, "b"), []byte("replace")); err != nil {
		t.Fatal(err)
	}
	if err := writeFile(target, []byte("directory")); err == nil {
		t.Fatal("write over directory succeeded")
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := build(canceled, filepath.Join(root, "canceled"), publicService(t, "direct"), "v0.3.0"); err == nil {
		t.Fatal("canceled build succeeded")
	}
	rootFile := filepath.Join(root, "root-file")
	_ = os.WriteFile(rootFile, []byte("x"), 0o644)
	if _, err := build(context.Background(), rootFile, publicService(t, "direct"), "v0.3.0"); err == nil {
		t.Fatal("build into file succeeded")
	}
	dataBlocked := filepath.Join(root, "data-blocked")
	_ = os.Mkdir(dataBlocked, 0o755)
	_ = os.WriteFile(filepath.Join(dataBlocked, "data"), []byte("file"), 0o644)
	if _, err := build(context.Background(), dataBlocked, publicService(t, "direct"), "v0.3.0"); err == nil {
		t.Fatal("build through data file succeeded")
	}
	manifestBlocked := filepath.Join(root, "manifest-blocked")
	_ = os.MkdirAll(filepath.Join(manifestBlocked, "data", "manifest.json"), 0o755)
	if _, err := build(context.Background(), manifestBlocked, publicService(t, "direct"), "v0.3.0"); err == nil {
		t.Fatal("build over manifest directory succeeded")
	}
	if _, err := marshalStable(func() {}); err == nil {
		t.Fatal("unsupported JSON succeeded")
	}
	if _, err := relativeFiles(filepath.Join(root, "absent")); err == nil {
		t.Fatal("missing tree succeeded")
	}
}
