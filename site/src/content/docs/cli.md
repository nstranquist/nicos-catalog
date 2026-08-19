---
title: CLI
description: nicos-catalog validate, reindex, search, graph, drift, collate, and related commands.
---

The `nicos-catalog` command is part of the public contract. Documented flags and exit codes are stable; see [API stability](https://github.com/nstranquist/nicos-catalog/blob/main/docs/api-stability.md) in the module tree.

## Global flags

| Flag | Default | Meaning |
| --- | --- | --- |
| `--root` | `.` | Host root used to resolve relative layout paths |
| `--corpus` | `catalog` | Authored catalog corpus directory |
| `--config` | `.nicos-catalog` | Host configuration directory |
| `--cache` | `.nicos-catalog/cache` | Derived index directory |
| `--sidecars` | `.nicos-catalog/sidecars` | Host-owned sidecar data directory |
| `--json` | off | Machine-readable JSON |

## Commands

### `init`

Writes a minimal or sample starter corpus. It refuses a different existing
file. Use `--dry-run` to inspect the complete plan without a write.

```sh
nicos-catalog init --template sample --dry-run
nicos-catalog init --template sample
```

### `validate`

Runs providers, normalizes, and reports entity counts and warnings. Does not write the index.

```sh
nicos-catalog --root . --corpus demo/catalog validate
```

### `reindex`

Writes the deterministic JSON index and BM25 document model.

```sh
nicos-catalog --root . --corpus demo/catalog reindex
```

### `search`

BM25 full-text search over the derived index. See [Search](/search/).

```sh
nicos-catalog --root . --corpus demo/catalog search --limit 3 "ownership graph"
```

`--kinds` is a comma-separated kind filter.

### `graph`

Compiles the typed relationship graph. Default format is Mermaid; `--format json` or `--json` emits JSON. See [Graph](/graph/).

```sh
nicos-catalog --root . --corpus demo/catalog graph
```

### `drift`

Re-runs providers and compares the canonical source digest to the index. Exit code **3** means drift detected (CI can distinguish “the catalog moved” from “the tool broke”). See [Drift and reconcile](/drift/).

```sh
nicos-catalog --root . --corpus demo/catalog drift
```

A `SchemaVersion` bump is reported as `index_schema_mismatch` so an upgrade is a reindex prompt, not a crash.

### `collate`

Walks local git clones named in host settings (`<config>/settings.yaml`) and reports which ones belong to the configured GitHub profile and already carry catalog registration. Dry-run is the default. `--apply` rebuilds the host derived index only; it does not clone, fetch, pull, call the GitHub API, or write scanned repos.

When collation is enabled, `validate`, `reindex`, `drift`, and `reconcile` include those same records, so a later `reindex` does not wipe a collated index.

Identity comes from the checkout's git config: a `.git` directory, a `gitdir:` file (worktree, submodule, `--separate-git-dir`), a `.git` symlink, or a bare git directory. Remotes are read through `[include]`, `includeIf gitdir`, and `url.insteadOf` (repo config plus `~/.gitconfig`).

Collation is off until settings set `github.collation.enabled: true` and `github.profile`. Unregistered clones and remotes owned by someone else appear only as report buckets.

```sh
nicos-catalog --root . --json collate
nicos-catalog --root . --json collate --apply
nicos-catalog --root . --json collate --from-snapshot
nicos-catalog --root . --json collate --profile-repos owner/a,owner/b
nicos-catalog --root . --json collate --enroll-manifest /path/external-projects.yaml
```

Walk budget and skip names live in settings (`max_repos`, `skip_dir_names`). A capped walk sets `walk_capped` and `walked` in the report. Two checkouts of
the same GitHub remote count as one clone. The same product id on different remotes is a `duplicates` bucket: the
command still prints the report, then exits 1. `--apply`, `reindex`, and
`validate` do not first-win. `--apply` and `--write-snapshot` persist `collation-snapshot.json` next to the derived index after a successful apply. `--from-snapshot --apply` rebuilds the index from stored snapshot records without re-walking. `--profile-repos` opts into the missing-clone bucket. Enrollment gaps are observe-only.

### `project`

Publishes the closed public DTO. Require an explicit visibility and URL host allowlist.

```sh
nicos-catalog --root . --corpus demo/catalog --json project --visibility public --allow-hosts example.com
```

### `serve`

Runs Explorer on a loopback address. Rebuild a stale index before this command.

```sh
nicos-catalog reindex
nicos-catalog serve --open
```

Use `--projection public --allow-hosts example.com` for a public projection
review. The server rejects a non-loopback bind and an unknown request host.

### `export explorer`

Writes a deterministic static public site. `--visibility public` is required.

```sh
nicos-catalog export explorer --out ./public-catalog --visibility public --allow-hosts example.com
```

See [Static export](/static-export/) for target safety and hosting rules.

### `mcp`

Runs the bounded read-only MCP server over standard input and output.

```sh
nicos-catalog mcp --stdio
```

Do not combine this command with `--json`. See [Read-only MCP](/mcp/).

### `demo` / `version`

See [install](/install/).

## Exit codes

| Code | Meaning |
| --- | --- |
| 0 | success |
| 1 | operational error |
| 2 | usage error |
| 3 | drift detected (`drift` only) |
