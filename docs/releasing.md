# Release Nicos Catalog

This procedure publishes a stable SemVer release. Only an authorized operator
can push commits or tags and create or edit a GitHub Release.

## Keep two records

Use two different documents:

- `docs/releases/vX.Y.Z.md` contains timeless release notes. It describes the
  product and measured build evidence.
- `docs/releases/vX.Y.Z-publication-review.md` is dated. It records mutable
  external evidence, such as CI runs, proxy resolution, and residual risks.

Do not put pending steps or current status in the release notes. The
publication test rejects common temporary phrases.

## Prepare the version

1. Select the next SemVer version.
2. Update `VERSION`, the Explorer package version, the changelog, and the
   release notes.
3. Run the complete local gates:

```sh
make verify-publication
make fuzz
make perf
golangci-lint run ./...
go mod tidy -diff
go mod verify
govulncheck ./...
```

4. Review the exact diff. Commit it through the guarded commit path.
5. Get explicit authority before a push, tag, or GitHub Release mutation.

## Publish

1. Push the reviewed release commit.
2. Wait for public CI to pass on that commit.
3. Create a signed annotated tag and verify it locally:

```sh
git tag -s vX.Y.Z -m "Nicos Catalog vX.Y.Z"
git verify-tag vX.Y.Z
```

4. After the operator gives explicit approval, push the tag.
5. Wait for tag CI to pass.
6. Create the GitHub Release from the timeless notes:

```sh
gh release create vX.Y.Z \
  --repo nstranquist/nicos-catalog \
  --title "Nicos Catalog vX.Y.Z" \
  --notes-file docs/releases/vX.Y.Z.md \
  --verify-tag
```

7. Read the GitHub Release. Confirm that its title, tag, target, and body match
   the reviewed files.

Never move or replace a public tag. If published content needs a code or
documentation fix, publish a new patch version.

## Verify public distribution

Use new Go caches. Do not let the developer checkout or an existing module
cache satisfy the proof.

```sh
NICOS_RELEASE_TMP="$(mktemp -d)"
GOMODCACHE="$NICOS_RELEASE_TMP/gomod" \
GOCACHE="$NICOS_RELEASE_TMP/gocache" \
GOPATH="$NICOS_RELEASE_TMP/gopath" \
GOBIN="$NICOS_RELEASE_TMP/bin" \
GOPROXY=https://proxy.golang.org \
GOSUMDB=sum.golang.org \
go install github.com/nstranquist/nicos-catalog/cmd/nicos-catalog@vX.Y.Z
"$NICOS_RELEASE_TMP/bin/nicos-catalog" version --expect vX.Y.Z
```

Also run `go list -m -json` with the same proxy settings. Record the resolved
version, origin hash, CI run, Release URL, tag signature result, and install
result in the dated publication review.

Public distribution does not prove deployment, announcement, adoption, active
use, or revenue. Record a later state only with direct evidence.
