package catalog

import (
	"reflect"
	"testing"
)

// TestCorpusPolicyGoldenDecisions is the decision table both engines are
// asserted against. Corpus membership is a privacy boundary — an archive tree
// that one reader skips and another walks resurrects tombstoned entities — so
// every rule is pinned by name rather than inferred from behavior.
func TestCorpusPolicyGoldenDecisions(t *testing.T) {
	dirs := []struct {
		name        string
		wantDefault SkipReason
		wantStrict  SkipReason
	}{
		{"services", SkipNone, SkipNone},
		{"_archive", SkipUnderscoreDir, SkipUnderscoreDir},
		{"_generated", SkipUnderscoreDir, SkipUnderscoreDir},
		{"_graphs", SkipUnderscoreDir, SkipUnderscoreDir},
		{"_scratch", SkipUnderscoreDir, SkipUnderscoreDir},
		{".cache", SkipDotDir, SkipDeniedDir},
		{".git", SkipDotDir, SkipNone},
		{"node_modules", SkipDeniedDir, SkipNone},
		{"vendor", SkipDeniedDir, SkipNone},
		{"products", SkipNone, SkipNone},
	}
	defaultPolicy := DefaultCorpusPolicy().Normalize()
	strictPolicy := StrictMarkdownCorpusPolicy().Normalize()
	for _, tt := range dirs {
		t.Run("dir/"+tt.name, func(t *testing.T) {
			if got := defaultPolicy.DecideDir(tt.name); got.Reason != tt.wantDefault {
				t.Fatalf("default DecideDir(%q) = %+v, want reason %q", tt.name, got, tt.wantDefault)
			}
			if got := strictPolicy.DecideDir(tt.name); got.Reason != tt.wantStrict {
				t.Fatalf("strict DecideDir(%q) = %+v, want reason %q", tt.name, got, tt.wantStrict)
			}
		})
	}

	files := []struct {
		name        string
		wantDefault SkipReason
		wantStrict  SkipReason
	}{
		{"service.alpha.md", SkipNone, SkipNone},
		{"service.alpha.yaml", SkipNone, SkipUnknownExtension},
		{"service.alpha.yml", SkipNone, SkipUnknownExtension},
		{"service.alpha.json", SkipNone, SkipUnknownExtension},
		{"shard.mmd", SkipUnknownExtension, SkipUnknownExtension},
		{"README", SkipUnknownExtension, SkipUnknownExtension},
		{"_OVERVIEW.md", SkipNone, SkipUnderscoreFile},
		{"_GRAPH.mmd", SkipUnknownExtension, SkipUnderscoreFile},
		{"_INDEX.md", SkipNone, SkipUnderscoreFile},
		// Case folding is on by default and off in the strict preset, so a
		// host comparing generated output byte-for-byte cannot have its corpus
		// grow by someone committing an uppercase extension.
		{"Service.Alpha.MD", SkipNone, SkipUnknownExtension},
	}
	for _, tt := range files {
		t.Run("file/"+tt.name, func(t *testing.T) {
			if got := defaultPolicy.DecideFile(tt.name); got.Reason != tt.wantDefault {
				t.Fatalf("default DecideFile(%q) = %+v, want reason %q", tt.name, got, tt.wantDefault)
			}
			if got := strictPolicy.DecideFile(tt.name); got.Reason != tt.wantStrict {
				t.Fatalf("strict DecideFile(%q) = %+v, want reason %q", tt.name, got, tt.wantStrict)
			}
		})
	}
}

func TestCorpusPolicyNormalizeIsIdempotent(t *testing.T) {
	policy := CorpusPolicy{
		SkipDirNames:       []string{" vendor ", "vendor", "", "node_modules"},
		Extensions:         []string{"MD", ".yaml", " .yaml ", ""},
		CaseFoldExtensions: true,
	}
	once := policy.Normalize()
	twice := once.Normalize()
	if !reflect.DeepEqual(once, twice) {
		t.Fatalf("Normalize is not idempotent:\n once=%+v\ntwice=%+v", once, twice)
	}
	if !reflect.DeepEqual(once.Extensions, []string{".md", ".yaml"}) {
		t.Fatalf("extensions = %v, want [.md .yaml] with the dot restored and case folded", once.Extensions)
	}
	if !reflect.DeepEqual(once.SkipDirNames, []string{"node_modules", "vendor"}) {
		t.Fatalf("skip dirs = %v, want deduped and sorted", once.SkipDirNames)
	}
}

func TestCorpusPolicyEmptyExtensionsAcceptsEverything(t *testing.T) {
	policy := CorpusPolicy{}.Normalize()
	for _, name := range []string{"a.md", "b.txt", "c", "d.MMD"} {
		if got := policy.DecideFile(name); got.Skip {
			t.Fatalf("empty extension set skipped %q: %+v", name, got)
		}
	}
}

// The provider's zero Policy must behave exactly as the historical hardcoded
// rules did, so an existing caller upgrading does not silently change corpora.
func TestFilesystemProviderZeroPolicyMatchesDefault(t *testing.T) {
	resolved := FilesystemProvider{}.resolvedPolicy()
	want := DefaultCorpusPolicy().Normalize()
	if !reflect.DeepEqual(resolved, want) {
		t.Fatalf("zero policy resolved to %+v, want %+v", resolved, want)
	}
}

func TestFilesystemProviderExcludeDirsFoldIntoPolicy(t *testing.T) {
	resolved := FilesystemProvider{ExcludeDirs: []string{"drafts"}}.resolvedPolicy()
	if got := resolved.DecideDir("drafts"); got.Reason != SkipDeniedDir {
		t.Fatalf("ExcludeDirs entry was not honored: %+v", got)
	}
	// The defaults must survive the fold.
	if got := resolved.DecideDir("node_modules"); got.Reason != SkipDeniedDir {
		t.Fatalf("default denied dirs lost after folding ExcludeDirs: %+v", got)
	}
	if got := resolved.DecideDir("_archive"); got.Reason != SkipUnderscoreDir {
		t.Fatalf("archive rule lost after folding ExcludeDirs: %+v", got)
	}
}
