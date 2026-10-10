import { test } from "node:test";
import assert from "node:assert/strict";
import { startFake } from "./fake-api.js";

test("list", async (t) => {
  const f = await startFake(t);
  const ps = await f.client.projects.list();
  assert.deepEqual(ps.map((p) => [p.id, p.role]), [["p-123", "owner"], ["p-456", "viewer"]]);
  assert.ok(ps[1].suspended_reason);
});

test("get by name or id", async (t) => {
  const f = await startFake(t);
  for (const ref of ["shop", "p-123"]) {
    const p = await f.client.projects.get(ref);
    assert.equal(f.last().path, `/v1/projects/${ref}`);
    assert.equal(p.id, "p-123");
  }
  await assert.rejects(f.client.projects.get("nope"), { code: "not_found", status: 404 });
});

test("a slash in a name never becomes a path segment", async (t) => {
  const f = await startFake(t);
  await f.client.projects.get("a/b").catch(() => {});
  assert.equal(f.last().escaped, "/v1/projects/a%2Fb");
});

test("create", async (t) => {
  const f = await startFake(t);
  const p = await f.client.projects.create("fresh");
  const r = f.last();
  assert.deepEqual([r.method, r.path, r.contentType], ["POST", "/v1/projects", "application/json"]);
  assert.deepEqual(r.body, { name: "fresh" });
  assert.deepEqual([p.id, p.name], ["p-789", "fresh"]);
  await assert.rejects(f.client.projects.create("shop"), { code: "project_exists", status: 409 });
});

test("status", async (t) => {
  const f = await startFake(t);
  const st = await f.client.projects.status("shop");
  assert.equal(f.last().path, "/v1/projects/shop/status");
  assert.equal(st.project_id, "p-123");
  assert.equal(st.services.length, 4);
  assert.equal(st.platform.converging, true);
  assert.equal(st.usage_today.micro_usd, 141467);
  const web = st.services.find((s) => s.name === "web");
  assert.equal(web.actual.ready_replicas, 1);
  assert.equal(web.domains.at(-1).generated, true);
  const migrate = st.services.find((s) => s.name === "migrate");
  assert.deepEqual([migrate.actual.run.revision, migrate.actual.run.phase, migrate.actual.run.exit_code], [7, "failed", 3]);
});

test("serviceStatus", async (t) => {
  const f = await startFake(t);
  const db = await f.client.projects.serviceStatus("shop", "db");
  assert.equal(db.kind, "postgres");
  assert.equal(db.volume_usage.capacity_bytes, 10737418240);
  await assert.rejects(f.client.projects.serviceStatus("shop", "nope"), { code: "not_found" });
});
