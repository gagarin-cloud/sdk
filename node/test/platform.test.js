import { test } from "node:test";
import assert from "node:assert/strict";
import { createServer } from "node:http";
import { Gagarin } from "../dist/index.js";
import { startFake } from "./fake-api.js";

test("healthy", async (t) => {
  const f = await startFake(t);
  const h = await f.client.platform.health();
  assert.equal(f.last().path, "/healthz/platform");
  assert.equal(h.ok, true);
  assert.equal(h.cluster.ok, true);
});

test("unhealthy is an answer, not a rejection", async (t) => {
  const f = await startFake(t);
  f.healthDown = true;
  const h = await f.client.platform.health();
  assert.equal(h.ok, false);
  assert.equal(h.reconciler.ok, false);
  assert.ok(h.reconciler.detail);
});

test("health from something that is not gagarin rejects", async (t) => {
  const server = createServer((req, res) => (res.writeHead(502), res.end("upstream connect error")));
  await new Promise((r) => server.listen(0, "127.0.0.1", r));
  t.after(() => server.close());
  const gg = new Gagarin({ token: "tok", baseUrl: `http://127.0.0.1:${server.address().port}` });
  await assert.rejects(gg.platform.health(), { code: "http_502" });
});
