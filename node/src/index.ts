/**
 * A client for the gagarin API.
 *
 * Its first job is the one `gg run` does: start a job from an image that is
 * already in gagarin's registry, and optionally wait for it to finish.
 *
 * ```ts
 * const gg = new Gagarin(); // GAGARIN_TOKEN, GAGARIN_API
 * const res = await gg.jobs.runAndWait("shop", "migrate", { image: "migrate:v3", deps: ["db"] });
 * process.exit(res.exitCode);
 * ```
 */

export const DEFAULT_BASE_URL = "https://api.gagarin.cloud";
export const DEFAULT_REGISTRY = "registry.gagarin.cloud";
/** The longest a run may take, and what a new job gets. */
export const MAX_TIMEOUT_SECONDS = 3600;

export type Size = "s" | "m" | "l";
export type Phase = "pending" | "running" | "done" | "failed" | "suspended";

export interface ClientOptions {
  /** Defaults to `$GAGARIN_TOKEN`. */
  token?: string;
  /** Defaults to `$GAGARIN_API`, then {@link DEFAULT_BASE_URL}. */
  baseUrl?: string;
  /** Registry host a bare image name is expanded against. Defaults to `$GAGARIN_REGISTRY`, then {@link DEFAULT_REGISTRY}. */
  registry?: string;
  /** Replaces the global `fetch`. */
  fetch?: typeof fetch;
}

/** What the API answers when it refuses a request. Branch on `code`; the rest is prose. */
export class GagarinError extends Error {
  readonly status: number;
  readonly code: string;
  readonly fixHint?: string;

  constructor(status: number, code: string, message: string, fixHint?: string) {
    super(fixHint ? `[${code}] ${message}\nhint: ${fixHint}` : `[${code}] ${message}`);
    this.name = "GagarinError";
    this.status = status;
    this.code = code;
    this.fixHint = fixHint;
  }
}

/** Everything the API accepts for a job. As with `gg run`, the image must already be in gagarin's registry. */
export interface RunOptions {
  /**
   * Full reference in gagarin's registry (`registry.gagarin.cloud/<project-id>/migrate:v3`),
   * or a bare `migrate:v3` that the client expands against the project.
   */
  image: string;
  /** Pins the exact image: `sha256:...` as docker push reported it. */
  digest?: string;
  /** The complete environment for this run. Replaces the job's; not merged. Absent empties it. */
  env?: Record<string, string>;
  /** Resources and services this run may reach, added to what the job already declares. */
  deps?: string[];
  /** CPU and memory envelope. Absent keeps the job's current size. */
  size?: Size;
  /** Stop the run after this many seconds, 1–3600, counted from submission. Absent keeps the current one. */
  timeoutSeconds?: number;
  /** Five cron fields or a descriptor like `@daily`. Nothing runs now; each firing is a run. Cannot be removed. */
  schedule?: string;
  /** IANA zone the schedule is read in. Absent keeps the current one (UTC for a new job). */
  timeZone?: string;
}

export interface Job {
  project: string;
  name: string;
  kind: string;
  size?: string;
  image: string;
  digest?: string;
  timeout_seconds?: number;
  schedule?: string;
  time_zone?: string;
  updated_by?: string;
  created_at: string;
  updated_at: string;
}

/** The job as stored, and the revision this run is — the run's name. */
export interface Submission extends Job {
  revision: number;
}

export interface RunState {
  revision: number;
  phase: Phase;
  exit_code?: number;
  started_at?: string;
  finished_at?: string;
}

export interface JobStatus extends Job {
  needs: string[];
  in_sync: boolean;
  sentence: string;
  env_keys?: string[];
  actual: {
    exists: boolean;
    message?: string;
    run?: RunState;
    schedule?: { next_at?: string; last_scheduled_at?: string; suspended?: boolean };
  };
}

export interface WaitOptions {
  /** How often status is read, in ms. Default 3000. */
  pollIntervalMs?: number;
  /** Give up after this long, in ms. Default 65 minutes: the platform stops a run at 60. */
  timeoutMs?: number;
  /** Called each time the run's phase changes. */
  onUpdate?: (run: RunState) => void;
  signal?: AbortSignal;
}

