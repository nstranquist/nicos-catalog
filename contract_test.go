package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

func TestLayoutValidateRejectsUnsafePlacements(t *testing.T) {
	base := t.TempDir()
	join := func(parts ...string) string { return filepath.Join(append([]string{base}, parts...)...) }
	tests := []struct {
		name   string
		layout Layout
	}{
		{"missing corpus", Layout{ConfigDir: join("c"), CacheDir: join("k"), SidecarDataDir: join("s")}},
		{"missing cache", Layout{CorpusDir: join("p"), ConfigDir: join("c"), SidecarDataDir: join("s")}},
		{"relative path", Layout{CorpusDir: "relative", ConfigDir: join("c"), CacheDir: join("k"), SidecarDataDir: join("s")}},
		{"cache equals corpus", Layout{CorpusDir: join("p"), ConfigDir: join("c"), CacheDir: join("p"), SidecarDataDir: join("s")}},
		// A cache nested inside the corpus would make derived output part of the
		// authored input on the next run.
		{"cache inside corpus", Layout{CorpusDir: join("p"), ConfigDir: join("c"), CacheDir: join("p", "k"), SidecarDataDir: join("s")}},
		{"corpus inside cache", Layout{CorpusDir: join("k", "p"), ConfigDir: join("c"), CacheDir: join("k"), SidecarDataDir: join("s")}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.layout.Validate(); !errors.Is(err, ErrInvalidLayout) {
				t.Fatalf("Validate error = %v, want ErrInvalidLayout", err)
			}
		})
	}
}

func TestLayoutResolveJoinsAndPreserves(t *testing.T) {
	root := t.TempDir()
	absolute := filepath.Join(t.TempDir(), "elsewhere")
	layout, err := (Layout{
		CorpusDir: "catalog", ConfigDir: "conf", CacheDir: absolute, SidecarDataDir: "side",
	}).Resolve(root)
	if err != nil {
		t.Fatal(err)
	}
	if layout.CorpusDir != filepath.Join(root, "catalog") {
		t.Fatalf("relative corpus resolved to %q", layout.CorpusDir)
	}
	if layout.CacheDir != absolute {
		t.Fatalf("absolute cache was rewritten to %q", layout.CacheDir)
	}
	if got := DefaultLayout(root); got.CorpusDir != filepath.Join(root, "catalog") {
		t.Fatalf("DefaultLayout corpus = %q", got.CorpusDir)
	}
}

func TestReconcileModeJSONRoundTrip(t *testing.T) {
	for _, mode := range []ReconcileMode{ReconcileDryRun, ReconcileApply} {
		payload, err := json.Marshal(mode)
		if err != nil {
			t.Fatal(err)
		}
		var decoded ReconcileMode
		if err := json.Unmarshal(payload, &decoded); err != nil {
			t.Fatal(err)
		}
		if decoded != mode {
			t.Fatalf("round trip changed %v into %v", mode, decoded)
		}
		if !mode.Valid() || mode.String() == "unknown" {
			t.Fatalf("mode %v should be valid and named", mode)
		}
	}
	if _, err := json.Marshal(ReconcileMode(42)); err == nil {
		t.Fatal("an unknown reconcile mode should not marshal")
	}
	var decoded ReconcileMode
	if err := json.Unmarshal([]byte(`"sideways"`), &decoded); err == nil {
		t.Fatal("an unknown reconcile mode should not unmarshal")
	}
	if err := json.Unmarshal([]byte(`5`), &decoded); err == nil {
		t.Fatal("a non-string reconcile mode should not unmarshal")
	}
	if ReconcileMode(42).String() != "unknown" {
		t.Fatal("an out-of-range mode should render as unknown")
	}
}

func TestReconcileRejectsUnknownMode(t *testing.T) {
	engine, err := New(testLayout(t), WithProviders(StaticProvider{Entities: testEntities()}))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := engine.Reconcile(context.Background(), ReconcileMode(42)); !errors.Is(err, ErrPolicyViolation) {
		t.Fatalf("Reconcile error = %v, want ErrPolicyViolation", err)
	}
}

func TestReconcileCleanCorpusReportsNoChange(t *testing.T) {
	engine, err := New(testLayout(t), WithProviders(StaticProvider{Entities: testEntities()}))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := engine.Reindex(context.Background()); err != nil {
		t.Fatal(err)
	}
	report, err := engine.Reconcile(context.Background(), ReconcileApply)
	if err != nil {
		t.Fatal(err)
	}
	if report.Applied || report.Drift.Changed {
		t.Fatalf("clean corpus reconcile = %+v, want no write", report)
	}
}

