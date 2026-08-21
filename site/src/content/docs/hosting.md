---
title: Host a static Explorer
description: Build, deploy, and verify a public read-only Explorer.
---

Nicos Catalog exports one deterministic, public, read-only site. The site has
no catalog daemon and no write path.

## Build the public site

From the `site/` directory, run:

```sh
pnpm verify
```

The Astro build writes `dist/`. The demonstration contains synthetic entities
only.

## Deploy to Workers Static Assets

Use the account-pinned NDev broker and the checked-in `wrangler.toml`:

```sh
pnpm ship
```

The command verifies the retained Pages URL first, verifies the Worker URL
after deployment, and writes a credential-free typed receipt. Do not put a
token in this repository or in a command argument.

Open the maintained synthetic demo at
<https://nicos-catalog-explorer.nstranquist.workers.dev/>.

## Verify the deployment

Check `/`, `/catalog`, `/graph`, `/health`, and `data/manifest.json`. Confirm
the product version, content digests, public projection mode, and direct-route
fallback. Run the browser smoke and scan the deployed bytes for private paths,
credentials, user corpus, and host-only fields.

A successful deployment does not prove independent adoption.
