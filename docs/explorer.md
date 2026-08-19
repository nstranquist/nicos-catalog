# Nicos Catalog Explorer

Explorer is the read-only reference UI for the portable catalog contract. The
Go binary embeds the production web assets. Node is not required at run time.

## Open the synthetic demo

```sh
nicos-catalog demo --ui --open
```

The command creates a temporary synthetic corpus. It starts a loopback server
on a random port. Press `Ctrl-C` to stop the server and remove the temporary
directory.

## Open an authored corpus

If the index is missing or stale, rebuild it first:

```sh
nicos-catalog reindex
nicos-catalog serve --open
```

The server accepts IPv4 loopback, IPv6 loopback, or `localhost`. It rejects a
non-loopback address before it opens a listener.

Use the public projection for a publication review:

```sh
nicos-catalog serve --projection public --allow-hosts example.com --open
```

## Views

Explorer has five routes and seven user jobs:

| Route | Job |
| --- | --- |
| `/` | Explain corpus shape and urgent health state. |
| `/catalog` | Browse, search, filter, sort, and open a page. |
| `/entity/:id` | Read one entity and its incoming and outgoing relationships. |
| `/graph` | Move from aggregates to a region or bounded neighborhood. |
| `/health` | Read redacted validation and drift findings. |

Search and graph scope use validated URL state. Invalid values use safe
defaults and produce a notice. Explorer stores one selected entity ID in the
browser session. It does not store query history or entity bodies.

## Keyboard use

- Press `/` to focus the global search field.
- Press `g`, then `o`, to open Overview.
- Press `g`, then `c`, to open Catalog.
- Press `g`, then `g`, to open Graph.
- Press `g`, then `h`, to open Health.
- Press `Escape` to close the page drawer.

The drawer traps `Tab` focus and restores focus when it closes. Each graph has
an equivalent node and relationship table.

Explorer supports light, dark, and automatic themes. It honors reduced-motion,
increased-contrast, and forced-colors browser settings. All text and focus
tokens meet a 4.5:1 contrast floor in the light and dark themes.

## Data limits

- One entity page contains at most 100 rows.
- One search result contains at most 50 rows.
- One graph contains at most 500 nodes and 1,500 edges.
- A graph neighborhood has a maximum depth of two.
- One HTTP response contains less than 1 MiB.

Explorer starts with graph aggregates. It does not request the full entity
graph on the overview route.

## Local data and public data

Local mode can show bounded owner and entrypoint labels. It cannot show source
paths, annotations, sidecars, credentials, query history, telemetry, or
valuation data.

Public mode starts from `ProjectPublic`. It cannot add fields after projection.
An edge enters the public view only when both endpoints enter the view.

## Troubleshooting

If Explorer reports a stale index, run `nicos-catalog reindex`.

If a public URL fails projection, add its host to `--allow-hosts`. Do not add a
host until you review every matching URL.

If a graph requests refinement, select a smaller region or one entity. Explorer
does not render a misleading partial graph.
