import unittest

from fake_api import FakeAPITestCase

from gagarin import GagarinError


class ProjectsTest(FakeAPITestCase):
    def test_list(self):
        ps = self.gg.projects.list()
        self.assertEqual([(p["id"], p["role"]) for p in ps], [("p-123", "owner"), ("p-456", "viewer")])
        self.assertEqual(ps[0]["cluster"], "fr-par-1")
        self.assertTrue(ps[1]["suspended_reason"])

    def test_get_by_name_or_id(self):
        for ref in ("shop", "p-123"):
            p = self.gg.projects.get(ref)
            self.assertEqual(self.api.last()["path"], f"/v1/projects/{ref}")
            self.assertEqual(p["id"], "p-123")
        with self.assertRaises(GagarinError) as cm:
            self.gg.projects.get("nope")
        self.assertEqual((cm.exception.status, cm.exception.code), (404, "not_found"))

    def test_a_slash_in_a_name_never_becomes_a_path_segment(self):
        with self.assertRaises(GagarinError):
            self.gg.projects.get("a/b")
        self.assertEqual(self.api.last()["escaped"], "/v1/projects/a%2Fb")

    def test_create(self):
        p = self.gg.projects.create("fresh")
        r = self.api.last()
        self.assertEqual((r["method"], r["path"], r["content_type"]), ("POST", "/v1/projects", "application/json"))
        self.assertEqual(r["body"], {"name": "fresh"})
        self.assertEqual((p["id"], p["name"]), ("p-789", "fresh"))
        with self.assertRaises(GagarinError) as cm:
            self.gg.projects.create("shop")
        self.assertEqual((cm.exception.status, cm.exception.code), (409, "project_exists"))

    def test_status(self):
        st = self.gg.projects.status("shop")
        self.assertEqual(self.api.last()["path"], "/v1/projects/shop/status")
        self.assertEqual(st["project_id"], "p-123")
        self.assertEqual(st["cluster"], "fr-par-1")
        self.assertEqual(len(st["services"]), 4)
        self.assertTrue(st["platform"]["converging"])
        self.assertEqual(st["usage_today"]["micro_usd"], 141467)
        web = next(s for s in st["services"] if s["name"] == "web")
        self.assertEqual(web["actual"]["ready_replicas"], 1)
        self.assertTrue(web["domains"][-1]["generated"])
        run = next(s for s in st["services"] if s["name"] == "migrate")["actual"]["run"]
        self.assertEqual((run["revision"], run["phase"], run["exit_code"]), (7, "failed", 3))

    def test_service_status(self):
        db = self.gg.projects.service_status("shop", "db")
        self.assertEqual(db["kind"], "postgres")
        self.assertEqual(db["volume_usage"]["capacity_bytes"], 10737418240)
        with self.assertRaises(GagarinError) as cm:
            self.gg.projects.service_status("shop", "nope")
        self.assertEqual(cm.exception.code, "not_found")


if __name__ == "__main__":
    unittest.main()
