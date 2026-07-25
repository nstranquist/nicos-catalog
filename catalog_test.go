package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

func testLayout(t *testing.T) Layout {
	t.Helper()
	root := t.TempDir()
	layout, err := DefaultLayout(root).Resolve(root)
	if err != nil {
		t.Fatal(err)
	}
	return layout
}

func testEntities() []Entity {
	return []Entity{
		{ID: "system.alpha", Name: "Alpha Platform", Kind: "system", Description: "Developer ownership graph and service catalog.", Tags: []string{"platform", "public"}, Visibility: VisibilityPublic, PublicURL: "https://example.com/alpha", Entrypoint: "/private/public-entity-path", Annotations: map[string]string{"query": "private public-entity query"}, Refs: []Ref{{Kind: "contains", Target: "service.beta"}}},
		{ID: "service.beta", Name: "Beta API", Kind: "service", Description: "Go API for inventory and dependency search.", Tags: []string{"go", "public"}, Visibility: VisibilityPublic, PublicURL: "https://example.com/beta"},
		{ID: "telemetry.secret", Name: "Host Telemetry", Kind: "telemetry", Description: "Host-only query sample.", Visibility: VisibilityPrivate, Entrypoint: "/private/path", Annotations: map[string]string{"query": "private query"}},
	}
}