export interface RunResult {
  submission: Submission;
  run: RunState;
  /** The script's own exit code, 0 on success, 1 for a run that failed without one — what `gg run` exits with. */
  exitCode: number;
}

export interface LogsOptions {
  limit?: number;
  /** A duration back from now (`30m`, `6h`, `2d`) or an RFC 3339 time. */
  since?: string;
  /** As `since`; usually the `next` of the previous page. */
  until?: string;
  /** Keep only lines containing this text. */
  contains?: string;
}

export interface Logs {
  /** The lines, oldest first, newline-terminated. */
  logs: string;
  source: "store" | "cluster";
  next: string | null;
  notice?: string;
}

export interface Project {
  id: string;
  name: string;
  role: string;
  created_at: string;
}

const env = (k: string): string | undefined =>
  typeof process !== "undefined" ? process.env[k] || undefined : undefined;

export class Gagarin {
  readonly jobs: Jobs;
  readonly #token: string;
  readonly #baseUrl: string;
  readonly #registry: string;
  readonly #fetch: typeof fetch;

  constructor(opts: ClientOptions = {}) {
    const token = opts.token ?? env("GAGARIN_TOKEN");
    if (!token) throw new Error("gagarin: no token: pass { token } or set GAGARIN_TOKEN");
    this.#token = token;
    this.#baseUrl = (opts.baseUrl ?? env("GAGARIN_API") ?? DEFAULT_BASE_URL).replace(/\/+$/, "");
    this.#registry = (opts.registry ?? env("GAGARIN_REGISTRY") ?? DEFAULT_REGISTRY).replace(/\/+$/, "");
    this.#fetch = opts.fetch ?? globalThis.fetch;
    this.jobs = new Jobs(this);
  }

  /** @internal */
  async request<T>(method: string, path: string, opts: { query?: Record<string, string | undefined>; body?: unknown; signal?: AbortSignal } = {}): Promise<T> {
    let url = this.#baseUrl + path;
    const q = new URLSearchParams();
    for (const [k, v] of Object.entries(opts.query ?? {})) if (v !== undefined && v !== "") q.set(k, v);
    if ([...q].length) url += "?" + q.toString();
    const headers: Record<string, string> = {
      authorization: `Bearer ${this.#token}`,
      accept: "application/json",
      "user-agent": "gagarin-sdk-node",
    };
    if (opts.body !== undefined) headers["content-type"] = "application/json";
    const res = await this.#fetch(url, {
      method,
      headers,
      body: opts.body === undefined ? undefined : JSON.stringify(opts.body),
      signal: opts.signal,
    });
    const text = await res.text();
    if (!res.ok) {
      let e: { code?: string; message?: string; fix_hint?: string } | undefined;
      try {
        e = JSON.parse(text)?.error;
      } catch {
        /* not the envelope */
      }
      if (e?.code) throw new GagarinError(res.status, e.code, e.message ?? "", e.fix_hint);
      throw new GagarinError(res.status, `http_${res.status}`, text.trim());
    }
    return (text ? JSON.parse(text) : undefined) as T;
  }

  /** Resolves a project name or id. */
  project(project: string): Promise<Project> {
    return this.request("GET", `/v1/projects/${encodeURIComponent(project)}`);
  }

  /**
   * Expands a bare repository reference (`migrate:v3`) into the full one gagarin runs
   * from. A reference that already contains a `/` is returned unchanged.
   */
  async imageRef(project: string, image: string): Promise<string> {
    if (image.includes("/")) return image;
    const p = await this.project(project);
    return `${this.#registry}/${p.id}/${image}`;
  }
}

const servicePath = (project: string, name: string) =>
  `/v1/projects/${encodeURIComponent(project)}/services/${encodeURIComponent(name)}`;

const finished = (r: RunState) => r.phase === "done" || r.phase === "failed";

function exitCodeOf(r: RunState): number {
  if (r.exit_code !== undefined && r.exit_code !== null) return r.exit_code;
  return r.phase === "done" ? 0 : 1;
}

const retryable = (e: unknown) =>
  e instanceof GagarinError ? e.status >= 500 || e.status === 429 : !(e instanceof Error && e.name === "AbortError");

