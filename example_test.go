package catalog_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	catalog "github.com/nstranquist/nicos-catalog"
)

// exampleEntities is a small synthetic corpus shared by the examples.
func exampleEntities() []catalog.Entity {
	return []catalog.Entity{
		{
			ID: "system.orchard", Name: "Orchard", Kind: "system",
			Description: "Ownership graph for the platform.",
			Tags:        []string{"platform"}, Visibility: catalog.VisibilityPublic,
			Refs: []catalog.Ref{{Kind: "contains", Target: "service.press"}},
		},
		{
			ID: "service.press", Name: "Press API", Kind: "service",
			Description: "Inventory and dependency search.",
			Tags:        []string{"go"}, Visibility: catalog.VisibilityPublic,
		},
		{
			ID: "telemetry.sample", Name: "Query Sample", Kind: "telemetry",
			Description: "Host-only.", Visibility: catalog.VisibilityPrivate,
			Owner: "platform-team", Entrypoint: "cmd/sample/main.go",
		},
	}
}

// exampleEngine builds an engine over a temporary host root.
func exampleEngine() (*catalog.Engine, func()) {
	root, err := os.MkdirTemp("", "nicos-catalog-example-")
	if err != nil {
		panic(err)
	}
	layout, err := catalog.DefaultLayout(root).Resolve(root)
	if err != nil {
		panic(err)
	}
	engine, err := catalog.New(layout, catalog.WithProviders(
		catalog.StaticProvider{ProviderName: "example", Entities: exampleEntities()},
	))
	if err != nil {
		panic(err)
	}
	return engine, func() { os.RemoveAll(root) }
}

// The engine compiles authored facts into a deterministic index, then answers
// queries and publishes a closed projection from it.
func Example() {
	engine, cleanup := exampleEngine()
	defer cleanup()
	ctx := context.Background()

	report, err := engine.Reindex(ctx)
	if err != nil {
		panic(err)
	}
	fmt.Println("indexed:", report.EntityCount)

	results, err := engine.Search(ctx, "ownership graph", catalog.SearchOptions{Limit: 1})
	if err != nil {
		panic(err)
	}
	fmt.Println("top match:", results[0].Entity.ID)

	index, err := engine.LoadIndex(ctx)
	if err != nil {
		panic(err)
	}
	projection, err := catalog.ProjectPublic(ctx, index, catalog.ProjectionPolicy{})
	if err != nil {
		panic(err)
	}
	fmt.Println("published:", len(projection.Items))
	// Output:
	// indexed: 3
	// top match: system.orchard
	// published: 2
}

func ExampleNew() {
	root, err := os.MkdirTemp("", "nicos-catalog-new-")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(root)

	layout, err := catalog.DefaultLayout(root).Resolve(root)
	if err != nil {
		panic(err)
	}
	engine, err := catalog.New(layout,
		catalog.WithProviders(catalog.StaticProvider{Entities: exampleEntities()}),
		catalog.WithLimits(catalog.Limits{MaxEntities: 100}),
	)
	if err != nil {
		panic(err)
	}
	records, err := engine.Discover(context.Background())
	if err != nil {
		panic(err)
	}
	fmt.Println(len(records), "records")
	// Output: 3 records
}

func ExampleDefaultLayout() {
	layout := catalog.DefaultLayout("/srv/host")
	fmt.Println(filepath.ToSlash(layout.CorpusDir))
	fmt.Println(filepath.ToSlash(layout.CacheDir))
	// Output:
	// /srv/host/catalog
	// /srv/host/.nicos-catalog/cache
}

func ExampleEngine_Validate() {
	engine, cleanup := exampleEngine()
	defer cleanup()

	report, err := engine.Validate(context.Background())
	if err != nil {
		panic(err)
	}
	fmt.Println("ok:", report.OK, "entities:", report.EntityCount)
	// Output: ok: true entities: 3
}

// WithStrictReferences promotes a dangling reference from a warning to an
// error, which is what a publication gate usually wants.
func ExampleWithStrictReferences() {
	root, err := os.MkdirTemp("", "nicos-catalog-strict-")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(root)

	layout, err := catalog.DefaultLayout(root).Resolve(root)
	if err != nil {
		panic(err)
	}
	engine, err := catalog.New(layout, catalog.WithProviders(catalog.StaticProvider{
		Entities: []catalog.Entity{{
			ID: "system.alpha", Name: "Alpha", Kind: "system",
			Refs: []catalog.Ref{{Kind: "contains", Target: "service.absent"}},
		}},
	}))
	if err != nil {
		panic(err)
	}
	lenient, _ := engine.Validate(context.Background())
	strict, _ := engine.Validate(context.Background(), catalog.WithStrictReferences())
	fmt.Println("lenient ok:", lenient.OK, "strict ok:", strict.OK)
	// Output: lenient ok: true strict ok: false
}

func ExampleEngine_Drift() {
	engine, cleanup := exampleEngine()
	defer cleanup()
	ctx := context.Background()

	// Before any reindex there is no derived state to compare against.
	report, err := engine.Drift(ctx)
	if err != nil {
		panic(err)
	}
	fmt.Println("reason:", report.Reason)

	if _, err := engine.Reindex(ctx); err != nil {
		panic(err)
	}
	report, err = engine.Drift(ctx)
	if err != nil {
		panic(err)
	}
	fmt.Println("clean:", report.OK)
	// Output:
	// reason: index_missing
	// clean: true
}

