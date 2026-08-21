# Changelog

## [Unreleased]

## v0.3.4 — 2026-08-21

### Added

- Continuous integration now checks and builds the Astro documentation site.
- Publication tests enforce the pinned pnpm version and documentation-site
  dependency coverage.

### Changed

- Supported Go, Explorer, documentation, and GitHub Actions dependencies are
  updated to their compatible current versions.
- The embedded Explorer bundle is rebuilt from the updated dependency set.

### Fixed

- The documentation build now uses the repository-authored 404 page without
  also generating Starlight's default 404 route.
- The documentation project remains on its supported TypeScript major while
  the Explorer uses the newer compiler independently.

## v0.3.3 — 2026-08-20

### Added

- A repository-owned Code of Conduct defines expected behavior, private
  reporting, and proportionate enforcement.
- Static Explorer exports now include a deterministic Cloudflare Pages
  `_headers` control file with restrictive security headers.
- `SUPPORT.md` and a pull request template define contribution, privacy, and
  private security-reporting paths without a public issue tracker.
- Dependabot now covers the Explorer and documentation pnpm projects.

### Changed

- Product metadata now records the hosted synthetic Explorer and distinguishes
  deployment from a public launch or independent adoption.
- The repository no longer uses GitHub Issues for support, feedback, or backlog
  management.

### Fixed

- Hosting guidance no longer classifies a public URL and synthetic browser
  proof as a launch.
- Publication tests now enforce the no-Issues, dependency-monitoring,
  response-header, and launch-evidence boundaries.

## v0.3.2 — 2026-08-20

### Fixed

- Two portfolio-manifest links now name the current README headings.
- A publication regression test now checks every repository-local GitHub
  fragment in the portfolio manifest against the README heading anchors.
- The release procedure now requires an isolated performance sample and
  rejects a contended sample as evidence.

## v0.3.1 — 2026-08-20

### Fixed

- Public URL projection now rejects an at-sign in raw or decoded URL paths.
  Accepted URLs stay free of credential delimiters.
- CLI commands now fail when a successful human-readable result cannot be
  written to standard output.
- GitHub-local collation now restricts new and existing snapshot directories
  and files to owner-only permissions on POSIX systems.
- The linter now checks Explorer and host-collation code without broad
  package-level suppressions.
- Release notes are now timeless. A separate runbook and publication review
  keep mutable release state out of the GitHub Release body.
- Release tag verification now uses a repository-owned SSH allowed-signers
  file.
- The fuzz gate now binds Go scheduler concurrency to its worker limit on
  shared hosts.

## v0.3.0 — 2026-08-20

### Added

- **Nicos Catalog Explorer**, an embedded read-only web application with
  Overview, Catalog, Search, EntityDetail, Relationships, Graph, and Health views.
- `init` for a safe minimal or sample starter corpus.
- `serve` for a Host-locked loopback Explorer over a versioned HTTP contract.
- `demo --ui` for a temporary synthetic Explorer with no authored-corpus write.
- `export explorer` for a deterministic static site from the closed public
  projection. Each data file has a manifest-bound SHA-256 digest.
- `mcp --stdio` for bounded read-only search, page, graph, and health tools.
- Generated JSON Schema and TypeScript contracts, route-level code splitting,
  embedded-asset byte checks, accessibility tests, and bundle-size budgets.
- **`nicos-catalog collate`**, an opt-in local GitHub collation report. It has
  bounded walks, snapshots, duplicate detection, and observe-only enrollment
  gaps.

### Changed

- The public repository is now the source authority for the portable engine,
  CLI, Explorer, docs, fixtures, and release assets.
- Explorer starts graph reads with aggregates. Region and neighborhood reads
  have fixed node, edge, and depth limits.
- `validate`, `reindex`, `drift`, and `reconcile` retain enabled collation
  records instead of replacing them with filesystem records only.

### Fixed

- Explorer now keeps the browser `fetch` receiver bound. Chrome previously
  rejected live catalog reads with an illegal-invocation error.
- Closing a restored page now clears its stored selection. The prior effect
  reopened the drawer immediately after an Escape-key close.
- Collation now recognizes linked worktrees, submodules, bare repositories,
  `.git` symlinks, Git config includes, and `url.insteadOf` rewrites.
- Disabled and empty collation reports now use empty JSON arrays instead of
  `null` bucket values.
- Collation rejects host-corpus ID collisions and duplicate IDs across distinct
  remotes before it applies an index.
- A capped walk now reports `walked` and `walk_capped` explicitly.

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
