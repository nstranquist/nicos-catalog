package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	catalog "github.com/nstranquist/nicos-catalog"
	"github.com/nstranquist/nicos-catalog/internal/explorerapi"
	"github.com/nstranquist/nicos-catalog/internal/explorerbundle"
	"github.com/nstranquist/nicos-catalog/internal/explorercontract"
	"github.com/nstranquist/nicos-catalog/internal/explorerinit"
	"github.com/nstranquist/nicos-catalog/internal/explorermcp"
	"github.com/nstranquist/nicos-catalog/internal/exploreropen"
	"github.com/nstranquist/nicos-catalog/internal/explorerweb"
)

var commandStdin io.Reader = os.Stdin

var errIndexNeedsReindex = errors.New("explorer index is missing or stale; run nicos-catalog reindex")

func runInit(args []string, globalRoot string, jsonOutput bool, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("init", flag.ContinueOnError)
	flags.SetOutput(stderr)
	root := flags.String("root", globalRoot, "host root for starter files")
	template := flags.String("template", "minimal", "starter template: minimal or sample")
	dryRun := flags.Bool("dry-run", false, "report the complete write plan without writing")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 0 {
		return usageFailure(stderr, "init accepts flags only")
	}
	receipt, err := explorerinit.Run(explorerinit.Options{Root: *root, Template: *template, DryRun: *dryRun})
	if jsonOutput {
		return writeCommandReceipt(stdout, stderr, "init", explorercontract.ProjectionLocal, "", receipt, err)
	}
	if err != nil {
		return fail(stderr, err)
	}
	for _, path := range receipt.Written {
		if _, err := fmt.Fprintf(stdout, "written\t%s\n", path); err != nil {
			return fail(stderr, err)
		}
	}
	for _, path := range receipt.Present {
		if _, err := fmt.Fprintf(stdout, "present\t%s\n", path); err != nil {
			return fail(stderr, err)
		}
	}
	if receipt.DryRun {
		if _, err := fmt.Fprintf(stdout, "dry-run: %d file(s) planned\n", len(receipt.Planned)); err != nil {
			return fail(stderr, err)
		}
	}
	return 0
}

func runServe(ctx context.Context, engine *catalog.Engine, args []string, jsonOutput bool, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("serve", flag.ContinueOnError)
	flags.SetOutput(stderr)
	listen := flags.String("listen", "127.0.0.1:0", "loopback listen address")
	projection := flags.String("projection", "local", "Explorer projection: local or public")
	open := flags.Bool("open", false, "open Explorer after the listener is ready")
	kinds := flags.String("kinds", "", "comma-separated public kind allowlist")
	tags := flags.String("tags", "", "comma-separated public tag allowlist")
	hosts := flags.String("allow-hosts", "", "comma-separated public URL hostname allowlist")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 0 {
		return usageFailure(stderr, "serve accepts flags only")
	}
	mode, err := projectionMode(*projection)
	if err != nil {
		return fail(stderr, err)
	}
	service, err := explorerService(ctx, engine, mode, projectionPolicy(*kinds, *tags, *hosts))
	if err != nil {
		return fail(stderr, err)
	}
	err = explorerapi.RunServer(ctx, explorerapi.ServerConfig{
		Listen: *listen, ProductVersion: catalog.Version(), Service: service, Web: explorerweb.Handler(),
		OnReady: func(ready explorerapi.Ready) error {
			if jsonOutput {
				if code := writeCommandReceipt(stdout, stderr, "serve", mode, service.Dataset().SourceDigest, ready.Receipt, nil); code != 0 {
					return fmt.Errorf("could not write serve receipt")
				}
			} else {
				if _, err := fmt.Fprintf(stdout, "Explorer: %s\nsource: %s\n", ready.URL, service.Dataset().SourceDigest); err != nil {
					return fmt.Errorf("write Explorer address: %w", err)
				}
			}
			if *open {
				return exploreropen.Open(ctx, ready.URL)
			}
			return nil
		},
	})
	if err != nil {
		return fail(stderr, err)
	}
	return 0
}

