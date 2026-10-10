import type { Gagarin } from "./client.js";
import type { Billing, Whoami } from "./types.js";

/** The account the credential belongs to. */
export class Account {
  readonly #c: Gagarin;

  constructor(client: Gagarin) {
    this.#c = client;
  }

  /** Which account this credential acts as, and what it may do (`can` holds "deploy" and/or "destroy"). */
  whoami(): Promise<Whoami> {
    return this.#c.request("GET", "/v1/whoami");
  }

  /** The account's balance, burn and runway. */
  billing(): Promise<Billing> {
    return this.#c.request("GET", "/v1/billing");
  }
}
