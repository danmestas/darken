"""Darkish Factory public API for Slice 1."""

from __future__ import annotations

from .classifier.answers import HumanAnswer, RequestHumanInput
from .classifier.api import Classifier
from .classifier.audit import AuditLog, JSONLAuditLog, NullAuditLog
from .classifier.errors import (
    ClassifierOutageError,
    ConstitutionConflictError,
    EscalationRequired,
    PolicyDriftError,
)

__version__ = "0.1.0"

__all__ = [
    "AuditLog",
    "Classifier",
    "ClassifierOutageError",
    "ConstitutionConflictError",
    "EscalationRequired",
    "HumanAnswer",
    "JSONLAuditLog",
    "NullAuditLog",
    "PolicyDriftError",
    "RequestHumanInput",
    "__version__",
]
