# Contributing

This public repository is the source authority for the portable engine, CLI,
Explorer, documentation, fixtures, and releases. Submit changes here. Do not
sync product source from a private mirror.

Run the complete local gate before proposing a change:

```sh
make verify
make docs-site
```

## Explorer changes

Install the pinned dependency graph:

```sh
make explorer-install
```

Run the loopback development server:

```sh
corepack pnpm@11.13.0 --dir explorer dev
```

Run the UI and embedded-asset gates:

```sh
make explorer-check
make verify-explorer-embed
make perf
```

The Explorer must not use a `file:` or `link:` dependency outside this
repository. Use system fonts. Do not add a third-party network request.

Go owns the transport contract. Do not edit generated contract files directly.
Regenerate them after a Go contract change:

```sh
go generate ./internal/explorercontract
make verify-explorer-contract
```

Commit `explorer/dist` with its source changes. `go install` embeds these files
and never invokes Node.

Add direct tests for URL state, selection, filters, pagination, error states,
graph limits, and keyboard behavior. Explorer logic must keep at least 80
percent branch coverage.

`make perf` applies a latency ratchet only to a matching hardware class. Add a
reviewed baseline before you use a new hardware class as a release gate. Do not
raise a latency budget without a performance review.

Keep the core host-independent. New fields require a concrete cross-host need;
private telemetry, valuation, credentials, and publication policy belong in
host adapters. Add tests for determinism, drift behavior, and public projection
whenever those boundaries change.

## Release ownership

This repository is the source authority. Do not copy a change from a private
upstream or send a change back to one.

Only an operator pushes release commits, creates a tag, creates a GitHub
Release, or deploys a static site. A local green gate means publish-ready. It
does not mean released or deployed.

Use [the release runbook](docs/releasing.md). Keep release notes free of
temporary status text. Record live publication evidence in a separate, dated
review.
