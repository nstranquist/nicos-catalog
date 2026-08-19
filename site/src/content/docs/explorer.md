---
title: Explorer
description: Open the read-only local UI and use its bounded catalog views.
---

Nicos Catalog Explorer is the reference UI for the portable engine contract.
The Go binary embeds the production web assets. Node is not required at run
time.

## Open the synthetic demo

```sh
nicos-catalog demo --ui --open
```

Press `Ctrl-C` to stop the server. The command then removes its temporary
synthetic corpus.

## Open an authored catalog

If the index is stale, rebuild it:

```sh
nicos-catalog reindex
nicos-catalog serve --open
```

The server accepts loopback addresses only. It validates the request host and
does not set a permissive CORS header.

## Views

| Route | Job |
| --- | --- |
| `/` | Explain corpus shape and urgent health state. |
| `/catalog` | Browse, search, filter, sort, and open a page. |
| `/entity/:id` | Read one entity and its direct relationships. |
| `/graph` | Move from aggregates to a bounded region or neighborhood. |
| `/health` | Read redacted validation and drift findings. |

Press `/` to focus global search. Press `g` and a navigation key to change a
view. The keys are `o`, `c`, `g`, and `h`.

Explorer supports light, dark, and automatic themes. It honors reduced-motion,
increased-contrast, and forced-colors browser settings. The page keeps
keyboard focus inside while open and returns focus to its opener when closed.

Explorer stores one selected entity ID in session storage. It does not store
query history or entity bodies. See the full
[Explorer contract](https://github.com/nstranquist/nicos-catalog/blob/main/docs/explorer.md).
