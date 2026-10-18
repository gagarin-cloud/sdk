from __future__ import annotations

import time
from dataclasses import dataclass, field
from typing import TYPE_CHECKING, Any, Callable, Mapping, Sequence

from .client import service_path
from .errors import GagarinError, SupersededError, SuspendedError, WaitTimeout
from .types import Logs, ServiceStatus

if TYPE_CHECKING:  # pragma: no cover
    from .client import Gagarin

#: The longest a run may take, and what a new job gets.
MAX_TIMEOUT_SECONDS = 3600

#: The longest a finished run is kept, and what a new job gets.
MAX_TTL_SECONDS = 28800

PENDING, RUNNING, DONE, FAILED, SUSPENDED = "pending", "running", "done", "failed", "suspended"


@dataclass(frozen=True)
class RunState:
    """What a run did."""

    revision: int
    phase: str
    exit_code: int | None = None
    started_at: str | None = None
    finished_at: str | None = None
    #: A scheduled firing whose TTL ran out: no longer in the cluster; this is
    #: the platform's record of it.
    expired: bool = False

    @property
    def finished(self) -> bool:
        return self.phase in (DONE, FAILED)

    @property
    def succeeded(self) -> bool:
        return self.phase == DONE and (self.exit_code is None or self.exit_code == 0)

    @classmethod
    def from_json(cls, d: Mapping[str, Any]) -> RunState:
        return cls(
            revision=d["revision"],
            phase=d["phase"],
            exit_code=d.get("exit_code"),
            started_at=d.get("started_at"),
            finished_at=d.get("finished_at"),
            expired=d.get("expired", False),
        )


@dataclass(frozen=True)
class Submission:
    """The job as stored, and the revision this run is — the run's name."""

    project: str
    name: str
    revision: int
    image: str
    kind: str = "job"
    size: str | None = None
    digest: str | None = None
    timeout_seconds: int | None = None
    ttl_seconds: int | None = None
    schedule: str | None = None
    time_zone: str | None = None
    raw: Mapping[str, Any] = field(default_factory=dict, repr=False)

    @classmethod
    def from_json(cls, d: Mapping[str, Any]) -> Submission:
        return cls(
            project=d["project"],
            name=d["name"],
            revision=d["revision"],
            image=d["image"],
            kind=d.get("kind", "job"),
            size=d.get("size"),
            digest=d.get("digest"),
            timeout_seconds=d.get("timeout_seconds"),
            ttl_seconds=d.get("ttl_seconds"),
            schedule=d.get("schedule"),
            time_zone=d.get("time_zone"),
            raw=d,
        )


@dataclass(frozen=True)
class RunResult:
    """A submission and how its run ended."""

    submission: Submission
    run: RunState

    @property
    def exit_code(self) -> int:
        """The script's own exit code, 0 on success, 1 for a run that failed
        without one — what ``gg run`` exits with."""
        if self.run.exit_code is not None:
            return self.run.exit_code
        return 0 if self.run.phase == DONE else 1


def _retryable(e: Exception) -> bool:
    if isinstance(e, GagarinError):
        return e.status >= 500 or e.status == 429
    return isinstance(e, OSError)


