---
title: API stability
description: Go SemVer, on-disk SchemaVersion, public surface, errors, and determinism.
---

Two contracts version separately.

**The Go API** follows SemVer. While the module is `v0.x`, a minor bump can break the Go API. Each break is listed under `Breaking changes` in `CHANGELOG.md` with the replacement. After `v1.0.0`, a breaking Go API change requires a major bump.

**`SchemaVersion`** governs the on-disk derived index. It moves independently of the module version. When it advances, the new engine cannot read the old index. `Drift` reports `index_schema_mismatch` so the host reindexes instead of crashing. Current value: **2** (`v0.2.0`).

A host that commits generated artifacts must treat a `SchemaVersion` bump as an expected whole-file diff.

## What is public

Everything exported from the root package, plus the `nicos-catalog` command's documented flags and [exit codes](/cli/#exit-codes).

The closed [publication DTO](/privacy/) is the narrowest contract. Adding a field to `PublicEntity` is a privacy decision. It must update the golden field list, the golden JSON, and `SECURITY.md` in the same commit.

## Errors

Sentinels (`ErrIndexMissing`, `ErrCorpusEscape`, `ErrEmptyQuery`, …) and typed errors (`*PolicyError`, `*DuplicateIDError`, …) are part of the API. Match them with `errors.Is` and `errors.As`. Error **strings** are not stable.

An error never reproduces the value that caused the rejection. A `*PolicyError` names the entity, the field, and the rule.

```go
if errors.Is(err, catalog.ErrIndexMissing) {
    // run reindex
}

var policy *catalog.PolicyError
if errors.As(err, &policy) {
    log.Printf("entity %s field %s violated %s", policy.EntityID, policy.Field, policy.Rule)
}
```

## Determinism

Identical provider output produces identical index bytes. The index has no wall-clock timestamps. Every collection is sorted.

A change that perturbs output ordering is breaking even when no signature changes. Hosts depend on this for committed artifacts.

[Search scores](/search/#scores-and-rank) are not part of the guarantee.

## Supported Go

The `go` directive in `go.mod` is the minimum. Raising it is a minor-version change.

## Deprecation

A symbol slated for removal is marked `Deprecated:` in its doc comment for at least one minor release, with the replacement named in the same comment.

See [Migrate from v0.1](/migrate/) for the v0.2.0 replacements.
