.PHONY: darkish
darkish:
	mkdir -p bin
	go build -trimpath -ldflags="-s -w" -o bin/darkish ./cmd/darkish

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
