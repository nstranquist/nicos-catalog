---
title: Migrate from v0.1
description: Mechanical replacements when you upgrade nicos-catalog from v0.1.x to v0.2.0.
---

`Index.SchemaVersion` advanced to 2. The first v0.2.0 run rebuilds any existing index. `Drift` reports `index_schema_mismatch` instead of failing.

`Record.Digest` is now per-entity. The previous whole-payload value is `Record.SourceDigest`.

## API replacements

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

Also available on `New`: `WithLogger`, `WithLimits`. Reconcile's zero mode is `ReconcileDryRun`.

Every exported struct has an unexported guard field. Unkeyed composite literals no longer compile.

## Errors

Match sentinels with `errors.Is`. Recover detail with `errors.As`. Do not compare message text.

```go
if errors.Is(err, catalog.ErrIndexMissing) { /* run reindex */ }

var policy *catalog.PolicyError
if errors.As(err, &policy) {
    log.Printf("entity %s field %s violated %s", policy.EntityID, policy.Field, policy.Rule)
}
```

See [API stability](/stability/) for the versioning policy and [CHANGELOG.md](https://github.com/nstranquist/nicos-catalog/blob/main/CHANGELOG.md) for the full v0.2.0 list.
