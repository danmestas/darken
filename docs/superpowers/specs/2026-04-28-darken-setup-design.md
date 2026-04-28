# `darken setup` — design

**Date:** 2026-04-28
**Status:** Approved (brainstorming)
**Versioning target:** `v0.1.13`

## Goal

Collapse the fresh-repo onboarding sequence (`darken init` + `darken bootstrap`) into a single command. After `darken setup` ships, a new operator on a new project runs **one command** to get scaffolds, machine prereqs, hub secrets, and per-harness skill staging done.

## Non-goals

- Replacing or deprecating `darken init` / `darken bootstrap` / `darken creds`. Those remain for targeted ops (e.g. `darken creds` after credential rotation, `darken init --refresh --force` for explicit scaffold regeneration).
- Covering the existing-repo refresh path. That's `darken upgrade-init` (post-`brew upgrade darken`).
- Smart-skip via doctor pre-check. Underlying commands already short-circuit on healthy state — no extra wiring needed.
- Setup flags beyond `--force`. `--dry-run` and `--refresh` have better-named homes (init standalone, upgrade-init).

## Architecture

`darken setup` is a single new subcommand in `cmd/darken/setup.go` (~15 LOC). It composes the existing `runInit` and `runBootstrap` functions:

```go
func runSetup(args []string) error {
    if err := runInit(args); err != nil {
        return err
    }
    return runBootstrap(nil)
}
```

That's the entire behavior. Both underlying commands already handle their own idempotency:
- `runInit` skips existing CLAUDE.md unless `--force` (or `--refresh --force`)
- `runBootstrap`'s 7 steps each short-circuit on healthy state (docker running → skip; images built → skip; secrets pushed → skip)

So `darken setup` is safe to re-run on partial state — it picks up where the operator left off.

### Flag surface

One flag: `--force`, passed through to `runInit` for CLAUDE.md overwrite when running setup against a partially-init'd repo. `runBootstrap` takes no flags.

### Hard-fail order

- `runInit` failure aborts (matches init's standalone contract — CLAUDE.md write failure is critical)
- `runBootstrap` step failure aborts (matches bootstrap's standalone contract)

### Subcommand registration

In `cmd/darken/main.go`'s `subcommands` slice, place after `init`:

```go
{"setup", "scaffold project + bring machine prereqs online (one-shot fresh-repo onboarding)", runSetup},
```

## `darken doctor` integration

`doctorBroad` gains a single nudge in its failure footer. When any check fails, the report ends with:

```
→ for a fresh project, run `darken setup` to bring everything online
```

One line of added code in `cmd/darken/doctor.go`:

```go
if len(failed) > 0 {
    sb.WriteString("\n→ for a fresh project, run `darken setup` to bring everything online\n")
    return sb.String(), fmt.Errorf(...)
}
```

No per-check remediation rewrites (would hammer "darken setup" five times in one run on a fresh machine). No `runInitDoctor` edits (its failure modes are about scaffold drift, owned by `upgrade-init`'s mental model). One nudge, one entry point, zero noise on healthy doctor runs.

## README + CLAUDE.md edits

Single source of truth for "how do I onboard this thing": the README quick-start + the project-root CLAUDE.md operator reference.

**`README.md` Quick Start:**
- Today's three-command sequence (`darken init` + `darken creds` + `darken bootstrap`) → single `darken setup`
- Add a one-liner pointing at `darken upgrade-init` for the existing-repo / post-`brew upgrade` path
- "What runs" detail (init scaffolds + bootstrap stages prereqs + creds, etc.) stays under per-command sections, not in the headline

**`CLAUDE.md`** (project-root operator-side ops reference):
- Same three-command-to-one collapse if it carries that sequence
- Verify with `grep -n "darken init" CLAUDE.md` and patch the relevant block

## Testing

**`cmd/darken/setup_test.go`** (3 tests):

1. **`TestSetup_RunsInitThenBootstrap`** — stub bash + scion + docker + make so both phases exercise their full path; assert init's scaffolds appear (`CLAUDE.md` exists) AND bootstrap's per-step output appears (`[1/7]`, `[7/7]`).
2. **`TestSetup_ForceFlagPassedToInit`** — pre-plant `<target>/CLAUDE.md` with a unique sentinel byte sequence (e.g. `[]byte("STALE-SENTINEL\n")`); run setup with `--force`; assert the file no longer contains the sentinel (init regenerated it from the embedded template).
3. **`TestSetup_AbortsOnInitFailure`** — pass a non-existent positional target (e.g. `"/tmp/does-not-exist-darken-setup-test"`); init fails with "target dir does not exist"; assert the returned error contains "target dir does not exist" AND assert bootstrap was NOT called (e.g. by stubbing `scion` to log every invocation and checking no `scion server status` call was logged).

**`cmd/darken/doctor_test.go`** — one new test:

4. **`TestDoctorBroad_FooterMentionsSetupOnFailure`** — make at least one check fail (e.g. stub `scion` to exit 1); capture stdout from `runDoctor`; assert the report ends with `darken setup`.

## Versioning

Ships in `v0.1.13`. Two-line bump from `v0.1.12`:
- `darken setup` (new subcommand)
- `darken doctor` failure footer

## Risks / open questions

1. **`darken setup` is a thin wrapper — does it deserve to exist?** Yes: the README headline drops from 3 lines to 1, the cognitive load on new operators drops from "remember three commands and their order" to "run setup". The 15 LOC are paid back by every fresh onboarding.

2. **What if init's `--refresh` flag is passed to setup?** Setup forwards args to init verbatim. `darken setup --refresh` would skip CLAUDE.md (refresh's behavior) then run bootstrap. Useful or confusing? Acceptable: if an operator types `darken setup --refresh`, they get a sensible behavior (refresh skills, then ensure machine prereqs). Not a primary use case.

3. **Doctor's footer is unconditional advice** — even when the failure is "scion server not running" (which `darken setup` doesn't fix; bootstrap calls `scion server start` but fails first if scion CLI isn't installed). Mitigation: `darken setup` triggers `darken bootstrap` which DOES try `scion server start` (line 39-42 of bootstrap.go). So the footer is correct for almost all failure modes. The one exception: scion CLI not present at all. Acceptable false-positive; doctor's per-check `remediationFor` already says "make install in ~/projects/scion" for that case, which is louder than the footer.

## What's next after this ships

- README polish: examples of post-setup workflows (spawning a researcher, running orchestrator-mode)
- Possibly a `darken setup --machine-only` flag for operators who want to bootstrap a new machine without running init in the current dir
- v0.2.0 cut consideration: setup + uninstall-init + upgrade-init form a coherent lifecycle triad ("operator-grade complete")

## Cross-references

- DX roadmap: `docs/superpowers/specs/2026-04-28-darken-DX-roadmap-design.md`
- `cmd/darken/init.go` — `runInit` (consumed via composition)
- `cmd/darken/bootstrap.go` — `runBootstrap` (consumed via composition)
- `cmd/darken/doctor.go` — `doctorBroad` (gains the failure footer)
- `cmd/darken/main.go` — `subcommands` slice registration
