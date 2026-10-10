"""Drives a real gagarin API. Skipped unless GAGARIN_INTEGRATION=1; see
../../scripts/integration.sh for what it needs."""

import os
import time
import unittest

from gagarin import FAILED, Gagarin, GagarinError

ENABLED = os.environ.get("GAGARIN_INTEGRATION") == "1"
WAIT = {"poll_interval": 1.0}


def env(k):
    v = os.environ.get(k)
    if not v:
        raise AssertionError(f"{k} is not set")
    return v


@unittest.skipUnless(ENABLED, "set GAGARIN_INTEGRATION=1 to run against a real API")
class IntegrationTest(unittest.TestCase):
    """In order: each step builds on the one before."""

    def test_integration(self):
        project = env("GAGARIN_IT_PROJECT")
        image_ok = env("GAGARIN_IT_IMAGE_OK")
        image_fail = env("GAGARIN_IT_IMAGE_FAIL")
        gg = Gagarin()

        with self.subTest("platform health"):
            self.assertTrue(gg.platform.health()["serving"]["ok"])

        with self.subTest("whoami"):
            w = gg.account.whoami()
            self.assertTrue(w["account"])
            self.assertTrue(w["registry"])
            self.assertIn("deploy", w["can"])

        with self.subTest("projects"):
            try:
                gg.projects.create(project)
            except GagarinError as e:
                self.assertEqual(e.code, "project_exists")
            with self.assertRaises(GagarinError) as cm:
                gg.projects.create(project)
            self.assertEqual(cm.exception.code, "project_exists")
            p = gg.projects.get(project)
            self.assertEqual(p["name"], project)
            self.assertEqual(gg.projects.get(p["id"])["name"], project)
            self.assertIn(p["id"], [q["id"] for q in gg.projects.list()])
            with self.assertRaises(GagarinError):
                gg.projects.get("no-such-project-here")

        with self.subTest("a job that succeeds"):
            res = gg.jobs.run_and_wait(
                project, "it-py-ok", image=image_ok, env={"GREETING": "from-python"}, size="s", timeout_seconds=300, **WAIT
            )
            self.assertEqual(res.exit_code, 0)
            self.assertTrue(res.run.succeeded)
            self.assertEqual(res.run.revision, res.submission.revision)
            row = gg.jobs.get(project, "it-py-ok")
            self.assertEqual(row["timeout_seconds"], 300)
            self.assertIn("GREETING", row["env_keys"])
            self.assertEqual(gg.services.deployments(project, "it-py-ok")[0]["revision"], res.submission.revision)
            logs = ""
            for _ in range(30):
                try:
                    logs = gg.jobs.logs(project, "it-py-ok")["logs"]
                except GagarinError:
                    logs = ""
                if "hello" in logs:
                    break
                time.sleep(1)
            self.assertIn("hello", logs)

        with self.subTest("a job that fails with its own code"):
            res = gg.jobs.run_and_wait(project, "it-py-fail", image=image_fail, **WAIT)
            self.assertEqual((res.run.phase, res.exit_code), (FAILED, 3))

        with self.subTest("status reports the jobs"):
            st = gg.projects.status(project)
            for name in ("it-py-ok", "it-py-fail"):
                row = next(s for s in st["services"] if s["name"] == name)
                self.assertEqual(row["kind"], "job")
                self.assertIn(row["actual"]["run"]["phase"], ("done", "failed"))

        with self.subTest("refusals are typed"):
            with self.assertRaises(GagarinError) as cm:
                gg.jobs.run(project, "it-py-ok", image=image_ok, size="xl")
            self.assertEqual((cm.exception.status, cm.exception.code), (400, "invalid_size"))
            with self.assertRaises(GagarinError) as cm:
                gg.jobs.run(project, "it-py-ok", image=image_ok, timeout_seconds=99999)
            self.assertEqual(cm.exception.code, "invalid_timeout")


if __name__ == "__main__":
    unittest.main()