func TestProjectionPolicyValidate(t *testing.T) {
	tests := []struct {
		name   string
		policy ProjectionPolicy
		ok     bool
	}{
		{"zero value", ProjectionPolicy{}, true},
		{"explicit public", ProjectionPolicy{RequireVisibility: VisibilityPublic}, true},
		{"non-public visibility", ProjectionPolicy{RequireVisibility: VisibilityInternal}, false},
		{"negative summary bound", ProjectionPolicy{MaxSummaryBytes: -1}, false},
		{"unknown url mode", ProjectionPolicy{URLMode: URLMode(9)}, false},
		{"tag in both lists", ProjectionPolicy{IncludeTags: []string{"x"}, ExcludeTags: []string{"x"}}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.policy.Validate()
			if tt.ok && err != nil {
				t.Fatalf("Validate() = %v, want nil", err)
			}
			if !tt.ok && err == nil {
				t.Fatal("Validate() accepted an unsatisfiable policy")
			}
		})
	}
}

func TestProjectPublicTagFilters(t *testing.T) {
	index := Index{Entities: []Entity{
		{ID: "a.one", Name: "One", Kind: "service", Visibility: VisibilityPublic, Tags: []string{"keep", "shared"}},
		{ID: "a.two", Name: "Two", Kind: "service", Visibility: VisibilityPublic, Tags: []string{"shared", "hide"}},
		{ID: "a.three", Name: "Three", Kind: "service", Visibility: VisibilityPublic},
	}}
	ids := func(p PublicProjection) []string {
		var out []string
		for _, item := range p.Items {
			out = append(out, item.ID)
		}
		return out
	}
	t.Run("include", func(t *testing.T) {
		projection, err := ProjectPublic(context.Background(), index, ProjectionPolicy{IncludeTags: []string{"shared"}})
		if err != nil {
			t.Fatal(err)
		}
		if got := ids(projection); len(got) != 2 {
			t.Fatalf("include filter = %v, want two entities", got)
		}
	})
	t.Run("exclude beats include", func(t *testing.T) {
		projection, err := ProjectPublic(context.Background(), index, ProjectionPolicy{
			IncludeTags: []string{"shared"}, ExcludeTags: []string{"hide"},
		})
		if err != nil {
			t.Fatal(err)
		}
		got := ids(projection)
		if len(got) != 1 || got[0] != "a.one" {
			t.Fatalf("exclude filter = %v, want only a.one", got)
		}
	})
}

func TestProjectPublicURLModeDrop(t *testing.T) {
	index := Index{Entities: []Entity{
		{ID: "a.one", Name: "One", Kind: "service", Visibility: VisibilityPublic, PublicURL: "https://blocked.test/x"},
	}}
	projection, err := ProjectPublic(context.Background(), index, ProjectionPolicy{
		AllowHosts: []string{"example.com"}, URLMode: URLModeDrop,
	})
	if err != nil {
		t.Fatalf("URLModeDrop should keep the entity: %v", err)
	}
	if len(projection.Items) != 1 || projection.Items[0].URL != "" {
		t.Fatalf("projection = %+v, want the entity kept with its URL dropped", projection.Items)
	}
}

func TestProjectPublicConnectionsOnlyReferenceIncludedItems(t *testing.T) {
	index := Index{Entities: []Entity{
		{ID: "a.one", Name: "One", Kind: "service", Visibility: VisibilityPublic, Refs: []Ref{
			{Kind: "uses", Target: "a.two"},
			{Kind: "uses", Target: "b.private"},
			{Kind: "uses", Target: "c.absent"},
		}},
		{ID: "a.two", Name: "Two", Kind: "service", Visibility: VisibilityPublic},
		{ID: "b.private", Name: "Private", Kind: "service", Visibility: VisibilityPrivate},
	}}
	projection, err := ProjectPublic(context.Background(), index, ProjectionPolicy{})
	if err != nil {
		t.Fatal(err)
	}
	var connections []PublicConnection
	for _, item := range projection.Items {
		if item.ID == "a.one" {
			connections = item.Connections
		}
	}
	if len(connections) != 1 || connections[0].Target != "a.two" {
		t.Fatalf("connections = %+v, want only the included target", connections)
	}
}

func TestProjectPublicRespectsCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := ProjectPublic(ctx, Index{}, ProjectionPolicy{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("ProjectPublic error = %v, want context.Canceled", err)
	}
}

