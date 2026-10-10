"""A client for the gagarin API.

Its first job is the one ``gg run`` does: start a job from an image that is
already in gagarin's registry, and optionally wait for it to finish.

    from gagarin import Gagarin

    gg = Gagarin()  # GAGARIN_TOKEN, GAGARIN_API
    res = gg.jobs.run_and_wait("shop", "migrate", image="migrate:v3", deps=["db"])
    raise SystemExit(res.exit_code)
"""

from __future__ import annotations

import json
import os
import time
import urllib.error
import urllib.parse
import urllib.request
from dataclasses import dataclass, field
from typing import Any, Callable, Mapping, Optional, Sequence

DEFAULT_BASE_URL = "https://api.gagarin.cloud"
DEFAULT_REGISTRY = "registry.gagarin.cloud"
#: The longest a run may take, and what a new job gets.
MAX_TIMEOUT_SECONDS = 3600

PENDING, RUNNING, DONE, FAILED, SUSPENDED = "pending", "running", "done", "failed", "suspended"

#: (method, url, headers, body) -> (status, body). Replaceable for tests or a
#: different HTTP stack.
Transport = Callable[[str, str, Mapping[str, str], Optional[bytes]], "tuple[int, bytes]"]


class GagarinError(Exception):
    """What the API answers when it refuses a request. Branch on ``code``."""

    def __init__(self, status: int, code: str, message: str, fix_hint: str | None = None):
        self.status = status
        self.code = code
        self.message = message
        self.fix_hint = fix_hint
        text = f"[{code}] {message}"
        if fix_hint:
            text += f"\nhint: {fix_hint}"
        super().__init__(text)


class SupersededError(GagarinError):
    """A newer run of the job started before the awaited one was seen to finish."""


class SuspendedError(GagarinError):
    """The project was suspended, which stops its runs."""


class WaitTimeout(GagarinError):
    """The run had not finished when the wait gave up."""


@dataclass(frozen=True)
class RunState:
    revision: int
    phase: str
    exit_code: int | None = None
    started_at: str | None = None
    finished_at: str | None = None

    @property
    def finished(self) -> bool:
        return self.phase in (DONE, FAILED)

    @property
    def succeeded(self) -> bool:
        return self.phase == DONE and (self.exit_code is None or self.exit_code == 0)

    @classmethod
    def from_json(cls, d: Mapping[str, Any]) -> "RunState":
        return cls(
            revision=d["revision"],
            phase=d["phase"],
            exit_code=d.get("exit_code"),
            started_at=d.get("started_at"),
            finished_at=d.get("finished_at"),
        )


@dataclass(frozen=True)
class Submission:
    """The job as stored, and the revision this run is — the run's name."""

    project: str
    name: str
    revision: int
    image: str
    size: str | None = None
    digest: str | None = None
    timeout_seconds: int | None = None
    schedule: str | None = None
    time_zone: str | None = None
    raw: Mapping[str, Any] = field(default_factory=dict, repr=False)

    @classmethod
    def from_json(cls, d: Mapping[str, Any]) -> "Submission":
        return cls(
            project=d["project"],
            name=d["name"],
            revision=d["revision"],
            image=d["image"],
            size=d.get("size"),
            digest=d.get("digest"),
            timeout_seconds=d.get("timeout_seconds"),
            schedule=d.get("schedule"),
            time_zone=d.get("time_zone"),
            raw=d,
        )


@dataclass(frozen=True)
class RunResult:
    submission: Submission
    run: RunState

    @property
    def exit_code(self) -> int:
        """The script's own exit code, 0 on success, 1 for a run that failed
        without one — what ``gg run`` exits with."""
        if self.run.exit_code is not None:
            return self.run.exit_code
        return 0 if self.run.phase == DONE else 1


def _urllib_transport(timeout: float) -> Transport:
    def send(method: str, url: str, headers: Mapping[str, str], body: bytes | None) -> tuple[int, bytes]:
        req = urllib.request.Request(url, data=body, method=method, headers=dict(headers))
        try:
            with urllib.request.urlopen(req, timeout=timeout) as resp:
                return resp.status, resp.read()
        except urllib.error.HTTPError as e:
            return e.code, e.read()

    return send


