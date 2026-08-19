---
title: Contribute
description: Local gate and host-independence rules for nicos-catalog.
---

Run the complete local gate before you propose a change:

```sh
go test ./...
make verify
make docs-site
```

`make verify` runs Go and Explorer checks. It also checks reproducible builds,
generated contracts, and embedded assets.

## Explorer source

Install the pinned package graph before you change the UI:

```sh
make explorer-install
```

Run the Explorer development server on loopback:

```sh
corepack pnpm@11.13.0 --dir explorer dev
```

Run the complete Explorer check before you commit:

```sh
make explorer-check
make verify-explorer-embed
```

The UI must not use a `file:` or `link:` dependency outside this repository.
Do not edit generated contract files directly.

If a Go contract changes, regenerate both contract files:

```sh
go generate ./internal/explorercontract
make verify-explorer-contract
```

Commit the new `explorer/dist` bytes with their source changes. The Go install
path embeds these bytes and does not run Node.

To rebuild this book from the package root:

```sh
make docs-site
```

## Rules

Keep the core host-independent. The engine must not read environment-specific home directories, shell out to host commands, or infer a repository layout.

New fields need a concrete cross-host need. Private telemetry, valuation, credentials, and publication policy belong in host adapters.

Add tests for determinism, [drift](/drift/), and [public projection](/privacy/) whenever those boundaries change.

Adding a field to `PublicEntity` is a privacy review. Update the golden field list, the golden JSON, and `SECURITY.md` in the same commit.
