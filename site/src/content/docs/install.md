---
title: Install
description: Install the nicos-catalog CLI and run the five-minute smoke.
---

Requires **Go 1.24** or newer.

## Module and CLI

```sh
go install github.com/nstranquist/nicos-catalog/cmd/nicos-catalog@v0.3.4
nicos-catalog version --expect v0.3.4
```

From a source checkout of this package:

```sh
go test ./...
go install ./cmd/nicos-catalog
# or, without installing:
go run ./cmd/nicos-catalog version --expect v0.3.4
```

`VERSION` in the package root is the published SemVer. `nicos-catalog version --expect` fails unless the running binary matches.

## Five-minute smoke

The built-in demo contains synthetic entities only and writes to a temporary directory that is removed on exit.

```sh
nicos-catalog demo
nicos-catalog --json demo --query "developer platform"
nicos-catalog demo --ui --open
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

Press `Ctrl-C` to stop the synthetic Explorer. The command removes its temporary
directory after the server stops.

## Start a new catalog

Run these commands in an empty project directory:

```sh
nicos-catalog init --template sample
nicos-catalog validate
nicos-catalog reindex
nicos-catalog serve --open
```

`init` writes only missing starter files. `serve` accepts a loopback address
only and does not edit the corpus.

Next: the [docs index](/docs/), [host contract](/host/), and the [CLI reference](/cli/).
