import { apiError } from "./errors.js";
import { Account } from "./account.js";
import { Jobs } from "./jobs.js";
import { Platform } from "./platform.js";
import { Projects } from "./projects.js";
import { Services } from "./services.js";

export const DEFAULT_BASE_URL = "https://api.gagarin.cloud";
export const DEFAULT_REGISTRY = "registry.gagarin.cloud";

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

export interface RequestOptions {
  query?: Record<string, string | number | boolean | undefined>;
  body?: unknown;
  signal?: AbortSignal;
}

const env = (k: string): string | undefined =>
  typeof process !== "undefined" ? process.env[k] || undefined : undefined;

export const projectPath = (project: string) => `/v1/projects/${encodeURIComponent(project)}`;
export const servicePath = (project: string, name: string) =>
  `${projectPath(project)}/services/${encodeURIComponent(name)}`;

export class Gagarin {
  /** Submits, waits on and reads jobs. */
  readonly jobs: Jobs;
  /** The account the credential belongs to. */
  readonly account: Account;
  /** Lists, creates and reads projects, and their status. */
  readonly projects: Projects;
  /** Any service's logs and deploy history. */
  readonly services: Services;
  /** Whether gagarin itself is working. */
  readonly platform: Platform;

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
    this.account = new Account(this);
    this.projects = new Projects(this);
    this.services = new Services(this);
    this.platform = new Platform(this);
  }

  /** Makes one request and hands back the status and body, whatever the status was. */
  async send(method: string, path: string, opts: RequestOptions = {}): Promise<{ status: number; text: string }> {
    let url = this.#baseUrl + path;
    const q = new URLSearchParams();
    for (const [k, v] of Object.entries(opts.query ?? {})) {
      if (v !== undefined && v !== "" && v !== false) q.set(k, String(v));
    }
    const qs = q.toString();
    if (qs) url += "?" + qs;
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
    return { status: res.status, text: await res.text() };
  }

  /** Makes one request, throws a {@link GagarinError} on a refusal, and parses the answer. */
  async request<T>(method: string, path: string, opts: RequestOptions = {}): Promise<T> {
    const { status, text } = await this.send(method, path, opts);
    if (status < 200 || status >= 300) throw apiError(status, text);
    return (text ? JSON.parse(text) : undefined) as T;
  }

  /**
   * Expands a bare repository reference (`migrate:v3`) into the full one gagarin runs
   * from. A reference that already contains a `/` is returned unchanged.
   */
  async imageRef(project: string, image: string): Promise<string> {
    if (image.includes("/")) return image;
    const p = await this.projects.get(project);
    return `${this.#registry}/${p.id}/${image}`;
  }
}