func runExportExplorer(ctx context.Context, engine *catalog.Engine, layout catalog.Layout, args []string, jsonOutput bool, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] != "explorer" {
		return usageFailure(stderr, "export requires the explorer subcommand")
	}
	args = args[1:]
	flags := flag.NewFlagSet("export explorer", flag.ContinueOnError)
	flags.SetOutput(stderr)
	out := flags.String("out", "", "required static output directory")
	visibility := flags.String("visibility", "", "required visibility; public is the only supported value")
	kinds := flags.String("kinds", "", "comma-separated kind allowlist")
	tags := flags.String("tags", "", "comma-separated tag allowlist")
	hosts := flags.String("allow-hosts", "", "comma-separated public URL hostname allowlist")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 0 || *out == "" || !hasFlag(args, "visibility") {
		return usageFailure(stderr, "export explorer requires --out DIR --visibility public")
	}
	if *visibility != "public" {
		return fail(stderr, fmt.Errorf("static Explorer export supports public visibility only"))
	}
	service, err := explorerService(ctx, engine, explorercontract.ProjectionPublic, projectionPolicy(*kinds, *tags, *hosts))
	if err != nil {
		return fail(stderr, err)
	}
	receipt, err := explorerbundle.Export(ctx, service, explorerbundle.Options{
		OutDir: *out, ProductVersion: catalog.Version(),
		ForbiddenRoots: []string{layout.CorpusDir, layout.ConfigDir, layout.CacheDir, layout.SidecarDataDir},
	})
	if jsonOutput {
		return writeCommandReceipt(stdout, stderr, "export explorer", explorercontract.ProjectionPublic, service.Dataset().SourceDigest, receipt, err)
	}
	if err != nil {
		return fail(stderr, err)
	}
	if _, err := fmt.Fprintf(stdout, "exported %d entities and %d edges across %d file(s)\n", receipt.EntityCount, receipt.EdgeCount, len(receipt.Files)); err != nil {
		return fail(stderr, err)
	}
	return 0
}

func runMCP(ctx context.Context, engine *catalog.Engine, args []string, jsonOutput bool, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("mcp", flag.ContinueOnError)
	flags.SetOutput(stderr)
	stdio := flags.Bool("stdio", false, "serve MCP over standard input and output")
	projection := flags.String("projection", "local", "Explorer projection: local or public")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 0 || !*stdio {
		return usageFailure(stderr, "mcp requires --stdio")
	}
	if jsonOutput {
		return usageFailure(stderr, "mcp --stdio owns stdout; do not combine it with --json")
	}
	mode, err := projectionMode(*projection)
	if err != nil {
		return fail(stderr, err)
	}
	service, err := explorerService(ctx, engine, mode, projectionPolicy("", "", ""))
	if err != nil {
		return fail(stderr, err)
	}
	server, err := explorermcp.New(service, catalog.Version())
	if err != nil {
		return fail(stderr, err)
	}
	if err := server.Serve(ctx, commandStdin, stdout); err != nil && !errors.Is(err, context.Canceled) {
		return fail(stderr, err)
	}
	return 0
}

