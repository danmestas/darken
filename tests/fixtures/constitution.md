# Constitution

## Coding conventions
Use snake_case for module names. Prefer pure functions.

## Architectural invariants
- INVARIANT: no_top_level_module_named_admin
- INVARIANT: no_unbounded_recursion

## Security
- INVARIANT: no_plaintext_credentials_in_repo
- INVARIANT: no_egress_to_third_party_without_review

## Ethics
- INVARIANT: no_pii_logging
