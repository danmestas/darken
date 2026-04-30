# Ousterhout Review: v0.1.18

## Executive Summary

Verdict: **BLOCK**.

The branch adds useful fixes, but several changes make shallow interfaces deeper in the wrong direction: backend-specific Claude behavior leaks into every image prelude, `command_args` is parsed as manifest data but forwarded to the wrong CLI layer, `darken setup` splits project identity between the requested target and current working directory, and the stage-skills concurrency fix still has a non-atomic publish step. `go test ./...` and `go vet ./...` pass, but the tests mostly validate the intended happy path, not the boundary failures.

Review method: commits were grouped by disjoint ownership and reviewed in parallel: prelude hooks/settings, stage-skills, scion output/look, setup grove init, and manifests/templates/embed preservation. The requested `superpowers:dispatching-parallel-agents` skill was not installed in this environment, so parallel subagents were dispatched directly.

## Per-Commit Findings

### bug-24: per-template command_args for 1M context on long-running roles

- **High: `command_args` crosses the wrong abstraction boundary.** `cmd/darken/spawn.go:53` appends manifest `command_args` directly to `scion start`. Installed `scion start --help` has no `--betas` flag, so roles with `command_args` can fail before the harness starts. In POSD terms, the manifest now looks like a deep module, but its implementation leaks raw harness flags into the orchestration CLI.

- **Medium: malformed manifests are silently ignored.** `cmd/darken/spawn.go:41` treats manifest load/parse failure as non-fatal, which makes `command_args` disappear without signal. That weakens the manifest as a configuration boundary and increases operator debugging load.

### bug18: stage-skills concurrency safety via per-process tmp dir

- **High: publish is still not atomic.** `scripts/stage-skills.sh:145` and `internal/substrate/data/scripts/stage-skills.sh:145` use `rm -rf "${STAGE_DIR}"` followed by `mv "${stage_tmp}" "${STAGE_DIR}"`. Two concurrent publishers can interleave so the second `mv` nests its temp directory under an existing `STAGE_DIR` instead of replacing it. The interface says "last writer wins"; the implementation can produce malformed staging.

- **Medium: the regression test does not prove tree shape.** `scripts/test-stage-skills-concurrency.sh` checks expected files, but not exact top-level contents or absence of temp directories. It can pass with extra nested `.tmp.<pid>` directories.

- **Low: duplicated shell scripts increase drift.** The source and embedded `stage-skills.sh` copies carry the same concurrency logic. A single staging contract is maintained in two places.

### bug-20: darken setup runs scion grove init for project-scoped grove

- **High: setup target and grove target can diverge.** `cmd/darken/setup.go:15` allows `darken setup <target>`, but `ensureGroveInit` uses `os.Getwd()` at `cmd/darken/setup.go:32`. Running setup for another directory can initialize/check the wrong `.scion/grove-id`. Project identity should be resolved once and passed through the setup pipeline.

- **Low: interface comment drift.** `cmd/darken/scion_client.go:14` still says the interface has five methods after adding `GroveInit`.

### bug17: subharness hooks route SessionStop to operator via scion message

- **High: Claude hook configuration was copied into non-Claude images.** `images/codex/darkish-prelude.sh`, `images/gemini/darkish-prelude.sh`, and `images/pi/darkish-prelude.sh` write Claude Code hook config under `~/.claude/settings.json`, but those harnesses do not consume Claude hooks. The hook feature belongs at a Scion/harness-level boundary or in backend-native integrations.

- **Medium: hook insertion is not idempotent.** `images/claude/darkish-prelude.sh:183` appends identical `Stop` and `PreToolUse` hooks on each prelude run. Persistent homes or reruns can produce duplicate operator messages.

### bug-22: auto-allow .claude/skills writes in image-level settings.json

- **High: settings write can destroy existing operator config.** `images/claude/darkish-prelude.sh:115` writes a full new `~/.claude/settings.json` whenever `.claude/skills` is absent. That can erase unrelated permissions, hooks, theme/model settings, and future config. The code should merge one Darken-owned allow rule into existing JSON instead of treating a grep miss as permission to replace the file.

### bug-26: add darken look subcommand with ANSI escape stripping

