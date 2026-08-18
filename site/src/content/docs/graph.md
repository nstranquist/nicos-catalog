---
title: Graph
description: Compile the typed entity relationship graph as Mermaid or JSON.
---

`BuildGraph` compiles the typed relationship graph from a loaded index. Nodes follow entity order. Edges sort by source, kind, then target so the bytes stay stable.

```go
index, err := engine.LoadIndex(ctx)
graph := catalog.BuildGraph(index)
fmt.Print(graph.Mermaid())
```

```sh
nicos-catalog --root . --corpus demo/catalog graph
nicos-catalog --root . --corpus demo/catalog graph --format json
nicos-catalog --root . --corpus demo/catalog --json graph
```

Default CLI output is Mermaid (`graph LR`). `--format json` or `--json` emits the typed `Graph`.

## Shape

Each **node** is one entity: `id`, `name`, `kind`, and optional `status`.

Each **edge** is one `Ref`:

| Field | Meaning |
| --- | --- |
| `source` | Entity id that declared the ref |
| `kind` | Relationship kind from the authored `Ref` |
| `target` | Target id. It need not exist as a node |

A dangling edge is valid. The host decides how to render a missing target.

## Mermaid

`Graph.Mermaid` writes a flowchart. Names and kinds are escaped so a quote or newline cannot break the diagram.

Use the Mermaid in a README or docs page. Use JSON when a host will lay out the graph itself.

The graph is derived state. After you change refs, run [reindex](/cli/#reindex) or [reconcile](/drift/) before you compile again.
