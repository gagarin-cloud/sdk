from __future__ import annotations

from typing import TYPE_CHECKING

from .types import Billing, Whoami

if TYPE_CHECKING:  # pragma: no cover
    from .client import Gagarin


class Account:
    """The account the credential belongs to."""

    def __init__(self, client: Gagarin):
        self._c = client

    def whoami(self) -> Whoami:
        """Which account this credential acts as, and what it may do
        (``can`` holds ``"deploy"`` and/or ``"destroy"``)."""
        return self._c.request("GET", "/v1/whoami")

    def billing(self) -> Billing:
        """The account's balance, burn and runway, in micro-USD."""
        return self._c.request("GET", "/v1/billing")
