package hostcollate

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunReportsWalkCapped(t *testing.T) {
	host := materializeFixtures(t, true, "nstranquist")
	body, err := os.ReadFile(SettingsPath(host.ConfigDir))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(SettingsPath(host.ConfigDir), []byte(string(body)+"    max_repos: 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	report, err := Run(context.Background(), runOpts(host, false))
	if err != nil {
		t.Fatal(err)
	}
	if !report.WalkCapped || report.Walked != 1 {
		t.Fatalf("capped walk report = walked=%d capped=%t", report.Walked, report.WalkCapped)
	}
}

func TestWalkBudgetCapsCheckouts(t *testing.T) {
	host := materializeFixtures(t, true, "nstranquist")
	settings, err := LoadSettings(SettingsPath(host.ConfigDir))
	if err != nil {
		t.Fatal(err)
	}
	settings.GitHub.Collation.MaxRepos = 1
	roots, err := settings.ExpandRoots("", host.HostRoot)
	if err != nil {
		t.Fatal(err)
	}
	clones, err := WalkRoots(roots, settings.WalkPolicy())
	if err != nil {
		t.Fatal(err)
	}
	if len(clones) != 1 {
		t.Fatalf("max_repos=1 got %d clones: %#v", len(clones), clones)
	}
}

func TestWalkSkipDirNames(t *testing.T) {
	root := t.TempDir()
	hidden := filepath.Join(root, "node_modules", "secret")
	writeGitConfig(t, hidden, "https://github.com/nstranquist/hidden.git")
	if err := os.MkdirAll(filepath.Join(hidden, ".nicos"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(hidden, ".nicos", "product.yaml"), []byte("id: product.hidden\nname: Hidden\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	visible := filepath.Join(root, "visible")
	writeGitConfig(t, visible, "https://github.com/nstranquist/visible.git")
	if err := os.MkdirAll(filepath.Join(visible, ".nicos"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(visible, ".nicos", "product.yaml"), []byte("id: product.visible\nname: Visible\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	clones, err := WalkRoots([]string{root}, WalkOptions{SkipDirNames: []string{"node_modules"}})
	if err != nil {
		t.Fatal(err)
	}
	for _, clone := range clones {
		if strings.Contains(clone.Path, "node_modules") || strings.Contains(clone.Path, "hidden") {
			t.Fatalf("skipped tree was visited: %s", clone.Path)
		}
	}
	if len(clones) != 1 {
		t.Fatalf("clones = %#v", clones)
	}
}

func TestSnapshotWriteAndReadWithoutRewalk(t *testing.T) {
	host := materializeFixtures(t, true, "nstranquist")
	report, err := Run(context.Background(), runOpts(host, true))
	if err != nil {
		t.Fatal(err)
	}
	if report.SnapshotDigest == "" || report.SnapshotPath == "" {
		t.Fatalf("apply did not write snapshot: %+v", report)
	}
	if _, err := os.Stat(report.SnapshotPath); err != nil {
		t.Fatal(err)
	}
	opts := runOpts(host, false)
	opts.FromSnapshot = true
	opts.Walk = func(roots []string, wo WalkOptions) ([]Clone, error) {
		t.Fatal("snapshot read walked roots")
		return nil, nil
	}
	loaded, err := Run(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.RecordCount != report.RecordCount || len(loaded.Collated) != len(report.Collated) {
		t.Fatalf("snapshot report drifted: %+v vs %+v", loaded, report)
	}
	if loaded.SnapshotDigest != "" && loaded.SnapshotDigest != report.SnapshotDigest {
		t.Fatalf("digest %q vs %q", loaded.SnapshotDigest, report.SnapshotDigest)
	}
}

func TestStrictEmitDropsApplyInvalid(t *testing.T) {
	host := materializeFixtures(t, true, "nstranquist")
	bad := filepath.Join(host.Repos, "registered-invalid")
	if err := os.MkdirAll(filepath.Join(bad, ".nicos"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeGitConfig(t, bad, "https://github.com/nstranquist/invalid.git")
	if err := os.WriteFile(filepath.Join(bad, ".nicos", "product.yaml"), []byte("id: not a valid id\nname: Bad\nkind: product\nvisibility: nope\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	dry, err := Run(context.Background(), runOpts(host, false))
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range dry.Collated {
		for _, id := range item.EntityIDs {
			if strings.Contains(id, " ") || id == "not" {
				t.Fatalf("invalid id counted as collated: %q", id)
			}
		}
	}
	apply, err := Run(context.Background(), runOpts(host, true))
	if err != nil {
		t.Fatal(err)
	}
	if dry.RecordCount != apply.RecordCount {
		t.Fatalf("dry-run %d != apply %d", dry.RecordCount, apply.RecordCount)
	}
}

func TestMissingCloneInjectedList(t *testing.T) {
	host := materializeFixtures(t, true, "nstranquist")
	body, err := os.ReadFile(SettingsPath(host.ConfigDir))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(SettingsPath(host.ConfigDir), []byte(string(body)+"    compare:\n      enabled: true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	opts := runOpts(host, false)
	opts.ProfileRepos = []string{"nstranquist/collate-docs", "nstranquist/only-on-profile"}
	on, err := Run(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	if len(on.Missing) != 1 || on.Missing[0].Repo != "nstranquist/only-on-profile" {
		t.Fatalf("missing = %+v", on.Missing)
	}
	offHost := materializeFixtures(t, true, "nstranquist")
	off, err := Run(context.Background(), runOpts(offHost, false))
	if err != nil {
		t.Fatal(err)
	}
	if len(off.Missing) != 0 {
		t.Fatalf("compare off leaked missing: %+v", off.Missing)
	}
}

func TestFromSnapshotApplyUsesStoredRecords(t *testing.T) {
	host := materializeFixtures(t, true, "nstranquist")
	if _, err := Run(context.Background(), runOpts(host, true)); err != nil {
		t.Fatal(err)
	}
	opts := runOpts(host, true)
	opts.FromSnapshot = true
	opts.Walk = func(roots []string, wo WalkOptions) ([]Clone, error) {
		t.Fatal("from-snapshot apply walked roots")
		return nil, nil
	}
	report, err := Run(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Applied || report.RecordCount != 2 {
		t.Fatalf("from-snapshot apply = %+v", report)
	}
}

func TestFromSnapshotApplyFailsClosedOnNewHostCollision(t *testing.T) {
	host := materializeFixtures(t, true, "nstranquist")
	if _, err := Run(context.Background(), runOpts(host, true)); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(host.CorpusDir, "product.collate-docs.yaml"), []byte("id: product.collate-docs\nname: Later Host\nkind: product\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	opts := runOpts(host, true)
	opts.FromSnapshot = true
	if _, err := Run(context.Background(), opts); err == nil {
		t.Fatal("from-snapshot apply must fail closed on new host collision")
	}
}

func TestWalkSkipSymlinkDoesNotHideSiblings(t *testing.T) {
	root := t.TempDir()
	if err := os.Symlink(t.TempDir(), filepath.Join(root, "node_modules")); err != nil {
		t.Fatal(err)
	}
	visible := filepath.Join(root, "visible")
	writeGitConfig(t, visible, "https://github.com/nstranquist/after-skip.git")
	if err := os.MkdirAll(filepath.Join(visible, ".nicos"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(visible, ".nicos", "product.yaml"), []byte("id: product.after-skip\nname: After\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	clones, err := WalkRoots([]string{root}, WalkOptions{SkipDirNames: []string{"node_modules"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(clones) != 1 || !strings.HasSuffix(clones[0].Path, "visible") {
		t.Fatalf("sibling after skip symlink hidden: %#v", clones)
	}
}

func TestProfileReposOptsInWithoutCompareSetting(t *testing.T) {
	host := materializeFixtures(t, true, "nstranquist")
	opts := runOpts(host, false)
	opts.ProfileRepos = []string{"nstranquist/collate-docs", "nstranquist/only-on-profile"}
	report, err := Run(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Missing) != 1 || report.Missing[0].Repo != "nstranquist/only-on-profile" {
		t.Fatalf("injected list without compare.enabled: %+v", report.Missing)
	}
}

func TestSnapshotAfterApplyRecordsApplied(t *testing.T) {
	host := materializeFixtures(t, true, "nstranquist")
	report, err := Run(context.Background(), runOpts(host, true))
	if err != nil {
		t.Fatal(err)
	}
	snap, err := ReadSnapshot(host.CacheDir)
	if err != nil {
		t.Fatal(err)
	}
	if !snap.Report.Applied || snap.Report.IndexPath == "" {
		t.Fatalf("snapshot after apply = %+v", snap.Report)
	}
	if report.SnapshotDigest == "" {
		t.Fatal("apply report missing digest")
	}
}

func TestDryRunFailsClosedOnHostCorpusCollision(t *testing.T) {
	host := materializeFixtures(t, true, "nstranquist")
	if err := os.WriteFile(filepath.Join(host.CorpusDir, "product.collate-docs.yaml"), []byte("id: product.collate-docs\nname: Host Copy\nkind: product\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Run(context.Background(), runOpts(host, false))
	if err == nil {
		t.Fatal("host/collated ID collision must fail closed on dry-run")
	}
}

func TestEnrollmentGapsObserveOnly(t *testing.T) {
	host := materializeFixtures(t, true, "nstranquist")
	manifest := filepath.Join(host.HostRoot, "external-projects.yaml")
	if err := os.WriteFile(manifest, []byte("schema_version: 1\nprojects:\n  - id: product.other\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(manifest)
	if err != nil {
		t.Fatal(err)
	}
	opts := runOpts(host, false)
	opts.EnrollManifest = manifest
	report, err := Run(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, gap := range report.Gaps {
		if gap.Repo == "product.collate-docs" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected product.collate-docs gap, got %+v", report.Gaps)
	}
	after, err := os.ReadFile(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatal("enroll manifest was written")
	}
}
