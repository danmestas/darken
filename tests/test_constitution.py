"""Constitution loader tests."""

from __future__ import annotations

from pathlib import Path

import pytest

from darkish_factory.classifier.constitution import (
    Constitution,
    ConstitutionConflictError,
    load_constitution,
)

FIXTURE = Path(__file__).parent / "fixtures" / "constitution.md"


def test_loader_returns_constitution_with_named_sections() -> None:
    c = load_constitution(FIXTURE)
    assert isinstance(c, Constitution)
    assert "Coding conventions" in c.sections
    assert "Architectural invariants" in c.sections
    assert "Security" in c.sections
    assert "Ethics" in c.sections


def test_loader_records_section_text_verbatim() -> None:
    c = load_constitution(FIXTURE)
    assert "INVARIANT: no_pii_logging" in c.sections["Ethics"]


def test_loader_computes_stable_sha256_hash() -> None:
    a = load_constitution(FIXTURE)
    b = load_constitution(FIXTURE)
    assert a.hash == b.hash
    assert len(a.hash) == 64
    assert all(ch in "0123456789abcdef" for ch in a.hash)


def test_loader_extracts_invariants() -> None:
    c = load_constitution(FIXTURE)
    inv = c.invariants()
    sections = {section for section, _ in inv}
    assert "Architectural invariants" in sections
    assert ("Ethics", "no_pii_logging") in inv


def test_structural_post_check_raises_on_match() -> None:
    c = load_constitution(FIXTURE)
    text = "Adds top-level module: admin/handlers.py for ops only."
    with pytest.raises(ConstitutionConflictError) as info:
        c.assert_no_conflict(decision_text=text, files_touched=["admin/handlers.py"])
    assert info.value.invariant == "no_top_level_module_named_admin"


def test_structural_post_check_passes_on_unrelated_text() -> None:
    c = load_constitution(FIXTURE)
    c.assert_no_conflict(decision_text="Refactor logging to be pure.", files_touched=["src/log.py"])


def test_render_for_system_prompt_includes_all_sections() -> None:
    c = load_constitution(FIXTURE)
    rendered = c.render_for_system_prompt()
    assert "## Coding conventions" in rendered
    assert "no_egress_to_third_party_without_review" in rendered
