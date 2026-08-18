# Architecture

Nicos Catalog separates authored facts, derived state, and public projection.
This public repository is the only editable authority for the portable product
code. Hosts consume a released module version and keep host-only policy in their
own repositories; they do not maintain an upstream product mirror.

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

The engine does not read environment-specific home directories, shell out to
host commands, or infer a repository layout. This keeps two hosts independent:
they may execute the same core with unrelated Layouts and providers.

GitHub-local collation lives in the host CLI (`nicos-catalog collate`). Settings,
remote parsing, and root walks stay outside this package. Collated records enter
through the existing Provider interface.

## Determinism

Records, relationships, tags, and results use stable ordering. The index omits
wall-clock timestamps. Reindexing identical provider output produces identical
bytes, which makes a tracked or pre-commit drift gate trustworthy.

## Current compatibility surface

Schema version 1 accepts YAML, JSON, and Markdown frontmatter for the portable
entity fields. Unknown host fields are ignored by the filesystem provider and
therefore never cross into the public core. Future breaking schema changes will
follow SemVer.
