---
title: Static export
description: Build and host a deterministic Explorer from the closed public projection.
---

If the index is stale, rebuild it first:

```sh
nicos-catalog reindex
nicos-catalog export explorer --out ./public-catalog --visibility public --allow-hosts example.com
```

The command accepts the public projection only. It refuses a filesystem root,
a symlink path, a protected catalog directory, and an unknown non-empty
directory.

The export contains hashed application assets and four versioned data files.
`data/manifest.json` binds each data file to a SHA-256 digest. The browser
checks that digest before it uses the file.

Equal inputs produce equal output bytes. The manifest contains no time, local
path, host name, user name, machine ID, or operator identity.

See the full [static export safety guide](https://github.com/nstranquist/nicos-catalog/blob/main/docs/static-export.md).
