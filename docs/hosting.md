# Host a static Explorer

Nicos Catalog exports one deterministic, public, read-only site. The site has
no catalog daemon and no write path.

## Build the synthetic demo

From a source checkout, run:

```sh
make demo-export
```

The target reindexes `demo/catalog` and writes `.deploy/explorer`. The demo
contains synthetic entities only. The export command rejects symlink paths,
protected catalog directories, and unknown non-Explorer output directories.

Preview the directory with a static server that sends `index.html` for unknown
application routes. Then run the browser smoke against that server.

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

The maintained synthetic demo is at
<https://nicos-catalog-explorer.pages.dev/>.

## Verify the deployment

Check all of these items after each deploy:

1. `/`, `/catalog`, `/graph`, and `/health` return the Explorer application.
2. `data/manifest.json` returns JSON with `projection_mode: public`.
3. The product version matches the release commit.
4. Each manifest digest matches its data file.
5. The HTML response includes the export's Content Security Policy,
   `X-Frame-Options: DENY`, and `X-Content-Type-Options: nosniff`.
6. Browser smoke reports no failed or third-party request.
7. A scan finds no private path, credential, user corpus, or host-only field.

Record the deployment URL, deployment ID, source commit, manifest digest,
browser run, and scan result in a dated publication review.

## Lifecycle boundary

An export on disk is built. A Pages upload is deployed. A public URL and a
verified user journey prove a hosted deployment. They do not prove an
announcement, a launch, independent adoption, retention, revenue, or
production use by another organization.
