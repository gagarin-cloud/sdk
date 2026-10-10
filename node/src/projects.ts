import { projectPath, type Gagarin } from "./client.js";
import { GagarinError } from "./errors.js";
import type { Project, ProjectStatus, ServiceStatus } from "./types.js";

/** Lists, creates and reads projects. */
export class Projects {
  readonly #c: Gagarin;

  constructor(client: Gagarin) {
    this.#c = client;
  }

  /** Every project the caller can see, their own and shared. */
  async list(): Promise<Project[]> {
    return (await this.#c.request<{ projects: Project[] }>("GET", "/v1/projects")).projects;
  }

  /** Resolves a project name or id. */
  get(project: string): Promise<Project> {
    return this.#c.request("GET", projectPath(project));
  }

  /**
   * Makes a project. Names are 2–30 lowercase letters, digits and hyphens, starting with
   * a letter, and unique per account; a clash is refused `project_exists`.
   */
  create(name: string): Promise<Project> {
    return this.#c.request("POST", "/v1/projects", { body: { name } });
  }

  /** What the project is asked to run, and what is running. */
  status(project: string, signal?: AbortSignal): Promise<ProjectStatus> {
    return this.#c.request("GET", `${projectPath(project)}/status`, { signal });
  }

  /** One row of the project's status; rejects `not_found` when there is no such name. */
  async serviceStatus(project: string, name: string, signal?: AbortSignal): Promise<ServiceStatus> {
    const row = (await this.status(project, signal)).services.find((s) => s.name === name);
    if (!row) throw new GagarinError(404, "not_found", `nothing named ${name} in project ${project}`);
    return row;
  }
}
