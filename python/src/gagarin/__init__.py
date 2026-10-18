"""A client for the gagarin API.

Its first job is the one ``gg run`` does: start a job from an image that is
already in gagarin's registry, and optionally wait for it to finish.

    from gagarin import Gagarin

    gg = Gagarin()  # GAGARIN_TOKEN, GAGARIN_API
    res = gg.jobs.run_and_wait("shop", "migrate", image="migrate:v3", deps=["db"])
    raise SystemExit(res.exit_code)

Around it are the basics: ``account``, ``projects`` (with their status),
``services`` (logs and history) and ``platform`` (gagarin's own health).
"""

from .account import Account
from .client import DEFAULT_BASE_URL, DEFAULT_REGISTRY, Gagarin, Transport
from .errors import GagarinError, SupersededError, SuspendedError, WaitTimeout
from .jobs import (
    DONE,
    FAILED,
    MAX_TIMEOUT_SECONDS,
    MAX_TTL_SECONDS,
    PENDING,
    RUNNING,
    SUSPENDED,
    Jobs,
    RunResult,
    RunState,
    Submission,
)
from .platform import Platform
from .projects import Projects
from .services import Services

__all__ = [
    "DEFAULT_BASE_URL",
    "DEFAULT_REGISTRY",
    "DONE",
    "FAILED",
    "MAX_TIMEOUT_SECONDS",
    "MAX_TTL_SECONDS",
    "PENDING",
    "RUNNING",
    "SUSPENDED",
    "Account",
    "Gagarin",
    "GagarinError",
    "Jobs",
    "Platform",
    "Projects",
    "RunResult",
    "RunState",
    "Services",
    "Submission",
    "SupersededError",
    "SuspendedError",
    "Transport",
    "WaitTimeout",
]