class Gagarin:
    def __init__(
        self,
        token: str | None = None,
        base_url: str | None = None,
        registry: str | None = None,
        *,
        timeout: float = 120.0,
        transport: Transport | None = None,
    ):
        """``token`` defaults to ``$GAGARIN_TOKEN``, ``base_url`` to ``$GAGARIN_API``
        and ``registry`` to ``$GAGARIN_REGISTRY``."""
        token = token or os.environ.get("GAGARIN_TOKEN")
        if not token:
            raise ValueError("gagarin: no token: pass token= or set GAGARIN_TOKEN")
        self._token = token
        self._base_url = (base_url or os.environ.get("GAGARIN_API") or DEFAULT_BASE_URL).rstrip("/")
        self._registry = (registry or os.environ.get("GAGARIN_REGISTRY") or DEFAULT_REGISTRY).rstrip("/")
        self._send = transport or _urllib_transport(timeout)
        self.jobs = Jobs(self)

    def request(
        self,
        method: str,
        path: str,
        *,
        query: Mapping[str, Any] | None = None,
        body: Any = None,
    ) -> Any:
        url = self._base_url + path
        q = {k: str(v) for k, v in (query or {}).items() if v is not None and v != ""}
        if q:
            url += "?" + urllib.parse.urlencode(q)
        headers = {
            "Authorization": f"Bearer {self._token}",
            "Accept": "application/json",
            "User-Agent": "gagarin-sdk-python",
        }
        data = None
        if body is not None:
            headers["Content-Type"] = "application/json"
            data = json.dumps(body).encode()
        status, raw = self._send(method, url, headers, data)
        if status >= 300:
            err = None
            try:
                err = json.loads(raw).get("error")
            except (ValueError, AttributeError):
                pass
            if isinstance(err, dict) and err.get("code"):
                raise GagarinError(status, err["code"], err.get("message", ""), err.get("fix_hint"))
            raise GagarinError(status, f"http_{status}", raw.decode(errors="replace").strip())
        return json.loads(raw) if raw else None

    def project(self, project: str) -> dict:
        """Resolves a project name or id."""
        return self.request("GET", f"/v1/projects/{_q(project)}")

    def image_ref(self, project: str, image: str) -> str:
        """Expands a bare ``migrate:v3`` into the full reference gagarin runs from.
        A reference that already contains a ``/`` is returned unchanged."""
        if "/" in image:
            return image
        return f"{self._registry}/{self.project(project)['id']}/{image}"


def _q(s: str) -> str:
    return urllib.parse.quote(s, safe="")


def _service_path(project: str, name: str) -> str:
    return f"/v1/projects/{_q(project)}/services/{_q(name)}"


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
        - ``timeout_seconds``: 1–3600, counted from submission. ``None`` keeps it.
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
        if schedule:
            body["schedule"] = schedule
        if time_zone:
            body["time_zone"] = time_zone
        return Submission.from_json(self._c.request("PUT", _service_path(project, name), body=body))

    def get(self, project: str, name: str) -> dict:
        """The job's row from the project's status, with ``actual.run`` saying
        how its latest run went."""
        st = self._c.request("GET", f"/v1/projects/{_q(project)}/status")
        for row in st.get("services", []):
            if row.get("name") == name:
                if row.get("kind") != "job":
                    raise GagarinError(409, "not_a_job", f"{name} is a {row.get('kind')} in this project, not a job")
                return row
        raise GagarinError(404, "not_found", f"no job {name} in project {project}")

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
        ``exit_code``. The default timeout outlasts the platform's 60 minutes."""
        deadline = time.monotonic() + timeout
        last_phase, transient = None, 0
        while True:
            try:
                d = (self.get(project, name).get("actual") or {}).get("run")
                transient = 0
            except GagarinError as e:
                if not (e.status >= 500 or e.status == 429) or transient >= 5:
                    raise
                transient += 1
                d = None
            except OSError:
                if transient >= 5:
                    raise
                transient += 1
                d = None
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
        Takes the settings ``run`` takes, except ``schedule``."""
        if run_options.get("schedule"):
            raise ValueError("gagarin: a scheduled job does not run now; submit it with run()")
        sub = self.run(project, name, **run_options)
        run = self.wait(project, name, sub.revision, poll_interval=poll_interval, timeout=timeout, on_update=on_update)
        return RunResult(submission=sub, run=run)

    def logs(
        self,
        project: str,
        name: str,
        *,
        limit: int | None = None,
        since: str | None = None,
        until: str | None = None,
        contains: str | None = None,
    ) -> dict:
        """Reads the job's logs: ``{"logs", "source", "next", "notice"}``.
        ``since``/``until`` are a duration back from now (``30m``, ``6h``) or an
        RFC 3339 time; pass the previous page's ``next`` as ``until`` to page back."""
        return self._c.request(
            "GET",
            _service_path(project, name) + "/logs",
            query={"limit": limit, "since": since, "until": until, "q": contains},
        )