function sleep(ms: number, signal?: AbortSignal): Promise<void> {
  return new Promise((resolve, reject) => {
    const t = setTimeout(resolve, ms);
    signal?.addEventListener("abort", () => (clearTimeout(t), reject(signal.reason)), { once: true });
  });
}

/** Jobs: images that run to completion and listen on nothing. */
export class Jobs {
  readonly #c: Gagarin;

  constructor(client: Gagarin) {
    this.#c = client;
  }

  /** Submits a run of the named job and returns without waiting. Every call is a new run and a new revision. */
  async run(project: string, name: string, opts: RunOptions): Promise<Submission> {
    if (!opts?.image) throw new Error("gagarin: image is required");
    const body: Record<string, unknown> = {
      kind: "job",
      image: await this.#c.imageRef(project, opts.image),
      // Replaced on every run, so always sent.
      env: opts.env ?? {},
    };
    if (opts.digest) body.digest = opts.digest;
    if (opts.deps?.length) body.deps = opts.deps;
    if (opts.size) body.size = opts.size;
    if (opts.timeoutSeconds) body.timeout_seconds = opts.timeoutSeconds;
    if (opts.schedule) body.schedule = opts.schedule;
    if (opts.timeZone) body.time_zone = opts.timeZone;
    return this.#c.request("PUT", servicePath(project, name), { body });
  }

  /** Reads the named job's status: its row and how its latest run went. */
  async get(project: string, name: string, signal?: AbortSignal): Promise<JobStatus> {
    const st = await this.#c.request<{ services: JobStatus[] }>(
      "GET",
      `/v1/projects/${encodeURIComponent(project)}/status`,
      { signal },
    );
    const row = st.services.find((s) => s.name === name);
    if (!row) throw new GagarinError(404, "not_found", `no job ${name} in project ${project}`);
    if (row.kind !== "job") throw new GagarinError(409, "not_a_job", `${name} is a ${row.kind} in this project, not a job`);
    return row;
  }

  /**
   * Resolves when the given revision of the job has finished, with how it ended.
   * A run that failed is not a rejection: read `phase` and `exit_code`. Rejects if a
   * newer run supersedes it, if the project is suspended, or on timeout.
   */
  async wait(project: string, name: string, revision: number, opts: WaitOptions = {}): Promise<RunState> {
    const interval = opts.pollIntervalMs ?? 3000;
    const deadline = Date.now() + (opts.timeoutMs ?? 65 * 60_000);
    let lastPhase: string | undefined;
    let transient = 0;
    for (;;) {
      try {
        const run = (await this.get(project, name, opts.signal)).actual?.run;
        transient = 0;
        if (run && run.revision > revision)
          throw new GagarinError(409, "superseded", `run ${revision} of ${name} was superseded by run ${run.revision}`);
        if (run && run.revision === revision) {
          if (run.phase !== lastPhase) opts.onUpdate?.(run);
          lastPhase = run.phase;
          if (finished(run)) return run;
          if (run.phase === "suspended")
            throw new GagarinError(409, "suspended", `the project is suspended and run ${revision} of ${name} was stopped`);
        }
      } catch (e) {
        if (!retryable(e) || ++transient > 5) throw e;
      }
      if (Date.now() + interval > deadline)
        throw new GagarinError(408, "wait_timeout", `run ${revision} of ${name} had not finished in time`);
      await sleep(interval, opts.signal);
    }
  }

  /** Submits a run and waits for it to finish — what `gg run` does. */
  async runAndWait(project: string, name: string, opts: RunOptions, wait: WaitOptions = {}): Promise<RunResult> {
    if (opts.schedule) throw new Error("gagarin: a scheduled job does not run now; submit it with run()");
    const submission = await this.run(project, name, opts);
    const run = await this.wait(project, name, submission.revision, wait);
    return { submission, run, exitCode: exitCodeOf(run) };
  }

  /** Reads the job's logs. */
  logs(project: string, name: string, opts: LogsOptions = {}): Promise<Logs> {
    return this.#c.request("GET", servicePath(project, name) + "/logs", {
      query: { limit: opts.limit?.toString(), since: opts.since, until: opts.until, q: opts.contains },
    });
  }
}

export default Gagarin;
