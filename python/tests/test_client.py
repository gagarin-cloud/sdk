import os
import threading
import unittest
from http.server import BaseHTTPRequestHandler, HTTPServer
from unittest import mock

from fake_api import FakeAPITestCase

from gagarin import Gagarin, GagarinError


def plain_server(status, text):
    """A server that is not gagarin: it answers everything with one status and text."""

    class H(BaseHTTPRequestHandler):
        def log_message(self, *a):
            pass

        def do_GET(self):
            b = text.encode()
            self.send_response(status)
            self.send_header("Content-Length", str(len(b)))
            self.end_headers()
            self.wfile.write(b)

    s = HTTPServer(("127.0.0.1", 0), H)
    threading.Thread(target=s.serve_forever, args=(0.01,), daemon=True).start()
    return s


class ClientTest(FakeAPITestCase):
    def test_a_token_is_required(self):
        with mock.patch.dict(os.environ, {}, clear=True):
            with self.assertRaisesRegex(ValueError, "GAGARIN_TOKEN"):
                Gagarin()

    def test_the_client_reads_the_environment(self):
        env = {"GAGARIN_TOKEN": "tok", "GAGARIN_API": self.api.url, "GAGARIN_REGISTRY": "registry.from-env"}
        with mock.patch.dict(os.environ, env, clear=True):
            gg = Gagarin()
        gg.account.whoami()
        self.assertEqual(self.api.last()["path"], "/v1/whoami")
        self.assertEqual(gg.image_ref("shop", "migrate:v3"), "registry.from-env/p-123/migrate:v3")

    def test_every_request_carries_the_token_and_identifies_the_sdk(self):
        self.gg.projects.list()
        self.assertEqual(self.api.last()["auth"], "Bearer tok")
        self.assertEqual(self.api.last()["user_agent"], "gagarin-sdk-python")

    def test_a_refusal_is_a_typed_error(self):
        gg = Gagarin(token="wrong", base_url=self.api.url)
        with self.assertRaises(GagarinError) as cm:
            gg.account.whoami()
        e = cm.exception
        self.assertEqual((e.status, e.code), (401, "unauthorized"))
        self.assertTrue(e.fix_hint)
        self.assertRegex(str(e), r"^\[unauthorized\] .*\nhint: ")

    def test_a_failure_without_the_envelope_still_carries_the_status(self):
        s = plain_server(502, "bad gateway from a proxy")
        self.addCleanup(s.server_close)
        self.addCleanup(s.shutdown)
        with self.assertRaises(GagarinError) as cm:
            Gagarin(token="tok", base_url=f"http://127.0.0.1:{s.server_port}").projects.list()
        self.assertEqual((cm.exception.status, cm.exception.code), (502, "http_502"))
        self.assertEqual(cm.exception.message, "bad gateway from a proxy")

    def test_an_unreachable_api_raises(self):
        gg = Gagarin(token="tok", base_url="http://127.0.0.1:1")
        with self.assertRaises(OSError):
            gg.account.billing()
        with self.assertRaises(OSError):
            gg.platform.health()

    def test_a_custom_transport_is_used_for_every_request(self):
        seen = []

        def transport(method, url, headers, body):
            seen.append((method, url))
            return 200, b'{"projects": []}'

        gg = Gagarin(token="tok", base_url="http://example.invalid", transport=transport)
        self.assertEqual(gg.projects.list(), [])
        self.assertEqual(seen, [("GET", "http://example.invalid/v1/projects")])

    def test_image_ref_leaves_a_full_reference_alone_without_a_request(self):
        self.assertEqual(self.gg.image_ref("shop", "registry.gagarin.cloud/p-123/m:1"), "registry.gagarin.cloud/p-123/m:1")
        self.assertEqual(self.api.requests, [])

    def test_image_ref_of_an_unknown_project_raises(self):
        with self.assertRaises(GagarinError) as cm:
            self.gg.image_ref("nope", "m:1")
        self.assertEqual(cm.exception.code, "not_found")


if __name__ == "__main__":
    unittest.main()
