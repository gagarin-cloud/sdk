import { servicePath, type Gagarin } from "./client.js";
import { GagarinError } from "./errors.js";
import type { Logs, LogsOptions, Phase, RunState, Service, ServiceStatus, Size } from "./types.js";

/** The longest a run may take, and what a new job gets. */
export const MAX_TIMEOUT_SECONDS = 3600;

/** The longest a finished run is kept, and what a new job gets. */
export const MAX_TTL_SECONDS = 28800;

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
  /** Stop the run after this many seconds, 1–3600, counted from when the run gets a machine (not while it waits for one). Absent keeps the current one. */
  timeoutSeconds?: number;
  /** Keep a finished run this many seconds, 1–28800, counted from when it ended. Then a one-shot job is deleted entirely; a scheduled job loses only that firing. Absent keeps the current one. */
  ttlSeconds?: number;
  /** Five cron fields or a descriptor like `@daily`. Nothing runs now; each firing is a run. Cannot be removed. */
  schedule?: string;
  /** IANA zone the schedule is read in. Absent keeps the current one (UTC for a new job). */
  timeZone?: string;
}

/** The job as stored, and the revision this run is — the run's name. */
export interface Submission extends Service {
  revision: number;
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

const finished = (r: RunState) => r.phase === "done" || r.phase === "failed";

function exitCodeOf(r: RunState): number {
  if (r.exit_code !== undefined && r.exit_code !== null) return r.exit_code;
  return r.phase === "done" ? 0 : 1;
}

const retryable = (e: unknown) =>
  e instanceof GagarinError ? e.status >= 500 || e.status === 429 : !(e instanceof Error && e.name === "AbortError");

function sleep(ms: number, signal?: AbortSignal): Promise<void> {
  return new Promise((resolve, reject) => {
    if (signal?.aborted) return reject(signal.reason);
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
    if (opts.ttlSeconds) body.ttl_seconds = opts.ttlSeconds;
    if (opts.schedule) body.schedule = opts.schedule;
    if (opts.timeZone) body.time_zone = opts.timeZone;
    return this.#c.request("PUT", servicePath(project, name), { body });
  }

  /** The job's row from the project's status, with how its latest run went in `actual.run`. */
  async get(project: string, name: string, signal?: AbortSignal): Promise<ServiceStatus> {
    const row = await this.#c.projects.serviceStatus(project, name, signal);
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

  /** The job's logs. Every run's lines are there, newest last. */
  logs(project: string, name: string, opts: LogsOptions = {}): Promise<Logs> {
    return this.#c.services.logs(project, name, opts);
  }
}
