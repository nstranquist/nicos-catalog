# Explorer performance

Nicos Catalog uses two performance controls:

- Fixed bundle, response, graph-node, and graph-edge limits prevent unbounded
  browser work.
- A hardware-specific latency baseline detects a material service regression.

Run the latency gate from the repository root:

```sh
make perf
```

The command measures service load, list, search, and aggregate graph requests
with 500, 4,000, and 10,000 synthetic entities. It runs three rounds of 31
samples. The result is the median of the three round-level p50 and p95 values.
The command fails when the p95 value is higher than the matching recorded
budget.

The first baseline uses an Apple M1 Pro with a 10-core CPU, Go 1.26.6, and
macOS on arm64. The accepted values came from this command with 51 samples:

```sh
NICOS_CATALOG_PERF=1 NICOS_CATALOG_PERF_SAMPLES=51 \
  go test -run '^TestExplorerPerformanceRatchet$' -count=1 -v \
  ./internal/explorerapi
```

| Operation | Entities | p50 | p95 | p95 budget |
|---|---:|---:|---:|---:|
| load | 500 | 2.47 ms | 4.92 ms | 15 ms |
| list | 500 | 0.02 ms | 0.20 ms | 1 ms |
| search | 500 | 0.17 ms | 0.77 ms | 3 ms |
| graph | 500 | 0.06 ms | 0.25 ms | 1 ms |
| load | 4,000 | 20.24 ms | 56.66 ms | 170 ms |
| list | 4,000 | 0.25 ms | 2.44 ms | 8 ms |
| search | 4,000 | 2.74 ms | 5.42 ms | 17 ms |
| graph | 4,000 | 0.58 ms | 1.94 ms | 6 ms |
| load | 10,000 | 52.16 ms | 132.06 ms | 400 ms |
| list | 10,000 | 0.42 ms | 1.38 ms | 8 ms |
| search | 10,000 | 5.51 ms | 7.38 ms | 40 ms |
| graph | 10,000 | 1.37 ms | 3.03 ms | 10 ms |

The baseline file is in
`internal/explorerapi/testdata/performance/darwin-arm64-10cpu.json`. A machine
without a matching baseline can emit measurements with
`NICOS_CATALOG_PERF=1`, but the release gate fails until the repository has a
reviewed baseline for that hardware class. Do not compare values from different
hardware classes as if they were equivalent.

The normal p95 budget is at least three times the accepted p95. The 10,000
entity list and search budgets are wider because repeated release-gate runs
showed garbage-collection and workstation-scheduler variance. A later accepted
baseline can lower a budget. It must not raise a budget without a recorded
performance review.
