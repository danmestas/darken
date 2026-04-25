"""Smoke test that proves the public Classifier symbol is importable.

This must fail first: `uv init --package` writes a `__version__` for us, so a
version assertion would pass immediately. Importing `Classifier` does not pass
until we define it (see Step 7), giving us the red-green-refactor cycle.
"""

from darkish_factory import Classifier


def test_classifier_symbol_is_importable() -> None:
    assert Classifier is not None
