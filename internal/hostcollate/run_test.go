package hostcollate

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	catalog "github.com/nstranquist/nicos-catalog"
)

func TestdataRepos(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Join(filepath.Dir(file), "..", "..", "testdata", "collate", "repos")
}

type fixtureHost struct {
	HostRoot  string
	ConfigDir string
	CacheDir  string
	CorpusDir string
	Sidecars  string
	Repos     string
}

func materializeFixtures(t *testing.T, enabled bool, profile string) fixtureHost {
	t.Helper()
	host := t.TempDir()
	repos := filepath.Join(host, "repos")
	if err := os.CopyFS(repos, os.DirFS(TestdataRepos(t))); err != nil {
		t.Fatal(err)
	}
	writeGitConfig(t, filepath.Join(repos, "registered-match"), "git@github.com:nstranquist/collate-docs.git")
	writeGitConfig(t, filepath.Join(repos, "unregistered-match"), "https://github.com/nstranquist/random-clone.git")
	writeGitConfig(t, filepath.Join(repos, "registered-wrong"), "https://github.com/other-owner/collate-other.git")
	writeGitConfig(t, filepath.Join(repos, "corpus-match"), "https://github.com/nstranquist/collated-seed.git")

	configDir := filepath.Join(host, ".nicos-catalog")
	cacheDir := filepath.Join(configDir, "cache")
	sidecars := filepath.Join(configDir, "sidecars")
	corpusDir := filepath.Join(host, "catalog")
	for _, dir := range []string{configDir, cacheDir, sidecars, corpusDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	settings := "github:\n  profile: " + profile + "\n  collation:\n    enabled: " + boolYAML(enabled) + "\n    roots:\n      - " + repos + "\n"
	if err := os.WriteFile(filepath.Join(configDir, SettingsFileName), []byte(settings), 0o644); err != nil {
		t.Fatal(err)
	}
	return fixtureHost{
		HostRoot: host, ConfigDir: configDir, CacheDir: cacheDir,
		CorpusDir: corpusDir, Sidecars: sidecars, Repos: repos,
	}
}

func boolYAML(v bool) string {
	if v {
		return "true"
	}
	return "false"
}

func writeGitConfig(t *testing.T, repo, url string) {
	t.Helper()
	gitDir := filepath.Join(repo, ".git")
	if err := os.MkdirAll(gitDir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "[remote \"origin\"]\n\turl = " + url + "\n"
	if err := os.WriteFile(filepath.Join(gitDir, "config"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func runOpts(host fixtureHost, apply bool) Options {
	return Options{
		HostRoot:  host.HostRoot,
		ConfigDir: host.ConfigDir,
		CacheDir:  host.CacheDir,
		CorpusDir: host.CorpusDir,
		Sidecars:  host.Sidecars,
		Home:      "/unused-home",
		Apply:     apply,
	}
}

func TestRunDisabledApplyStillRebuildsEmptyIndex(t *testing.T) {
	host := materializeFixtures(t, false, "nstranquist")
	report, err := Run(context.Background(), runOpts(host, true))
	if err != nil {
		t.Fatal(err)
	}
	if report.RecordCount != 0 || !report.Applied || report.IndexPath == "" {
		t.Fatalf("disabled apply = %+v", report)
	}
}

func TestRunDisabledContributesZeroRecords(t *testing.T) {
	host := materializeFixtures(t, false, "nstranquist")
	report, err := Run(context.Background(), runOpts(host, false))
	if err != nil {
		t.Fatal(err)
	}
	if report.Enabled || report.RecordCount != 0 || len(report.Collated) != 0 {
		t.Fatalf("disabled report = %+v", report)
	}
	if report.Reason != "collation disabled" {
		t.Fatalf("reason = %q", report.Reason)
	}
	payload, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	text := string(payload)
	for _, needle := range []string{`"collated":[]`, `"unregistered":[]`, `"wrong_owner":[]`} {
		if !strings.Contains(text, needle) {
			t.Fatalf("disabled JSON missing %s: %s", needle, text)
		}
	}
	if strings.Contains(text, `"collated":null`) || strings.Contains(text, `"unregistered":null`) || strings.Contains(text, `"wrong_owner":null`) {
		t.Fatalf("disabled JSON used null buckets: %s", text)
	}
}

func TestRunMissingProfileContributesZeroRecords(t *testing.T) {
	host := materializeFixtures(t, true, "")
	// rewrite settings with enabled true and no profile
	if err := os.WriteFile(filepath.Join(host.ConfigDir, SettingsFileName), []byte("github:\n  collation:\n    enabled: true\n    roots: ["+host.Repos+"]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	report, err := Run(context.Background(), runOpts(host, false))
	if err != nil {
		t.Fatal(err)
	}
	if report.RecordCount != 0 || len(report.Collated) != 0 {
		t.Fatalf("missing profile must emit nothing: %+v", report)
	}
	if report.Reason != "profile missing" {
		t.Fatalf("reason = %q", report.Reason)
	}
}

func TestRunClassifiesFixtureTrees(t *testing.T) {
	host := materializeFixtures(t, true, "nstranquist")
	report, err := Run(context.Background(), runOpts(host, false))
	if err != nil {
		t.Fatal(err)
	}
	if !report.Enabled || report.Profile != "nstranquist" {
		t.Fatalf("report identity = %+v", report)
	}
	if report.Applied {
		t.Fatal("dry-run must not apply")
	}
	assertBucketRepos(t, "collated", report.Collated, "nstranquist/collate-docs", "nstranquist/collated-seed")
	assertBucketRepos(t, "unregistered", report.Unregistered, "nstranquist/random-clone")
	assertBucketRepos(t, "wrong_owner", report.WrongOwner, "other-owner/collate-other")
	if containsRepo(report.Collated, "nstranquist/random-clone") || containsRepo(report.Collated, "other-owner/collate-other") {
		t.Fatalf("unregistered or wrong-owner leaked into collated: %+v", report.Collated)
	}
	if report.RecordCount != 2 {
		t.Fatalf("record_count = %d, want 2 (product + corpus)", report.RecordCount)
	}
	gotIDs := collectIDs(report.Collated)
	if !contains(gotIDs, "product.collate-docs") || !contains(gotIDs, "service.collated-seed") {
		t.Fatalf("entity ids = %v", gotIDs)
	}
	if contains(gotIDs, "product.collate-other") {
		t.Fatal("wrong-owner entity must not be emitted")
	}
}

func TestRunDryRunDoesNotWriteIndexOrMutateRepos(t *testing.T) {
	host := materializeFixtures(t, true, "nstranquist")
	before := snapshotTree(t, host.Repos)
	report, err := Run(context.Background(), runOpts(host, false))
	if err != nil {
		t.Fatal(err)
	}
	if report.Applied || report.IndexPath != "" {
		t.Fatalf("dry-run wrote index: %+v", report)
	}
	if _, err := os.Stat(filepath.Join(host.CacheDir, "index.json")); !os.IsNotExist(err) {
		t.Fatal("dry-run must not create index.json")
	}
	after := snapshotTree(t, host.Repos)
	if after != before {
		t.Fatal("dry-run mutated fixture repos")
	}
}

func TestRunApplyWritesDerivedIndexOnly(t *testing.T) {
	host := materializeFixtures(t, true, "nstranquist")
	before := snapshotTree(t, host.Repos)
	report, err := Run(context.Background(), runOpts(host, true))
	if err != nil {
		t.Fatal(err)
	}
	if !report.Applied || report.IndexPath == "" {
		t.Fatalf("apply report = %+v", report)
	}
	if !strings.HasPrefix(report.IndexPath, host.CacheDir) {
		t.Fatalf("index escaped cache: %s", report.IndexPath)
	}
	after := snapshotTree(t, host.Repos)
	if after != before {
		t.Fatal("apply mutated fixture repos")
	}
	payload, err := os.ReadFile(report.IndexPath)
	if err != nil {
		t.Fatal(err)
	}
	var index catalog.Index
	if err := json.Unmarshal(payload, &index); err != nil {
		t.Fatal(err)
	}
	ids := map[string]bool{}
	for _, entity := range index.Entities {
		ids[entity.ID] = true
	}
	if !ids["product.collate-docs"] || !ids["service.collated-seed"] {
		t.Fatalf("index entities = %v", ids)
	}
	if ids["product.collate-other"] {
		t.Fatal("wrong-owner entity present in derived index")
	}

	layout, err := (catalog.Layout{
		CorpusDir: host.CorpusDir, ConfigDir: host.ConfigDir,
		CacheDir: host.CacheDir, SidecarDataDir: host.Sidecars,
	}).Resolve(host.HostRoot)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := catalog.New(layout)
	if err != nil {
		t.Fatal(err)
	}
	results, err := loaded.Search(context.Background(), "collate docs", catalog.SearchOptions{Limit: 5})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, result := range results {
		if result.Entity.ID == "product.collate-docs" {
			found = true
		}
	}
	if !found {
		t.Fatalf("search missed collated product: %+v", results)
	}
}

func TestRunDuplicateIDsFailClosed(t *testing.T) {
	host := materializeFixtures(t, true, "nstranquist")
	dup := filepath.Join(host.Repos, "registered-dup")
	if err := os.CopyFS(dup, os.DirFS(filepath.Join(TestdataRepos(t), "registered-match"))); err != nil {
		t.Fatal(err)
	}
	writeGitConfig(t, dup, "https://github.com/nstranquist/collate-docs-dup.git")
	dry, err := Run(context.Background(), runOpts(host, false))
	if err == nil {
		t.Fatal("dry-run must return the duplicate-id error and still fill the report")
	}
	if len(dry.Duplicates) == 0 {
		t.Fatalf("dry-run missing duplicate bucket: %+v", dry)
	}
	_, err = Run(context.Background(), runOpts(host, true))
	if err == nil {
		t.Fatal("duplicate ids must fail closed on apply")
	}
	if !strings.Contains(err.Error(), "product.collate-docs") {
		t.Fatalf("error = %v", err)
	}
}

func TestSameRemoteTwoCheckoutsDedupe(t *testing.T) {
	host := materializeFixtures(t, true, "nstranquist")
	alias := filepath.Join(host.Repos, "registered-alias")
	if err := os.CopyFS(alias, os.DirFS(filepath.Join(TestdataRepos(t), "registered-match"))); err != nil {
		t.Fatal(err)
	}
	writeGitConfig(t, alias, "git@github.com:nstranquist/collate-docs.git")
	report, err := Run(context.Background(), runOpts(host, false))
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, item := range report.Collated {
		if item.Repo == "nstranquist/collate-docs" {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("same remote should collapse to one clone, got %d: %+v", n, report.Collated)
	}
	if len(report.Duplicates) != 0 {
		t.Fatalf("same remote should not be a duplicate bucket: %+v", report.Duplicates)
	}
}

func TestClassifyInjectedFactsDoNotNeedDisk(t *testing.T) {
	required := true
	settings := Settings{GitHub: GitHubSettings{
		Profile:   "nstranquist",
		Collation: CollationSettings{Enabled: true, RequireRegistration: &required},
	}}
	items := Classify(settings, []Clone{
		{
			Path:          "/tmp/a",
			Remotes:       []Remote{{Name: "origin", URL: "https://github.com/nstranquist/a.git"}},
			Registrations: []Registration{{Kind: RegistrationProductYAML, Path: "/tmp/a/.nicos/product.yaml"}},
		},
		{
			Path:    "/tmp/b",
			Remotes: []Remote{{Name: "origin", URL: "https://github.com/nstranquist/b.git"}},
		},
		{
			Path:          "/tmp/c",
			Remotes:       []Remote{{Name: "origin", URL: "https://github.com/other/c.git"}},
			Registrations: []Registration{{Kind: RegistrationCorpus, Path: "/tmp/c/catalog"}},
		},
	})
	if len(items) != 3 {
		t.Fatalf("items = %#v", items)
	}
	if itemsByBucket(items, BucketCollated)[0].Path != "/tmp/a" {
		t.Fatalf("collated = %#v", items)
	}
	if itemsByBucket(items, BucketUnregistered)[0].Path != "/tmp/b" {
		t.Fatalf("unregistered = %#v", items)
	}
	if itemsByBucket(items, BucketWrongOwner)[0].Path != "/tmp/c" {
		t.Fatalf("wrong_owner = %#v", items)
	}
}

func TestEmitRecordsSkipsNonCollatedBuckets(t *testing.T) {
	records, err := EmitRecords(context.Background(), Item{Bucket: BucketUnregistered, Path: "/nope"}, Clone{})
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 0 {
		t.Fatalf("unregistered emitted %#v", records)
	}
}

func TestRejectDuplicateIDs(t *testing.T) {
	records := []catalog.Record{
		{Entity: catalog.Entity{ID: "service.alpha"}, Source: "alpha.yaml"},
		{Entity: catalog.Entity{ID: "service.beta"}, Source: "beta.yaml"},
	}
	if err := rejectDuplicateIDs(records); err != nil {
		t.Fatalf("unique records: %v", err)
	}

	records = append(records, catalog.Record{
		Entity: catalog.Entity{ID: "service.alpha"},
		Source: "duplicate.yaml",
	})
	err := rejectDuplicateIDs(records)
	var duplicate *catalog.DuplicateIDError
	if !errors.As(err, &duplicate) {
		t.Fatalf("error = %T %v, want *catalog.DuplicateIDError", err, err)
	}
	if duplicate.EntityID != "service.alpha" || duplicate.First.Source != "alpha.yaml" || duplicate.Second.Source != "duplicate.yaml" {
		t.Fatalf("duplicate detail = %+v", duplicate)
	}
}

func assertBucketRepos(t *testing.T, name string, items []Item, want ...string) {
	t.Helper()
	got := make([]string, 0, len(items))
	for _, item := range items {
		got = append(got, item.Repo)
	}
	if len(got) != len(want) {
		t.Fatalf("%s repos = %v, want %v", name, got, want)
	}
	for _, repo := range want {
		if !contains(got, repo) {
			t.Fatalf("%s missing %s in %v", name, repo, got)
		}
	}
}

func containsRepo(items []Item, repo string) bool {
	for _, item := range items {
		if item.Repo == repo {
			return true
		}
	}
	return false
}

func collectIDs(items []Item) []string {
	var ids []string
	for _, item := range items {
		ids = append(ids, item.EntityIDs...)
	}
	return ids
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func itemsByBucket(items []Item, bucket Bucket) []Item {
	var out []Item
	for _, item := range items {
		if item.Bucket == bucket {
			out = append(out, item)
		}
	}
	return out
}

func TestFormatReportIncludesBuckets(t *testing.T) {
	text := FormatReport(Report{
		Enabled: true, Profile: "nstranquist", RecordCount: 1, Applied: true,
		IndexPath: "/tmp/index.json", Reason: "",
		Collated:     []Item{{Path: "/a", Repo: "nstranquist/a", Source: "product.yaml", EntityIDs: []string{"product.a"}}},
		Unregistered: []Item{{Path: "/b", Repo: "nstranquist/b"}},
		WrongOwner:   []Item{{Path: "/c", Repo: "other/c", Source: "product.yaml"}},
	})
	for _, needle := range []string{"profile=\"nstranquist\"", "collated: 1", "unregistered: 1", "wrong_owner: 1", "applied: /tmp/index.json", "product.a"} {
		if !strings.Contains(text, needle) {
			t.Fatalf("report missing %q:\n%s", needle, text)
		}
	}
}

func TestExpandPathEdges(t *testing.T) {
	if filepath.ToSlash(expandPath("~", "/Users/nico", "/host")) != "/Users/nico" {
		t.Fatal("tilde home")
	}
	if filepath.ToSlash(expandPath("~/", "/Users/nico", "/host")) != "/Users/nico" {
		t.Fatal("tilde slash")
	}
	if filepath.ToSlash(expandPath("~/dev", "/Users/nico", "/host")) != "/Users/nico/dev" {
		t.Fatal("tilde join")
	}
	if expandPath("~", "", "/host") != "" {
		t.Fatal("tilde without home must be empty")
	}
	if expandPath("  ", "/home", "/host") != "" {
		t.Fatal("blank path")
	}
	if filepath.ToSlash(expandPath("rel", "/home", "/host")) != "/host/rel" {
		t.Fatal("relative against host")
	}
}

func TestLooksLikeEntityJSON(t *testing.T) {
	if !looksLikeEntity("{\n  \"id\": \"service.x\"\n}") {
		t.Fatal("json id")
	}
	if looksLikeEntity("# just a comment") {
		t.Fatal("comment is not an entity")
	}
}

func TestWalkRootsSkipsMissingAndNonDir(t *testing.T) {
	file := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	clones, err := WalkRoots([]string{filepath.Join(t.TempDir(), "missing"), file}, WalkOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(clones) != 0 {
		t.Fatalf("clones = %#v", clones)
	}
}

func snapshotTree(t *testing.T, root string) string {
	t.Helper()
	var parts []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		if entry.IsDir() {
			parts = append(parts, rel+"/")
			return nil
		}
		info, statErr := entry.Info()
		if statErr != nil {
			return statErr
		}
		parts = append(parts, rel+":"+info.ModTime().UTC().String())
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return strings.Join(parts, "\n")
}
