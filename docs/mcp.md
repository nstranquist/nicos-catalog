# Read-only MCP server

Nicos Catalog includes a bounded stdio MCP server. It uses the same immutable
Explorer service as the browser and HTTP API.

## Start the server

If the index is stale, rebuild it:

```sh
nicos-catalog reindex
nicos-catalog mcp --stdio
```

Use `--projection public` when the client must receive public fields only:

```sh
nicos-catalog mcp --stdio --projection public
```

Do not combine `--json` with `mcp --stdio`. The MCP protocol owns standard
output.

## Tools

| Tool | Result |
| --- | --- |
| `catalog_search` | Ranked projected entities. |
| `catalog_get` | One entity dossier and direct relationships. |
| `catalog_graph` | An aggregate, region, or neighborhood graph. |
| `catalog_health` | Redacted validation and drift findings. |

The server has no write tool. It does not edit the corpus, rebuild the index,
open a network listener, or send telemetry.

One MCP response is less than 64 KiB. MCP graph requests use lower limits than
the browser. A graph contains at most 200 nodes and 500 edges.

## Errors

Errors contain a stable code and a bounded summary. They do not reproduce a
rejected query value, source path, or provider payload.
