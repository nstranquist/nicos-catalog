.PHONY: verify demo install release-check

verify:
	@test -z "$$(gofmt -l $$(find . -name '*.go' -not -path './.git/*'))" || (gofmt -l $$(find . -name '*.go' -not -path './.git/*'); exit 1)
	go test ./...
	go test -race ./...
	go vet ./...
	go build -trimpath -o /dev/null ./cmd/nicos-catalog

demo:
	go run ./cmd/nicos-catalog --json demo

install:
	go install ./cmd/nicos-catalog

release-check: verify
	go run ./cmd/nicos-catalog --json version --expect v0.1.1
