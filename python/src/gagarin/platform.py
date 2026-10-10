from __future__ import annotations

import json
from typing import TYPE_CHECKING

from .errors import api_error
from .types import Health

if TYPE_CHECKING:  # pragma: no cover
    from .client import Gagarin


class Platform:
    """Whether gagarin itself is working."""

    def __init__(self, client: Gagarin):
        self._c = client

    def health(self) -> Health:
        """The platform's health. An unhealthy platform is an answer, not an
        exception: read ``ok`` and each check's ``detail``. An exception means
        the API could not be asked at all."""
        status, raw = self._c.send("GET", "/healthz/platform")
        if status in (200, 503):
            try:
                return json.loads(raw)
            except ValueError:
                pass
        raise api_error(status, raw)
