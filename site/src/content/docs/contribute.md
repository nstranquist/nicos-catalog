---
title: Contribute
description: Local gate and host-independence rules for nicos-catalog.
---

Run the complete local gate before you propose a change:

```sh
go test ./...
go test -race ./...
go vet ./...
go build -trimpath ./cmd/nicos-catalog
go run ./cmd/nicos-catalog --json demo
```

To rebuild this book from the package root:

```sh
make docs-site
```

## Rules

Keep the core host-independent. The engine must not read environment-specific home directories, shell out to host commands, or infer a repository layout.

New fields need a concrete cross-host need. Private telemetry, valuation, credentials, and publication policy belong in host adapters.

Add tests for determinism, [drift](/drift/), and [public projection](/privacy/) whenever those boundaries change.

Adding a field to `PublicEntity` is a privacy review. Update the golden field list, the golden JSON, and `SECURITY.md` in the same commit.
