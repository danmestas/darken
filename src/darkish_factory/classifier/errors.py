"""Custom exception hierarchy for Slice 1.

`EscalationRequired` is the base; `ConstitutionConflictError` and
`ClassifierOutageError` both ARE escalations and inherit from it so a single
`except EscalationRequired` in `Classifier.decide` covers all flows. The two
non-escalation errors (`PolicyDriftError`) are administrative.
"""

from __future__ import annotations


class EscalationRequired(Exception):  # noqa: N818
    """Raised internally when the deterministic gate decides to escalate.

    Carries the trigger name and the set of escalation axes that fired. The
    public `Classifier.decide` catches it and returns a `RequestHumanInput`.
    """

    def __init__(
        self,
        trigger: str,
        categories: list[str] | None = None,
        message: str | None = None,
    ) -> None:
        self.trigger = trigger
        self.categories = list(categories) if categories else []
        super().__init__(message or f"escalation required: {trigger}")


class ConstitutionConflictError(EscalationRequired):
    """Decision violated a named constitution invariant."""

    def __init__(self, section: str, invariant: str) -> None:
        self.section = section
        self.invariant = invariant
        super().__init__(
            trigger="constitution_conflict",
            categories=[section] if section in {"taste", "architecture", "ethics"} else [],
            message=f"constitution conflict: {section}/{invariant}",
        )


class ClassifierOutageError(EscalationRequired):
    """Stage-2 LLM call failed; fail closed → escalate."""

    def __init__(self, message: str) -> None:
        super().__init__(trigger="classifier_outage", categories=[], message=message)


class PolicyDriftError(RuntimeError):
    """Spot-check disagreement passed the threshold for an axis."""
