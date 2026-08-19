---
title: Read-only MCP
description: Connect an agent to bounded search, dossier, graph, and health tools.
---

If the index is stale, rebuild it:

```sh
nicos-catalog reindex
nicos-catalog mcp --stdio
```

The server exposes four read-only tools:

- `catalog_search`
- `catalog_get`
- `catalog_graph`
- `catalog_health`

It has no write tool and opens no network listener. One response is less than
64 KiB. One MCP graph contains at most 200 nodes and 500 edges.

Use `--projection public` when the client must receive public fields only. Do
not combine `mcp --stdio` with `--json`.
