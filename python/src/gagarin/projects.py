from __future__ import annotations

from typing import TYPE_CHECKING, List

from .client import project_path
from .errors import GagarinError
from .types import Project, ProjectStatus, ServiceStatus

if TYPE_CHECKING:  # pragma: no cover
    from .client import Gagarin


class Projects:
    """Lists, creates and reads projects."""

    def __init__(self, client: Gagarin):
        self._c = client

    def list(self) -> List[Project]:
        """Every project the caller can see, their own and shared."""
        return self._c.request("GET", "/v1/projects")["projects"]

    def get(self, project: str) -> Project:
        """Resolves a project name or id."""
        return self._c.request("GET", project_path(project))

    def create(self, name: str) -> Project:
        """Makes a project. Names are 2–30 lowercase letters, digits and hyphens,
        starting with a letter, and unique per account; a clash is refused
        ``project_exists``."""
        return self._c.request("POST", "/v1/projects", body={"name": name})

    def status(self, project: str) -> ProjectStatus:
        """What the project is asked to run, and what is running."""
        return self._c.request("GET", project_path(project) + "/status")

    def service_status(self, project: str, name: str) -> ServiceStatus:
        """One row of the project's status; raises ``not_found`` when there is no such name."""
        for row in self.status(project).get("services", []):
            if row.get("name") == name:
                return row
        raise GagarinError(404, "not_found", f"nothing named {name} in project {project}")
