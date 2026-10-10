"""The client, and the one place requests are made."""

from __future__ import annotations

import json
import os
import urllib.error
import urllib.parse
import urllib.request
from typing import Any, Callable, Mapping, Optional, Tuple

from .errors import api_error

DEFAULT_BASE_URL = "https://api.gagarin.cloud"
DEFAULT_REGISTRY = "registry.gagarin.cloud"

#: (method, url, headers, body) -> (status, body). Replaceable to use a
#: different HTTP stack.
Transport = Callable[[str, str, Mapping[str, str], Optional[bytes]], Tuple[int, bytes]]


def _urllib_transport(timeout: float) -> Transport:
    def send(method: str, url: str, headers: Mapping[str, str], body: bytes | None) -> tuple[int, bytes]:
        req = urllib.request.Request(url, data=body, method=method, headers=dict(headers))
        try:
            with urllib.request.urlopen(req, timeout=timeout) as resp:
                return resp.status, resp.read()
        except urllib.error.HTTPError as e:
            return e.code, e.read()

    return send


def quote(s: str) -> str:
    return urllib.parse.quote(s, safe="")


def project_path(project: str) -> str:
    return f"/v1/projects/{quote(project)}"


def service_path(project: str, name: str) -> str:
    return f"{project_path(project)}/services/{quote(name)}"


class Gagarin:
    """A client for the gagarin API.

    - ``jobs``: submit, wait on and read jobs — what ``gg run`` does.
    - ``account``: who the credential is, and the account's balance.
    - ``projects``: list, create and read projects, and their status.
    - ``services``: any service's logs and deploy history.
    - ``platform``: whether gagarin itself is working.
    """

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
        from .account import Account
        from .jobs import Jobs
        from .platform import Platform
        from .projects import Projects
        from .services import Services

        token = token or os.environ.get("GAGARIN_TOKEN")
        if not token:
            raise ValueError("gagarin: no token: pass token= or set GAGARIN_TOKEN")
        self._token = token
        self._base_url = (base_url or os.environ.get("GAGARIN_API") or DEFAULT_BASE_URL).rstrip("/")
        self._registry = (registry or os.environ.get("GAGARIN_REGISTRY") or DEFAULT_REGISTRY).rstrip("/")
        self._send = transport or _urllib_transport(timeout)
        self.jobs = Jobs(self)
        self.account = Account(self)
        self.projects = Projects(self)
        self.services = Services(self)
        self.platform = Platform(self)

    def send(
        self,
        method: str,
        path: str,
        *,
        query: Mapping[str, Any] | None = None,
        body: Any = None,
    ) -> tuple[int, bytes]:
        """Makes one request and hands back the status and body, whatever the status was."""
        url = self._base_url + path
        q = {}
        for k, v in (query or {}).items():
            if v is None or v == "" or v is False:
                continue
            q[k] = "true" if v is True else str(v)
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
        return self._send(method, url, headers, data)

    def request(
        self,
        method: str,
        path: str,
        *,
        query: Mapping[str, Any] | None = None,
        body: Any = None,
    ) -> Any:
        """Makes one request, raises :class:`GagarinError` on a refusal, and parses the answer."""
        status, raw = self.send(method, path, query=query, body=body)
        if not 200 <= status < 300:
            raise api_error(status, raw)
        return json.loads(raw) if raw else None

    def image_ref(self, project: str, image: str) -> str:
        """Expands a bare ``migrate:v3`` into the full reference gagarin runs from.
        A reference that already contains a ``/`` is returned unchanged."""
        if "/" in image:
            return image
        return f"{self._registry}/{self.projects.get(project)['id']}/{image}"
