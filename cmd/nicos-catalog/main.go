package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	catalog "github.com/nstranquist/nicos-catalog"
)

func main() {
	os.Exit(run(context.Background(), os.Args[1:], os.Stdout, os.Stderr))
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	global := flag.NewFlagSet("nicos-catalog", flag.ContinueOnError)
	global.SetOutput(stderr)
	root := global.String("root", ".", "host root used to resolve relative layout paths")
	corpus := global.String("corpus", "catalog", "authored catalog corpus directory")
	config := global.String("config", ".nicos-catalog", "host configuration directory")
	cache := global.String("cache", ".nicos-catalog/cache", "derived index directory")
	sidecars := global.String("sidecars", ".nicos-catalog/sidecars", "host-owned sidecar data directory")
	jsonOutput := global.Bool("json", false, "emit machine-readable JSON")
	global.Usage = func() { printUsage(stderr) }
	if err := global.Parse(args); err != nil {
		return 2
	}
	remaining := global.Args()
	if len(remaining) == 0 || remaining[0] == "help" || remaining[0] == "--help" || remaining[0] == "-h" {
		printUsage(stdout)
		return 0
	}
	command := remaining[0]
	commandArgs := remaining[1:]
	if command == "version" {
		return runVersion(commandArgs, *jsonOutput, stdout, stderr)
	}
	if command == "demo" {
		return runDemo(ctx, commandArgs, *jsonOutput, stdout, stderr)
	}
	layout, err := (catalog.Layout{
		CorpusDir: *corpus, ConfigDir: *config, CacheDir: *cache, SidecarDataDir: *sidecars,
	}).Resolve(*root)
	if err != nil {
		return fail(stderr, err)
	}
	engine, err := catalog.New(layout, catalog.WithProviders(catalog.FilesystemProvider{ProviderName: "host-filesystem", Strict: true}))
	if err != nil {
		return fail(stderr, err)
	}

	switch command {
	case "validate":
		report, err := engine.Validate(ctx)
		return reportResult(stdout, stderr, *jsonOutput, report, err, func() {
			_, _ = fmt.Fprintf(stdout, "valid: %d entities from %d provider(s); %d warning(s)\n", report.EntityCount, report.ProviderCount, len(report.Warnings))
		})
	case "reindex":
		report, err := engine.Reindex(ctx)
		return reportResult(stdout, stderr, *jsonOutput, report, err, func() {
			_, _ = fmt.Fprintf(stdout, "indexed %d entities at %s\n", report.EntityCount, report.IndexPath)
		})
	case "search":
		return runSearch(ctx, engine, commandArgs, *jsonOutput, stdout, stderr)
	case "graph":
		return runGraph(ctx, engine, commandArgs, *jsonOutput, stdout, stderr)
	case "drift":
		report, err := engine.Drift(ctx)
		code := reportResult(stdout, stderr, *jsonOutput, report, err, func() {
			if report.Changed {
				_, _ = fmt.Fprintf(stdout, "drift: %s\n", report.Reason)
			} else {
				_, _ = fmt.Fprintln(stdout, "drift: clean")
			}
		})
		if code == 0 && report.Changed {
			return 3
		}
		return code
	case "reconcile":
		return runReconcile(ctx, engine, commandArgs, *jsonOutput, stdout, stderr)
	case "project":
		return runProject(ctx, engine, commandArgs, *jsonOutput, stdout, stderr)
	default:
		_, _ = fmt.Fprintf(stderr, "unknown command %q\n\n", command)
		printUsage(stderr)
		return 2
	}
}

func runVersion(args []string, jsonOutput bool, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("version", flag.ContinueOnError)
	flags.SetOutput(stderr)
	expect := flags.String("expect", "", "fail unless this exact SemVer is running")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	info := catalog.VersionInfo()
	if *expect != "" && *expect != info.Version {
		return fail(stderr, fmt.Errorf("version mismatch: running %s, expected %s", info.Version, *expect))
	}
	if jsonOutput {
		return writeJSON(stdout, stderr, info)
	}
	_, _ = fmt.Fprintln(stdout, info.Version)
	return 0
}

