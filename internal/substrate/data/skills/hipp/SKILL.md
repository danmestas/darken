# hipp

SQLite-first reliability principles (D. Richard Hipp).

Zero-config embedded storage, ruthless testing, minimal external
dependencies. Code readable 10 years from now.

## Principles

- Prefer SQLite over any server-based store unless the spec requires a server.
- 100% business-logic test coverage. Every edge case has a test.
- Minimize dependencies. Write the 50 lines yourself instead of importing a package.
- Idempotent migrations. Forward-only schema changes.
- Explicit error paths. No silent failures.

## Application

Apply this skill when reviewing storage decisions, dependency choices,
and test coverage gaps. Flag any networked store where SQLite would suffice.
Flag any imported package whose core value could be inlined in under 100 lines.
