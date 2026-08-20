# Static Explorer export

A static export is a deterministic public site. It contains no live catalog
server and no write path.

## Build the export

If the index is stale, rebuild it:

```sh
nicos-catalog reindex
```

Select the public visibility boundary and output directory:

```sh
nicos-catalog export explorer \
  --out ./public-catalog \
  --visibility public \
  --allow-hosts example.com
```

Serve the output with any static file server. The server must provide the
application fallback to `index.html` for client routes.

Use the [hosting guide](hosting.md) for the synthetic demo, Cloudflare Pages,
and post-deploy checks.

## Output contract

The export contains these files:

```text
index.html
assets/<content-hash>.js
assets/<content-hash>.css
data/manifest.json
data/entities.json
data/graph.json
data/health.json
data/search.json
```

`data/manifest.json` binds each data file to a SHA-256 digest. The browser
checks each digest before it uses the file.

The manifest has no time, local path, host name, user name, machine ID, or
operator identity. Equal inputs and equal application assets produce equal
output bytes.

## Safety rules

CAUTION: Select a new output directory or a prior Explorer output directory.
The command refuses an unknown non-empty directory.

The command also refuses these targets:

- a filesystem root;
- a path that contains a symlink;
- a path inside the corpus, settings, cache, or sidecar directory;
- a prior output with an invalid ownership manifest.

The command builds a unique sibling temporary directory. It replaces a prior
complete output only after the new output passes all checks.

## Hosting state

An export on disk is not deployed. An uploaded export is not launched or
adopted. Record each state from direct evidence.
