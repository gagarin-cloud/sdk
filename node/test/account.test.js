import { test } from "node:test";
import assert from "node:assert/strict";
import { startFake } from "./fake-api.js";

test("whoami", async (t) => {
  const f = await startFake(t);
  const w = await f.client.account.whoami();
  assert.deepEqual([f.last().method, f.last().path], ["GET", "/v1/whoami"]);
  assert.equal(w.account, "dev@example.com");
  assert.equal(w.registry, "registry.example");
  assert.equal(w.platform, "linux/amd64");
  assert.deepEqual(w.can, ["deploy"]);
});

test("billing", async (t) => {
  const f = await startFake(t);
  const b = await f.client.account.billing();
  assert.equal(f.last().path, "/v1/billing");
  assert.equal(b.micro_usd, 12345678);
  assert.equal(b.balance, "$12.35");
  assert.equal(b.runway_hours, 440.9);
  assert.equal(b.services, 2);
});
