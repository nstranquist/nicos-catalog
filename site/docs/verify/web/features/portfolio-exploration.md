# Portfolio exploration

## Sub-features

- The public page renders the reviewed synthetic catalog without authentication.
- A visitor can filter and inspect a product card.
- Valuation labels remain explicit about synthetic data.
- The page makes no third-party analytics request.

## How to get to it (user POV)

1. Open the explorer.
2. Filter the catalog.
3. Select a product.
4. Review its description, technology, and valuation fields.

## Driving it with ndev browser

1. Start the Astro site on its cataloged localhost URL.
2. Open it in a managed browser session named `nicos-catalog-web-verify`.
3. Exercise one filter and one product detail.
4. Confirm that the synthetic-data disclosure is visible.
5. Repeat the landing and filter checks against the public Worker.
6. Save privacy-safe evidence in `docs/verify/web/evidence/`.

## Gotchas

- Synthetic proof does not establish visitors, adoption, or revenue.
- Do not add third-party telemetry to this demonstration surface.
