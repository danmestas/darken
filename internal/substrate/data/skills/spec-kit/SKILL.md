# spec-kit

Unit-spec authoring skill for Darkish Factory planners and implementers.

## Purpose

Spec-kit provides the vocabulary and structure for writing unit specs
that are precise enough for a tdd-implementer to execute without
ambiguity. A well-formed spec has a verifiable acceptance criterion and
a named failing test description.

## Unit spec schema

```
Unit {
  name:        "<kebab-case identifier>",
  files:       ["<relative path>", ...],
  description: "<one sentence: what this unit adds>",
  test: {
    description: "<what the failing test asserts, in plain English>",
    location:    "<file and function name of the test>",
  },
  acceptance:  "<observable, testable outcome that proves the unit is done>",
  depends_on:  ["<unit name>", ...]  // omit if none
}
```

## Rules for spec authors

- Every spec gets exactly one acceptance criterion. No compound criteria.
- The acceptance criterion must be checkable by a machine (test output,
  file existence, HTTP response code). No subjective criteria.
- The failing test description names the behavior under test, not the
  implementation. Write "lookup returns nil for unknown key" not
  "TestLookup exists".
- Files lists only the files the unit is allowed to create or modify.
  Anything outside that list is out of scope.
- Dependencies are unit names already committed on the branch, not
  concepts. If a dep is not committed, note it as a blocker.

## Acceptance

A spec passes spec-kit review when: a tdd-implementer reading it for
the first time can write the failing test without asking a clarifying
question.
