# Nicos Catalog

[![Go Reference](https://pkg.go.dev/badge/github.com/nstranquist/nicos-catalog.svg)](https://pkg.go.dev/github.com/nstranquist/nicos-catalog)
[![CI](https://github.com/nstranquist/nicos-catalog/actions/workflows/ci.yml/badge.svg)](https://github.com/nstranquist/nicos-catalog/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/nstranquist/nicos-catalog)](https://goreportcard.com/report/github.com/nstranquist/nicos-catalog)
[![License](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](LICENSE)

Nicos Catalog is a local software-catalog engine for repositories, services,
products, documents, and the relationships between them. A host supplies the
folder layout and data plugins. The engine validates records, indexes them,
searches them (BM25), builds a relationship graph, and fails a drift check
when the catalog no longer matches the source files. The public export format
omits private data.

The public core leaves out personal telemetry, business valuation, private
query text, runtime credentials, and host-only portfolio policy. Those stay
in host adapters.

## Install

Requires Go 1.24 or newer.

```sh
go install github.com/nstranquist/nicos-catalog/cmd/nicos-catalog@v0.2.0
nicos-catalog version --expect v0.2.0
```

For a source checkout:

```sh
go test ./...
go install ./cmd/nicos-catalog
```

## Five-minute smoke

The built-in demo contains synthetic entities only and writes to a temporary
directory that is removed on exit.

```sh
nicos-catalog demo
nicos-catalog --json demo --query "developer platform"
```

To exercise an authored corpus from this repository:

```sh
nicos-catalog --root . --corpus demo/catalog validate
nicos-catalog --root . --corpus demo/catalog reindex
nicos-catalog --root . --corpus demo/catalog search --limit 3 "ownership graph"
nicos-catalog --root . --corpus demo/catalog graph
nicos-catalog --root . --corpus demo/catalog drift
nicos-catalog --root . --corpus demo/catalog --json project --visibility public --allow-hosts example.com
```

## Host contract

```go
layout, _ := (catalog.Layout{
    CorpusDir: "catalog",
    ConfigDir: ".catalog",
    CacheDir: ".catalog/cache",
    SidecarDataDir: ".catalog/sidecars",
}).Resolve(hostRoot)

engine, _ := catalog.New(layout, catalog.WithProviders(myProvider))
_, _ = engine.Reindex(ctx)
results, _ := engine.Search("ownership graph", catalog.SearchOptions{Limit: 5})
```

A provider implements one small interface:

```go
type Provider interface {
    Name() string
    Provide(context.Context, catalog.Layout) ([]catalog.Record, error)
}
```

`FilesystemProvider` handles YAML, JSON, and Markdown with YAML frontmatter. Its
`Strict` mode rejects unknown fields, malformed frontmatter, trailing
documents, and records without IDs; the CLI enables strict mode. It skips
generated/cache directories plus `_archive` by default; hosts can add directory
names through `ExcludeDirs`.
`StaticProvider` supports embedded fixtures and API-backed hosts. Provider output
is normalized and sorted. Duplicate IDs are rejected, even across providers.

## Privacy boundary

`ProjectPublic` produces a closed `PublicEntity` export. It cannot encode source
paths, annotations, owner fields, telemetry, query text, valuation, or sidecar
data. Hosts may further restrict visibility, kinds, tags, URL hosts, and summary
length. Publication should use this export rather than filtering the private
index after serialization.

## Why this exists

Large personal and organizational ecosystems become hard to reason about long
before they become large enough to justify a heavyweight service catalog. Nicos
Catalog keeps the data portable and reviewable while still providing the pieces
that make a catalog operational: provider boundaries, deterministic derived
state, search, typed relationships, and drift enforcement.

See [docs/architecture.md](docs/architecture.md) for design boundaries and
[SECURITY.md](SECURITY.md) for publication guidance.

## License

Apache-2.0.

## Migrating from v0.1.x

| v0.1.x | v0.2.0 |
| --- | --- |
| `catalog.New(layout, p1, p2)` | `catalog.New(layout, catalog.WithProviders(p1, p2))` |
| `engine.LoadIndex()` | `engine.LoadIndex(ctx)` |
| `engine.Search(q, opts)` | `engine.Search(ctx, q, opts)` |
| `catalog.ProjectPublic(index, policy)` | `catalog.ProjectPublic(ctx, index, policy)` |
| `engine.Reconcile(ctx, true)` | `engine.Reconcile(ctx, catalog.ReconcileApply)` |
| `catalog.Version` | `catalog.Version()` |
| `Visibility: "public"` | `Visibility: catalog.VisibilityPublic` |
| `report.Warnings []string` | `report.Warnings []catalog.ValidationIssue` |

`Index.SchemaVersion` advanced to 2, so the first v0.2.0 run rebuilds any
existing index. `Drift` reports `index_schema_mismatch` instead of failing, so
this surfaces as a reindex prompt rather than an error. `Record.Digest` is now
per-entity; the previous whole-payload value is `Record.SourceDigest`.

Errors are now typed. Match sentinels with `errors.Is` and recover detail with
`errors.As` rather than comparing message text:

```go
if errors.Is(err, catalog.ErrIndexMissing) { /* run reindex */ }

var policy *catalog.PolicyError
if errors.As(err, &policy) {
    log.Printf("entity %s field %s violated %s", policy.EntityID, policy.Field, policy.Rule)
}
```

See [`docs/api-stability.md`](docs/api-stability.md) for the versioning policy.
