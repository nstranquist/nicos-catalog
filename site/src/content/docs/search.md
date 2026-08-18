---
title: Search
description: BM25 full-text search over the deterministic Nicos Catalog index.
---

`Engine.Search` runs BM25 over the derived index. It does not read provider output or the corpus. If the index is missing, run [reindex](/cli/#reindex) first.

```go
results, err := engine.Search(ctx, "ownership graph", catalog.SearchOptions{
    Limit: 5,
    Kinds: []string{"service", "system"},
})
```

```sh
nicos-catalog --root . --corpus demo/catalog search --limit 3 "ownership graph"
nicos-catalog --root . --corpus demo/catalog search --kinds service,system "ownership"
```

## Options

| Field | Default | Meaning |
| --- | --- | --- |
| `Limit` | 10 | Maximum results. The engine can cap this through `WithLimits`. |
| `Kinds` | all | Case-insensitive kind filter. |

A query with no usable tokens returns `ErrEmptyQuery`. Tokens are lowercase `[a-z0-9][a-z0-9._+-]*` runs.

## Scores and rank

A higher score is a better match **in this result set**. Scores are not comparable across queries, hosts, or engine versions.

Depend on **rank order** and `MatchedTerms`. Do not persist scores as a contract.

The CLI prints rank order. `--json` includes `score` and `matched_terms` for inspection only.

## What is searched

The BM25 document model is built at [reindex](/cli/#reindex) from portable entity text (id, name, kind, description, tags, and related published fields). Host-only sidecar state is not in the document.

Search reads only the index. After you change the corpus, run `reindex` or [reconcile](/drift/) before you search.
