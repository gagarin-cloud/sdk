import unittest

from fake_api import FakeAPITestCase


class ServicesTest(FakeAPITestCase):
    def test_logs_sends_every_filter(self):
        logs = self.gg.services.logs(
            "shop", "migrate", limit=50, since="6h", until="2026-10-10T09:00:00Z", contains="error", previous=True
        )
        self.assertEqual(
            self.api.last()["query"],
            {"limit": "50", "since": "6h", "until": "2026-10-10T09:00:00Z", "q": "error", "previous": "true"},
        )
        self.assertEqual(logs["source"], "store")
        self.assertEqual(logs["next"], "2026-10-10T09:00:00Z")
        self.assertIn("already exists", logs["logs"])

    def test_logs_with_no_filters_sends_no_query(self):
        self.gg.services.logs("shop", "migrate")
        self.assertEqual(self.api.last()["query"], {})

    def test_deployments(self):
        ds = self.gg.services.deployments("shop", "migrate")
        self.assertEqual(self.api.last()["path"], "/v1/projects/shop/services/migrate/deployments")
        self.assertEqual([d["revision"] for d in ds], [7, 6])
        self.assertEqual(ds[1]["restored_from"], 4)


if __name__ == "__main__":
    unittest.main()
