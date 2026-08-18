---
title: Architecture
description: Authored facts, derived state, and public projection in Nicos Catalog.
---

Nicos Catalog separates authored facts, derived state, and public projection.

1. A host injects a `Layout` and one or more `Provider` implementations.
2. Providers emit portable `Record` values with provenance.
3. `Engine.Discover` normalizes, validates, de-duplicates, and orders records.
4. `Engine.Reindex` writes a deterministic JSON index and BM25 document model.
5. Search and graph operations read only the derived index.
6. Drift re-runs providers and compares the canonical source digest.
7. Reconcile is explicit: dry-run by default, mutation only with `--apply`.
8. Public projection maps the private `Entity` type to a closed public DTO.

## Boundaries

The core owns:

- entity and relationship contracts;
- provider orchestration;
- deterministic index generation;
- BM25 full-text retrieval;
- graph compilation;
- drift and reconcile;
- privacy-safe public projection.

Hosts own:

- catalog corpus and configuration;
- discovery policy and runtime integrations;
- telemetry and sidecar state;
- business, valuation, and portfolio policy;
- deployment and publication approval.

The engine does not read environment-specific home directories, shell out to host commands, or infer a repository layout. This keeps two hosts independent: they may execute the same core with unrelated Layouts and providers.

GitHub-local collation is a host command (`nicos-catalog collate`). It reads `ConfigDir/settings.yaml`, walks configured clone roots, and feeds portable records through `WithProviders`. The engine still does not parse remotes or call GitHub.

## Determinism

Records, relationships, tags, and results use stable ordering. The index omits wall-clock timestamps. Reindexing identical provider output produces identical bytes, which makes a tracked or pre-commit drift gate trustworthy.

`Index.SchemaVersion` is 2 on the current module (`v0.2.0`). A bump is an expected whole-file diff for hosts that commit generated artifacts. `Drift` reports `index_schema_mismatch` instead of failing closed on first run. See [API stability](/stability/) and [Drift and reconcile](/drift/).

## Compatibility surface

The filesystem provider accepts YAML, JSON, and Markdown frontmatter for portable entity fields. Unknown host fields are ignored by the filesystem provider and therefore never cross into the public core. Go API and on-disk schema version independently; see `docs/api-stability.md` in the module.
