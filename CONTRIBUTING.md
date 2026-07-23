# Contributing

Run the complete local gate before proposing a change:

```sh
go test ./...
go test -race ./...
go vet ./...
go build -trimpath ./cmd/nicos-catalog
go run ./cmd/nicos-catalog --json demo
```

Keep the core host-independent. New fields require a concrete cross-host need;
private telemetry, valuation, credentials, and publication policy belong in
host adapters. Add tests for determinism, drift behavior, and public projection
whenever those boundaries change.
