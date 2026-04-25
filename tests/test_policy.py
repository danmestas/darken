"""Policy loader tests."""

from __future__ import annotations

from pathlib import Path

import pytest

from darkish_factory.classifier.policy import (
    Policy,
    PolicyDriftError,
    load_policy,
)

FIXTURE = Path(__file__).parent / "fixtures" / "policy.yaml"


def test_load_valid_policy_file() -> None:
    p = load_policy(FIXTURE)
    assert isinstance(p, Policy)
    assert p.thresholds.confidence_floor == 0.7
    assert p.thresholds.batch_size == 5
    assert p.thresholds.max_queue_latency_min == 30
    assert p.spot_check_rate == 0.05
    assert "main" in p.thresholds.protected_branches


def test_policy_normalizes_flat_regulated_domain(tmp_path: Path) -> None:
    body = """
escalate_on:
  taste: {triggers: []}
  architecture: {triggers: []}
  ethics:
    triggers: [pii_collection_or_logging,
               regulated_domain: [health, finance]]
  reversibility:
    triggers: [data_deletion, spend_above: 50_usd]
thresholds:
  confidence_floor: 0.7
  batch_size: 5
  max_queue_latency_min: 30
"""
    target = tmp_path / "policy.yaml"
    target.write_text(body)
    p = load_policy(target)
    assert p.escalate_on.ethics.regulated_domain == {"kinds": ["health", "finance"]}


def test_policy_normalizes_nested_regulated_domain(tmp_path: Path) -> None:
    body = """
escalate_on:
  taste: {triggers: []}
  architecture: {triggers: []}
  ethics:
    triggers: [pii_collection_or_logging]
    regulated_domain:
      kinds: [health, finance]
  reversibility:
    triggers: [data_deletion]
thresholds:
  confidence_floor: 0.7
  batch_size: 5
  max_queue_latency_min: 30
"""
    target = tmp_path / "policy.yaml"
    target.write_text(body)
    p = load_policy(target)
    assert p.escalate_on.ethics.regulated_domain == {"kinds": ["health", "finance"]}


def test_policy_default_protected_branches(tmp_path: Path) -> None:
    body = """
escalate_on:
  taste: {triggers: []}
  architecture: {triggers: []}
  ethics: {triggers: []}
  reversibility:
    triggers: [data_deletion]
thresholds:
  confidence_floor: 0.7
  batch_size: 5
  max_queue_latency_min: 30
"""
    target = tmp_path / "policy.yaml"
    target.write_text(body)
    p = load_policy(target)
    assert p.thresholds.protected_branches == ["main", "master"]


def test_policy_rejects_missing_required_section(tmp_path: Path) -> None:
    body = """
escalate_on:
  taste: {triggers: []}
"""
    target = tmp_path / "policy.yaml"
    target.write_text(body)
    with pytest.raises(ValueError):
        load_policy(target)


def test_policy_rejects_relaxed_reversibility_default(tmp_path: Path) -> None:
    body = """
escalate_on:
  taste: {triggers: []}
  architecture: {triggers: []}
  ethics: {triggers: []}
  reversibility:
    triggers: []
thresholds:
  confidence_floor: 0.7
  batch_size: 5
  max_queue_latency_min: 30
"""
    target = tmp_path / "policy.yaml"
    target.write_text(body)
    with pytest.raises(PolicyDriftError):
        load_policy(target)


def test_policy_rejects_invalid_threshold_range(tmp_path: Path) -> None:
    body = """
escalate_on:
  taste: {triggers: []}
  architecture: {triggers: []}
  ethics: {triggers: []}
  reversibility:
    triggers: [data_deletion]
thresholds:
  confidence_floor: 2.0
  batch_size: 5
  max_queue_latency_min: 30
"""
    target = tmp_path / "policy.yaml"
    target.write_text(body)
    with pytest.raises(ValueError):
        load_policy(target)


def test_policy_hash_is_stable() -> None:
    a = load_policy(FIXTURE)
    b = load_policy(FIXTURE)
    assert a.hash == b.hash
    assert len(a.hash) == 64


def test_spend_threshold_parsed_to_float_usd() -> None:
    p = load_policy(FIXTURE)
    assert p.escalate_on.reversibility.spend_above_usd == 50.0


def test_policy_parses_custom_tool_wrapper_matcher(tmp_path: Path) -> None:
    body = """
escalate_on:
  taste: {triggers: []}
  architecture: {triggers: []}
  ethics: {triggers: []}
  reversibility:
    triggers: [data_deletion]
thresholds:
  confidence_floor: 0.7
  batch_size: 5
  max_queue_latency_min: 30
tool_wrapper_matchers:
  send_pager_duty: "\\\\bpd_trigger\\\\b"
"""
    target = tmp_path / "policy.yaml"
    target.write_text(body)
    p = load_policy(target)
    assert p.tool_wrapper_matchers == {"send_pager_duty": r"\bpd_trigger\b"}


def test_policy_parses_custom_routing_rubric(tmp_path: Path) -> None:
    body = """
escalate_on:
  taste: {triggers: []}
  architecture: {triggers: []}
  ethics: {triggers: []}
  reversibility:
    triggers: [data_deletion]
thresholds:
  confidence_floor: 0.7
  batch_size: 5
  max_queue_latency_min: 30
routing_rubric:
  blast_radius: "How many users could this affect if shipped wrong?"
"""
    target = tmp_path / "policy.yaml"
    target.write_text(body)
    p = load_policy(target)
    assert p.routing_rubric["blast_radius"].startswith("How many")
