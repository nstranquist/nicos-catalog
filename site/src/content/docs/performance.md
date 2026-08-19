---
title: Performance
description: Measure Explorer latency and apply the hardware-specific p95 ratchet.
---

Explorer has fixed limits for bundle bytes, response bytes, graph nodes, and
graph edges. These limits prevent unbounded browser work.

Run the latency gate from a matching hardware class:

```sh
make perf
```

The command measures load, list, search, and aggregate graph requests. It uses
500, 4,000, and 10,000 synthetic entities. The command records p50 and p95
latency and fails when p95 is higher than the reviewed budget.

Each baseline names its hardware, Go version, sample count, accepted values,
and budgets. A machine without a matching baseline can record measurements. It
cannot pass the release latency gate until the repository has a reviewed
baseline for that hardware class.

See the complete [performance contract](https://github.com/nstranquist/nicos-catalog/blob/main/docs/performance.md).
