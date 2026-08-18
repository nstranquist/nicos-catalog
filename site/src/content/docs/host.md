---
title: Host contract
description: Layout, Provider, FilesystemProvider, and StaticProvider as hosts inject them.
---

Hosts inject a filesystem `Layout` and one or more `Provider` implementations. The engine does not read environment-specific home directories, shell out to host commands, or infer a repository layout.

## Layout

```go
layout, _ := (catalog.Layout{
    CorpusDir:      "catalog",
    ConfigDir:      ".catalog",
    CacheDir:       ".catalog/cache",
    SidecarDataDir: ".catalog/sidecars",
}).Resolve(hostRoot)

engine, _ := catalog.New(layout, catalog.WithProviders(myProvider))
_, _ = engine.Reindex(ctx)
results, _ := engine.Search(ctx, "ownership graph", catalog.SearchOptions{Limit: 5})
```

`Resolve` turns those relative directories into a concrete layout under `hostRoot`. Two hosts may run the same core with unrelated layouts.

## Provider

A provider implements one small interface:

```go
type Provider interface {
    Name() string
    Provide(context.Context, catalog.Layout) ([]catalog.Record, error)
}
```

Provider output is normalized and sorted. Duplicate IDs fail closed across provider boundaries.

### FilesystemProvider

Handles YAML, JSON, and Markdown with YAML frontmatter.

- **Strict** mode rejects unknown fields, malformed frontmatter, trailing documents, and records without IDs. The CLI enables strict mode.
- Skips generated/cache directories plus `_archive` by default.
- Hosts can add directory names through `ExcludeDirs`.
- Refuses symlinked corpus files rather than resolving them (see [privacy](/privacy/)).

### StaticProvider

Supports embedded fixtures and API-backed hosts.

## What the host still owns

Corpus and configuration, discovery policy, telemetry and sidecar state, business/valuation/portfolio policy, and publication approval. Those stay out of this engine. See [architecture](/architecture/).

`nicos-catalog collate` is a host command that reads `ConfigDir/settings.yaml`
and walks local clones. It is not part of `Engine.Discover`. See [CLI](/cli/).
