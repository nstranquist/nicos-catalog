# API stability

## Two contracts, versioned separately

**The Go API** follows SemVer. While the module is `v0.x`, a minor bump may
break the Go API; every such break is listed under a `Breaking changes` heading
in [`CHANGELOG.md`](../CHANGELOG.md) with the mechanical replacement. After
`v1.0.0`, a breaking Go API change requires a major bump.

**`SchemaVersion`** governs the on-disk derived index and moves independently of
the module version. When it advances, an existing index is not readable by the
new engine — but this is deliberately not an error. `Drift` reports
`index_schema_mismatch`, so a host upgrading the library sees a prompt to
reindex rather than a crash on first run.

A host that commits generated artifacts should treat a `SchemaVersion` bump as
an expected whole-file diff.

## What is public

Everything exported from the root package, plus the `nicos-catalog` command's
documented flags and exit codes:

| Code | Meaning |
| --- | --- |
| 0 | success |
| 1 | operational error |
| 2 | usage error |
| 3 | drift detected (`drift` only) |

Exit code 3 is a contract: CI gates branch on it to distinguish "the catalog
moved" from "the tool broke."

## The closed publication DTO

`PublicEntity` is the narrowest contract in the module and the one most likely
to be relied on for compliance rather than convenience. Its field set is frozen
by tests that assert:

- the exact ordered field list, including JSON tags;
- that no reachable type is a map, interface, pointer, channel, function, or
  foreign struct — so an escape hatch cannot be added by accident;
- that no field name or tag collides with a private `Entity` field;
- that every `Entity` field is classified as either forbidden or publishable, so
  growing the private contract cannot silently bypass review;
- a golden JSON key set, which is what catches a tag rename.

**Adding a field to `PublicEntity` is a privacy decision, not a routine
change.** It means new data leaves the host. Such a change must update the
golden field list, the golden JSON, and [`../SECURITY.md`](../SECURITY.md) in
the same commit, and warrants a minor version bump even pre-1.0 so downstream
publishers notice.

Removing or renaming a field is breaking for both Go callers and every consumer
reading the published artifact by key.

## Errors

Sentinels (`ErrIndexMissing`, `ErrCorpusEscape`, …) and typed errors
(`*PolicyError`, `*DuplicateIDError`, …) are part of the API. Match them with
`errors.Is` and `errors.As`; error *strings* are not stable and may be reworded
in any release.

One guarantee is load-bearing: **an error never reproduces the value that caused
it to be rejected.** A `*PolicyError` names the entity, the field, and the rule.
Errors travel to logs and CI output, so echoing a rejected secret or path would
defeat the boundary the projection exists to enforce.

## Supported Go versions

The `go` directive in `go.mod` is the supported minimum, and CI proves it: the
oldest row of the test matrix is that exact version, and a dedicated job fails
when `go.mod`, the README, and the matrix floor disagree. Raising the minimum is
a minor-version change and is called out in the changelog.

## Determinism

Identical inputs produce byte-identical output. The index carries no wall-clock
time and every collection is sorted. Hosts are expected to depend on this — the
usual pattern is to commit a generated artifact and fail the build when a fresh
compile does not match it — so a change that perturbs output ordering is treated
as breaking even when no signature changes.

Search *scores* are explicitly not part of this guarantee. They are relative
within one result set, are not comparable across queries, and may change with
any scoring improvement. Depend on rank order, not on score values.

## Deprecation

A symbol slated for removal is marked `Deprecated:` in its doc comment for at
least one minor release before it is removed, with the replacement named in the
same comment.
