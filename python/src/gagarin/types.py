"""The shapes the API answers with. Plain dicts at runtime; typed for editors."""

from __future__ import annotations

import sys
from typing import TYPE_CHECKING, Dict, List, Optional, TypedDict

# NotRequired is only ever named inside annotations, which this module never
# evaluates, so older Pythons need nothing installed for it.
if sys.version_info >= (3, 11):
    from typing import NotRequired
elif TYPE_CHECKING:  # pragma: no cover
    from typing_extensions import NotRequired


class Whoami(TypedDict):
    account: str
    account_created_at: str
    client: str
    #: "deploy", "destroy", both or neither.
    can: List[str]
    base_domain: str
    #: The registry host images are run from.
    registry: str
    #: The cluster's OS/architecture, e.g. "linux/amd64".
    platform: str


class Billing(TypedDict):
    micro_usd: int
    balance: str
    burn_per_hour_micro_usd: int
    #: None when nothing is burning: the balance lasts forever.
    runway_hours: Optional[float]
    has_card: bool
    auto_top_up: bool
    suspended: bool
    top_up_at: str
    top_up_for: str
    top_up_at_micro_usd: int
    top_up_for_micro_usd: int
    services: int
    memories: int


class Project(TypedDict):
    id: str
    name: str
    created_at: str
    #: "owner", "editor" or "viewer".
    role: str
    suspended_at: NotRequired[str]
    suspended_reason: NotRequired[str]
    account_suspended_at: NotRequired[str]


class RunStateDict(TypedDict):
    revision: int
    #: "pending", "running", "done", "failed" or "suspended".
    phase: str
    exit_code: NotRequired[int]
    started_at: NotRequired[str]
    finished_at: NotRequired[str]
    #: A scheduled firing whose TTL ran out: no longer in the cluster; this is
    #: the platform's record of it.
    expired: NotRequired[bool]


class ScheduleState(TypedDict, total=False):
    next_at: str
    last_scheduled_at: str
    suspended: bool


class Actual(TypedDict):
    exists: bool
    ready_replicas: int
    desired_replicas: int
    ingress_exists: bool
    superseded_replicas: NotRequired[int]
    running_image: NotRequired[str]
    running_port: NotRequired[int]
    message: NotRequired[str]
    stalled: NotRequired[bool]
    #: A job's latest run; absent for anything else, and before the first.
    run: NotRequired[RunStateDict]
    #: A scheduled job's schedule; absent for anything else.
    schedule: NotRequired[ScheduleState]


class Domain(TypedDict):
    url: str
    domain: str
    generated: bool
    state: str
    sentence: str
    blocked_on: NotRequired[str]
    dns: NotRequired[Dict[str, str]]


class ServiceStatus(TypedDict):
    project: str
    name: str
    #: "container", "job", or a resource's type ("postgres", ...).
    kind: str
    image: str
    port: int
    created_at: str
    updated_at: str
    actual: Actual
    needs: List[str]
    in_sync: bool
    domains: List[Domain]
    sentence: str
    #: Names of its environment variables, never the values.
    env_keys: List[str]
    backups_enabled: bool
    size: NotRequired[str]
    digest: NotRequired[str]
    volume_size_gb: NotRequired[int]
    timeout_seconds: NotRequired[int]
    #: A job's; absent when it is the default, 28800 (8 hours).
    ttl_seconds: NotRequired[int]
    schedule: NotRequired[str]
    time_zone: NotRequired[str]
    custom_domains: NotRequired[List[str]]
    updated_by: NotRequired[str]
    volume_usage: NotRequired[Dict[str, int]]
    object_usage: NotRequired[Dict[str, object]]
    restore: NotRequired[Dict[str, object]]


class PlatformState(TypedDict):
    converging: bool
    reconciler_last_run: NotRequired[str]


class ProjectStatus(TypedDict):
    project: str
    project_id: str
    services: List[ServiceStatus]
    platform: PlatformState
    #: Sentences a reader should be told before the table.
    notices: List[str]
    usage_today: Dict[str, object]


class Deployment(TypedDict):
    revision: int
    project: str
    service: str
    image: str
    port: int
    public: bool
    needs: List[str]
    env_keys: List[str]
    created_at: str
    digest: NotRequired[str]
    url: NotRequired[str]
    volume_path: NotRequired[str]
    volume_size_gb: NotRequired[int]
    timeout_seconds: NotRequired[int]
    ttl_seconds: NotRequired[int]
    schedule: NotRequired[str]
    time_zone: NotRequired[str]
    deployed_by: NotRequired[str]
    restored_from: NotRequired[int]


class Logs(TypedDict):
    #: The lines, oldest first, newline-terminated.
    logs: str
    #: "store" or "cluster".
    source: str
    #: The ``until`` that reads the page before this one, when there is one.
    next: Optional[str]
    notice: NotRequired[str]


class Check(TypedDict):
    ok: bool
    ms: int
    detail: NotRequired[str]


class Health(TypedDict):
    ok: bool
    serving: Check
    cluster: NotRequired[Check]
    reconciler: NotRequired[Check]
