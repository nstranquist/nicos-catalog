.PHONY: verify demo install release-check cover fuzz bench repro

# Coverage floors ratchet upward only. Raising them is a normal change; lowering
# them is a decision that should be argued for in review.
COVER_FLOOR_ROOT ?= 90.0
COVER_FLOOR_CMD  ?= 80.0

# The published version lives in exactly one place.
VERSION := $(shell cat VERSION)

# Flags shared with the CI "reproducible" job so local and GHA exercise the same
# dual-build contract. Empty -buildid= zeros the linker's non-deterministic ID;
# -buildvcs=false keeps VCS stamps out of the binary identity under test.
REPRO_FLAGS := -trimpath -buildvcs=false -ldflags=-buildid=

verify: cover
	@test -z "$$(gofmt -l $$(find . -name '*.go' -not -path './.git/*'))" || (gofmt -l $$(find . -name '*.go' -not -path './.git/*'); exit 1)
	go test ./...
	go test -race ./...
	go vet ./...
	go build $(REPRO_FLAGS) -o /dev/null ./cmd/nicos-catalog
	$(MAKE) repro

# Two profiles, because `go tool cover -func` totals per profile and the library
# and its CLI carry different floors.
cover:
	go test -covermode=atomic -coverprofile=cover-root.out .
	go test -covermode=atomic -coverprofile=cover-cmd.out ./cmd/...
	@go tool cover -func=cover-root.out | awk -v f=$(COVER_FLOOR_ROOT) \
	  '/^total:/{gsub(/%/,"",$$3); if ($$3+0 < f) {printf "root coverage %.1f%% is below the %.1f%% floor\n",$$3,f; exit 1} \
	   else {printf "root coverage %.1f%% (floor %.1f%%)\n",$$3,f}}'
	@go tool cover -func=cover-cmd.out | awk -v f=$(COVER_FLOOR_CMD) \
	  '/^total:/{gsub(/%/,"",$$3); if ($$3+0 < f) {printf "cmd coverage %.1f%% is below the %.1f%% floor\n",$$3,f; exit 1} \
	   else {printf "cmd coverage %.1f%% (floor %.1f%%)\n",$$3,f}}'

# A short smoke over every fuzz target. The nightly workflow runs these longer.
fuzz:
	@for target in $$(go test -list 'Fuzz.*' ./... | grep '^Fuzz'); do \
	  echo "fuzzing $$target"; \
	  go test -run '^$$' -fuzz "^$$target$$" -fuzztime=30s . || exit 1; \
	done

# Benchmarks are a crash and allocation smoke, never a timing gate: shared CI
# runners are too noisy for a p95 assertion to mean anything.
bench:
	go test -run '^$$' -bench=. -benchtime=10x -benchmem ./...

demo:
	go run ./cmd/nicos-catalog --json demo

install:
	go install ./cmd/nicos-catalog

release-check: verify
	go run ./cmd/nicos-catalog --json version --expect $(VERSION)

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
