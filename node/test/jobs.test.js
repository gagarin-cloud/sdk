import { test } from "node:test";
import assert from "node:assert/strict";
import { startFake, run } from "./fake-api.js";

const fast = { pollIntervalMs: 1 };

test("run sends every setting", async (t) => {
  const f = await startFake(t);
  const sub = await f.client.jobs.run("shop", "migrate", {
    image: "migrate:v3", digest: "sha256:abc", env: { A: "1" }, deps: ["db"], size: "m",
    timeoutSeconds: 600, ttlSeconds: 3600, schedule: "@daily", timeZone: "Europe/Berlin",
  });
  const r = f.last();
  assert.deepEqual([r.method, r.path, r.contentType], ["PUT", "/v1/projects/shop/services/migrate", "application/json"]);
  assert.deepEqual(r.body, {
    kind: "job", image: "registry.example/p-123/migrate:v3", env: { A: "1" }, digest: "sha256:abc",
    deps: ["db"], size: "m", timeout_seconds: 600, ttl_seconds: 3600, schedule: "@daily", time_zone: "Europe/Berlin",
  });
  assert.equal(sub.revision, 8);
  assert.equal(sub.kind, "job");
  assert.equal(sub.schedule, "@daily");
  assert.equal(sub.ttl_seconds, 3600);
});

test("run sends the GPU size g and reads it back", async (t) => {
  const f = await startFake(t);
  const sub = await f.client.jobs.run("shop", "migrate", { image: "m:1", size: "g" });
  assert.deepEqual(f.last().body, { kind: "job", image: "registry.example/p-123/m:1", env: {}, size: "g" });
  assert.equal(sub.size, "g");
});

test("run omits unset settings but always sends env", async (t) => {
  const f = await startFake(t);
  await f.client.jobs.run("shop", "migrate", { image: "registry.gagarin.cloud/p-123/m:1" });
  assert.deepEqual(f.last().body, { kind: "job", image: "registry.gagarin.cloud/p-123/m:1", env: {} });
  assert.equal(f.count("GET", "/v1/projects/shop"), 0);
});

test("run needs an image, and sends nothing without one", async (t) => {
  const f = await startFake(t);
  await assert.rejects(f.client.jobs.run("shop", "migrate", {}), /image is required/);
  assert.equal(f.requests.length, 0);
});

test("a refused run is typed", async (t) => {
  const f = await startFake(t);
  await assert.rejects(f.client.jobs.run("shop", "migrate", { image: "m:1", size: "xl" }), {
    code: "invalid_size", status: 400,
  });
});

test("each run is a new revision", async (t) => {
  const f = await startFake(t);
  const a = await f.client.jobs.run("shop", "migrate", { image: "m:1" });
  const b = await f.client.jobs.run("shop", "migrate", { image: "m:1" });
  assert.equal(b.revision, a.revision + 1);
});

test("get reads the job's row, and refuses what is not a job", async (t) => {
  const f = await startFake(t);
  const j = await f.client.jobs.get("shop", "migrate");
  assert.deepEqual([j.actual.run.revision, j.actual.run.phase], [7, "failed"]);
  await assert.rejects(f.client.jobs.get("shop", "web"), { code: "not_a_job" });
  await assert.rejects(f.client.jobs.get("shop", "nope"), { code: "not_found" });
});

test("wait ignores the previous run and reports each phase once", async (t) => {
  const f = await startFake(t);
  f.runs = [run(7, "done", 0), run(8, "pending"), run(8, "running"), run(8, "running"), run(8, "failed", 3)];
  const seen = [];
  const r = await f.client.jobs.wait("shop", "migrate", 8, { ...fast, onUpdate: (x) => seen.push(x.phase) });
  assert.deepEqual([r.revision, r.phase, r.exit_code], [8, "failed", 3]);
  assert.deepEqual(seen, ["pending", "running", "failed"]);
});

test("wait with no run yet", async (t) => {
  const f = await startFake(t);
  f.runs = [null, null, run(8, "done", 0)];
  const r = await f.client.jobs.wait("shop", "migrate", 8, fast);
  assert.equal(r.phase, "done");
});

test("wait rides out transient failures", async (t) => {
  const f = await startFake(t);
  f.statusFailures = 3;
  f.runs = [run(8, "done", 0)];
  assert.equal((await f.client.jobs.wait("shop", "migrate", 8, fast)).phase, "done");
});

test("wait gives up on persistent failures after five retries", async (t) => {
  const f = await startFake(t);
  f.statusFailures = 100;
  await assert.rejects(f.client.jobs.wait("shop", "migrate", 8, fast), { code: "cluster_error" });
  assert.equal(f.count("GET", "/v1/projects/shop/status"), 6);
});

test("wait does not retry a refusal", async (t) => {
  const f = await startFake(t);
  await assert.rejects(f.client.jobs.wait("shop", "web", 8, fast), { code: "not_a_job" });
  assert.equal(f.count("GET", "/v1/projects/shop/status"), 1);
});

test("wait: superseded", async (t) => {
  const f = await startFake(t);
  f.runs = [run(9, "running")];
  await assert.rejects(f.client.jobs.wait("shop", "migrate", 8, fast), { code: "superseded" });
});

test("wait: suspended", async (t) => {
  const f = await startFake(t);
  f.runs = [run(8, "suspended")];
  await assert.rejects(f.client.jobs.wait("shop", "migrate", 8, fast), { code: "suspended" });
});

test("wait: times out", async (t) => {
  const f = await startFake(t);
  f.runs = [run(8, "running")];
  await assert.rejects(f.client.jobs.wait("shop", "migrate", 8, { pollIntervalMs: 5, timeoutMs: 30 }), {
    code: "wait_timeout",
  });
});

test("wait: honours an abort signal", async (t) => {
  const f = await startFake(t);
  f.runs = [run(8, "running")];
  const ac = new AbortController();
  setTimeout(() => ac.abort(), 20);
  await assert.rejects(f.client.jobs.wait("shop", "migrate", 8, { pollIntervalMs: 5, signal: ac.signal }), {
    name: "AbortError",
  });
});

for (const [name, last, exit] of [
  ["exits zero", run(8, "done", 0), 0],
  ["exits with the script's code", run(8, "failed", 3), 3],
  ["stopped without a code", run(8, "failed"), 1],
]) {
  test(`runAndWait: ${name}`, async (t) => {
    const f = await startFake(t);
    f.runs = [run(7, "done", 0), run(8, "running"), last];
    const res = await f.client.jobs.runAndWait("shop", "migrate", { image: "m:1" }, fast);
    assert.deepEqual([res.submission.revision, res.run.revision, res.exitCode], [8, 8, exit]);
  });
}

test("runAndWait refuses a schedule, and submits nothing", async (t) => {
  const f = await startFake(t);
  await assert.rejects(f.client.jobs.runAndWait("shop", "migrate", { image: "m:1", schedule: "@daily" }));
  assert.equal(f.requests.length, 0);
});

test("runAndWait passes the refusal through", async (t) => {
  const f = await startFake(t);
  await assert.rejects(f.client.jobs.runAndWait("shop", "migrate", { image: "m:1", size: "xl" }, fast), {
    code: "invalid_size",
  });
});

test("job logs", async (t) => {
  const f = await startFake(t);
  const l = await f.client.jobs.logs("shop", "migrate", { contains: "error" });
  assert.equal(f.last().query.q, "error");
  assert.ok(l.logs);
});
