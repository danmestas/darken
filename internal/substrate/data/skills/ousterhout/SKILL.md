# ousterhout

Complexity management principles (John Ousterhout, A Philosophy of Software Design).

## Principles

- Deep modules: small public interface, rich internal behavior.
- Pull complexity downward so callers stay simple.
- Information hiding: expose only what callers need to know.
- Define errors out of existence: make invalid states unrepresentable.
- No shallow wrappers. No pass-through methods. No god objects.
- Comments describe WHY and the non-obvious, not WHAT the code does.

## Application

Apply this skill during design review and refactoring. Flag shallow wrappers
that add a call layer without adding abstraction. Flag exported types that
expose internal implementation details. Prefer one well-named function that
encapsulates a concept over three functions the caller must sequence correctly.
