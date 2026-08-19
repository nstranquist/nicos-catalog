---
title: Troubleshooting
description: Fix common index, Explorer, export, and MCP errors.
---

## Explorer reports a stale index

Rebuild the derived index:

```sh
nicos-catalog reindex
```

`serve`, `export explorer`, and `mcp` refuse a missing or stale index.

## The server rejects the listen address

Use an IPv4 loopback, IPv6 loopback, or `localhost` address:

```sh
nicos-catalog serve --listen 127.0.0.1:0
```

Explorer does not accept a LAN or public bind in v0.3.0.

## Public projection rejects a URL

Review the URL host. Then add the reviewed host to `--allow-hosts`.

Do not use a broad host list. An empty list drops URLs for Explorer public mode
and fails URL publication in the base `project` command.

## Static export rejects the output directory

Select a new directory or a prior Explorer output directory. Do not select a
corpus, settings, cache, or sidecar directory.

Explorer refuses symlinks and unknown non-empty targets. This rule prevents an
accidental recursive replacement of caller-owned files.

## A graph asks for refinement

Select a smaller region or one entity. Use depth one before depth two.

Explorer does not silently trim a large graph into a misleading partial view.

## MCP writes JSON errors to the client

Remove the global `--json` flag. `mcp --stdio` owns standard output for the MCP
protocol.
