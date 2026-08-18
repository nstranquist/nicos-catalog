# Contributing

This public repository is the source authority for the portable engine, CLI,
Explorer, documentation, fixtures, and releases. Submit changes here. Do not
sync product source from a private mirror.

Run the complete local gate before proposing a change:

```sh
go test ./...
go test -race ./...
go vet ./...
go build -trimpath ./cmd/nicos-catalog
go run ./cmd/nicos-catalog --json demo
corepack pnpm@11.13.0 --dir site install --frozen-lockfile
corepack pnpm@11.13.0 --dir site build
```

Keep the core host-independent. New fields require a concrete cross-host need;
private telemetry, valuation, credentials, and publication policy belong in
host adapters. Add tests for determinism, drift behavior, and public projection
whenever those boundaries change.
