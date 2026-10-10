from __future__ import annotations

from typing import TYPE_CHECKING, List

from .client import service_path
from .types import Deployment, Logs

if TYPE_CHECKING:  # pragma: no cover
    from .client import Gagarin


class Services:
    """Reads what any service or job has printed and been."""

    def __init__(self, client: Gagarin):
        self._c = client

    def logs(
        self,
        project: str,
        name: str,
        *,
        limit: int | None = None,
        since: str | None = None,
        until: str | None = None,
        contains: str | None = None,
        previous: bool = False,
    ) -> Logs:
        """A service's or job's logs. ``since``/``until`` are a duration back from
        now (``30m``, ``6h``) or an RFC 3339 time; pass the previous page's
        ``next`` as ``until`` to page back. ``previous`` reads the last crashed
        container's output instead."""
        return self._c.request(
            "GET",
            service_path(project, name) + "/logs",
            query={"limit": limit, "since": since, "until": until, "q": contains, "previous": previous},
        )

    def deployments(self, project: str, name: str) -> List[Deployment]:
        """A service's or job's revisions, newest first."""
        return self._c.request("GET", service_path(project, name) + "/deployments")["deployments"]
