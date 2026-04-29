DARKEN_VERSION ?= dev
.PHONY: darken
darken:
	mkdir -p bin
	go build -trimpath -ldflags="-s -w -X main.version=$(DARKEN_VERSION)" -o bin/darken ./cmd/darken

# Sync host-mode skills from the canonical agent-skills repo into the
# project-local .claude/skills/ tree so Claude Code in this repo
# auto-discovers them. Canonical source: ~/projects/agent-skills.
SKILLS_CANONICAL ?= $(HOME)/projects/agent-skills/skills
HOST_SKILLS      := orchestrator-mode subagent-to-subharness

.PHONY: sync-skills
sync-skills:
	@for s in $(HOST_SKILLS); do \
		mkdir -p .claude/skills/$$s && \
		cp -f $(SKILLS_CANONICAL)/$$s/SKILL.md .claude/skills/$$s/SKILL.md && \
		echo "synced .claude/skills/$$s/SKILL.md ← $(SKILLS_CANONICAL)/$$s/SKILL.md"; \
	done

# Mirror canonical substrate sources into internal/substrate/data/ for
# go:embed. The data/ tree is committed so `go install` works for
# fresh consumers. Drift is detected by scripts/test-embed-drift.sh.
#
# Inputs (canonical):
#   - .scion/templates/                      (role manifests + system prompts)
#   - scripts/{bootstrap,spawn,stage-creds,stage-skills}.sh
#   - images/{Makefile,README.md,<backend>/Dockerfile,<backend>/darkish-prelude.sh}
#   - .claude/skills/{orchestrator-mode,subagent-to-subharness}/
#   - templates/CLAUDE.md.tmpl               (init template)
#
# Excluded from the embed:
#   - .scion/skills-staging/  (regenerated per spawn)
#   - .scion/agents/          (per-spawn worktrees; gitignored)
#   - .scion/{state.yaml,grove-id,server.log,server.pid,...}  (runtime state)
#   - scripts/test-*.sh       (host dev tests, not runtime substrate)
#   - images/test-*.sh, images/<backend>/test-bones.sh  (host dev tests)
#   - images/<backend>/bin/   (pre-built bones; gitignored, regenerable)
SUBSTRATE_DATA := internal/substrate/data

# Vendor substrate skills from DARKEN_SKILLS_SOURCE into
# internal/substrate/data/skills/ for go:embed.  Skills authored
# directly in this repo (superpowers, spec-kit) are committed and are
# silently skipped when absent from DARKEN_SKILLS_SOURCE.
#
# Usage:
#   make vendor-skills                              # uses default source
#   make vendor-skills DARKEN_SKILLS_SOURCE=/path  # override source
DARKEN_SKILLS_SOURCE ?= $(HOME)/projects/agent-config/skills
SKILLS_MANIFEST     := internal/substrate/skills.manifest.txt

.PHONY: vendor-skills
vendor-skills:
	@while IFS= read -r skill || [ -n "$$skill" ]; do \
		[ -z "$$skill" ] && continue; \
		case "$$skill" in \#*) continue ;; esac; \
		src="$(DARKEN_SKILLS_SOURCE)/$$skill"; \
		dst="$(SUBSTRATE_DATA)/skills/$$skill"; \
		if [ ! -d "$$src" ]; then \
			echo "vendor-skills: skip $$skill (not found at $$src)"; \
			continue; \
		fi; \
		rm -rf "$$dst"; \
		cp -R "$$src" "$$dst"; \
		echo "vendor-skills: copied $$skill"; \
	done < "$(SKILLS_MANIFEST)"

# e2e-smoke runs the C5 integration smoke tests via the Go test runner.
# Uses a fake scion binary in a temp dir; no containers are started.
# Full container e2e is deferred to CI.
.PHONY: e2e-smoke
e2e-smoke:
	go test -v -run TestSmoke ./cmd/darken/...

.PHONY: sync-embed-data
sync-embed-data:
	rm -rf $(SUBSTRATE_DATA)
	mkdir -p $(SUBSTRATE_DATA)/.scion $(SUBSTRATE_DATA)/scripts \
		$(SUBSTRATE_DATA)/images $(SUBSTRATE_DATA)/skills \
		$(SUBSTRATE_DATA)/templates
	cp -R .scion/templates $(SUBSTRATE_DATA)/.scion/
	cp scripts/bootstrap.sh scripts/spawn.sh scripts/stage-creds.sh \
		scripts/stage-skills.sh $(SUBSTRATE_DATA)/scripts/
	cp images/Makefile images/README.md $(SUBSTRATE_DATA)/images/
	for backend in claude codex gemini pi; do \
		mkdir -p $(SUBSTRATE_DATA)/images/$$backend && \
		cp images/$$backend/Dockerfile images/$$backend/darkish-prelude.sh \
			$(SUBSTRATE_DATA)/images/$$backend/; \
	done
	cp -R .claude/skills/orchestrator-mode $(SUBSTRATE_DATA)/skills/
	cp -R .claude/skills/subagent-to-subharness $(SUBSTRATE_DATA)/skills/
	cp -R .claude/skills/writing-plans $(SUBSTRATE_DATA)/skills/
	cp -R .claude/skills/superpowers $(SUBSTRATE_DATA)/skills/
	cp -R .claude/skills/spec-kit $(SUBSTRATE_DATA)/skills/
	cp templates/CLAUDE.md.tmpl $(SUBSTRATE_DATA)/templates/
	@echo "synced $(SUBSTRATE_DATA) from canonical sources"
