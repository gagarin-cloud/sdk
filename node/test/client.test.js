import { test } from "node:test";
import assert from "node:assert/strict";
import { createServer } from "node:http";
import { Gagarin, GagarinError } from "../dist/index.js";
import { startFake } from "./fake-api.js";

function withEnv(vars, fn) {
  const prev = {};
  for (const k of Object.keys(vars)) {
    prev[k] = process.env[k];
    if (vars[k] === undefined) delete process.env[k];
    else process.env[k] = vars[k];
  }
  try {
    return fn();
  } finally {
    for (const k of Object.keys(prev)) {
      if (prev[k] === undefined) delete process.env[k];
      else process.env[k] = prev[k];
    }
  }
}

test("a token is required", () => {
  withEnv({ GAGARIN_TOKEN: undefined }, () => assert.throws(() => new Gagarin(), /GAGARIN_TOKEN/));
});

test("the client reads the environment", async (t) => {
  const f = await startFake(t);
  const gg = withEnv(
    { GAGARIN_TOKEN: "tok", GAGARIN_API: f.url, GAGARIN_REGISTRY: "registry.from-env" },
    () => new Gagarin(),
  );
  await gg.account.whoami();
  assert.equal(f.last().path, "/v1/whoami");
  assert.equal(await gg.imageRef("shop", "migrate:v3"), "registry.from-env/p-123/migrate:v3");
});

test("every request carries the token and identifies the SDK", async (t) => {
  const f = await startFake(t);
  await f.client.projects.list();
  assert.equal(f.last().auth, "Bearer tok");
  assert.equal(f.last().userAgent, "gagarin-sdk-node");
});

test("a refusal is a typed error", async (t) => {
  const f = await startFake(t);
  const gg = new Gagarin({ token: "wrong", baseUrl: f.url });
  await assert.rejects(gg.account.whoami(), (e) => {
    assert.ok(e instanceof GagarinError);
    assert.equal(e.status, 401);
    assert.equal(e.code, "unauthorized");
    assert.ok(e.fixHint);
    assert.match(e.message, /^\[unauthorized\] .*\nhint: /);
    return true;
  });
});

test("a failure without the envelope still carries the status", async (t) => {
  const server = createServer((req, res) => (res.writeHead(502), res.end("bad gateway from a proxy")));
  await new Promise((r) => server.listen(0, "127.0.0.1", r));
  t.after(() => server.close());
  const gg = new Gagarin({ token: "tok", baseUrl: `http://127.0.0.1:${server.address().port}` });
  await assert.rejects(gg.projects.list(), { code: "http_502", status: 502, message: "[http_502] bad gateway from a proxy" });
});

test("an unreachable API rejects", async () => {
  const gg = new Gagarin({ token: "tok", baseUrl: "http://127.0.0.1:1" });
  await assert.rejects(gg.account.billing());
  await assert.rejects(gg.platform.health());
});

test("a custom fetch is used for every request", async (t) => {
  const f = await startFake(t);
  let n = 0;
  const gg = new Gagarin({ token: "tok", baseUrl: f.url, fetch: (...a) => (n++, fetch(...a)) });
  await gg.projects.list();
  await gg.account.whoami();
  assert.equal(n, 2);
});

test("imageRef leaves a full reference alone, without a request", async (t) => {
  const f = await startFake(t);
  assert.equal(await f.client.imageRef("shop", "registry.gagarin.cloud/p-123/m:1"), "registry.gagarin.cloud/p-123/m:1");
  assert.equal(f.requests.length, 0);
});

test("imageRef of an unknown project rejects", async (t) => {
  const f = await startFake(t);
  await assert.rejects(f.client.imageRef("nope", "m:1"), { code: "not_found" });
});
