package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func testdataRepos(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Join(filepath.Dir(file), "..", "..", "testdata", "collate", "repos")
}

func writeCollateHost(t *testing.T, enabled bool) string {
	t.Helper()
	host := t.TempDir()
	repos := filepath.Join(host, "repos")
	if err := os.CopyFS(repos, os.DirFS(testdataRepos(t))); err != nil {
		t.Fatal(err)
	}
	writeRemote(t, filepath.Join(repos, "registered-match"), "git@github.com:nstranquist/collate-docs.git")
	writeRemote(t, filepath.Join(repos, "unregistered-match"), "https://github.com/nstranquist/random-clone.git")
	writeRemote(t, filepath.Join(repos, "registered-wrong"), "https://github.com/other-owner/collate-other.git")
	writeRemote(t, filepath.Join(repos, "corpus-match"), "https://github.com/nstranquist/collated-seed.git")
	configDir := filepath.Join(host, ".nicos-catalog")
	if err := os.MkdirAll(filepath.Join(configDir, "cache"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(host, "catalog"), 0o755); err != nil {
		t.Fatal(err)
	}
	enabledYAML := "false"
	if enabled {
		enabledYAML = "true"
	}
	settings := "github:\n  profile: nstranquist\n  collation:\n    enabled: " + enabledYAML + "\n    roots:\n      - " + repos + "\n"
	if err := os.WriteFile(filepath.Join(configDir, "settings.yaml"), []byte(settings), 0o644); err != nil {
		t.Fatal(err)
	}
	return host
}

func writeRemote(t *testing.T, repo, url string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, ".git", "config"), []byte("[remote \"origin\"]\n\turl = "+url+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

type collateReport struct {
	Enabled      bool             `json:"enabled"`
	Profile      string           `json:"profile"`
	Applied      bool             `json:"applied"`
	Collated     []map[string]any `json:"collated"`
	Unregistered []map[string]any `json:"unregistered"`
	WrongOwner   []map[string]any `json:"wrong_owner"`
	RecordCount  int              `json:"record_count"`
	IndexPath    string           `json:"index_path"`
	Reason       string           `json:"reason"`
}

func TestCollateCommandDryRunTwice(t *testing.T) {
	root := writeCollateHost(t, true)
	firstCode, firstOut, firstErr := exec(t, "--json", "--root", root, "collate")
	if firstCode != 0 {
		t.Fatalf("first collate returned %d: %s", firstCode, firstErr)
	}
	secondCode, secondOut, secondErr := exec(t, "--json", "--root", root, "collate")
	if secondCode != 0 {
		t.Fatalf("second collate returned %d: %s", secondCode, secondErr)
	}
	var first, second collateReport
	if err := json.Unmarshal([]byte(firstOut), &first); err != nil {
		t.Fatalf("first json: %v\n%s", err, firstOut)
	}
	if err := json.Unmarshal([]byte(secondOut), &second); err != nil {
		t.Fatalf("second json: %v\n%s", err, secondOut)
	}
	if !first.Enabled || first.Applied || first.RecordCount != 2 {
		t.Fatalf("first report = %+v", first)
	}
	if first.RecordCount != second.RecordCount || len(first.Collated) != len(second.Collated) {
		t.Fatalf("reports drifted: %+v vs %+v", first, second)
	}
	if len(first.Collated) != 2 || len(first.Unregistered) != 1 || len(first.WrongOwner) != 1 {
		t.Fatalf("buckets = collated=%d unregistered=%d wrong_owner=%d", len(first.Collated), len(first.Unregistered), len(first.WrongOwner))
	}
	if repoIn(first.Collated, "other-owner/collate-other") || repoIn(first.Collated, "nstranquist/random-clone") {
		t.Fatalf("non-consent clone leaked into collated: %+v", first.Collated)
	}
	if _, err := os.Stat(filepath.Join(root, ".nicos-catalog", "cache", "index.json")); !os.IsNotExist(err) {
		t.Fatal("dry-run wrote an index")
	}
}

func TestCollateCommandApplyWritesIndex(t *testing.T) {
	root := writeCollateHost(t, true)
	before, err := os.ReadFile(filepath.Join(root, "repos", "registered-match", ".nicos", "product.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr := exec(t, "--json", "--root", root, "collate", "--apply")
	if code != 0 {
		t.Fatalf("apply returned %d: %s", code, stderr)
	}
	var report collateReport
	if err := json.Unmarshal([]byte(stdout), &report); err != nil {
		t.Fatalf("apply json: %v\n%s", err, stdout)
	}
	if !report.Applied || report.IndexPath == "" {
		t.Fatalf("apply report = %+v", report)
	}
	after, err := os.ReadFile(filepath.Join(root, "repos", "registered-match", ".nicos", "product.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatal("apply mutated a scanned repo")
	}
	searchCode, searchOut, searchErr := exec(t, "--root", root, "search", "collate docs")
	if searchCode != 0 {
		t.Fatalf("search returned %d: %s", searchCode, searchErr)
	}
	if !strings.Contains(searchOut, "product.collate-docs") {
		t.Fatalf("search missed collated product: %q", searchOut)
	}
	if strings.Contains(searchOut, "product.collate-other") {
		t.Fatalf("search returned wrong-owner entity: %q", searchOut)
	}
}

func TestReindexKeepsCollatedRecords(t *testing.T) {
	root := writeCollateHost(t, true)
	if code, _, stderr := exec(t, "--json", "--root", root, "collate", "--apply"); code != 0 {
		t.Fatalf("apply returned %d: %s", code, stderr)
	}
	if code, _, stderr := exec(t, "--root", root, "reindex"); code != 0 {
		t.Fatalf("reindex returned %d: %s", code, stderr)
	}
	code, stdout, stderr := exec(t, "--root", root, "search", "collate docs")
	if code != 0 {
		t.Fatalf("search returned %d: %s", code, stderr)
	}
	if !strings.Contains(stdout, "product.collate-docs") {
		t.Fatalf("reindex dropped collated product: %q", stdout)
	}
}

func TestCollateCommandDisabled(t *testing.T) {
	root := writeCollateHost(t, false)
	code, stdout, stderr := exec(t, "--json", "--root", root, "collate")
	if code != 0 {
		t.Fatalf("disabled collate returned %d: %s", code, stderr)
	}
	var report collateReport
	if err := json.Unmarshal([]byte(stdout), &report); err != nil {
		t.Fatal(err)
	}
	if report.Enabled || report.RecordCount != 0 || len(report.Collated) != 0 {
		t.Fatalf("disabled report = %+v", report)
	}
	for _, needle := range []string{`"collated": []`, `"unregistered": []`, `"wrong_owner": []`} {
		if !strings.Contains(stdout, needle) {
			t.Fatalf("disabled CLI JSON missing %s: %s", needle, stdout)
		}
	}
	if strings.Contains(stdout, `"collated": null`) {
		t.Fatalf("disabled CLI JSON used null buckets: %s", stdout)
	}
}

func TestCollateFromSnapshotApplyKeepsIndex(t *testing.T) {
	root := writeCollateHost(t, true)
	if code, _, stderr := exec(t, "--root", root, "collate", "--apply"); code != 0 {
		t.Fatalf("apply returned %d: %s", code, stderr)
	}
	if code, _, stderr := exec(t, "--json", "--root", root, "collate", "--from-snapshot", "--apply"); code != 0 {
		t.Fatalf("from-snapshot --apply returned %d: %s", code, stderr)
	}
	code, stdout, stderr := exec(t, "--root", root, "search", "collate docs")
	if code != 0 {
		t.Fatalf("search returned %d: %s", code, stderr)
	}
	if !strings.Contains(stdout, "product.collate-docs") {
		t.Fatalf("from-snapshot apply dropped collated product: %q", stdout)
	}
}

func TestCollateProfileReposWithoutCompareSetting(t *testing.T) {
	root := writeCollateHost(t, true)
	code, stdout, stderr := exec(t, "--json", "--root", root, "collate", "--profile-repos", "nstranquist/collate-docs,nstranquist/only-on-profile")
	if code != 0 {
		t.Fatalf("returned %d: %s", code, stderr)
	}
	if !strings.Contains(stdout, "nstranquist/only-on-profile") {
		t.Fatalf("missing-clone not reported: %s", stdout)
	}
}

func TestCollateBadFlagExitsTwo(t *testing.T) {
	if code, _, _ := exec(t, "--root", t.TempDir(), "collate", "--nope"); code != 2 {
		t.Fatalf("bad flag returned %d, want 2", code)
	}
}

func repoIn(items []map[string]any, repo string) bool {
	for _, item := range items {
		if item["repo"] == repo {
			return true
		}
	}
	return false
}
