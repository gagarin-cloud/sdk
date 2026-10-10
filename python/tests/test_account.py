import unittest

from fake_api import FakeAPITestCase


class AccountTest(FakeAPITestCase):
    def test_whoami(self):
        w = self.gg.account.whoami()
        self.assertEqual((self.api.last()["method"], self.api.last()["path"]), ("GET", "/v1/whoami"))
        self.assertEqual(w["account"], "dev@example.com")
        self.assertEqual(w["registry"], "registry.example")
        self.assertEqual(w["platform"], "linux/amd64")
        self.assertEqual(w["can"], ["deploy"])

    def test_billing(self):
        b = self.gg.account.billing()
        self.assertEqual(self.api.last()["path"], "/v1/billing")
        self.assertEqual(b["micro_usd"], 12345678)
        self.assertEqual(b["balance"], "$12.35")
        self.assertEqual(b["runway_hours"], 440.9)
        self.assertEqual(b["services"], 2)


if __name__ == "__main__":
    unittest.main()