func runSearch(ctx context.Context, engine *catalog.Engine, args []string, jsonOutput bool, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("search", flag.ContinueOnError)
	flags.SetOutput(stderr)
	limit := flags.Int("limit", 10, "maximum number of results")
	kinds := flags.String("kinds", "", "comma-separated kind filter")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	query := strings.Join(flags.Args(), " ")
	results, err := engine.Search(ctx, query, catalog.SearchOptions{Limit: *limit, Kinds: splitCSV(*kinds)})
	if err != nil {
		return fail(stderr, err)
	}
	if jsonOutput {
		return writeJSON(stdout, stderr, results)
	}
	for _, result := range results {
		_, _ = fmt.Fprintf(stdout, "%.3f\t%s\t%s\n", result.Score, result.Entity.ID, result.Entity.Name)
	}
	return 0
}

func runGraph(ctx context.Context, engine *catalog.Engine, args []string, jsonOutput bool, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("graph", flag.ContinueOnError)
	flags.SetOutput(stderr)
	format := flags.String("format", "mermaid", "output format: mermaid or json")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	index, err := engine.LoadIndex(ctx)
	if err != nil {
		return fail(stderr, err)
	}
	graph := catalog.BuildGraph(index)
	if jsonOutput || *format == "json" {
		return writeJSON(stdout, stderr, graph)
	}
	if *format != "mermaid" {
		return fail(stderr, fmt.Errorf("unsupported graph format %q", *format))
	}
	_, _ = fmt.Fprint(stdout, graph.Mermaid())
	return 0
}

func runReconcile(ctx context.Context, engine *catalog.Engine, args []string, jsonOutput bool, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("reconcile", flag.ContinueOnError)
	flags.SetOutput(stderr)
	apply := flags.Bool("apply", false, "rebuild the derived index when drift exists")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	mode := catalog.ReconcileDryRun
	if *apply {
		mode = catalog.ReconcileApply
	}
	report, err := engine.Reconcile(ctx, mode)
	return reportResult(stdout, stderr, jsonOutput, report, err, func() {
		switch {
		case report.Applied:
			_, _ = fmt.Fprintln(stdout, "reconciled: index rebuilt")
		case report.Drift.Changed:
			_, _ = fmt.Fprintln(stdout, "reconcile needed; rerun with --apply")
		default:
			_, _ = fmt.Fprintln(stdout, "reconcile: clean")
		}
	})
}

func runProject(ctx context.Context, engine *catalog.Engine, args []string, jsonOutput bool, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("project", flag.ContinueOnError)
	flags.SetOutput(stderr)
	visibility := flags.String("visibility", "public", "required entity visibility")
	kinds := flags.String("kinds", "", "comma-separated kind allowlist")
	tags := flags.String("tags", "", "comma-separated tag allowlist")
	hosts := flags.String("allow-hosts", "", "comma-separated public URL hostname allowlist")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	index, err := engine.LoadIndex(ctx)
	if err != nil {
		return fail(stderr, err)
	}
	projection, err := catalog.ProjectPublic(ctx, index, catalog.ProjectionPolicy{
		RequireVisibility: catalog.Visibility(*visibility), IncludeKinds: splitCSV(*kinds), IncludeTags: splitCSV(*tags), AllowHosts: splitCSV(*hosts),
	})
	if err != nil {
		return fail(stderr, err)
	}
	if jsonOutput {
		return writeJSON(stdout, stderr, projection)
	}
	for _, item := range projection.Items {
		_, _ = fmt.Fprintf(stdout, "%s\t%s\t%s\n", item.ID, item.Kind, item.Name)
	}
	return 0
}

type demoReport struct {
	Reindex    catalog.ReindexReport    `json:"reindex"`
	Search     []catalog.SearchResult   `json:"search"`
	Projection catalog.PublicProjection `json:"projection"`
}

