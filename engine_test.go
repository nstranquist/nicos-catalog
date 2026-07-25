package catalog

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// failingProvider reports a fixed error, so provider-attribution can be tested
// without touching the filesystem.
type failingProvider struct {
	name string
	err  error
}

func (p failingProvider) Name() string { return p.name }
func (p failingProvider) Provide(context.Context, Layout) ([]Record, error) {
	return nil, p.err
}

// recordingProvider notes whether it was asked for records at all.
type recordingProvider struct {
	name   string
	called *bool
}

func (p recordingProvider) Name() string { return p.name }
func (p recordingProvider) Provide(context.Context, Layout) ([]Record, error) {
	*p.called = true
	return nil, nil
}

// blankNameProvider reports an empty name, which must fail closed rather than
// silently shadowing another provider.
type blankNameProvider struct{}

func (blankNameProvider) Name() string                                      { return "" }
func (blankNameProvider) Provide(context.Context, Layout) ([]Record, error) { return nil, nil }

func TestNewRejectsInvalidConstruction(t *testing.T) {
	layout := testLayout(t)
	tests := []struct {
		name string
		opts []Option
		want error
	}{
		{"nil provider", []Option{WithProviders(nil)}, ErrInvalidProvider},
		{"blank name", []Option{WithProviders(blankNameProvider{})}, ErrInvalidProvider},
		{"duplicate name", []Option{WithProviders(StaticProvider{ProviderName: "dup"}, StaticProvider{ProviderName: "dup"})}, ErrInvalidProvider},
		{"negative limit", []Option{WithLimits(Limits{MaxEntities: -1})}, ErrInvalidLayout},
		{"negative source bytes", []Option{WithLimits(Limits{MaxSourceBytes: -1})}, ErrInvalidLayout},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := New(layout, tt.opts...); !errors.Is(err, tt.want) {
				t.Fatalf("New error = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestNewRejectsInvalidLayout(t *testing.T) {
	if _, err := New(Layout{}); !errors.Is(err, ErrInvalidLayout) {
		t.Fatalf("New with empty layout error = %v, want ErrInvalidLayout", err)
	}
}

func TestNewAcceptsOptionsAndDefaults(t *testing.T) {
	layout := testLayout(t)
	engine, err := New(layout,
		nil, // a nil option is ignored rather than panicking
		WithLogger(slog.New(slog.NewTextHandler(os.Stderr, nil))),
		WithLimits(DefaultLimits()),
	)
	if err != nil {
		t.Fatal(err)
	}
	if got := engine.Layout(); got.CorpusDir != layout.CorpusDir {
		t.Fatalf("Layout() = %+v, want corpus %s", got, layout.CorpusDir)
	}
	// With no WithProviders the engine falls back to a filesystem provider.
	if err := os.MkdirAll(layout.CorpusDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.Discover(context.Background()); err != nil {
		t.Fatalf("default provider discovery failed: %v", err)
	}
}

func TestDiscoverRespectsCancelledContext(t *testing.T) {
	called := false
	engine, err := New(testLayout(t), WithProviders(recordingProvider{name: "recorder", called: &called}))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := engine.Discover(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("Discover error = %v, want context.Canceled", err)
	}
	if called {
		t.Fatal("Discover consulted a provider after the context was cancelled")
	}
}

func TestDiscoverWrapsProviderError(t *testing.T) {
	sentinel := errors.New("upstream exploded")
	engine, err := New(testLayout(t), WithProviders(failingProvider{name: "boom", err: sentinel}))
	if err != nil {
		t.Fatal(err)
	}
	_, err = engine.Discover(context.Background())
	var providerErr *ProviderError
	if !errors.As(err, &providerErr) {
		t.Fatalf("Discover error = %v, want *ProviderError", err)
	}
	if providerErr.Provider != "boom" {
		t.Fatalf("provider = %q, want boom", providerErr.Provider)
	}
	if !errors.Is(err, sentinel) {
		t.Fatal("provider error did not preserve the underlying cause")
	}
	if !strings.Contains(providerErr.Error(), "boom") {
		t.Fatalf("error text %q does not name the provider", providerErr.Error())
	}
}

func TestDiscoverDuplicateIDCarriesBothOrigins(t *testing.T) {
	entity := Entity{ID: "service.dup", Name: "Dup", Kind: "service"}
	engine, err := New(testLayout(t), WithProviders(
		StaticProvider{ProviderName: "a", Entities: []Entity{entity}},
		StaticProvider{ProviderName: "b", Entities: []Entity{entity}},
	))
	if err != nil {
		t.Fatal(err)
	}
	_, err = engine.Discover(context.Background())
	var duplicate *DuplicateIDError
	if !errors.As(err, &duplicate) {
		t.Fatalf("Discover error = %v, want *DuplicateIDError", err)
	}
	if !errors.Is(err, ErrDuplicateEntityID) {
		t.Fatal("duplicate error does not match ErrDuplicateEntityID")
	}
	if duplicate.First.Provider == duplicate.Second.Provider {
		t.Fatalf("both origins report the same provider: %+v", duplicate)
	}
	if !strings.Contains(duplicate.Error(), "service.dup") {
		t.Fatalf("error text %q does not name the entity", duplicate.Error())
	}
}

func TestDiscoverEnforcesLimits(t *testing.T) {
	entities := []Entity{
		{ID: "service.a", Name: "A", Kind: "service"},
		{ID: "service.b", Name: "B", Kind: "service"},
	}
	t.Run("max entities", func(t *testing.T) {
		engine, err := New(testLayout(t), WithProviders(StaticProvider{Entities: entities}), WithLimits(Limits{MaxEntities: 1}))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := engine.Discover(context.Background()); !errors.Is(err, ErrInvalidEntity) {
			t.Fatalf("Discover error = %v, want ErrInvalidEntity", err)
		}
	})
	t.Run("max records per provider", func(t *testing.T) {
		engine, err := New(testLayout(t), WithProviders(StaticProvider{Entities: entities}), WithLimits(Limits{MaxRecordsPerProvider: 1}))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := engine.Discover(context.Background()); !errors.Is(err, ErrInvalidProvider) {
			t.Fatalf("Discover error = %v, want ErrInvalidProvider", err)
		}
	})
}

func TestDiscoverRejectsInvalidEntity(t *testing.T) {
	tests := []struct {
		name   string
		entity Entity
	}{
		{"bad id", Entity{ID: "Not Valid", Name: "N", Kind: "service"}},
		{"missing name", Entity{ID: "service.a", Kind: "service"}},
		{"missing kind", Entity{ID: "service.a", Name: "A"}},
		{"unknown visibility", Entity{ID: "service.a", Name: "A", Kind: "service", Visibility: "semi-public"}},
		{"ref without target", Entity{ID: "service.a", Name: "A", Kind: "service", Refs: []Ref{{Kind: "uses"}}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			engine, err := New(testLayout(t), WithProviders(StaticProvider{Entities: []Entity{tt.entity}}))
			if err != nil {
				t.Fatal(err)
			}
			_, err = engine.Discover(context.Background())
			var entityErr *EntityError
			if !errors.As(err, &entityErr) {
				t.Fatalf("Discover error = %v, want *EntityError", err)
			}
			if !errors.Is(err, ErrInvalidEntity) {
				t.Fatalf("Discover error = %v, want ErrInvalidEntity", err)
			}
		})
	}
}

// TestPerEntityDigestChangesIndependentlyInMultiEntityFile proves the digest is
// a property of the entity rather than of the file that carried it. Before the
// split, every entity in a multi-entity payload shared one digest, so per-entity
// change detection was impossible.
func TestPerEntityDigestChangesIndependentlyInMultiEntityFile(t *testing.T) {
	layout := testLayout(t)
	path := filepath.Join(layout.CorpusDir, "pair.yaml")
	if err := os.MkdirAll(layout.CorpusDir, 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(secondName string) {
		body := "- id: service.first\n  name: First\n  kind: service\n" +
			"- id: service.second\n  name: " + secondName + "\n  kind: service\n"
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("Second")
	engine, err := New(layout, WithProviders(FilesystemProvider{}))
	if err != nil {
		t.Fatal(err)
	}
	before, err := engine.Discover(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(before) != 2 {
		t.Fatalf("discovered %d records, want 2", len(before))
	}
	if before[0].Digest == before[1].Digest {
		t.Fatal("two distinct entities in one file share a digest")
	}
	if before[0].SourceDigest != before[1].SourceDigest {
		t.Fatal("entities from one payload should share a SourceDigest")
	}

	write("Second Renamed")
	after, err := engine.Discover(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if after[0].Digest != before[0].Digest {
		t.Fatal("the unchanged entity's digest moved")
	}
	if after[1].Digest == before[1].Digest {
		t.Fatal("the changed entity's digest did not move")
	}
	if after[0].SourceDigest == before[0].SourceDigest {
		t.Fatal("the payload digest should change when the file changes")
	}
}

func TestValidateReportsReferenceIssues(t *testing.T) {
	entities := []Entity{
		{ID: "system.alpha", Name: "Alpha", Kind: "system", Refs: []Ref{
			{Kind: "contains", Target: "service.missing"},
			{Kind: "relates", Target: "system.alpha"},
		}},
		{ID: "service.beta", Name: "Beta", Kind: "service", Refs: []Ref{
			{Kind: "uses", Target: "system.alpha"},
		}},
	}
	engine, err := New(testLayout(t), WithProviders(StaticProvider{Entities: entities}))
	if err != nil {
		t.Fatal(err)
	}
	report, err := engine.Validate(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !report.OK {
		t.Fatalf("report should be OK when only warnings exist: %+v", report)
	}
	if report.EntityCount != 2 || report.ProviderCount != 1 {
		t.Fatalf("report counts = %+v", report)
	}
	kinds := map[ValidationIssueKind]int{}
	for _, issue := range report.Warnings {
		kinds[issue.Kind]++
	}
	if kinds[IssueDanglingReference] != 1 {
		t.Fatalf("dangling reference warnings = %d, want 1: %+v", kinds[IssueDanglingReference], report.Warnings)
	}
	if kinds[IssueSelfReference] != 1 {
		t.Fatalf("self reference warnings = %d, want 1: %+v", kinds[IssueSelfReference], report.Warnings)
	}
}

func TestValidateStrictReferencesFailsTheReport(t *testing.T) {
	entities := []Entity{
		{ID: "system.alpha", Name: "Alpha", Kind: "system", Refs: []Ref{{Kind: "contains", Target: "service.missing"}}},
	}
	engine, err := New(testLayout(t), WithProviders(StaticProvider{Entities: entities}))
	if err != nil {
		t.Fatal(err)
	}
	report, err := engine.Validate(context.Background(), WithStrictReferences())
	if err != nil {
		t.Fatal(err)
	}
	if report.OK {
		t.Fatal("strict references should fail a corpus with a dangling ref")
	}
	if len(report.Errors) != 1 || report.Errors[0].Severity != SeverityError {
		t.Fatalf("report errors = %+v", report.Errors)
	}
}

func TestValidateDetectsDuplicateReference(t *testing.T) {
	entities := []Entity{
		{ID: "system.alpha", Name: "Alpha", Kind: "system"},
		{ID: "service.beta", Name: "Beta", Kind: "service", Refs: []Ref{
			{Kind: "uses", Target: "system.alpha"},
			{Kind: "uses", Target: "system.alpha"},
		}},
	}
	engine, err := New(testLayout(t), WithProviders(StaticProvider{Entities: entities}))
	if err != nil {
		t.Fatal(err)
	}
	report, err := engine.Validate(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, issue := range report.Warnings {
		if issue.Kind == IssueDuplicateReference {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a duplicate-reference warning, got %+v", report.Warnings)
	}
}

func TestValidateOnEmptyCorpus(t *testing.T) {
	engine, err := New(testLayout(t), WithProviders(StaticProvider{}))
	if err != nil {
		t.Fatal(err)
	}
	report, err := engine.Validate(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !report.OK || report.EntityCount != 0 || len(report.Warnings) != 0 {
		t.Fatalf("empty corpus report = %+v", report)
	}
}

func TestValidatePropagatesDiscoverError(t *testing.T) {
	engine, err := New(testLayout(t), WithProviders(failingProvider{name: "boom", err: errors.New("nope")}))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := engine.Validate(context.Background()); err == nil {
		t.Fatal("expected the discover failure to propagate")
	}
}

func TestSearchSurfaces(t *testing.T) {
	layout := testLayout(t)
	engine, err := New(layout, WithProviders(StaticProvider{Entities: testEntities()}),
		WithLimits(Limits{MaxSearchResults: 1}))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := engine.Reindex(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Run("empty query", func(t *testing.T) {
		if _, err := engine.Search(context.Background(), "   ", SearchOptions{}); !errors.Is(err, ErrEmptyQuery) {
			t.Fatalf("Search error = %v, want ErrEmptyQuery", err)
		}
	})
	t.Run("cancelled context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if _, err := engine.Search(ctx, "alpha", SearchOptions{}); !errors.Is(err, context.Canceled) {
			t.Fatalf("Search error = %v, want context.Canceled", err)
		}
	})
	t.Run("limit clamped by engine limits", func(t *testing.T) {
		results, err := engine.Search(context.Background(), "ownership graph inventory", SearchOptions{Limit: 50})
		if err != nil {
			t.Fatal(err)
		}
		if len(results) > 1 {
			t.Fatalf("returned %d results, want at most the configured max of 1", len(results))
		}
	})
	t.Run("no match", func(t *testing.T) {
		results, err := engine.Search(context.Background(), "zzzznotpresent", SearchOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if len(results) != 0 {
			t.Fatalf("returned %d results for a non-matching query", len(results))
		}
	})
}

func TestSearchPropagatesMissingIndex(t *testing.T) {
	engine, err := New(testLayout(t), WithProviders(StaticProvider{Entities: testEntities()}))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := engine.Search(context.Background(), "alpha", SearchOptions{}); !errors.Is(err, ErrIndexMissing) {
		t.Fatalf("Search error = %v, want ErrIndexMissing", err)
	}
}