// Reconcile defaults to a dry run: the zero ReconcileMode never writes.
func ExampleEngine_Reconcile() {
	engine, cleanup := exampleEngine()
	defer cleanup()
	ctx := context.Background()

	report, err := engine.Reconcile(ctx, catalog.ReconcileDryRun)
	if err != nil {
		panic(err)
	}
	fmt.Println("drift:", report.Drift.Changed, "applied:", report.Applied)

	report, err = engine.Reconcile(ctx, catalog.ReconcileApply)
	if err != nil {
		panic(err)
	}
	fmt.Println("applied:", report.Applied)
	// Output:
	// drift: true applied: false
	// applied: true
}

// ProjectPublic emits only entities that declare public visibility, and only
// the fields the closed DTO can represent.
func ExampleProjectPublic() {
	engine, cleanup := exampleEngine()
	defer cleanup()
	ctx := context.Background()
	if _, err := engine.Reindex(ctx); err != nil {
		panic(err)
	}
	index, err := engine.LoadIndex(ctx)
	if err != nil {
		panic(err)
	}
	projection, err := catalog.ProjectPublic(ctx, index, catalog.ProjectionPolicy{})
	if err != nil {
		panic(err)
	}
	for _, item := range projection.Items {
		fmt.Println(item.ID, "|", item.Kind, "|", len(item.Connections), "connections")
	}
	// Output:
	// service.press | service | 0 connections
	// system.orchard | system | 1 connections
}

// AllowHosts must be non-empty whenever any projected entity declares a
// PublicURL. An empty allowlist is a hard error rather than an implicit permit,
// so forgetting to configure one cannot publish an unreviewed link.
func ExampleProjectPublic_hostAllowlist() {
	index := catalog.Index{Entities: []catalog.Entity{{
		ID: "service.press", Name: "Press API", Kind: "service",
		Visibility: catalog.VisibilityPublic,
		PublicURL:  "https://example.com/press",
	}}}
	ctx := context.Background()

	_, err := catalog.ProjectPublic(ctx, index, catalog.ProjectionPolicy{})
	fmt.Println("without an allowlist:", errors.Is(err, catalog.ErrHostAllowlistRequired))

	projection, err := catalog.ProjectPublic(ctx, index, catalog.ProjectionPolicy{
		AllowHosts: []string{"example.com"},
	})
	if err != nil {
		panic(err)
	}
	fmt.Println("with an allowlist:", projection.Items[0].URL)
	// Output:
	// without an allowlist: true
	// with an allowlist: https://example.com/press
}

// ScanPublicText lets a host reuse the library's publication rules instead of
// reimplementing them. The error names the rule and never echoes the value.
func ExampleScanPublicText() {
	const rejected = "Built from /Users/someone/private/notes.md"

	err := catalog.ScanPublicText("summary", rejected)
	var policy *catalog.PolicyError
	if errors.As(err, &policy) {
		fmt.Println("rule:", policy.Rule)
		fmt.Println("field:", policy.Field)
		// The rejected text never appears in the rendered error, because that
		// error travels to logs and CI output.
		fmt.Println("error echoes the value:", strings.Contains(policy.Error(), "/Users/someone"))
	}
	fmt.Println("safe text accepted:", catalog.ScanPublicText("summary", "An ordinary summary.") == nil)
	// Output:
	// rule: path-disclosure
	// field: summary
	// error echoes the value: false
	// safe text accepted: true
}

func ExampleBuildGraph() {
	index := catalog.Index{Entities: []catalog.Entity{
		{ID: "system.orchard", Name: "Orchard", Kind: "system",
			Refs: []catalog.Ref{{Kind: "contains", Target: "service.press"}}},
		{ID: "service.press", Name: "Press API", Kind: "service"},
	}}
	graph := catalog.BuildGraph(index)
	fmt.Println(len(graph.Nodes), "nodes,", len(graph.Edges), "edges")
	// Output: 2 nodes, 1 edges
}

func ExampleVersionInfo() {
	info := catalog.VersionInfo()
	fmt.Println("schema:", info.SchemaVersion)
	fmt.Println("bm25:", info.Has(catalog.CapabilityBM25Search))
	// Output:
	// schema: 2
	// bm25: true
}

func ExampleFilesystemProvider() {
	root, err := os.MkdirTemp("", "nicos-catalog-fs-")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(root)

	layout, err := catalog.DefaultLayout(root).Resolve(root)
	if err != nil {
		panic(err)
	}
	if err := os.MkdirAll(layout.CorpusDir, 0o755); err != nil {
		panic(err)
	}
	if err := os.WriteFile(filepath.Join(layout.CorpusDir, "service.press.yaml"),
		[]byte("id: service.press\nname: Press API\nkind: service\n"), 0o644); err != nil {
		panic(err)
	}
	records, err := catalog.FilesystemProvider{Strict: true}.
		Provide(context.Background(), layout)
	if err != nil {
		panic(err)
	}
	fmt.Println(records[0].Entity.ID, "from", records[0].Source)
	// Output: service.press from service.press.yaml
}