func runDemo(ctx context.Context, args []string, jsonOutput bool, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("demo", flag.ContinueOnError)
	flags.SetOutput(stderr)
	query := flags.String("query", "ownership graph", "synthetic search query")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	root, err := os.MkdirTemp("", "nicos-catalog-demo-")
	if err != nil {
		return fail(stderr, err)
	}
	defer func() { _ = os.RemoveAll(root) }()
	layout, err := catalog.DefaultLayout(root).Resolve(root)
	if err != nil {
		return fail(stderr, err)
	}
	engine, err := catalog.New(layout, catalog.WithProviders(catalog.StaticProvider{ProviderName: "synthetic-demo", Entities: syntheticEntities()}))
	if err != nil {
		return fail(stderr, err)
	}
	reindexed, err := engine.Reindex(ctx)
	if err != nil {
		return fail(stderr, err)
	}
	results, err := engine.Search(ctx, *query, catalog.SearchOptions{Limit: 3})
	if err != nil {
		return fail(stderr, err)
	}
	index, err := engine.LoadIndex(ctx)
	if err != nil {
		return fail(stderr, err)
	}
	projection, err := catalog.ProjectPublic(ctx, index, catalog.ProjectionPolicy{RequireVisibility: catalog.VisibilityPublic, AllowHosts: []string{"example.com"}})
	if err != nil {
		return fail(stderr, err)
	}
	report := demoReport{Reindex: reindexed, Search: results, Projection: projection}
	if jsonOutput {
		return writeJSON(stdout, stderr, report)
	}
	_, _ = fmt.Fprintf(stdout, "Nicos Catalog demo: %d synthetic entities, %d public items\n", reindexed.EntityCount, len(projection.Items))
	for _, result := range results {
		_, _ = fmt.Fprintf(stdout, "  %.3f  %s — %s\n", result.Score, result.Entity.ID, result.Entity.Name)
	}
	return 0
}

func syntheticEntities() []catalog.Entity {
	return []catalog.Entity{
		{ID: "system.orchard", Name: "Orchard Platform", Kind: "system", Status: "shipped", Description: "A synthetic developer platform with typed ownership and dependency graph search.", Tags: []string{"demo", "platform"}, Visibility: "public", PublicURL: "https://example.com/orchard", Refs: []catalog.Ref{{Kind: "contains", Target: "service.seed-api"}, {Kind: "contains", Target: "web.grove-console"}}},
		{ID: "service.seed-api", Name: "Seed API", Kind: "service", Status: "shipped", Description: "A synthetic Go service exposing catalog-safe seed inventory.", Tags: []string{"demo", "go"}, Visibility: "public", PublicURL: "https://example.com/seed-api", Refs: []catalog.Ref{{Kind: "serves", Target: "web.grove-console"}}},
		{ID: "web.grove-console", Name: "Grove Console", Kind: "web-app", Status: "beta", Description: "A synthetic operations console for graph and ownership exploration.", Tags: []string{"demo", "react"}, Visibility: "public", PublicURL: "https://example.com/grove"},
		{ID: "telemetry.private-sample", Name: "Private Sample", Kind: "telemetry", Status: "active", Description: "Synthetic host-only data proving the public projection is closed.", Tags: []string{"demo"}, Visibility: "private", Annotations: map[string]string{"query": "never projected"}},
	}
}

func splitCSV(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		if value := strings.TrimSpace(part); value != "" {
			values = append(values, value)
		}
	}
	return values
}

func reportResult[T any](stdout, stderr io.Writer, jsonOutput bool, report T, err error, human func()) int {
	if err != nil {
		return fail(stderr, err)
	}
	if jsonOutput {
		return writeJSON(stdout, stderr, report)
	}
	human()
	return 0
}

func writeJSON(stdout, stderr io.Writer, value any) int {
	encoder := json.NewEncoder(stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		return fail(stderr, err)
	}
	return 0
}

func fail(stderr io.Writer, err error) int {
	if err == nil {
		err = errors.New("unknown error")
	}
	_, _ = fmt.Fprintf(stderr, "nicos-catalog: %v\n", err)
	return 1
}

func printUsage(w io.Writer) {
	name := filepath.Base(os.Args[0])
	_, _ = fmt.Fprintf(w, `Nicos Catalog — typed software-catalog engine

Usage:
  %s [layout flags] <command> [command flags]

Commands:
  validate    validate provider output and reference integrity
  reindex     build the deterministic local full-text index
  search      BM25 full-text search over the current index
  graph       emit the typed relationship graph
  drift       compare authored sources with the derived index
  reconcile   report drift or rebuild with --apply
  project     emit the closed, privacy-safe public DTO
  demo        run an entirely synthetic end-to-end host
  version     print build identity; supports --expect %s

Layout flags:
  --root PATH       host root (default .)
  --corpus PATH     authored catalog directory (default catalog)
  --config PATH     host configuration directory
  --cache PATH      derived cache directory
  --sidecars PATH   host-owned sidecar directory
  --json            emit JSON
`, name, catalog.Version())
}
