# Nicos Catalog

Nicos Catalog is a typed, local-first software-catalog engine for repositories,
services, products, documents, and the relationships between them. Hosts inject
their own filesystem layout and providers; the engine supplies deterministic
validation, indexing, BM25 full-text search, graph compilation, drift/reconcile
gates, and a closed privacy-safe publication DTO.

The public core deliberately excludes personal telemetry, business valuation,
private query text, runtime credentials, and host-specific portfolio policy.
Those stay in host adapters.

## Install

Requires Go 1.24 or newer.

```sh
go install github.com/nstranquist/nicos-catalog/cmd/nicos-catalog@v0.1.1
nicos-catalog version --expect v0.1.1
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

engine, _ := catalog.New(layout, myProvider)
_, _ = engine.Reindex(context.Background())
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
is normalized and sorted; duplicate IDs fail closed across provider boundaries.

## Privacy boundary

`ProjectPublic` produces a closed `PublicEntity` DTO. It cannot encode source
paths, annotations, owner fields, telemetry, query text, valuation, or sidecar
data. Hosts may further restrict visibility, kinds, tags, URL hosts, and summary
length. Publication should consume this DTO rather than filtering the private
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
