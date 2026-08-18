---
title: Install
description: Install the nicos-catalog CLI and run the five-minute smoke.
---

Requires **Go 1.24** or newer.

## Module and CLI

```sh
go install github.com/nstranquist/nicos-catalog/cmd/nicos-catalog@v0.2.0
nicos-catalog version --expect v0.2.0
```

From a source checkout of this package:

```sh
go test ./...
go install ./cmd/nicos-catalog
# or, without installing:
go run ./cmd/nicos-catalog version --expect v0.2.0
```

`VERSION` in the package root is the published SemVer. `nicos-catalog version --expect` fails unless the running binary matches.

## Five-minute smoke

The built-in demo contains synthetic entities only and writes to a temporary directory that is removed on exit.

```sh
nicos-catalog demo
nicos-catalog --json demo --query "developer platform"
```

To exercise the authored demo corpus in this repository:

```sh
nicos-catalog --root . --corpus demo/catalog validate
nicos-catalog --root . --corpus demo/catalog reindex
nicos-catalog --root . --corpus demo/catalog search --limit 3 "ownership graph"
nicos-catalog --root . --corpus demo/catalog graph
nicos-catalog --root . --corpus demo/catalog drift
nicos-catalog --root . --corpus demo/catalog --json project --visibility public --allow-hosts example.com
```

Next: the [docs index](/docs/), [host contract](/host/), and the [CLI reference](/cli/).