func runDemoUI(ctx context.Context, open, jsonOutput bool, stdout, stderr io.Writer) int {
	root, err := os.MkdirTemp("", "nicos-catalog-demo-")
	if err != nil {
		return fail(stderr, err)
	}
	defer func() { _ = os.RemoveAll(root) }()
	//nolint:gosec // The private demo directory needs owner execute permission.
	if err := os.Chmod(root, 0o700); err != nil {
		return fail(stderr, err)
	}
	layout, err := catalog.DefaultLayout(root).Resolve(root)
	if err != nil {
		return fail(stderr, err)
	}
	engine, err := catalog.New(layout, catalog.WithProviders(catalog.StaticProvider{ProviderName: "synthetic-demo", Entities: syntheticEntities()}))
	if err != nil {
		return fail(stderr, err)
	}
	if _, err := engine.Reindex(ctx); err != nil {
		return fail(stderr, err)
	}
	index, err := engine.LoadIndex(ctx)
	if err != nil {
		return fail(stderr, err)
	}
	dataset, err := explorerapi.Compile(ctx, index, explorercontract.ProjectionLocal, catalog.ProjectionPolicy{})
	if err != nil {
		return fail(stderr, err)
	}
	service, err := explorerapi.NewService(dataset)
	if err != nil {
		return fail(stderr, err)
	}
	err = explorerapi.RunServer(ctx, explorerapi.ServerConfig{
		Listen: "127.0.0.1:0", ProductVersion: catalog.Version(), Service: service, Web: explorerweb.Handler(),
		OnReady: func(ready explorerapi.Ready) error {
			if jsonOutput {
				if code := writeCommandReceipt(stdout, stderr, "demo --ui", explorercontract.ProjectionLocal, dataset.SourceDigest, ready.Receipt, nil); code != 0 {
					return fmt.Errorf("could not write demo receipt")
				}
			} else {
				if _, err := fmt.Fprintf(stdout, "Nicos Catalog synthetic Explorer: %s\n", ready.URL); err != nil {
					return fmt.Errorf("write Explorer address: %w", err)
				}
			}
			if open {
				return exploreropen.Open(ctx, ready.URL)
			}
			return nil
		},
	})
	if err != nil {
		return fail(stderr, err)
	}
	return 0
}

func explorerService(ctx context.Context, engine *catalog.Engine, mode explorercontract.ProjectionMode, policy catalog.ProjectionPolicy) (*explorerapi.Service, error) {
	drift, err := engine.Drift(ctx)
	if err != nil || drift.Changed {
		return nil, errIndexNeedsReindex
	}
	index, err := engine.LoadIndex(ctx)
	if err != nil {
		return nil, errIndexNeedsReindex
	}
	dataset, err := explorerapi.Compile(ctx, index, mode, policy)
	if err != nil {
		return nil, err
	}
	return explorerapi.NewService(dataset)
}

func projectionMode(raw string) (explorercontract.ProjectionMode, error) {
	mode := explorercontract.ProjectionMode(strings.TrimSpace(raw))
	if !mode.Valid() {
		return "", fmt.Errorf("projection must be local or public")
	}
	return mode, nil
}
func projectionPolicy(kinds, tags, hosts string) catalog.ProjectionPolicy {
	allow := splitCSV(hosts)
	mode := catalog.URLModeAllowlist
	if len(allow) == 0 {
		mode = catalog.URLModeDrop
	}
	return catalog.ProjectionPolicy{RequireVisibility: catalog.VisibilityPublic, IncludeKinds: splitCSV(kinds), IncludeTags: splitCSV(tags), AllowHosts: allow, URLMode: mode}
}
func hasFlag(args []string, name string) bool {
	prefix := "--" + name
	for _, arg := range args {
		if arg == prefix || strings.HasPrefix(arg, prefix+"=") {
			return true
		}
	}
	return false
}
func usageFailure(stderr io.Writer, summary string) int {
	_, _ = fmt.Fprintf(stderr, "nicos-catalog: %s\n", summary)
	return 2
}

func writeCommandReceipt(stdout, stderr io.Writer, command string, mode explorercontract.ProjectionMode, digest string, data any, commandErr error) int {
	payload, err := json.Marshal(data)
	if err != nil {
		return fail(stderr, err)
	}
	envelope := explorercontract.Envelope{SchemaVersion: explorercontract.SchemaVersion, Command: command, OK: commandErr == nil, ProjectionMode: mode, SourceDigest: digest, Data: payload, Meta: explorercontract.Meta{Truncated: false}}
	if commandErr != nil {
		envelope.Error = &explorercontract.Error{Code: "command_failed", Summary: boundedCommandError(commandErr)}
	}
	if code := writeJSON(stdout, stderr, envelope); code != 0 {
		return code
	}
	if commandErr != nil {
		return 1
	}
	return 0
}

func boundedCommandError(err error) string {
	if errors.Is(err, errIndexNeedsReindex) {
		return errIndexNeedsReindex.Error()
	}
	return "The command could not complete safely."
}
