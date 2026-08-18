# Nicos Catalog user docs (Starlight)

Viewer for the portable engine book. Not the ndev host operator surface.

```sh
pnpm install
pnpm build
pnpm preview   # http://127.0.0.1:4332
```

`cookie@2.0.1` is a direct dependency so Node prerender does not walk up into the monorepo’s `cookie@0.7.2` (no `parseCookie` export).

Content lives in `src/content/docs/`. Source of truth for facts is the parent `README.md`, `docs/architecture.md`, `docs/api-stability.md`, `CHANGELOG.md`, and `SECURITY.md`.

The splash homepage primary action is **Docs** (`/docs/`). That page is the book index. Host-only `ndev` verbs stay out of this tree.
