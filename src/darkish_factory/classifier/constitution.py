"""Constitution loader, parser, hasher, and structural post-check.

Parsing strategy: split on level-2 markdown headings (`## `). Each section is
stored verbatim. Invariants are bullet lines of the form
`- INVARIANT: <name>` inside any section. The structural post-check runs each
known invariant matcher against the decision text + file list and raises
`ConstitutionConflictError` on the first hit.
"""

from __future__ import annotations

import ast
import hashlib
import re
from collections.abc import Callable
from dataclasses import dataclass
from pathlib import Path

from .errors import ConstitutionConflictError

INVARIANT_LINE = re.compile(
    r"^\s*-\s*INVARIANT:\s*(?P<name>[a-zA-Z_][a-zA-Z0-9_]*)\s*$",
    re.MULTILINE,
)
H2_LINE = re.compile(r"^##\s+(?P<title>.+?)\s*$", re.MULTILINE)


_StructuralMatcher = Callable[[str, list[str]], bool]


def _matcher_no_top_level_module_named_admin(text: str, files: list[str]) -> bool:
    if any(f.split("/", 1)[0] == "admin" for f in files):
        return True
    return bool(re.search(r"\btop[- ]level module:?\s*admin\b", text, re.IGNORECASE))


def _matcher_no_pii_logging(text: str, files: list[str]) -> bool:
    pattern = r"\b(log|print)[^.\n]{0,40}\b(email|ssn|phone|password|pii)\b"
    return bool(re.search(pattern, text, re.IGNORECASE))


def _matcher_no_plaintext_credentials_in_repo(text: str, files: list[str]) -> bool:
    return bool(re.search(r"(?i)(api[_-]?key|password|secret)\s*=\s*[\"'][A-Za-z0-9_]{8,}", text))


def _matcher_no_egress_to_third_party_without_review(text: str, files: list[str]) -> bool:
    pattern = r"\b(requests\.|urllib\.|httpx\.|fetch\()\s*[A-Za-z]+\s*[\"']https?://"
    return bool(re.search(pattern, text))


def _matcher_no_unbounded_recursion(text: str, files: list[str]) -> bool:
    """Structural check: any FunctionDef whose body calls itself by name with
    no visible base case is flagged as a candidate for unbounded recursion.

    The check is best-effort: we only look at snippets the caller treats as
    Python source. If `text` doesn't parse as Python, fall back to a literal
    phrase match so the matcher still flags decisions whose description
    explicitly says "unbounded recursion".
    """
    if re.search(r"\bunbounded recursion\b", text, re.IGNORECASE):
        return True
    try:
        tree = ast.parse(text)
    except SyntaxError:
        return False

    for node in ast.walk(tree):
        if not isinstance(node, ast.FunctionDef):
            continue
        name = node.name
        has_self_call = False
        has_guard = False
        for child in ast.walk(node):
            if isinstance(child, ast.Call):
                func = child.func
                if isinstance(func, ast.Name) and func.id == name:
                    has_self_call = True
            if isinstance(child, ast.If | ast.Return):
                has_guard = True
        if has_self_call and not has_guard:
            return True
    return False


_DEFAULT_MATCHERS: dict[str, _StructuralMatcher] = {
    "no_top_level_module_named_admin": _matcher_no_top_level_module_named_admin,
    "no_pii_logging": _matcher_no_pii_logging,
    "no_plaintext_credentials_in_repo": _matcher_no_plaintext_credentials_in_repo,
    "no_egress_to_third_party_without_review": _matcher_no_egress_to_third_party_without_review,
    "no_unbounded_recursion": _matcher_no_unbounded_recursion,
}


@dataclass(frozen=True)
class Constitution:
    """Parsed constitution document."""

    raw: str
    sections: dict[str, str]
    hash: str

    def invariants(self) -> list[tuple[str, str]]:
        """Return `[(section_name, invariant_name), ...]` discovered."""
        result: list[tuple[str, str]] = []
        for section_name, body in self.sections.items():
            for match in INVARIANT_LINE.finditer(body):
                result.append((section_name, match.group("name")))
        return result

    def render_for_system_prompt(self) -> str:
        """Render the full constitution for inclusion in the Stage-2 prompt."""
        parts: list[str] = []
        for name, body in self.sections.items():
            parts.append(f"## {name}\n{body.strip()}\n")
        return "\n".join(parts)

    def assert_no_conflict(self, decision_text: str, files_touched: list[str]) -> None:
        """Run structural matchers; raise on first hit."""
        for section_name, invariant in self.invariants():
            matcher = _DEFAULT_MATCHERS.get(invariant)
            if matcher is None:
                continue
            if matcher(decision_text, files_touched):
                raise ConstitutionConflictError(section=section_name, invariant=invariant)


def load_constitution(path: Path) -> Constitution:
    """Read, hash, and parse the constitution at `path`."""
    raw_bytes = path.read_bytes()
    digest = hashlib.sha256(raw_bytes).hexdigest()
    raw = raw_bytes.decode("utf-8")
    sections: dict[str, str] = {}

    headings = list(H2_LINE.finditer(raw))
    for index, heading in enumerate(headings):
        title = heading.group("title").strip()
        body_start = heading.end()
        body_end = headings[index + 1].start() if index + 1 < len(headings) else len(raw)
        sections[title] = raw[body_start:body_end].strip()

    return Constitution(raw=raw, sections=sections, hash=digest)


__all__ = ["Constitution", "ConstitutionConflictError", "load_constitution"]
