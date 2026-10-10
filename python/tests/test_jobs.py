import threading
import unittest

from fake_api import FakeAPITestCase, run

from gagarin import GagarinError, SupersededError, SuspendedError, WaitTimeout

FAST = {"poll_interval": 0.001}


class RunTest(FakeAPITestCase):
    def test_run_sends_every_setting(self):
        sub = self.gg.jobs.run(
            "shop", "migrate", image="migrate:v3", digest="sha256:abc", env={"A": "1"}, deps=["db"],
            size="m", timeout_seconds=600, schedule="@daily", time_zone="Europe/Berlin",
        )
        r = self.api.last()
        self.assertEqual((r["method"], r["path"], r["content_type"]), ("PUT", "/v1/projects/shop/services/migrate", "application/json"))
        self.assertEqual(r["body"], {
            "kind": "job", "image": "registry.example/p-123/migrate:v3", "env": {"A": "1"},
            "digest": "sha256:abc", "deps": ["db"], "size": "m", "timeout_seconds": 600,
            "schedule": "@daily", "time_zone": "Europe/Berlin",
        })
        self.assertEqual((sub.revision, sub.kind, sub.schedule, sub.size), (8, "job", "@daily", "m"))

    def test_run_omits_unset_settings_but_always_sends_env(self):
        self.gg.jobs.run("shop", "migrate", image="registry.gagarin.cloud/p-123/m:1")
        self.assertEqual(self.api.last()["body"], {"kind": "job", "image": "registry.gagarin.cloud/p-123/m:1", "env": {}})
        self.assertEqual(self.api.count("GET", "/v1/projects/shop"), 0)

    def test_run_needs_an_image_and_sends_nothing_without_one(self):
        with self.assertRaises(ValueError):
            self.gg.jobs.run("shop", "migrate", image="")
        self.assertEqual(self.api.requests, [])

    def test_a_refused_run_is_typed(self):
        with self.assertRaises(GagarinError) as cm:
            self.gg.jobs.run("shop", "migrate", image="m:1", size="xl")
        self.assertEqual((cm.exception.status, cm.exception.code), (400, "invalid_size"))
        self.assertTrue(cm.exception.fix_hint)

    def test_each_run_is_a_new_revision(self):
        a = self.gg.jobs.run("shop", "migrate", image="m:1")
        b = self.gg.jobs.run("shop", "migrate", image="m:1")
        self.assertEqual(b.revision, a.revision + 1)

    def test_get_reads_the_jobs_row_and_refuses_what_is_not_a_job(self):
        j = self.gg.jobs.get("shop", "migrate")
        self.assertEqual((j["actual"]["run"]["revision"], j["actual"]["run"]["phase"]), (7, "failed"))
        for name, code in (("web", "not_a_job"), ("nope", "not_found")):
            with self.assertRaises(GagarinError) as cm:
                self.gg.jobs.get("shop", name)
            self.assertEqual(cm.exception.code, code)

    def test_logs(self):
        logs = self.gg.jobs.logs("shop", "migrate", contains="error")
        self.assertEqual(self.api.last()["query"], {"q": "error"})
        self.assertTrue(logs["logs"])


class WaitTest(FakeAPITestCase):
    def test_ignores_the_previous_run_and_reports_each_phase_once(self):
        self.api.runs = [run(7, "done", 0), run(8, "pending"), run(8, "running"), run(8, "running"), run(8, "failed", 3)]
        seen = []
        r = self.gg.jobs.wait("shop", "migrate", 8, on_update=lambda x: seen.append(x.phase), **FAST)
        self.assertEqual((r.revision, r.phase, r.exit_code, r.finished, r.succeeded), (8, "failed", 3, True, False))
        self.assertEqual(seen, ["pending", "running", "failed"])

    def test_with_no_run_yet(self):
        self.api.runs = [None, None, run(8, "done", 0)]
        self.assertTrue(self.gg.jobs.wait("shop", "migrate", 8, **FAST).succeeded)

    def test_rides_out_transient_failures(self):
        self.api.status_failures = 3
        self.api.runs = [run(8, "done", 0)]
        self.assertEqual(self.gg.jobs.wait("shop", "migrate", 8, **FAST).phase, "done")

    def test_gives_up_on_persistent_failures_after_five_retries(self):
        self.api.status_failures = 100
        with self.assertRaises(GagarinError) as cm:
            self.gg.jobs.wait("shop", "migrate", 8, **FAST)
        self.assertEqual(cm.exception.code, "cluster_error")
        self.assertEqual(self.api.count("GET", "/v1/projects/shop/status"), 6)

    def test_does_not_retry_a_refusal(self):
        with self.assertRaises(GagarinError) as cm:
            self.gg.jobs.wait("shop", "web", 8, **FAST)
        self.assertEqual(cm.exception.code, "not_a_job")
        self.assertEqual(self.api.count("GET", "/v1/projects/shop/status"), 1)

    def test_superseded(self):
        self.api.runs = [run(9, "running")]
        with self.assertRaises(SupersededError):
            self.gg.jobs.wait("shop", "migrate", 8, **FAST)

    def test_suspended(self):
        self.api.runs = [run(8, "suspended")]
        with self.assertRaises(SuspendedError):
            self.gg.jobs.wait("shop", "migrate", 8, **FAST)

    def test_times_out(self):
        self.api.runs = [run(8, "running")]
        with self.assertRaises(WaitTimeout):
            self.gg.jobs.wait("shop", "migrate", 8, poll_interval=0.005, timeout=0.03)

    def test_an_unreachable_api_is_retried_then_raised(self):
        from gagarin import Gagarin

        gg = Gagarin(token="tok", base_url="http://127.0.0.1:1")
        with self.assertRaises(OSError):
            gg.jobs.wait("shop", "migrate", 8, **FAST)


class RunAndWaitTest(FakeAPITestCase):
    def check(self, last, exit_code):
        self.api.runs = [run(7, "done", 0), run(8, "running"), last]
        res = self.gg.jobs.run_and_wait("shop", "migrate", image="m:1", **FAST)
        self.assertEqual((res.submission.revision, res.run.revision, res.exit_code), (8, 8, exit_code))

    def test_exits_zero(self):
        self.check(run(8, "done", 0), 0)

    def test_exits_with_the_scripts_code(self):
        self.check(run(8, "failed", 3), 3)

    def test_stopped_without_a_code(self):
        self.check(run(8, "failed"), 1)

    def test_refuses_a_schedule_and_submits_nothing(self):
        with self.assertRaises(ValueError):
            self.gg.jobs.run_and_wait("shop", "migrate", image="m:1", schedule="@daily")
        self.assertEqual(self.api.requests, [])

    def test_passes_the_refusal_through(self):
        with self.assertRaises(GagarinError) as cm:
            self.gg.jobs.run_and_wait("shop", "migrate", image="m:1", size="xl", **FAST)
        self.assertEqual(cm.exception.code, "invalid_size")


if __name__ == "__main__":
    unittest.main()
