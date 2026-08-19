.PHONY: verify demo install release-check verify-publication browser-smoke cover fuzz bench perf docs-site repro verify-explorer-contract explorer-install explorer-check verify-explorer-embed

# Coverage floors ratchet upward only. Raising them is a normal change; lowering
# them is a decision that should be argued for in review.
COVER_FLOOR_ROOT ?= 90.0
COVER_FLOOR_CMD  ?= 80.0
COVER_FLOOR_HOST ?= 85.0
COVER_FLOOR_EXPLORER ?= 85.0
FUZZ_TIME ?= 30s
FUZZ_PARALLEL ?= 2

EXPLORER_GO_PACKAGES := \
	./internal/explorerapi \
	./internal/explorerbundle \
	./internal/explorercontract \
	./internal/explorerinit \
	./internal/explorermcp \
	./internal/exploreropen \
	./internal/explorerweb

# The published version lives in exactly one place.
VERSION := $(shell cat VERSION)
EXPLORER_BASE ?= http://127.0.0.1:7797

# Flags shared with the CI "reproducible" job so local and GHA exercise the same
# dual-build contract. Empty -buildid= zeros the linker's non-deterministic ID;
# -buildvcs=false keeps VCS stamps out of the binary identity under test.
REPRO_FLAGS := -trimpath -buildvcs=false -ldflags=-buildid=

verify: cover explorer-check verify-explorer-embed
	@test -z "$$(gofmt -l $$(find . -name '*.go' -not -path './.git/*'))" || (gofmt -l $$(find . -name '*.go' -not -path './.git/*'); exit 1)
	go test ./...
	go test -race ./...
	go vet ./...
	go build $(REPRO_FLAGS) -o /dev/null ./cmd/nicos-catalog
	$(MAKE) verify-explorer-contract
	$(MAKE) repro

# Three profiles: library, CLI, and host-only collation adapter.
cover:
	go test -covermode=atomic -coverprofile=cover-root.out .
	go test -covermode=atomic -coverprofile=cover-cmd.out ./cmd/...
	go test -covermode=atomic -coverprofile=cover-host.out ./internal/hostcollate
	@go tool cover -func=cover-root.out | awk -v f=$(COVER_FLOOR_ROOT) \
	  '/^total:/{gsub(/%/,"",$$3); if ($$3+0 < f) {printf "root coverage %.1f%% is below the %.1f%% floor\n",$$3,f; exit 1} \
	   else {printf "root coverage %.1f%% (floor %.1f%%)\n",$$3,f}}'
	@go tool cover -func=cover-cmd.out | awk -v f=$(COVER_FLOOR_CMD) \
	  '/^total:/{gsub(/%/,"",$$3); if ($$3+0 < f) {printf "cmd coverage %.1f%% is below the %.1f%% floor\n",$$3,f; exit 1} \
	   else {printf "cmd coverage %.1f%% (floor %.1f%%)\n",$$3,f}}'
	@go tool cover -func=cover-host.out | awk -v f=$(COVER_FLOOR_HOST) \
	  '/^total:/{gsub(/%/,"",$$3); if ($$3+0 < f) {printf "hostcollate coverage %.1f%% is below the %.1f%% floor\n",$$3,f; exit 1} \
	   else {printf "hostcollate coverage %.1f%% (floor %.1f%%)\n",$$3,f}}'
	@set -e; for pkg in $(EXPLORER_GO_PACKAGES); do \
		name=$${pkg##*/}; profile="cover-$$name.out"; \
		go test -covermode=atomic -coverprofile="$$profile" "$$pkg"; \
		go tool cover -func="$$profile" | awk -v f=$(COVER_FLOOR_EXPLORER) -v n="$$name" \
		  '/^total:/{gsub(/%/,"",$$3); if ($$3+0 < f) {printf "%s coverage %.1f%% is below the %.1f%% floor\n",n,$$3,f; exit 1} \
		   else {printf "%s coverage %.1f%% (floor %.1f%%)\n",n,$$3,f}}'; \
	done

# A short smoke over every fuzz target. Keep worker concurrency low so this
# gate remains stable on shared developer hosts and small CI runners.
fuzz:
	@for target in $$(go test -list 'Fuzz.*' ./... | grep '^Fuzz'); do \
	  echo "fuzzing $$target"; \
	  go test -run '^$$' -fuzz "^$$target$$" -fuzztime=$(FUZZ_TIME) -parallel=$(FUZZ_PARALLEL) . || exit 1; \
	done

# Benchmarks are a crash and allocation smoke. The separate perf target applies
# a p95 ratchet only when this repository contains a matching hardware baseline.
bench:
	go test -run '^$$' -bench=. -benchtime=10x -benchmem ./...

perf:
	NICOS_CATALOG_PERF=1 NICOS_CATALOG_PERF_REQUIRED=1 go test \
	  -run '^TestExplorerPerformanceRatchet$$' -count=1 -v ./internal/explorerapi

demo:
	go run ./cmd/nicos-catalog --json demo

docs-site:
	corepack pnpm@11.13.0 --dir site install --frozen-lockfile
	corepack pnpm@11.13.0 --dir site build

verify-explorer-contract:
	go run ./cmd/explorer-contract-gen --check

explorer-install:
	corepack pnpm@11.13.0 --dir explorer install --frozen-lockfile

explorer-check: explorer-install
	corepack pnpm@11.13.0 --dir explorer check

verify-explorer-embed: explorer-install
	node explorer/scripts/verify-embed.mjs

install:
	go install ./cmd/nicos-catalog

release-check: verify
	go run ./cmd/nicos-catalog --json version --expect $(VERSION)

# This is the local release-candidate gate. It does not push, tag, or deploy.
verify-publication: release-check docs-site
	go test -run '^TestPublication' .

# Start either the live Explorer or a static preview at EXPLORER_BASE first.
# ndev keeps the browser transcript and the per-step verdicts as local proof.
browser-smoke:
	ndev browser script validate smoke/explorer.yaml
	ndev browser script run smoke/explorer.yaml --session nicos-catalog-explorer-smoke --open --allow-drive --var base=$(EXPLORER_BASE) --json

# Dual-build bit-identity across two isolated GOCACHE trees, matching
# .github/workflows/ci.yml "reproducible". Never touches the developer's
# primary cache. Requires CGO_ENABLED=0 (pure Go).
repro:
	@set -e; \
	d=$$(mktemp -d); \
	export CGO_ENABLED=0; \
	GOCACHE=$$d/c1 go build $(REPRO_FLAGS) -o "$$d/first" ./cmd/nicos-catalog; \
	GOCACHE=$$d/c2 go build $(REPRO_FLAGS) -o "$$d/second" ./cmd/nicos-catalog; \
	if command -v sha256sum >/dev/null 2>&1; then sha256sum "$$d/first" "$$d/second"; else shasum -a 256 "$$d/first" "$$d/second"; fi; \
	cmp "$$d/first" "$$d/second"; \
	rm -rf "$$d"; \
	echo "repro: dual-build bit-identical"
