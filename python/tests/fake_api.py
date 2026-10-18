"""A local HTTP server that answers like the gagarin API, from the fixtures in
../../testdata/api, and records every request it was sent."""

from __future__ import annotations

import json
import threading
import unittest
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
from urllib.parse import parse_qs, unquote, urlparse

from gagarin import Gagarin

FIXTURES = Path(__file__).resolve().parents[2] / "testdata" / "api"


def fixture(name: str) -> bytes:
    return (FIXTURES / name).read_bytes()


def run(revision, phase, exit_code=None):
    r = {"revision": revision, "phase": phase}
    if exit_code is not None:
        r["exit_code"] = exit_code
    return r


class FakeAPI:
    def __init__(self):
        self.requests = []
        self.runs = None  # successive status reads' run for migrate; the last repeats
        self.status_failures = 0
        self.health_down = False
        self.next_revision = 8
        self.lock = threading.Lock()
        fake = self

        class Handler(BaseHTTPRequestHandler):
            def log_message(self, *args):
                pass

            def handle_any(self):
                with fake.lock:
                    fake.serve(self)

            do_GET = do_POST = do_PUT = do_DELETE = handle_any

        self.server = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
        self.url = f"http://127.0.0.1:{self.server.server_port}"
        threading.Thread(target=self.server.serve_forever, args=(0.01,), daemon=True).start()
        self.client = Gagarin(token="tok", base_url=self.url + "/", registry="registry.example")

    def close(self):
        self.server.shutdown()
        self.server.server_close()

    def last(self):
        assert self.requests, "no request was made"
        return self.requests[-1]

    def count(self, method, path):
        return sum(1 for r in self.requests if r["method"] == method and r["path"] == path)

    def serve(self, h: BaseHTTPRequestHandler):
        u = urlparse(h.path)
        n = int(h.headers.get("Content-Length") or 0)
        raw = h.rfile.read(n) if n else b""
        rec = {
            "method": h.command,
            "path": unquote(u.path),
            "escaped": u.path,
            "query": {k: v[0] for k, v in parse_qs(u.query).items()},
            "auth": h.headers.get("Authorization"),
            "content_type": h.headers.get("Content-Type"),
            "user_agent": h.headers.get("User-Agent"),
            "body": json.loads(raw) if raw else None,
        }
        self.requests.append(rec)

        def reply(status, body):
            if not isinstance(body, bytes):
                body = json.dumps(body).encode()
            h.send_response(status)
            h.send_header("Content-Type", "application/json")
            h.send_header("Content-Length", str(len(body)))
            h.end_headers()
            h.wfile.write(body)

        def fail(status, code, message):
            reply(status, {"error": {"code": code, "message": message}})

        if h.command == "GET" and u.path == "/healthz/platform":
            return reply(503, fixture("health_down.json")) if self.health_down else reply(200, fixture("health_ok.json"))
        if rec["auth"] != "Bearer tok":
            return reply(401, fixture("error_unauthorized.json"))

        route = f"{h.command} {u.path}"
        body = rec["body"] or {}
        if route == "GET /v1/whoami":
            return reply(200, fixture("whoami.json"))
        if route == "GET /v1/billing":
            return reply(200, fixture("billing.json"))
        if route == "GET /v1/projects":
            return reply(200, fixture("projects.json"))
        if route == "POST /v1/projects":
            if body.get("name") == "shop":
                return reply(409, fixture("error_project_exists.json"))
            return reply(201, {**json.loads(fixture("project_created.json")), "name": body["name"]})
        if route in ("GET /v1/projects/shop", "GET /v1/projects/p-123"):
            return reply(200, fixture("project.json"))
        if route == "GET /v1/projects/shop/status":
            if self.status_failures > 0:
                self.status_failures -= 1
                return fail(502, "cluster_error", "the cluster did not answer")
            st = json.loads(fixture("status.json"))
            if self.runs is not None:
                r = self.runs.pop(0) if len(self.runs) > 1 else self.runs[0]
                migrate = next(s for s in st["services"] if s["name"] == "migrate")
                if r is None:
                    migrate["actual"].pop("run", None)
                else:
                    migrate["actual"]["run"] = r
            return reply(200, st)
        if route == "GET /v1/projects/shop/services/migrate/deployments":
            return reply(200, fixture("deployments.json"))
        if route == "GET /v1/projects/shop/services/migrate/logs":
            return reply(200, fixture("logs.json"))
        if h.command == "PUT" and u.path.startswith("/v1/projects/shop/services/"):
            if body.get("size") == "xl":
                return reply(400, fixture("error_invalid_size.json"))
            out = {
                "project": "p-123", "name": u.path.rsplit("/", 1)[-1], "kind": body.get("kind"),
                "image": body.get("image"), "port": 0, "revision": self.next_revision,
                "created_at": "2026-09-01T12:00:00Z", "updated_at": "2026-10-10T10:00:00Z",
            }
            for k in ("size", "digest", "timeout_seconds", "ttl_seconds", "schedule", "time_zone"):
                if k in body:
                    out[k] = body[k]
            self.next_revision += 1
            return reply(200, out)
        if u.path.startswith("/v1/projects/"):
            return fail(404, "not_found", "no project or service by that name")
        return fail(404, "no_such_route", route)


class FakeAPITestCase(unittest.TestCase):
    def setUp(self):
        self.api = FakeAPI()
        self.addCleanup(self.api.close)
        self.gg = self.api.client