class Jobs:
    """Jobs: images that run to completion and listen on nothing."""

    def __init__(self, client: Gagarin):
        self._c = client

    def run(
        self,
        project: str,
        name: str,
        *,
        image: str,
        digest: str | None = None,
        env: Mapping[str, str] | None = None,
        deps: Sequence[str] | None = None,
        size: str | None = None,
        timeout_seconds: int | None = None,
        ttl_seconds: int | None = None,
        schedule: str | None = None,
        time_zone: str | None = None,
    ) -> Submission:
        """Submits a run of the named job and returns without waiting.

        As with ``gg run``, the image must already be in gagarin's registry: a
        full reference, or a bare ``migrate:v3`` expanded against the project.
        Every call is a new run and a new revision.

        - ``digest``: ``sha256:...`` as docker push reported it, pinning the image.
        - ``env``: the complete environment for this run. Replaces the job's;
          not merged, and ``None`` empties it.
        - ``deps``: resources and services this run may reach, added to what the
          job already declares. A migration must name its database here.
        - ``size``: ``"s"``, ``"m"`` or ``"l"``. ``None`` keeps the current size.
        - ``timeout_seconds``: 1–3600, counted from when the run gets a machine (not while
          it waits for one). ``None`` keeps it.
        - ``ttl_seconds``: 1–28800, how long a finished run is kept, counted from
          when it ended. Then a one-shot job is deleted entirely; a scheduled job
          loses only that firing. ``None`` keeps it.
        - ``schedule``: five cron fields or ``@daily``-style. Nothing runs now;
          each firing is a run. Cannot be removed except by destroying the job.
        - ``time_zone``: IANA zone the schedule is read in. ``None`` keeps it.
        """
        if not image:
            raise ValueError("gagarin: image is required")
        # The environment is replaced on every run, so it is always sent.
        body: dict[str, Any] = {"kind": "job", "image": self._c.image_ref(project, image), "env": dict(env or {})}
        if digest:
            body["digest"] = digest
        if deps:
            body["deps"] = list(deps)
        if size:
            body["size"] = size
        if timeout_seconds:
            body["timeout_seconds"] = timeout_seconds
        if ttl_seconds:
            body["ttl_seconds"] = ttl_seconds
        if schedule:
            body["schedule"] = schedule
        if time_zone:
            body["time_zone"] = time_zone
        return Submission.from_json(self._c.request("PUT", service_path(project, name), body=body))

    def get(self, project: str, name: str) -> ServiceStatus:
        """The job's row from the project's status, with how its latest run went
        in ``actual.run``. A name that is not a job is refused ``not_a_job``."""
        row = self._c.projects.service_status(project, name)
        if row.get("kind") != "job":
            raise GagarinError(409, "not_a_job", f"{name} is a {row.get('kind')} in this project, not a job")
        return row

    def wait(
        self,
        project: str,
        name: str,
        revision: int,
        *,
        poll_interval: float = 3.0,
        timeout: float = 65 * 60,
        on_update: Callable[[RunState], None] | None = None,
    ) -> RunState:
        """Blocks until the given revision of the job has finished, and returns
        how it ended. A failed run is not an exception: read ``phase`` and
        ``exit_code``. The default timeout outlasts the platform's 60 minutes.

        Raises :class:`SupersededError` if a newer run starts first,
        :class:`SuspendedError` if the project is suspended, and
        :class:`WaitTimeout` when ``timeout`` passes."""
        deadline = time.monotonic() + timeout
        last_phase, transient = None, 0
        while True:
            d = None
            try:
                d = (self.get(project, name).get("actual") or {}).get("run")
                transient = 0
            except Exception as e:
                if not _retryable(e) or transient >= 5:
                    raise
                transient += 1
            if d:
                run = RunState.from_json(d)
                if run.revision > revision:
                    raise SupersededError(
                        409, "superseded", f"run {revision} of {name} was superseded by run {run.revision}"
                    )
                if run.revision == revision:
                    if run.phase != last_phase and on_update:
                        on_update(run)
                    last_phase = run.phase
                    if run.finished:
                        return run
                    if run.phase == SUSPENDED:
                        raise SuspendedError(
                            409, "suspended", f"the project is suspended and run {revision} of {name} was stopped"
                        )
            if time.monotonic() + poll_interval > deadline:
                raise WaitTimeout(408, "wait_timeout", f"run {revision} of {name} had not finished in time")
            time.sleep(poll_interval)

    def run_and_wait(
        self,
        project: str,
        name: str,
        *,
        poll_interval: float = 3.0,
        timeout: float = 65 * 60,
        on_update: Callable[[RunState], None] | None = None,
        **run_options: Any,
    ) -> RunResult:
        """Submits a run and waits for it to finish — what ``gg run`` does.
        Takes the settings :meth:`run` takes, except ``schedule``."""
        if run_options.get("schedule"):
            raise ValueError("gagarin: a scheduled job does not run now; submit it with run()")
        sub = self.run(project, name, **run_options)
        run = self.wait(project, name, sub.revision, poll_interval=poll_interval, timeout=timeout, on_update=on_update)
        return RunResult(submission=sub, run=run)

    def logs(self, project: str, name: str, **options: Any) -> Logs:
        """The job's logs; takes the filters :meth:`Services.logs` takes."""
        return self._c.services.logs(project, name, **options)
