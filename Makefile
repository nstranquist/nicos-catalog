.PHONY: verify demo install release-check cover fuzz bench

# Coverage floors ratchet upward only. Raising them is a normal change; lowering
# them is a decision that should be argued for in review.
COVER_FLOOR_ROOT ?= 90.0
COVER_FLOOR_CMD  ?= 80.0

# The published version lives in exactly one place.
VERSION := $(shell cat VERSION)

verify: cover
	@test -z "$$(gofmt -l $$(find . -name '*.go' -not -path './.git/*'))" || (gofmt -l $$(find . -name '*.go' -not -path './.git/*'); exit 1)
	go test ./...
	go test -race ./...
	go vet ./...
	go build -trimpath -o /dev/null ./cmd/nicos-catalog

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
