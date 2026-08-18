---
title: Drift and reconcile
description: Compare authored provider output to the derived index. Reconcile is dry-run unless you apply.
---

`Drift` re-runs providers, hashes the normalized records, and compares that digest to the index. It does not write.

```go
report, err := engine.Drift(ctx)
if report.Changed {
    // reason is index_missing, index_schema_mismatch, or source_digest_mismatch
}
```

```sh
nicos-catalog --root . --corpus demo/catalog drift
```

CLI exit code **3** means drift. Use that in CI to tell "the catalog moved" from "the tool broke".

## Reasons

| `reason` | Meaning |
| --- | --- |
| `index_missing` | No derived index yet |
| `index_schema_mismatch` | `Index.SchemaVersion` advanced. Rebuild the index |
| `source_digest_mismatch` | Authored records no longer match the index |

A schema bump is drift, not a crash. The first run after a library upgrade must reindex.

## Reconcile

Reconcile is explicit. The zero mode is `ReconcileDryRun`, so a caller that omits the mode cannot write.

```go
_, err := engine.Reconcile(ctx, catalog.ReconcileDryRun)
_, err = engine.Reconcile(ctx, catalog.ReconcileApply)
```

`ReconcileApply` rebuilds the index only when drift exists. Identical input stays byte-identical.

## What hosts usually do

1. Commit the generated index if the host tracks derived artifacts.
2. Run `drift` in CI.
3. If exit code is 3, run `reindex` or `ReconcileApply` and review the diff.

See [API stability](/stability/) for how `SchemaVersion` moves independently of the Go module version.
