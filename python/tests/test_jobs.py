import json
import os
import unittest
from unittest import mock
from urllib.parse import parse_qs, urlparse

from gagarin import Gagarin, GagarinError


class FakeAPI:
    """Just enough of the gagarin API to submit a job and watch it run."""

    def __init__(self, phases=("pending",)):
        self.phases = list(phases)
        self.put = None
        self.auth = None
        self.statuses = 0
        self.client = Gagarin(token="tok", base_url="http://api.test/", registry="registry.example", transport=self)

    def __call__(self, method, url, headers, body):
        u = urlparse(url)
        self.auth = headers["Authorization"]
        route = f"{method} {u.path}"
        ok = lambda d: (200, json.dumps(d).encode())
        if route == "GET /v1/projects/shop":
            return ok({"id": "p-123", "name": "shop"})
        if route == "PUT /v1/projects/shop/services/migrate":
            self.put = json.loads(body)
            if self.put.get("size") == "xl":
                return 400, json.dumps({"error": {"code": "invalid_size", "message": "nope", "fix_hint": "s, m, l"}}).encode()
            return ok({"project": "p-123", "name": "migrate", "kind": "job", "image": self.put["image"], "revision": 7})
        if route == "GET /v1/projects/shop/status":
            i = min(self.statuses, len(self.phases) - 1)
            self.statuses += 1
            run = {"revision": 7, "phase": self.phases[i]}
            if self.phases[i] == "done":
                run["exit_code"] = 0
            if self.phases[i] == "failed":
                run["exit_code"] = 3
            if i == 0:
                run = {"revision": 6, "phase": "done", "exit_code": 0}  # the previous run
            return ok({"services": [
                {"name": "web", "kind": "container"},
                {"name": "migrate", "kind": "job", "actual": {"exists": True, "run": run}},
            ]})
        if route == "GET /v1/projects/shop/services/migrate/logs":
            return ok({"logs": f"q={parse_qs(u.query)['q'][0]}\n", "source": "store", "next": None})
        return 404, json.dumps({"error": {"code": "no_such_route", "message": route}}).encode()


class JobsTest(unittest.TestCase):
    def test_run_sends_every_setting(self):
        f = FakeAPI()
        sub = f.client.jobs.run(
            "shop", "migrate", image="migrate:v3", digest="sha256:abc", env={"A": "1"}, deps=["db"],
            size="m", timeout_seconds=600, schedule="@daily", time_zone="Europe/Berlin",
        )
        self.assertEqual(sub.revision, 7)
        self.assertEqual(f.put, {
            "kind": "job", "image": "registry.example/p-123/migrate:v3", "env": {"A": "1"},
            "digest": "sha256:abc", "deps": ["db"], "size": "m", "timeout_seconds": 600,
            "schedule": "@daily", "time_zone": "Europe/Berlin",
        })
        self.assertEqual(f.auth, "Bearer tok")

    def test_run_omits_unset_settings(self):
        f = FakeAPI()
        f.client.jobs.run("shop", "migrate", image="registry.gagarin.cloud/p-123/m:1")
        self.assertEqual(f.put, {"kind": "job", "image": "registry.gagarin.cloud/p-123/m:1", "env": {}})

    def test_run_and_wait_follows_the_revision(self):
        f = FakeAPI(["", "pending", "running", "failed"])
        seen = []
        res = f.client.jobs.run_and_wait("shop", "migrate", image="m:1", poll_interval=0.001,
                                         on_update=lambda r: seen.append(r.phase))
        self.assertEqual(res.run.revision, 7)
        self.assertEqual(res.exit_code, 3)
        self.assertEqual(seen, ["pending", "running", "failed"])

    def test_run_and_wait_succeeds(self):
        f = FakeAPI(["", "done"])
        res = f.client.jobs.run_and_wait("shop", "migrate", image="m:1", poll_interval=0.001)
        self.assertTrue(res.run.succeeded)
        self.assertEqual(res.exit_code, 0)

    def test_run_and_wait_refuses_a_schedule(self):
        with self.assertRaises(ValueError):
            FakeAPI().client.jobs.run_and_wait("shop", "migrate", image="m:1", schedule="@daily")

    def test_api_errors_are_typed(self):
        with self.assertRaises(GagarinError) as cm:
            FakeAPI().client.jobs.run("shop", "migrate", image="m:1", size="xl")
        self.assertEqual((cm.exception.status, cm.exception.code, cm.exception.fix_hint), (400, "invalid_size", "s, m, l"))

    def test_get_refuses_a_service(self):
        with self.assertRaises(GagarinError) as cm:
            FakeAPI().client.jobs.get("shop", "web")
        self.assertEqual(cm.exception.code, "not_a_job")

    def test_logs(self):
        self.assertEqual(FakeAPI().client.jobs.logs("shop", "migrate", contains="boom")["logs"], "q=boom\n")

    def test_a_token_is_required(self):
        with mock.patch.dict(os.environ, {}, clear=True):
            with self.assertRaises(ValueError):
                Gagarin()


if __name__ == "__main__":
    unittest.main()
