import unittest

from fake_api import FakeAPITestCase
from test_client import plain_server

from gagarin import Gagarin, GagarinError


class PlatformTest(FakeAPITestCase):
    def test_healthy(self):
        h = self.gg.platform.health()
        self.assertEqual(self.api.last()["path"], "/healthz/platform")
        self.assertTrue(h["ok"])
        self.assertTrue(h["cluster"]["ok"])

    def test_unhealthy_is_an_answer_not_an_exception(self):
        self.api.health_down = True
        h = self.gg.platform.health()
        self.assertFalse(h["ok"])
        self.assertFalse(h["reconciler"]["ok"])
        self.assertTrue(h["reconciler"]["detail"])

    def test_health_from_something_that_is_not_gagarin_raises(self):
        s = plain_server(502, "upstream connect error")
        self.addCleanup(s.server_close)
        self.addCleanup(s.shutdown)
        with self.assertRaises(GagarinError) as cm:
            Gagarin(token="tok", base_url=f"http://127.0.0.1:{s.server_port}").platform.health()
        self.assertEqual(cm.exception.code, "http_502")


if __name__ == "__main__":
    unittest.main()
