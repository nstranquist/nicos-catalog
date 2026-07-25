# Changelog

## [Unreleased]

## v0.2.0 — 2026-07-24

### Security and privacy

- **A rejected value is no longer reproduced in the error that rejects it.**
  Publication failures now return a typed `*PolicyError` carrying the entity,
  the field, and a `PolicyRule`, and never the offending text. The previous
  message embedded the match, so a rejected token or path was reproduced into
  stderr, CI logs, and host error paths.
- **`FilesystemProvider` no longer follows symlinked corpus files.** A symlink
  pointing outside the corpus resolved to content that decoded into perfectly
  valid entities. Strict mode reports `ErrCorpusEscape`; lax mode skips.
- The `PublicURL` path segment is now content-scanned like every other
  published field, and the path-disclosure patterns match mid-token.

### Breaking changes

- `New(layout, providers...)` → `New(layout, opts ...Option)`. Use
  `WithProviders`. Also available: `WithLogger`, `WithLimits`.
- `context.Context` added to `LoadIndex`, `Search`, and `ProjectPublic`.
- `Reconcile(ctx, apply bool)` → `Reconcile(ctx, mode ReconcileMode)`. The zero
  value is `ReconcileDryRun`, so a caller who omits the mode cannot write.
- `Validate` gained variadic `ValidateOption`; `ValidationReport.OK` is now
  derived from `Errors` instead of being hardcoded true, and `Warnings` is
  `[]ValidationIssue` rather than `[]string`.
- `Entity.Visibility` and `ProjectionPolicy.RequireVisibility` are the typed
  `Visibility`; `BuildInfo.Capabilities` is `[]Capability`.
- `Version` and `Commit` are no longer mutable package variables. Use the
  `Version()` and `Commit()` accessors; an importing host can no longer rewrite
  the identity the library reports.
- Every exported struct gained an unexported guard field, so unkeyed composite
  literals no longer compile and a future field is never a silent break.
- **`Index.SchemaVersion` advanced from 1 to 2.** `Record.Digest` is now a
  per-entity digest computed after normalization, and the whole-payload digest
  moved to the new `Record.SourceDigest`. Every existing index is rebuilt on
  first run; `Drift` reports `index_schema_mismatch` rather than failing, so the
  upgrade is a reindex prompt instead of a crash.

### Fixed

- `LoadIndex` wrapped its missing-index error with `%s` instead of `%w`, so
  `errors.Is(err, os.ErrNotExist)` was never true and `Drift`'s `index_missing`
  branch was unreachable. Both sentinels now match, and the `os.Stat` fallback
  the original author added in its place has been removed.
- `normalizeEntity` trimmed and re-sorted the caller's `Refs` through a shared
  backing array. Refs and annotations are copied before normalization.
- Summary truncation appended the ellipsis *after* enforcing
  `MaxSummaryBytes`, so the result could exceed the caller's declared bound. The
  marker is now charged against the budget.
- Public projection resolved each item's connections with a linear scan, making
  it quadratic in corpus size.
- `escapeMermaid` escaped only quotes and backslashes, so a newline in a name
  terminated the statement and corrupted the rest of the diagram.
- `boundedSummary` could emit invalid UTF-8 when handed already-invalid input
  (found by fuzzing).
- `writeJSONAtomic` never fsynced the parent directory after the rename.
- Kind filtering was case-sensitive in `ProjectPublic` and case-insensitive in
  `Search`; both now fold case.
- `Layout.Validate` errors now wrap `ErrInvalidLayout`.

### Added

- Typed errors: sentinels for `errors.Is`, plus `*ProviderError`,
  `*EntityError`, `*DuplicateIDError` (carrying both origins), `*DecodeError`,
  `*IndexError`, and `*PolicyError` for `errors.As`.
- `ScanPublicText`, so a host's own publication gate runs the library's rules
  rather than a copy that drifts.
- `ProjectionPolicy.ExcludeTags`, `URLMode`, `TruncationSuffix`, and a
  `Validate()` that fails at config load rather than at publication time.
- `EntityIDPattern` and `ValidateEntityID` for hosts asserting their own
  identifier grammar is a subset of the portable one.
- A package doc comment and runnable examples, so pkg.go.dev renders something.
- Coverage floors (90% library, 80% CLI), six fuzz targets, and benchmarks.

## v0.1.1 — 2026-07-22

- Move to the maintained `go.yaml.in/yaml/v3` module path at v3.0.4.
- Publish a reviewed screenshot of the independent Gallery host and mark the
  portfolio manifest public.
- Carry reviewed portfolio PNGs through the upstream release synchronizer under
  bounded size, dimension, path, and decode checks.

## v0.1.0 — 2026-07-22

- Introduce injected host `Layout` and provider registry.
- Add YAML, JSON, and Markdown-frontmatter filesystem provider.
- Add deterministic index generation and BM25 full-text search.
- Add typed graph, drift, and explicit reconcile operations.
- Add closed public projection DTO with visibility and URL-host allowlists.
- Add synthetic CLI demo and portable example corpus.
