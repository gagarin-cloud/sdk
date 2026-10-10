// Drives a real gagarin API. Skipped unless GAGARIN_INTEGRATION=1; see
// ../../scripts/integration.sh for what it needs.
import { test } from "node:test";
import assert from "node:assert/strict";
import { Gagarin, GagarinError } from "../dist/index.js";

const enabled = process.env.GAGARIN_INTEGRATION === "1";
const env = (k) => {
  if (!process.env[k]) throw new Error(`${k} is not set`);
  return process.env[k];
};
const wait = { pollIntervalMs: 1000 };

async function eventually(f, ok) {
  let last;
  for (let i = 0; i < 30; i++) {
    last = await f();
    if (ok(last)) return last;
    await new Promise((r) => setTimeout(r, 1000));
  }
  return last;
}

test("integration", { skip: !enabled && "set GAGARIN_INTEGRATION=1 to run against a real API", timeout: 600_000 }, async (t) => {
  const project = env("GAGARIN_IT_PROJECT");
  const imageOk = env("GAGARIN_IT_IMAGE_OK");
  const imageFail = env("GAGARIN_IT_IMAGE_FAIL");
  const gg = new Gagarin();

  await t.test("platform health", async () => {
    const h = await gg.platform.health();
    assert.equal(h.serving.ok, true);
  });

  await t.test("whoami", async () => {
    const w = await gg.account.whoami();
    assert.ok(w.account);
    assert.ok(w.registry);
    assert.ok(w.can.includes("deploy"));
  });

  await t.test("projects", async () => {
    await gg.projects.create(project).catch((e) => {
      if (e.code !== "project_exists") throw e;
    });
    await assert.rejects(gg.projects.create(project), { code: "project_exists" });
    const p = await gg.projects.get(project);
    assert.equal(p.name, project);
    assert.equal((await gg.projects.get(p.id)).name, project);
    assert.ok((await gg.projects.list()).some((q) => q.id === p.id));
    await assert.rejects(gg.projects.get("no-such-project-here"), GagarinError);
  });

  await t.test("a job that succeeds", async () => {
    const res = await gg.jobs.runAndWait(
      project,
      "it-node-ok",
      { image: imageOk, env: { GREETING: "from-node" }, size: "s", timeoutSeconds: 300 },
      wait,
    );
    assert.equal(res.exitCode, 0);
    assert.equal(res.run.phase, "done");
    assert.equal(res.run.revision, res.submission.revision);
    const row = await gg.jobs.get(project, "it-node-ok");
    assert.equal(row.timeout_seconds, 300);
    assert.ok(row.env_keys.includes("GREETING"));
    const ds = await gg.services.deployments(project, "it-node-ok");
    assert.equal(ds[0].revision, res.submission.revision);
    const logs = await eventually(
      () => gg.jobs.logs(project, "it-node-ok").catch(() => ({ logs: "" })),
      (l) => l.logs.includes("hello"),
    );
    assert.match(logs.logs, /hello/);
  });

  await t.test("a job that fails with its own code", async () => {
    const res = await gg.jobs.runAndWait(project, "it-node-fail", { image: imageFail }, wait);
    assert.equal(res.run.phase, "failed");
    assert.equal(res.exitCode, 3);
  });

  await t.test("status reports the jobs", async () => {
    const st = await gg.projects.status(project);
    for (const name of ["it-node-ok", "it-node-fail"]) {
      const row = st.services.find((s) => s.name === name);
      assert.equal(row.kind, "job");
      assert.ok(["done", "failed"].includes(row.actual.run.phase));
    }
  });

  await t.test("refusals are typed", async () => {
    await assert.rejects(gg.jobs.run(project, "it-node-ok", { image: imageOk, size: "xl" }), {
      code: "invalid_size",
      status: 400,
    });
    await assert.rejects(gg.jobs.run(project, "it-node-ok", { image: imageOk, timeoutSeconds: 99999 }), {
      code: "invalid_timeout",
    });
  });
});
