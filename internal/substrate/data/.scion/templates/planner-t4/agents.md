# Planner T4 — Worker Protocol

You receive an intent for a new product / system from the orchestrator.

1. Verify or create `.specify/memory/constitution.md` (operator ratified
   if new).
2. Use the spec-kit `/specify` slash command (or its skill equivalent
   from `/home/scion/skills/role/spec-kit/`) to produce a formal spec
   in `docs/specs/`.
3. Use `/plan` to produce a TDD-strict plan referencing the spec.
4. Use `/tasks` to break the plan into work units in `tasks/`.
5. Escalate any "MAY" / "should" ambiguity per README §11.1.

Communication: caveman standard to orchestrator.
