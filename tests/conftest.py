"""Shared test fixtures."""

from __future__ import annotations

import json
import random
from datetime import datetime
from pathlib import Path
from typing import Any

import pytest

from darkish_factory.classifier.audit import NullAuditLog

FIXTURE_DIR = Path(__file__).parent / "fixtures"


@pytest.fixture
def constitution_path() -> Path:
    return FIXTURE_DIR / "constitution.md"


@pytest.fixture
def policy_path() -> Path:
    return FIXTURE_DIR / "policy.yaml"


@pytest.fixture
def fixed_clock() -> datetime:
    return datetime(2026, 4, 25, 12, 0, 0)


@pytest.fixture
def fixed_rng() -> random.Random:
    return random.Random(0xDA4C0DE)


class FakeMessage:
    def __init__(self, text: str) -> None:
        self.content = [type("Block", (), {"type": "text", "text": text})()]


class FakeMessages:
    def __init__(self, payload: dict[str, Any] | Exception | list[dict[str, Any]]) -> None:
        self._payload = payload
        self._calls = 0
        self.calls: list[dict[str, Any]] = []

    def create(self, **kwargs: Any) -> FakeMessage:
        self.calls.append(kwargs)
        if isinstance(self._payload, Exception):
            raise self._payload
        if isinstance(self._payload, list):
            payload = self._payload[self._calls]
            self._calls += 1
            return FakeMessage(json.dumps(payload))
        return FakeMessage(json.dumps(self._payload))


class FakeClient:
    def __init__(self, payload: dict[str, Any] | Exception | list[dict[str, Any]]) -> None:
        self.messages = FakeMessages(payload)


@pytest.fixture
def null_log() -> NullAuditLog:
    return NullAuditLog()