func TestReindexSearchDriftAndDeterminism(t *testing.T) {
	ctx := context.Background()
	layout := testLayout(t)
	provider := &StaticProvider{ProviderName: "fixture", Entities: testEntities()}
	engine, err := New(layout, WithProviders(provider))
	if err != nil {
		t.Fatal(err)
	}
	first, err := engine.Reindex(ctx)
	if err != nil {
		t.Fatal(err)
	}
	firstBytes, err := os.ReadFile(first.IndexPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := engine.Reindex(ctx); err != nil {
		t.Fatal(err)
	}
	secondBytes, err := os.ReadFile(first.IndexPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(firstBytes) != string(secondBytes) {
		t.Fatal("identical provider output produced different index bytes")
	}
	results, err := engine.Search(context.Background(), "ownership graph", SearchOptions{Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) == 0 || results[0].Entity.ID != "system.alpha" {
		t.Fatalf("unexpected search results: %#v", results)
	}
	drift, err := engine.Drift(ctx)
	if err != nil || drift.Changed || !drift.OK {
		t.Fatalf("expected clean drift, got %#v err=%v", drift, err)
	}
	provider.Entities[0].Description += " changed"
	drift, err = engine.Drift(ctx)
	if err != nil || !drift.Changed || drift.OK {
		t.Fatalf("expected source drift, got %#v err=%v", drift, err)
	}
	reconciled, err := engine.Reconcile(ctx, ReconcileApply)
	if err != nil || !reconciled.Applied {
		t.Fatalf("expected applied reconcile, got %#v err=%v", reconciled, err)
	}
}

func TestPublicProjectionIsClosed(t *testing.T) {
	engine, err := New(testLayout(t), WithProviders(StaticProvider{Entities: testEntities()}))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := engine.Reindex(context.Background()); err != nil {
		t.Fatal(err)
	}
	index, err := engine.LoadIndex(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	projection, err := ProjectPublic(context.Background(), index, ProjectionPolicy{RequireVisibility: VisibilityPublic, AllowHosts: []string{"example.com"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(projection.Items) != 2 {
		t.Fatalf("expected 2 public entities, got %d", len(projection.Items))
	}
	payload, err := json.Marshal(projection)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"private query", "/private/path", "/private/public-entity-path", "private public-entity query", "annotations", "entrypoint", "telemetry.secret"} {
		if strings.Contains(string(payload), forbidden) {
			t.Fatalf("public projection leaked %q: %s", forbidden, payload)
		}
	}
}

func TestPublicProjectionTruncatesOnUTF8Boundary(t *testing.T) {
	entities := []Entity{{
		ID: "service.unicode", Name: "Unicode", Kind: "service", Visibility: VisibilityPublic,
		Description: "ééé", PublicURL: "https://example.com/unicode",
	}}
	engine, err := New(testLayout(t), WithProviders(StaticProvider{Entities: entities}))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := engine.Reindex(context.Background()); err != nil {
		t.Fatal(err)
	}
	index, err := engine.LoadIndex(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	projection, err := ProjectPublic(context.Background(), index, ProjectionPolicy{RequireVisibility: VisibilityPublic, AllowHosts: []string{"EXAMPLE.COM"}, MaxSummaryBytes: 5})
	if err != nil {
		t.Fatal(err)
	}
	// The ellipsis is charged against MaxSummaryBytes rather than appended
	// after it, so the result never exceeds the caller's declared bound.
	got := projection.Items[0].Summary
	if len(got) > 5 {
		t.Fatalf("summary = %q (%d bytes), want at most 5", got, len(got))
	}
	if !utf8.ValidString(got) {
		t.Fatalf("summary = %q, want valid UTF-8 after truncation", got)
	}
	if !strings.HasSuffix(got, "…") {
		t.Fatalf("summary = %q, want a truncation marker", got)
	}
}

func TestPublicProjectionZeroPolicyIsFailClosed(t *testing.T) {
	index := Index{Entities: testEntities()}
	projection, err := ProjectPublic(context.Background(), index, ProjectionPolicy{})
	if !errors.Is(err, ErrHostAllowlistRequired) {
		t.Fatalf("expected URL allowlist failure, got projection=%#v err=%v", projection, err)
	}

	index.Entities[0].PublicURL = ""
	index.Entities[1].PublicURL = ""
	projection, err = ProjectPublic(context.Background(), index, ProjectionPolicy{})
	if err != nil {
		t.Fatal(err)
	}
	if len(projection.Items) != 2 {
		t.Fatalf("zero policy projected %d items, want two public items", len(projection.Items))
	}
	for _, item := range projection.Items {
		if item.ID == "telemetry.secret" {
			t.Fatalf("zero policy leaked private item: %#v", item)
		}
	}
}

func TestPublicProjectionRejectsUnsafeURLsAndContent(t *testing.T) {
	tests := []struct {
		name  string
		edit  func(*Entity)
		want  PolicyRule
		field string
	}{
		{"credentials", func(e *Entity) { e.PublicURL = "https://user:pass@example.com/item" }, RuleURLCredentials, "public_url"},
		{"query", func(e *Entity) { e.PublicURL = "https://example.com/item?token=secret" }, RuleURLQuery, "public_url"},
		{"fragment", func(e *Entity) { e.PublicURL = "https://example.com/item#private" }, RuleURLQuery, "public_url"},
		{"port", func(e *Entity) { e.PublicURL = "https://example.com:8443/item" }, RuleURLPort, "public_url"},
		{"host", func(e *Entity) { e.PublicURL = "https://elsewhere.test/item" }, RuleURLHost, "public_url"},
		{"url_path", func(e *Entity) { e.PublicURL = "https://example.com/Users/someone/secret" }, RulePathDisclosure, "public_url"},
		{"path", func(e *Entity) { e.Description = "Built from /Users/private/catalog.yaml" }, RulePathDisclosure, "summary"},
		{"path_mid_token", func(e *Entity) { e.Description = "see cache/Users/private/x" }, RulePathDisclosure, "summary"},
		{"secret", func(e *Entity) { e.Tags = []string{"api_key=abcdefghijklmnop"} }, RuleCredentialPair, "tags[0]"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entity := Entity{ID: "system.public", Name: "Public", Kind: "system", Visibility: VisibilityPublic, PublicURL: "https://example.com/item"}
			tt.edit(&entity)
			_, err := ProjectPublic(context.Background(), Index{Entities: []Entity{entity}}, ProjectionPolicy{AllowHosts: []string{"example.com"}})
			var policy *PolicyError
			if !errors.As(err, &policy) {
				t.Fatalf("expected *PolicyError, got %v", err)
			}
			if policy.Rule != tt.want {
				t.Fatalf("rule = %q, want %q", policy.Rule, tt.want)
			}
			if policy.Field != tt.field {
				t.Fatalf("field = %q, want %q", policy.Field, tt.field)
			}
			if policy.EntityID != "system.public" {
				t.Fatalf("entity id = %q, want system.public", policy.EntityID)
			}
		})
	}
}

// TestProhibitedContentErrorNeverEchoesSecret locks the rule that a rejection
// must not reproduce the value it rejected. This error reaches stderr and CI
// logs; echoing the match would defeat the boundary the projection enforces.
func TestProhibitedContentErrorNeverEchoesSecret(t *testing.T) {
	secrets := []struct {
		name  string
		value string
	}{
		{"openai", "sk-" + strings.Repeat("a", 40)},
		{"github", "ghp_" + strings.Repeat("b", 36)},
		{"assignment", "api_key=swordfish-hunter2-value"},
		{"home_path", "/Users/someone/private/notes.md"},
	}
	for _, secret := range secrets {
		t.Run(secret.name, func(t *testing.T) {
			entity := Entity{
				ID: "system.public", Name: "Public", Kind: "system",
				Visibility: VisibilityPublic, Description: "leak " + secret.value,
			}
			_, err := ProjectPublic(context.Background(), Index{Entities: []Entity{entity}}, ProjectionPolicy{})
			if err == nil {
				t.Fatal("expected the value to be rejected")
			}
			if strings.Contains(err.Error(), secret.value) {
				t.Fatalf("error echoed the rejected value: %v", err)
			}
			// A substantial fragment must not survive either.
			if fragment := secret.value[:12]; strings.Contains(err.Error(), fragment) {
				t.Fatalf("error echoed a fragment %q of the rejected value: %v", fragment, err)
			}
		})
	}
}

func TestPublicProjectionRejectsNonPublicVisibilityPolicy(t *testing.T) {
	_, err := ProjectPublic(context.Background(), Index{}, ProjectionPolicy{RequireVisibility: VisibilityPrivate})
	var policy *PolicyError
	if !errors.As(err, &policy) || policy.Rule != RuleVisibility {
		t.Fatalf("expected non-public policy rejection, got %v", err)
	}
	if !errors.Is(err, ErrPolicyViolation) {
		t.Fatalf("expected ErrPolicyViolation, got %v", err)
	}
}

func TestDuplicateIDFailsAcrossProviders(t *testing.T) {
	entity := Entity{ID: "service.duplicate", Name: "Duplicate", Kind: "service"}
	engine, err := New(testLayout(t), WithProviders(StaticProvider{ProviderName: "a", Entities: []Entity{entity}}, StaticProvider{ProviderName: "b", Entities: []Entity{entity}}))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := engine.Discover(context.Background()); err == nil || !strings.Contains(err.Error(), "duplicate entity id") {
		t.Fatalf("expected duplicate-id error, got %v", err)
	}
}

func TestFilesystemProviderReadsSupportedFormats(t *testing.T) {
	root := t.TempDir()
	corpus := filepath.Join(root, "catalog")
	if err := os.MkdirAll(corpus, 0o755); err != nil {
		t.Fatal(err)
	}
	markdown := "---\nid: service.frontmatter\nname: Frontmatter Service\nkind: service\n---\n\n# Frontmatter\n\nPortable description from the Markdown body.\n"
	if err := os.WriteFile(filepath.Join(corpus, "service.md"), []byte(markdown), 0o644); err != nil {
		t.Fatal(err)
	}
	layout, err := (Layout{CorpusDir: "catalog", ConfigDir: ".state", CacheDir: ".state/cache", SidecarDataDir: ".state/sidecars"}).Resolve(root)
	if err != nil {
		t.Fatal(err)
	}
	engine, err := New(layout, WithProviders(FilesystemProvider{}))
	if err != nil {
		t.Fatal(err)
	}
	records, err := engine.Discover(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || !strings.Contains(records[0].Entity.Description, "Portable description") {
		t.Fatalf("unexpected records: %#v", records)
	}
}

func TestFilesystemProviderStrictRejectsUnknownFieldsAndMalformedFrontmatter(t *testing.T) {
	tests := map[string]string{
		"unknown.yaml": "id: service.strict\nname: Strict\nkind: service\nunknown_field: nope\n",
		"broken.md":    "---\nid: service.broken\nname: Broken\nkind: service\n",
	}
	for name, payload := range tests {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			corpus := filepath.Join(root, "catalog")
			if err := os.MkdirAll(corpus, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(corpus, name), []byte(payload), 0o644); err != nil {
				t.Fatal(err)
			}
			layout, err := (Layout{CorpusDir: "catalog", ConfigDir: ".state", CacheDir: ".state/cache", SidecarDataDir: ".state/sidecars"}).Resolve(root)
			if err != nil {
				t.Fatal(err)
			}
			engine, err := New(layout, WithProviders(FilesystemProvider{Strict: true}))
			if err != nil {
				t.Fatal(err)
			}
			if _, err := engine.Discover(context.Background()); err == nil {
				t.Fatal("expected strict provider failure")
			}
		})
	}
}

func TestMermaidIDsRemainUniqueAfterSanitization(t *testing.T) {
	left := mermaidID("service.a-b")
	right := mermaidID("service.a_b")
	if left == right {
		t.Fatalf("colliding Mermaid IDs: %q", left)
	}
	if left != mermaidID("service.a-b") {
		t.Fatal("Mermaid ID is not deterministic")
	}
}
