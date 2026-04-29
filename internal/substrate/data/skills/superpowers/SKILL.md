# superpowers

Structured escalation and decision-quality skill for Darkish Factory agents.

## Purpose

When an agent reaches a decision point that touches architecture, taste,
ethics, or reversibility, this skill provides the vocabulary and protocol
for surfacing the block cleanly rather than resolving it unilaterally.
Scaffold a RequestHumanInput and stop. Do not self-ratify.

## The Four Axes

Before acting autonomously, check whether the decision falls on any axis:

| Axis          | Examples                                                |
|---------------|---------------------------------------------------------|
| Architecture  | Storage choice, API shape, inter-service boundary       |
| Taste         | Naming, code style, UX copy                             |
| Ethics        | Privacy, data retention, user consent                   |
| Reversibility | Schema migration on live data, destructive git ops      |

## RequestHumanInput template

```
RequestHumanInput {
  question:       "<one specific question>",
  context:        "<unit name, what you tried, why it is blocked>",
  urgency:        "low" | "medium" | "high",
  format:         "free_text" | "multiple_choice",
  recommendation: "<what you would do if forced to choose>",
  categories:     ["architecture" | "taste" | "ethics" | "reversibility"]
}
```

One question per payload. Bundle nothing.

## When NOT to escalate

Routine implementation decisions covered by the unit spec, idiomatic Go
choices, test strategy, and refactoring within the current module do not
require escalation. Exhaust the spec before escalating.
