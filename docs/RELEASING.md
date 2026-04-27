# Releasing darken

Operator runbook for cutting a `darken` release.

## Prerequisites (one-time)

1. **GitHub Personal Access Token** with `repo` scope:
   - Visit https://github.com/settings/tokens (Tokens (classic))
   - Generate new token (classic) — name it "darken release homebrew-tap publish"
   - Scopes: `repo` (full)
   - Copy the token

2. **Add as repo secret**:

   ```bash
   echo "<paste-token>" | gh secret set HOMEBREW_TAP_GITHUB_TOKEN \
     --repo danmestas/darken
   ```

3. **Verify the homebrew tap repo exists**:

   ```bash
   gh repo view danmestas/homebrew-tap --json name
   ```

## Cutting a release

```bash
# From main, latest commit you want to ship
git checkout main
git pull --ff-only origin main

# Local sanity (optional but recommended)
goreleaser release --snapshot --clean
ls dist/  # should show 4 archives + checksums.txt

# Tag + push
VERSION=v0.1.0
git tag -a "${VERSION}" -m "darken ${VERSION}"
git push origin "${VERSION}"

# Watch the workflow
gh run watch
```

The release workflow runs goreleaser, which:

- Runs `scripts/test-embed-drift.sh` and `go test ./...` as a pre-build gate
- Cross-compiles 4 archives (darwin/linux × amd64/arm64)
- Creates a GitHub Release with the archives + a generated changelog
- Pushes `Formula/darken.rb` to `danmestas/homebrew-tap`

## Verifying the release

```bash
# go install
go install github.com/danmestas/darken/cmd/darken@v0.1.0
darken version  # should print v0.1.0 + substrate hash

# brew install
brew tap danmestas/tap
brew install danmestas/tap/darken
darken version

# direct download (e.g. for CI runners)
curl -L https://github.com/danmestas/darken/releases/download/v0.1.0/darken_0.1.0_darwin_arm64.tar.gz \
  -o darken.tar.gz
tar -xzf darken.tar.gz
./darken version
```

## Yanking a release (if something's broken)

```bash
gh release delete v0.1.0 --repo danmestas/darken --yes
git push --delete origin v0.1.0
git tag -d v0.1.0

# Manually delete the formula from the tap
gh repo clone danmestas/homebrew-tap /tmp/tap
cd /tmp/tap
git rm Formula/darken.rb
git commit -m "yank darken v0.1.0"
git push origin main
cd -
rm -rf /tmp/tap
```

## Versioning

- Pre-1.0: breaking changes possible at any minor bump. Currently `v0.1.x`.
- Tag conventions: `v<major>.<minor>.<patch>`. Pre-releases use `v0.1.0-rc1` etc.; goreleaser marks those as pre-releases automatically.
- Substrate hash (`darken version` output) changes whenever embedded templates/scripts/skills change. Operators should re-run `make sync-embed-data` and commit the resulting `internal/substrate/data/` diff before tagging.

## Troubleshooting

**`HOMEBREW_TAP_GITHUB_TOKEN` not set**: workflow logs will show `Error: ...token`. Re-add via `gh secret set`.

**Drift guard failed**: pre-build hook `scripts/test-embed-drift.sh` exits non-zero if `internal/substrate/data/` is stale. Run `make sync-embed-data`, commit, re-tag.

**Tap push fails**: confirm the PAT has `repo` scope (not `public_repo` — `repo` is required for branch creation in some cases). Confirm `danmestas/homebrew-tap` exists.
