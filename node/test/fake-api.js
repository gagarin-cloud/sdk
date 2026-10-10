// A local HTTP server that answers like the gagarin API, from the fixtures in
// ../../testdata/api, and records every request it was sent.
import { createServer } from "node:http";
import { readFileSync } from "node:fs";
import { Gagarin } from "../dist/index.js";

const fixtures = new URL("../../testdata/api/", import.meta.url);
export const fixture = (name) => readFileSync(new URL(name, fixtures), "utf8");

export async function startFake(t) {
  const f = {
    requests: [],
    runs: null, // successive status reads' run for migrate; the last repeats
    statusFailures: 0,
    healthDown: false,
    nextRevision: 8,
    last() {
      if (!this.requests.length) throw new Error("no request was made");
      return this.requests.at(-1);
    },
    count(method, path) {
      return this.requests.filter((r) => r.method === method && r.path === path).length;
    },
  };

  const server = createServer(async (req, res) => {
    let raw = "";
    for await (const chunk of req) raw += chunk;
    const u = new URL(req.url, "http://fake");
    const rec = {
      method: req.method,
      path: decodeURIComponent(u.pathname),
      escaped: u.pathname,
      query: Object.fromEntries(u.searchParams),
      auth: req.headers.authorization,
      contentType: req.headers["content-type"],
      userAgent: req.headers["user-agent"],
      body: raw ? JSON.parse(raw) : undefined,
    };
    f.requests.push(rec);
    const reply = (status, body) => {
      res.writeHead(status, { "content-type": "application/json" });
      res.end(typeof body === "string" ? body : JSON.stringify(body));
    };
    const fail = (status, code, message) => reply(status, { error: { code, message } });

    if (req.method === "GET" && u.pathname === "/healthz/platform") {
      return f.healthDown ? reply(503, fixture("health_down.json")) : reply(200, fixture("health_ok.json"));
    }
    if (rec.auth !== "Bearer tok") return reply(401, fixture("error_unauthorized.json"));

    const route = `${req.method} ${u.pathname}`;
    switch (route) {
      case "GET /v1/whoami":
        return reply(200, fixture("whoami.json"));
      case "GET /v1/billing":
        return reply(200, fixture("billing.json"));
      case "GET /v1/projects":
        return reply(200, fixture("projects.json"));
      case "POST /v1/projects":
        if (rec.body?.name === "shop") return reply(409, fixture("error_project_exists.json"));
        return reply(201, { ...JSON.parse(fixture("project_created.json")), name: rec.body.name });
      case "GET /v1/projects/shop":
      case "GET /v1/projects/p-123":
        return reply(200, fixture("project.json"));
      case "GET /v1/projects/shop/status": {
        if (f.statusFailures > 0) {
          f.statusFailures--;
          return fail(502, "cluster_error", "the cluster did not answer");
        }
        const st = JSON.parse(fixture("status.json"));
        if (f.runs) {
          const run = f.runs.length > 1 ? f.runs.shift() : f.runs[0];
          const migrate = st.services.find((s) => s.name === "migrate");
          if (run) migrate.actual.run = run;
          else delete migrate.actual.run;
        }
        return reply(200, st);
      }
      case "GET /v1/projects/shop/services/migrate/deployments":
        return reply(200, fixture("deployments.json"));
      case "GET /v1/projects/shop/services/migrate/logs":
        return reply(200, fixture("logs.json"));
    }
    if (req.method === "PUT" && u.pathname.startsWith("/v1/projects/shop/services/")) {
      if (rec.body?.size === "xl") return reply(400, fixture("error_invalid_size.json"));
      const out = {
        project: "p-123",
        name: u.pathname.split("/").at(-1),
        kind: rec.body.kind,
        image: rec.body.image,
        port: 0,
        revision: f.nextRevision++,
        created_at: "2026-09-01T12:00:00Z",
        updated_at: "2026-10-10T10:00:00Z",
      };
      for (const k of ["size", "digest", "timeout_seconds", "schedule", "time_zone"]) if (k in rec.body) out[k] = rec.body[k];
      return reply(200, out);
    }
    if (u.pathname.startsWith("/v1/projects/")) return fail(404, "not_found", "no project or service by that name");
    return fail(404, "no_such_route", route);
  });

  await new Promise((resolve) => server.listen(0, "127.0.0.1", resolve));
  f.url = `http://127.0.0.1:${server.address().port}`;
  f.client = new Gagarin({ token: "tok", baseUrl: f.url + "/", registry: "registry.example" });
  t.after(() => new Promise((resolve) => server.close(resolve)));
  return f;
}

export const run = (revision, phase, exit_code) =>
  exit_code === undefined ? { revision, phase } : { revision, phase, exit_code };
