import { test } from "node:test";
import assert from "node:assert/strict";
import { Gagarin, GagarinError } from "../dist/index.js";

// Just enough of the gagarin API to submit a job and watch it run.
function fakeApi(phases = ["pending"]) {
  const state = { put: undefined, auth: undefined, statuses: 0 };
  const json = (status, body) => new Response(JSON.stringify(body), { status });
  state.fetch = async (url, init) => {
    const u = new URL(url);
    state.auth = init.headers.authorization;
    const route = `${init.method} ${u.pathname}`;
    if (route === "GET /v1/projects/shop") return json(200, { id: "p-123", name: "shop" });
    if (route === "PUT /v1/projects/shop/services/migrate") {
      state.put = JSON.parse(init.body);
      if (state.put.size === "xl")
        return json(400, { error: { code: "invalid_size", message: "size \"xl\" is not one of s, m, l", fix_hint: "size is one of s, m, l" } });
      return json(200, { project: "p-123", name: "migrate", kind: "job", image: state.put.image, revision: 7 });
    }
    if (route === "GET /v1/projects/shop/status") {
      const i = Math.min(state.statuses++, phases.length - 1);
      let run = { revision: 7, phase: phases[i] };
      if (phases[i] === "done") run.exit_code = 0;
      if (phases[i] === "failed") run.exit_code = 3;
      if (i === 0) run = { revision: 6, phase: "done", exit_code: 0 }; // the previous run
      return json(200, {
        services: [
          { name: "web", kind: "container" },
          { name: "migrate", kind: "job", actual: { exists: true, run } },
        ],
      });
    }
    if (route === "GET /v1/projects/shop/services/migrate/logs")
      return json(200, { logs: `q=${u.searchParams.get("q")}\n`, source: "store", next: null });
    return json(404, { error: { code: "no_such_route", message: route } });
  };
  state.client = new Gagarin({ token: "tok", baseUrl: "http://api.test/", registry: "registry.example", fetch: state.fetch });
  return state;
}

test("run sends every setting", async () => {
  const f = fakeApi();
  const sub = await f.client.jobs.run("shop", "migrate", {
    image: "migrate:v3", digest: "sha256:abc", env: { A: "1" }, deps: ["db"], size: "m",
    timeoutSeconds: 600, schedule: "@daily", timeZone: "Europe/Berlin",
  });
  assert.equal(sub.revision, 7);
  assert.deepEqual(f.put, {
    kind: "job", image: "registry.example/p-123/migrate:v3", env: { A: "1" }, digest: "sha256:abc",
    deps: ["db"], size: "m", timeout_seconds: 600, schedule: "@daily", time_zone: "Europe/Berlin",
  });
  assert.equal(f.auth, "Bearer tok");
});

test("run omits unset settings", async () => {
  const f = fakeApi();
  await f.client.jobs.run("shop", "migrate", { image: "registry.gagarin.cloud/p-123/m:1" });
  assert.deepEqual(f.put, { kind: "job", image: "registry.gagarin.cloud/p-123/m:1", env: {} });
});

test("runAndWait follows the revision", async () => {
  const f = fakeApi(["", "pending", "running", "failed"]);
  const seen = [];
  const res = await f.client.jobs.runAndWait("shop", "migrate", { image: "m:1" },
    { pollIntervalMs: 1, onUpdate: (r) => seen.push(r.phase) });
  assert.equal(res.run.revision, 7);
  assert.equal(res.run.phase, "failed");
  assert.equal(res.exitCode, 3);
  assert.deepEqual(seen, ["pending", "running", "failed"]);
});

test("runAndWait succeeds", async () => {
  const f = fakeApi(["", "done"]);
  const res = await f.client.jobs.runAndWait("shop", "migrate", { image: "m:1" }, { pollIntervalMs: 1 });
  assert.equal(res.exitCode, 0);
});

test("runAndWait refuses a schedule", async () => {
  const f = fakeApi();
  await assert.rejects(f.client.jobs.runAndWait("shop", "migrate", { image: "m:1", schedule: "@daily" }));
});

test("API errors are typed", async () => {
  const f = fakeApi();
  await assert.rejects(f.client.jobs.run("shop", "migrate", { image: "m:1", size: "xl" }), (e) => {
    assert.ok(e instanceof GagarinError);
    assert.equal(e.code, "invalid_size");
    assert.equal(e.status, 400);
    assert.ok(e.fixHint);
    return true;
  });
});

test("get refuses a service", async () => {
  const f = fakeApi();
  await assert.rejects(f.client.jobs.get("shop", "web"), { code: "not_a_job" });
});

test("logs", async () => {
  const f = fakeApi();
  const l = await f.client.jobs.logs("shop", "migrate", { contains: "boom" });
  assert.equal(l.logs, "q=boom\n");
});

test("a token is required", () => {
  const prev = process.env.GAGARIN_TOKEN;
  delete process.env.GAGARIN_TOKEN;
  try {
    assert.throws(() => new Gagarin());
  } finally {
    if (prev !== undefined) process.env.GAGARIN_TOKEN = prev;
  }
});
