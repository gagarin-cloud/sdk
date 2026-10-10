import { servicePath, type Gagarin } from "./client.js";
import type { Deployment, Logs, LogsOptions } from "./types.js";

/** Reads what any service or job has printed and been. */
export class Services {
  readonly #c: Gagarin;

  constructor(client: Gagarin) {
    this.#c = client;
  }

  /** A service's or job's logs. */
  logs(project: string, name: string, opts: LogsOptions = {}): Promise<Logs> {
    return this.#c.request("GET", servicePath(project, name) + "/logs", {
      query: { limit: opts.limit, since: opts.since, until: opts.until, q: opts.contains, previous: opts.previous },
    });
  }

  /** A service's or job's revisions, newest first. */
  async deployments(project: string, name: string): Promise<Deployment[]> {
    return (await this.#c.request<{ deployments: Deployment[] }>("GET", servicePath(project, name) + "/deployments"))
      .deployments;
  }
}
