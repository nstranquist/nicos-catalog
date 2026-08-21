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

The export contains hashed application assets, four versioned data files, and
a `_headers` control file. `data/manifest.json` binds each data file to a
SHA-256 digest. The browser checks that digest before it uses the file.

Cloudflare Pages parses `_headers` and applies the restrictive Content
Security Policy and related response headers. It does not serve `_headers` as
a public asset. Another static host must apply equivalent response headers.

Equal inputs produce equal output bytes. The manifest contains no time, local
path, host name, user name, machine ID, or operator identity.

Use [Host a static Explorer](/hosting/) for deployment and post-deploy checks.

See the full [static export safety guide](https://github.com/nstranquist/nicos-catalog/blob/main/docs/static-export.md).
