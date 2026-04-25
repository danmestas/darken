"""Error-class shape tests."""

import pytest

from darkish_factory.classifier.errors import (
    ClassifierOutageError,
    ConstitutionConflictError,
    EscalationRequired,
    PolicyDriftError,
)


def test_escalation_required_carries_trigger_and_categories() -> None:
    err = EscalationRequired(trigger="data_deletion", categories=["reversibility"])
    assert isinstance(err, Exception)
    assert err.trigger == "data_deletion"
    assert err.categories == ["reversibility"]


def test_policy_drift_error_is_runtime_error() -> None:
    err = PolicyDriftError("disagreement floor exceeded for ethics")
    assert isinstance(err, RuntimeError)
    assert "ethics" in str(err)


def test_constitution_conflict_error_carries_section_name() -> None:
    err = ConstitutionConflictError(section="security", invariant="no_egress_to_third_party")
    assert isinstance(err, EscalationRequired)
    assert err.section == "security"
    assert err.invariant == "no_egress_to_third_party"


def test_classifier_outage_error_wraps_underlying() -> None:
    try:
        raise RuntimeError("upstream broken")
    except RuntimeError as underlying:
        try:
            raise ClassifierOutageError("stage_2 outage") from underlying
        except ClassifierOutageError as e:
            assert isinstance(e, EscalationRequired)
            assert "outage" in str(e)
            assert e.__cause__ is underlying


def test_escalation_required_default_categories() -> None:
    err = EscalationRequired(trigger="public_release")
    assert err.categories == []


def test_constitution_conflict_required_to_escalate() -> None:
    with pytest.raises(EscalationRequired):
        raise ConstitutionConflictError(section="ethics", invariant="no_pii_logging")
