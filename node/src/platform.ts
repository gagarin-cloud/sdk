import type { Gagarin } from "./client.js";
import { apiError } from "./errors.js";
import type { Health } from "./types.js";

/** Whether gagarin itself is working. */
export class Platform {
  readonly #c: Gagarin;

  constructor(client: Gagarin) {
    this.#c = client;
  }

  /**
   * The platform's health. An unhealthy platform is an answer, not a rejection: read `ok`
   * and each check's `detail`. A rejection means the API could not be asked at all.
   */
  async health(): Promise<Health> {
    const { status, text } = await this.#c.send("GET", "/healthz/platform");
    if (status === 200 || status === 503) {
      try {
        return JSON.parse(text) as Health;
      } catch {
        /* fall through */
      }
    }
    throw apiError(status, text);
  }
}
