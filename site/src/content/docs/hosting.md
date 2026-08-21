---
title: Host a static Explorer
description: Build, deploy, and verify a public read-only Explorer.
---

Nicos Catalog exports one deterministic, public, read-only site. The site has
no catalog daemon and no write path.

## Build the synthetic demo

From a source checkout, run:

```sh
make demo-export
```

The target reindexes `demo/catalog` and writes `.deploy/explorer`. The demo
contains synthetic entities only.

## Deploy to Cloudflare Pages

Create one Pages project. Then deploy the generated directory:

```sh
wrangler pages project create YOUR_PROJECT --production-branch main
wrangler pages deploy .deploy/explorer \
  --project-name YOUR_PROJECT \
  --branch main
```

Use an account-pinned credential workflow for production. Do not put a token
in this repository or in a command argument.

Open the maintained synthetic demo at
<https://nicos-catalog-explorer.pages.dev/>.

## Verify the deployment

Check `/`, `/catalog`, `/graph`, `/health`, and `data/manifest.json`. Confirm
the product version, content digests, public projection mode, and direct-route
fallback. Confirm that the HTML response includes the Content Security Policy,
`X-Frame-Options: DENY`, and `X-Content-Type-Options: nosniff`. Run the browser
smoke and scan the deployed bytes for private paths, credentials, user corpus,
and host-only fields.

A public URL and a verified user journey prove a hosted deployment. They do
not prove an announcement, a launch, or independent adoption.