func TestProjectPublicIsDeterministic(t *testing.T) {
	index := Index{Entities: []Entity{
		{ID: "z.last", Name: "Z", Kind: "service", Visibility: VisibilityPublic},
		{ID: "a.first", Name: "A", Kind: "service", Visibility: VisibilityPublic},
	}}
	first, err := ProjectPublic(context.Background(), index, ProjectionPolicy{})
	if err != nil {
		t.Fatal(err)
	}
	second, err := ProjectPublic(context.Background(), index, ProjectionPolicy{})
	if err != nil {
		t.Fatal(err)
	}
	firstBytes, _ := json.Marshal(first)
	secondBytes, _ := json.Marshal(second)
	if string(firstBytes) != string(secondBytes) {
		t.Fatal("ProjectPublic is not deterministic")
	}
	if first.Items[0].ID != "a.first" {
		t.Fatalf("items are not sorted by id: %+v", first.Items)
	}
}

func TestScanPublicTextRejectsInvalidUTF8(t *testing.T) {
	err := ScanPublicText("summary", string([]byte{0xff, 0xfe}))
	var policy *PolicyError
	if !errors.As(err, &policy) || policy.Rule != RuleInvalidUTF8 {
		t.Fatalf("ScanPublicText error = %v, want RuleInvalidUTF8", err)
	}
	if err := ScanPublicText("summary", "perfectly ordinary text"); err != nil {
		t.Fatalf("ScanPublicText rejected safe text: %v", err)
	}
}

func TestBuildGraphAndMermaid(t *testing.T) {
	index := Index{Entities: []Entity{
		{ID: "b.two", Name: "Two", Kind: "service", Status: "shipped", Refs: []Ref{{Kind: "uses", Target: "a.one"}}},
		{ID: "a.one", Name: "One", Kind: "system", Refs: []Ref{
			{Kind: "contains", Target: "b.two"},
			{Kind: "contains", Target: "c.three"},
		}},
	}}
	graph := BuildGraph(index)
	if len(graph.Nodes) != 2 || len(graph.Edges) != 3 {
		t.Fatalf("graph = %d nodes / %d edges, want 2/3", len(graph.Nodes), len(graph.Edges))
	}
	// Edges sort by source, then kind, then target.
	if graph.Edges[0].Source != "a.one" || graph.Edges[0].Target != "b.two" {
		t.Fatalf("edges are not deterministically ordered: %+v", graph.Edges)
	}
	mermaid := graph.Mermaid()
	if !strings.HasPrefix(mermaid, "graph LR\n") {
		t.Fatalf("mermaid = %q", mermaid)
	}
	if strings.Count(mermaid, "\n") != 1+len(graph.Nodes)+len(graph.Edges) {
		t.Fatalf("mermaid line count is wrong:\n%s", mermaid)
	}
	if empty := BuildGraph(Index{}); len(empty.Nodes) != 0 || len(empty.Edges) != 0 {
		t.Fatalf("empty index produced %+v", empty)
	}
	if got := BuildGraph(Index{}).Mermaid(); got != "graph LR\n" {
		t.Fatalf("empty mermaid = %q", got)
	}
}

func TestMermaidIDsAreUniqueAndSafe(t *testing.T) {
	first := mermaidID("a.one")
	second := mermaidID("a-one")
	if first == second {
		t.Fatal("distinct ids collided after sanitization")
	}
	for _, id := range []string{first, second} {
		if !strings.HasPrefix(id, "n_") {
			t.Fatalf("mermaid id %q lacks the n_ prefix", id)
		}
	}
}

func TestVersionInfoIsSelfConsistent(t *testing.T) {
	info := VersionInfo()
	if info.Version != Version() {
		t.Fatalf("BuildInfo.Version = %q, Version() = %q", info.Version, Version())
	}
	if info.SchemaVersion != SchemaVersion {
		t.Fatalf("BuildInfo.SchemaVersion = %d, want %d", info.SchemaVersion, SchemaVersion)
	}
	if !strings.HasPrefix(info.Version, "v") {
		t.Fatalf("version %q is not a v-prefixed SemVer", info.Version)
	}
	for _, capability := range Capabilities() {
		if !info.Has(capability) {
			t.Fatalf("BuildInfo does not advertise %q", capability)
		}
	}
	if info.Has(Capability("not-a-capability")) {
		t.Fatal("BuildInfo claims an unknown capability")
	}
	// Capabilities returns a copy; mutating it must not affect later calls.
	list := Capabilities()
	list[0] = "mutated"
	if Capabilities()[0] == "mutated" {
		t.Fatal("Capabilities returned a shared slice")
	}
}

func TestVisibilityVocabulary(t *testing.T) {
	for _, visibility := range []Visibility{"", VisibilityPublic, VisibilityInternal, VisibilityPrivate} {
		if !visibility.Valid() {
			t.Fatalf("%q should be valid", visibility)
		}
	}
	if Visibility("semi-public").Valid() {
		t.Fatal("an unknown visibility should not validate")
	}
	if VisibilityPublic.String() != "public" {
		t.Fatal("Visibility.String is wrong")
	}
}