- **Medium: ANSI stripping is too narrow for TUI output.** `cmd/darken/look.go:15` only handles CSI escapes of the form `ESC[` plus digits/semicolons plus a final letter. Common sequences such as `ESC[?25l`, `ESC[?1049h`, and OSC title updates survive.

- **Medium: `look` bypasses the Scion command boundary.** `cmd/darken/look.go:36` uses raw `exec.Command` instead of `scionCmdWithEnv` or `ScionClient`, and hardcodes `--no-hub`. That adds a second command policy path and conflicts with the branch's hub-oriented agent instructions.

- **Low: subcommand registration is hidden in `init`.** `cmd/darken/look.go:54` appends to `subcommands` nonlocally instead of registering in the explicit command table in `main.go`.

### bug-19: strip non-JSON prefix lines from scion list output before unmarshal

- **Low: JSON trimming is line-start only.** `cmd/darken/spawn_poller.go:89` only recognizes `[` or `{` at byte zero of a line. Legal JSON preceded by whitespace on the first JSON line can still fail. Trim JSON whitespace before choosing the start.

### bug-21: add Steering live subharnesses section to orchestrator-mode skill

- No blocking Ousterhout finding. The document addition is localized. Main risk is copy drift between `.claude/skills/orchestrator-mode/SKILL.md` and the embedded copy, already covered by existing skill rules tests.

### bug27: sync-embed-data preserves vendored skills listed in manifest

- No blocking Ousterhout finding. The preservation rule is pragmatic. It would benefit from extracting the preserve/restore loop into a script helper later, but the current Makefile change is narrow.

## Recommended Fixups

1. **`cmd/darken/spawn.go`: stop appending `command_args` to `scion start`.** Route these values to a harness/template field that Scion actually consumes, or remove the `command_args` forwarding until Scion supports it. Add a test whose stub rejects unknown `scion start` flags.

2. **`cmd/darken/spawn.go`: fail loudly on malformed manifests.** Keep compatibility only for true manifest absence if needed; parse errors in known roles should block spawn with a clear message.

3. **`scripts/stage-skills.sh` and `internal/substrate/data/scripts/stage-skills.sh`: serialize only the publish section.** Use a lock directory or other portable lock around `rm -rf && mv`, leaving copy work outside the lock. Update the concurrency test to assert exact top-level staging contents and no temp directories.

4. **`cmd/darken/setup.go` and `cmd/darken/scion_client.go`: resolve setup target once.** Pass that target into `ensureGroveInit(targetDir)` and run `scion grove init` with `cmd.Dir = targetDir`. Add a test where cwd differs from the setup target.

5. **`images/*/darkish-prelude.sh`: remove Claude hook setup from non-Claude images.** Either implement backend-native stop/question notification or move notification to a Scion-level hook shared by all harnesses.

6. **`images/claude/darkish-prelude.sh`: merge settings with jq.** Preserve existing top-level keys, `permissions.deny`, existing allow rules, and hooks; append only missing Darken-owned rules. Add a test with preexisting settings and hooks.

7. **`images/claude/darkish-prelude.sh`: make hook insertion idempotent.** De-duplicate by command plus matcher/event or replace a named Darken-owned hook entry.

8. **`cmd/darken/look.go`: use the Scion command abstraction.** Add `LookAgent` to `ScionClient` or use `scionCmdWithEnv`; do not hardcode `--no-hub` in `darken look`.

9. **`cmd/darken/look.go`: use a fuller ANSI stripper.** Cover CSI private modes and OSC sequences, with tests for `\x1b[?25l`, `\x1b[?1049h`, and `\x1b]0;title\x07`.

10. **`cmd/darken/main.go`: register `look` in the explicit subcommand table.** Keep command surface area local and discoverable.

11. **`cmd/darken/spawn_poller.go`: make `jsonStart` whitespace-aware.** Scan for the first non-space byte on each line and return from that byte when it is `[` or `{`.

## Decision

**BLOCK.** This should not land as minor polish. The branch needs architectural fixups around command argument ownership, project target resolution, concurrency publish semantics, and backend-specific hook/config boundaries before release.

Verification run:

- `go test ./...` passed.
- `go vet ./...` passed.
- `scion start --help` confirms `--betas` is not a `scion start` flag.
