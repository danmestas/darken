# Darken Phase 3 — Release Pipeline (goreleaser + tap + tag)

> **For agentic workers:** Use superpowers:subagent-driven-development to implement this plan.

**Goal:** `brew install danmestas/tap/darken` puts a working `darken` binary on a fresh Mac. Same binary works via `go install github.com/danmestas/darken/cmd/darken@latest` and direct download from GitHub Releases. First release is `v0.1.0`.

**Tech:** [goreleaser](https://goreleaser.com/) for cross-platform builds + GitHub Releases + homebrew tap publishing. GitHub Actions workflow on tag push. No code changes to `darken` itself — Phase 1 + 2 already produced a release-ready binary.

---

## File structure

### Created
- `.goreleaser.yaml` — goreleaser config: build matrix, archive naming, brew tap publishing
- `.github/workflows/release.yaml` — Actions workflow triggered on `v*` tag push
- `docs/RELEASING.md` — operator runbook for cutting a release (tag commands, secret setup, smoke test)

### New repo (created via `gh repo create`)
- `danmestas/homebrew-tap` — public repo where goreleaser pushes Formula files

### NOT modified
- `cmd/darken/`, `internal/substrate/` — Phase 1+2 already delivered a release-ready binary
- `Makefile` — `make darken` continues to work for source-tree dev; release builds use goreleaser instead
- `README.md` — documentation update lands in Phase 4

---

## Tasks

### Task 1: `.goreleaser.yaml` config

Builds darwin (amd64 + arm64) + linux (amd64 + arm64). Injects version via `-X main.version=`. Uploads archives to GitHub Releases. Pushes a homebrew Formula to `danmestas/homebrew-tap`.

- [ ] **Step 1: Create `.goreleaser.yaml`**

```yaml
# .goreleaser.yaml — darkish-factory release config.
# See https://goreleaser.com/customization/ for the full schema.
version: 2

project_name: darken

before:
  hooks:
    # Sanity: ensure the embedded substrate is in sync before building.
    - bash scripts/test-embed-drift.sh
    - go test ./...

builds:
  - id: darken
    main: ./cmd/darken
    binary: darken
    env:
      - CGO_ENABLED=0
    goos:
      - darwin
      - linux
    goarch:
      - amd64
      - arm64
    ldflags:
      - -s -w -X main.version={{ .Version }}
    flags:
      - -trimpath

archives:
  - id: darken-archive
    formats: [tar.gz]
    name_template: "darken_{{ .Version }}_{{ .Os }}_{{ .Arch }}"
    files:
      - LICENSE*
      - README.md

checksum:
  name_template: "checksums.txt"

snapshot:
  version_template: "{{ incpatch .Version }}-snapshot-{{ .ShortCommit }}"

changelog:
  sort: asc
  filters:
    exclude:
      - "^docs:"
      - "^chore:"
      - "Merge pull request"

release:
  github:
    owner: danmestas
    name: darkish-factory
  draft: false
  prerelease: auto
  name_template: "darken {{ .Version }}"
  header: |
    See [docs/RELEASING.md](https://github.com/danmestas/darken/blob/main/docs/RELEASING.md) for upgrade notes.

brews:
  - name: darken
    repository:
      owner: danmestas
      name: homebrew-tap
      branch: main
      token: "{{ .Env.HOMEBREW_TAP_GITHUB_TOKEN }}"
    homepage: "https://github.com/danmestas/darken"
    description: "darken — Darkish Factory orchestration substrate CLI"
    license: "MIT"
    install: |
      bin.install "darken"
    test: |
      system "#{bin}/darken version"
```

(LICENSE: this repo doesn't currently have one. The Formula references it; if there's no LICENSE file at release time, goreleaser silently skips it. Fine for now; file a follow-up to add LICENSE if you want strict compliance.)

- [ ] **Step 2: Smoke-test locally (snapshot mode, no actual release)**

Install goreleaser if not present:
```bash
brew install goreleaser
```

Run a snapshot build:
```bash
goreleaser release --snapshot --clean
```

Expected: `dist/` populated with archives for darwin-amd64, darwin-arm64, linux-amd64, linux-arm64. Each archive contains `darken` binary.

```bash
ls dist/
tar -tzf dist/darken_*_darwin_arm64.tar.gz | head
```

- [ ] **Step 3: Commit**

```
feat(release): add goreleaser config

Builds darken for darwin+linux × amd64+arm64. Injects version via
-ldflags="-X main.version=...". Publishes to GitHub Releases on tag
push and pushes a homebrew Formula to danmestas/homebrew-tap.

Pre-build hooks: scripts/test-embed-drift.sh + go test ./... — drift
or test failures block the release.

Snapshot mode (`goreleaser release --snapshot --clean`) verified
locally; produces dist/ with all 4 archives.
```

---

### Task 2: GitHub Actions release workflow

Triggers on tag push matching `v*.*.*`. Runs goreleaser. Uses `HOMEBREW_TAP_GITHUB_TOKEN` secret (operator-managed PAT).

- [ ] **Step 1: Create `.github/workflows/release.yaml`**

```yaml
name: release

on:
  push:
    tags:
      - "v*.*.*"

permissions:
  contents: write

jobs:
  release:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0

      - uses: actions/setup-go@v5
        with:
          go-version: "1.23"

      - uses: goreleaser/goreleaser-action@v6
        with:
          version: "~> v2"
          args: release --clean
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
          HOMEBREW_TAP_GITHUB_TOKEN: ${{ secrets.HOMEBREW_TAP_GITHUB_TOKEN }}
```

- [ ] **Step 2: Commit**

```
feat(release): GitHub Actions workflow on v*.*.* tag

Runs goreleaser on tag push. Uses GITHUB_TOKEN for the release itself
+ HOMEBREW_TAP_GITHUB_TOKEN (operator PAT, scoped to repo) for
publishing the Formula to danmestas/homebrew-tap.

Operator setup (one-time):
  1. Create a Classic PAT with `repo` scope on github.com
  2. Add as repo secret HOMEBREW_TAP_GITHUB_TOKEN
  3. Tag v0.1.0 (`git tag -a v0.1.0 -m "first release"; git push origin v0.1.0`)
```

---

### Task 3: Create `danmestas/homebrew-tap` repo

- [ ] **Step 1: Create the repo**

```bash
gh repo create danmestas/homebrew-tap --public \
  --description "Homebrew tap for danmestas binaries (currently: darken)"
```

- [ ] **Step 2: Seed with a README**

```bash
TAP_TMP=$(mktemp -d)
cd "${TAP_TMP}"
gh repo clone danmestas/homebrew-tap .
cat > README.md <<'EOF'
# danmestas/homebrew-tap

Homebrew tap for [danmestas](https://github.com/danmestas) binaries.

## Install

```bash
brew install danmestas/tap/darken
```

## Formulae

- `darken` — Darkish Factory orchestration substrate CLI ([source](https://github.com/danmestas/darken))

The `Formula/darken.rb` file is auto-generated by goreleaser on each release of `darkish-factory`. Don't edit it by hand.
EOF
git add README.md
git commit -m "docs: seed tap README"
git push origin main
cd -
rm -rf "${TAP_TMP}"
```

- [ ] **Step 3: No commit on the darkish-factory side for this task**

The tap repo is external; this task creates remote infrastructure only.

---

### Task 4: `docs/RELEASING.md` runbook

- [ ] **Step 1: Write the runbook**

```markdown
# Releasing darken

## Prerequisites (one-time)

1. **GitHub Personal Access Token** with `repo` scope:
   - Visit https://github.com/settings/tokens (Tokens (classic))
   - Generate new token (classic) — name it "darken release homebrew-tap publish"
   - Scopes: `repo` (full)
   - Copy the token

2. **Add as repo secret**:
   ```bash
   echo "<paste-token>" | gh secret set HOMEBREW_TAP_GITHUB_TOKEN \
     --repo danmestas/darkish-factory
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
darken version  # should print v0.1.0

# brew install (might need brew update first)
brew tap danmestas/tap
brew install danmestas/tap/darken
darken version
```

## Yanking a release (if something's broken)

```bash
gh release delete v0.1.0 --repo danmestas/darkish-factory --yes
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
```

- [ ] **Step 2: Commit**

```
docs: add RELEASING.md runbook

Operator runbook for cutting darken releases: PAT setup, tag commands,
verification steps (go install + brew install), yank procedure,
versioning conventions.
```

---

### Task 5: Final verification + push + PR

- [ ] **Step 1: Local snapshot test**

```bash
goreleaser release --snapshot --clean
ls dist/
tar -tzf dist/darken_*_darwin_arm64.tar.gz | grep darken
```

- [ ] **Step 2: Workflow YAML lint**

```bash
gh workflow view release --repo danmestas/darkish-factory 2>&1 | head -3 || echo "(workflow not yet pushed; that's fine)"
# After push, this confirms the workflow registered:
# gh workflow list --repo danmestas/darkish-factory
```

- [ ] **Step 3: Push branch + open PR**

```bash
git push -u origin feat/darken-phase-3
gh pr create --title "Phase 3: release pipeline (goreleaser + GitHub Actions + homebrew tap)" --body "..."
```

PR body should call out the operator-action items (PAT creation, secret addition, first tag).

---

## Done definition

Phase 3 ships when:

1. `.goreleaser.yaml` exists, passes `goreleaser release --snapshot --clean` locally
2. `.github/workflows/release.yaml` exists, lints clean
3. `danmestas/homebrew-tap` repo exists, has a README explaining the tap
4. `docs/RELEASING.md` documents the operator runbook
5. PR open, CI green, ready for review
6. After merge: operator creates PAT, adds secret, tags `v0.1.0` → workflow runs → release published → `brew install danmestas/tap/darken` works

## What Phase 4 picks up

- darkish-factory itself consumes the released `darken` binary (delete the source-tree symlink)
- README updated with install instructions + new-repo workflow
- Migration note for existing operators
