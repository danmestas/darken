"""Stage-2 adversarial LLM classifier tests."""

from __future__ import annotations

import json
from pathlib import Path
from typing import Any

import httpx
import pytest
from anthropic import APIError

from darkish_factory.classifier.audit import AuditContext, NullAuditLog
from darkish_factory.classifier.constitution import load_constitution
from darkish_factory.classifier.decisions import ProposedDecision
from darkish_factory.classifier.errors import ClassifierOutageError
from darkish_factory.classifier.llm_classifier import Stage2Classifier
from darkish_factory.classifier.policy import load_policy

POLICY = load_policy(Path(__file__).parent / "fixtures" / "policy.yaml")
CONSTITUTION = load_constitution(Path(__file__).parent / "fixtures" / "constitution.md")
CTX = AuditContext(
    decision_id="dec-1",
    constitution_hash=CONSTITUTION.hash,
    policy_hash=POLICY.hash,
)


class _FakeMessage:
    def __init__(self, text: str) -> None:
        self.content = [type("Block", (), {"type": "text", "text": text})()]


class _FakeMessages:
    def __init__(self, payload: dict[str, Any] | Exception) -> None:
        self._payload = payload
        self.calls: list[dict[str, Any]] = []

    def create(self, **kwargs: Any) -> _FakeMessage:
        self.calls.append(kwargs)
        if isinstance(self._payload, Exception):
            raise self._payload
        return _FakeMessage(json.dumps(self._payload))


class _FakeClient:
    def __init__(self, payload: dict[str, Any] | Exception) -> None:
        self.messages = _FakeMessages(payload)


def _decision(description: str = "ok") -> ProposedDecision:
    return ProposedDecision(
        decision_id="dec-1",
        title="t",
        description=description,
        files_touched=["src/x.py"],
        modules=["x"],
        diff_stats={"added": 5, "removed": 0},
        urgency="medium",
        spend_delta_usd=0.0,
        worktree_ref="wt@123",
    )


# Stub subclass to satisfy APIError's required `request` parameter (SDK v0.40+).
class _StubAPIError(APIError):
    def __init__(self) -> None:
        super().__init__(
            message="upstream 503",
            request=httpx.Request("GET", "https://api.anthropic.com"),
            body=None,
        )


def test_stage2_ratify_path() -> None:
    client = _FakeClient(
        {
            "escalate": False,
            "categories": [],
            "confidence": 0.95,
            "reasoning": "small private refactor.",
        }
    )
    s2 = Stage2Classifier(
        client=client, audit_log=NullAuditLog(), constitution=CONSTITUTION, policy=POLICY
    )
    v = s2.classify(_decision(), audit_ctx=CTX)
    assert v.escalate is False
    assert v.confidence == 0.95
    assert v.produced_by == "classifier"


def test_stage2_escalate_path() -> None:
    client = _FakeClient(
        {
            "escalate": True,
            "categories": ["taste"],
            "confidence": 0.91,
            "reasoning": "renames public API.",
        }
    )
    s2 = Stage2Classifier(
        client=client, audit_log=NullAuditLog(), constitution=CONSTITUTION, policy=POLICY
    )
    v = s2.classify(_decision(), audit_ctx=CTX)
    assert v.escalate is True
    assert v.categories == ["taste"]


def test_stage2_low_confidence_resolves_to_escalate() -> None:
    client = _FakeClient(
        {"escalate": False, "categories": [], "confidence": 0.4, "reasoning": "uncertain."}
    )
    s2 = Stage2Classifier(
        client=client, audit_log=NullAuditLog(), constitution=CONSTITUTION, policy=POLICY
    )
    v = s2.classify(_decision(), audit_ctx=CTX)
    assert v.escalate is True
    assert v.produced_by == "classifier"


def test_stage2_outage_raises_classifier_outage_error() -> None:
    # Use the SDK's base exception class directly — APIError is constructable
    # without a non-None Response, so the test does not depend on internal SDK
    # structure that has shifted across releases.
    err = _StubAPIError()
    client = _FakeClient(err)
    s2 = Stage2Classifier(
        client=client, audit_log=NullAuditLog(), constitution=CONSTITUTION, policy=POLICY
    )
    with pytest.raises(ClassifierOutageError):
        s2.classify(_decision(), audit_ctx=CTX)


def test_stage2_emits_resolved_verdict_event_with_full_envelope() -> None:
    captured: list[tuple[str, dict[str, Any]]] = []

    class CaptureLog:
        def emit(self, event_type: str, payload: dict[str, Any]) -> None:
            captured.append((event_type, payload))

    client = _FakeClient(
        {"escalate": False, "categories": [], "confidence": 0.5, "reasoning": "low conf."}
    )
    s2 = Stage2Classifier(
        client=client, audit_log=CaptureLog(), constitution=CONSTITUTION, policy=POLICY
    )
    s2.classify(_decision(), audit_ctx=CTX)
    types = [t for t, _ in captured]
    assert "stage_2_verdict" in types
    payload = next(p for t, p in captured if t == "stage_2_verdict")
    assert payload["escalate"] is True
    assert payload["produced_by"] == "classifier"
    for required in ("decision_id", "timestamp", "constitution_hash", "policy_hash"):
        assert required in payload


def test_stage2_holds_its_own_client_distinct_from_caller_client() -> None:
    deciding_client = _FakeClient(
        {"escalate": False, "categories": [], "confidence": 0.9, "reasoning": "x"}
    )
    s2_client = _FakeClient(
        {"escalate": False, "categories": [], "confidence": 0.9, "reasoning": "y"}
    )
    s2 = Stage2Classifier(
        client=s2_client, audit_log=NullAuditLog(), constitution=CONSTITUTION, policy=POLICY
    )
    assert s2.client is not deciding_client


def test_stage2_prompt_includes_constitution_and_policy_triggers() -> None:
    client = _FakeClient(
        {"escalate": False, "categories": [], "confidence": 0.95, "reasoning": "ok."}
    )
    s2 = Stage2Classifier(
        client=client, audit_log=NullAuditLog(), constitution=CONSTITUTION, policy=POLICY
    )
    s2.classify(_decision(), audit_ctx=CTX)
    sent_kwargs = client.messages.calls[0]
    system = sent_kwargs["system"]
    assert "no_pii_logging" in system
    assert "public_api_naming" in system
