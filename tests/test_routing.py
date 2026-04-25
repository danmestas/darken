"""Routing classifier tests with a fake anthropic client."""

from __future__ import annotations

import json
from typing import Any

import pytest

from darkish_factory.classifier.audit import AuditContext, NullAuditLog
from darkish_factory.classifier.decisions import RoutingInputs
from darkish_factory.classifier.routing import RoutingClassifier


class _FakeMessage:
    def __init__(self, text: str) -> None:
        self.content = [type("Block", (), {"type": "text", "text": text})()]


class _FakeMessages:
    def __init__(self, payload: dict[str, Any]) -> None:
        self._payload = payload
        self.last_kwargs: dict[str, Any] = {}

    def create(self, **kwargs: Any) -> _FakeMessage:
        self.last_kwargs = kwargs
        return _FakeMessage(json.dumps(self._payload))


class _FakeClient:
    def __init__(self, payload: dict[str, Any]) -> None:
        self.messages = _FakeMessages(payload)


CTX = AuditContext(decision_id="d?", constitution_hash="0" * 64, policy_hash="1" * 64)


def _ctx(decision_id: str) -> AuditContext:
    return AuditContext(decision_id=decision_id, constitution_hash="0" * 64, policy_hash="1" * 64)


def test_router_returns_light() -> None:
    client = _FakeClient({"label": "light", "reasoning": "single small file"})
    rc = RoutingClassifier(client=client, audit_log=NullAuditLog())
    out = rc.classify(
        inputs=RoutingInputs(loc_affected=20, modules_touched=["util"]),
        audit_ctx=_ctx("d1"),
    )
    assert out == "light"


def test_router_returns_heavy() -> None:
    client = _FakeClient({"label": "heavy", "reasoning": "cross-module data model change"})
    rc = RoutingClassifier(client=client, audit_log=NullAuditLog())
    out = rc.classify(
        inputs=RoutingInputs(
            loc_affected=900, modules_touched=["api", "db"], data_model_changes=True
        ),
        audit_ctx=_ctx("d2"),
    )
    assert out == "heavy"


def test_router_promotes_ambiguous_to_heavy() -> None:
    client = _FakeClient({"label": "ambiguous", "reasoning": "could go either way"})
    rc = RoutingClassifier(client=client, audit_log=NullAuditLog())
    out = rc.classify(
        inputs=RoutingInputs(loc_affected=200, modules_touched=["api"]),
        audit_ctx=_ctx("d3"),
    )
    assert out == "heavy"


def test_router_emits_event_with_resolved_label() -> None:
    captured: list[tuple[str, dict[str, Any]]] = []

    class CaptureLog:
        def emit(self, event_type: str, payload: dict[str, Any]) -> None:
            captured.append((event_type, payload))

    client = _FakeClient({"label": "ambiguous", "reasoning": "?"})
    rc = RoutingClassifier(client=client, audit_log=CaptureLog())
    rc.classify(inputs=RoutingInputs(), audit_ctx=_ctx("d4"))
    types = [t for t, _ in captured]
    assert "routing_verdict" in types
    payload = next(p for t, p in captured if t == "routing_verdict")
    assert payload["label"] == "heavy"
    assert payload["raw_label"] == "ambiguous"
    assert payload["decision_id"] == "d4"
    for required in ("decision_id", "timestamp", "constitution_hash", "policy_hash"):
        assert required in payload


def test_router_prompt_includes_rubric_inputs() -> None:
    client = _FakeClient({"label": "light", "reasoning": "small"})
    rc = RoutingClassifier(client=client, audit_log=NullAuditLog())
    rc.classify(
        inputs=RoutingInputs(loc_affected=42, security_concerns=True),
        audit_ctx=_ctx("d5"),
    )
    sent = client.messages.last_kwargs
    rendered = json.dumps(sent.get("messages", []))
    assert "loc_affected" in rendered
    assert "42" in rendered
    assert "security_concerns" in rendered


def test_router_prompt_includes_custom_policy_rubric_question() -> None:
    client = _FakeClient({"label": "light", "reasoning": "small"})
    custom = {"blast_radius": "How many users could this affect if shipped wrong?"}
    rc = RoutingClassifier(client=client, audit_log=NullAuditLog(), rubric_overrides=custom)
    rc.classify(inputs=RoutingInputs(), audit_ctx=_ctx("d-rubric"))
    sent = client.messages.last_kwargs
    system = sent["system"]
    assert "blast_radius" in system
    assert "How many users could this affect" in system


def test_router_rejects_invalid_label() -> None:
    client = _FakeClient({"label": "tiny", "reasoning": "x"})
    rc = RoutingClassifier(client=client, audit_log=NullAuditLog())
    with pytest.raises(ValueError):
        rc.classify(inputs=RoutingInputs(), audit_ctx=_ctx("d6"))