func TestEntityIDGrammarIsExported(t *testing.T) {
	if EntityIDPattern() == "" {
		t.Fatal("EntityIDPattern is empty")
	}
	for _, id := range []string{"a", "a.b", "a-b", "a_b", "a1.b2-c3"} {
		if err := ValidateEntityID(id); err != nil {
			t.Fatalf("ValidateEntityID(%q) = %v, want nil", id, err)
		}
	}
	for _, id := range []string{"", "A", ".a", "-a", "_a", "a b", "a/b"} {
		if err := ValidateEntityID(id); !errors.Is(err, ErrInvalidEntity) {
			t.Fatalf("ValidateEntityID(%q) = %v, want ErrInvalidEntity", id, err)
		}
	}
}

func TestLimitsValidate(t *testing.T) {
	if err := DefaultLimits().Validate(); err != nil {
		t.Fatalf("DefaultLimits is invalid: %v", err)
	}
	for _, limits := range []Limits{
		{MaxEntities: -1}, {MaxRecordsPerProvider: -1},
		{MaxSummaryBytes: -1}, {MaxSearchResults: -1}, {MaxSourceBytes: -1},
	} {
		if err := limits.Validate(); err == nil {
			t.Fatalf("Limits%+v should be rejected", limits)
		}
	}
}

func TestTypedErrorsRenderAndUnwrap(t *testing.T) {
	base := errors.New("cause")
	cases := []struct {
		name    string
		err     error
		wants   []string
		unwraps error
	}{
		{"provider", &ProviderError{Provider: "p", Err: base}, []string{"provider p", "cause"}, base},
		{"provider with source", &ProviderError{Provider: "p", Source: "s.yaml", Err: base}, []string{"p", "s.yaml"}, base},
		{"entity", &EntityError{EntityID: "a.one", Err: base}, []string{"entity a.one"}, base},
		{"entity field", &EntityError{EntityID: "a.one", Field: "name", Err: base}, []string{"field name"}, base},
		{"entity unnamed", &EntityError{Err: base}, []string{"<unnamed>"}, base},
		{"decode", &DecodeError{Path: "x.yaml", Err: base}, []string{"decode x.yaml"}, base},
		{"index", &IndexError{Path: "i.json", Err: base}, []string{"index i.json"}, base},
		{"duplicate", &DuplicateIDError{
			EntityID: "a.one",
			First:    RecordOrigin{Provider: "p1", Source: "s1"},
			Second:   RecordOrigin{Provider: "p2", Source: "s2"},
		}, []string{"a.one", "p1", "p2"}, ErrDuplicateEntityID},
		{"policy", &PolicyError{Field: "summary", Rule: RulePathDisclosure, Err: ErrProhibitedContent},
			[]string{"summary", "path-disclosure"}, ErrProhibitedContent},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			text := tt.err.Error()
			for _, want := range tt.wants {
				if !strings.Contains(text, want) {
					t.Fatalf("error text %q does not contain %q", text, want)
				}
			}
			if !errors.Is(tt.err, tt.unwraps) {
				t.Fatalf("%v does not unwrap to %v", tt.err, tt.unwraps)
			}
		})
	}
}

func TestPolicyErrorNamesEntityWhenKnown(t *testing.T) {
	withID := &PolicyError{EntityID: "a.one", Field: "summary", Rule: RuleTokenShape}
	if !strings.Contains(withID.Error(), "entity a.one") {
		t.Fatalf("error text = %q", withID.Error())
	}
	withoutID := &PolicyError{Field: "summary", Rule: RuleTokenShape}
	if strings.Contains(withoutID.Error(), "entity ") {
		t.Fatalf("error text = %q", withoutID.Error())
	}
}

func TestNormalizeStringsDedupesAndSorts(t *testing.T) {
	got := normalizeStrings([]string{" b ", "a", "b", "", "   ", "a"})
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("normalizeStrings = %v, want [a b]", got)
	}
	if normalizeStrings(nil) != nil {
		t.Fatal("normalizeStrings(nil) should stay nil")
	}
}

func TestTruncateUTF8EdgeCases(t *testing.T) {
	if got := truncateUTF8("abc", 0); got != "abc" {
		t.Fatalf("non-positive bound should pass through, got %q", got)
	}
	if got := truncateUTF8("abc", 10); got != "abc" {
		t.Fatalf("a bound above the length should pass through, got %q", got)
	}
	if got := truncateUTF8("ééé", 3); got != "é" {
		t.Fatalf("truncateUTF8 split a rune: %q", got)
	}
}
