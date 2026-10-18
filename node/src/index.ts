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
 *
 * Around it are the basics: `account`, `projects` (with their status), `services`
 * (logs and history) and `platform` (gagarin's own health).
 */

export { Gagarin, DEFAULT_BASE_URL, DEFAULT_REGISTRY } from "./client.js";
export type { ClientOptions, RequestOptions } from "./client.js";
export { GagarinError } from "./errors.js";
export { Jobs, MAX_TIMEOUT_SECONDS, MAX_TTL_SECONDS } from "./jobs.js";
export type { RunOptions, Submission, WaitOptions, RunResult } from "./jobs.js";
export { Account } from "./account.js";
export { Projects } from "./projects.js";
export { Services } from "./services.js";
export { Platform } from "./platform.js";
export type * from "./types.js";

import { Gagarin } from "./client.js";
export default Gagarin;
