from __future__ import annotations

import json


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


def api_error(status: int, raw: bytes) -> GagarinError:
    """The error for a refusal, from the API's envelope when there is one."""
    err = None
    try:
        err = json.loads(raw).get("error")
    except (ValueError, AttributeError):
        pass
    if isinstance(err, dict) and err.get("code"):
        return GagarinError(status, err["code"], err.get("message", ""), err.get("fix_hint"))
    return GagarinError(status, f"http_{status}", raw.decode(errors="replace").strip())
