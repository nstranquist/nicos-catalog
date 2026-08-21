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
3. Run the complete local gates in the listed order. Run `make perf` when no
   fuzz, build, browser, or other CPU-heavy job is active. A contended latency
   sample is invalid. Do not change the baseline to accept it. Wait for the
   load to end, and run `make perf` again.

```sh
make verify-publication
make fuzz
make perf
go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.1.6 run ./...
go mod tidy -diff
go mod verify
go run golang.org/x/vuln/cmd/govulncheck@v1.1.4 ./...
```

4. Review the exact diff. Commit it through the guarded commit path.
5. Get explicit authority before a push, tag, or GitHub Release mutation.

## Publish

1. Push the reviewed release commit.
2. Wait for public CI to pass on that commit.
3. Configure the maintainer SSH signing key. The public key must match one
   entry in `.github/allowed_signers`:

```sh
git config --local gpg.format ssh
git config --local user.signingkey /absolute/path/to/signing-key.pub
git config --local gpg.ssh.allowedSignersFile .github/allowed_signers
```

4. Create a signed annotated tag and verify it locally:

```sh
git tag -s vX.Y.Z -m "Nicos Catalog vX.Y.Z"
git verify-tag vX.Y.Z
```

5. Confirm that the same public key is registered as an SSH signing key for
   the GitHub account. Local verification and GitHub's `Verified` label are
   separate checks.
6. After the operator gives explicit approval, push the tag.
7. Wait for tag CI to pass.
8. Create the GitHub Release from the timeless notes:

```sh
gh release create vX.Y.Z \
  --repo nstranquist/nicos-catalog \
  --title "Nicos Catalog vX.Y.Z" \
  --notes-file docs/releases/vX.Y.Z.md \
  --verify-tag
```

9. Read the GitHub Release. Confirm that its title, tag, target, and body match
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

For a deployed static Explorer, verify the Content Security Policy,
`X-Frame-Options: DENY`, and `X-Content-Type-Options: nosniff` on the HTML
response. Compare every served artifact with the release-bound export.
Cloudflare Pages consumes `_headers`; do not expect that control file to be a
served artifact.

Public distribution does not prove deployment, announcement, adoption, active
use, or revenue. Record a later state only with direct evidence.
