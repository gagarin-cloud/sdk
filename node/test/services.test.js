import { test } from "node:test";
import assert from "node:assert/strict";
import { startFake } from "./fake-api.js";

test("logs sends every filter", async (t) => {
  const f = await startFake(t);
  const l = await f.client.services.logs("shop", "migrate", {
    limit: 50, since: "6h", until: "2026-10-10T09:00:00Z", contains: "error", previous: true,
  });
  assert.deepEqual(f.last().query, { limit: "50", since: "6h", until: "2026-10-10T09:00:00Z", q: "error", previous: "true" });
  assert.equal(l.source, "store");
  assert.equal(l.next, "2026-10-10T09:00:00Z");
  assert.match(l.logs, /already exists/);
});

test("logs with no filters sends no query", async (t) => {
  const f = await startFake(t);
  await f.client.services.logs("shop", "migrate");
  assert.deepEqual(f.last().query, {});
});

test("deployments", async (t) => {
  const f = await startFake(t);
  const ds = await f.client.services.deployments("shop", "migrate");
  assert.equal(f.last().path, "/v1/projects/shop/services/migrate/deployments");
  assert.deepEqual(ds.map((d) => d.revision), [7, 6]);
  assert.equal(ds[1].restored_from, 4);
});
