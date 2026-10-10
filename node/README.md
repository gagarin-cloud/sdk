# @gagarin-cloud/sdk — Node.js

```sh
npm install @gagarin-cloud/sdk
```

Node 18+, no runtime dependencies (uses the built-in `fetch`). Ships ESM with
TypeScript types.

```ts
import { Gagarin, GagarinError } from "@gagarin-cloud/sdk";

const gg = new Gagarin(); // GAGARIN_TOKEN, GAGARIN_API; or new Gagarin({ token })

// What `gg run` does: submit, wait, exit with the script's code.
const res = await gg.jobs.runAndWait(
  "shop",
  "migrate",
  {
    image: "migrate:v3", // or registry.gagarin.cloud/<project-id>/migrate:v3
    env: { LOG_LEVEL: "debug" },
    deps: ["db"],
    size: "m",
    timeoutSeconds: 900,
  },
  { onUpdate: (run) => console.log(run.phase) },
);
process.exit(res.exitCode);
```

Fire and forget, then check later:

```ts
const sub = await gg.jobs.run("shop", "backfill", { image: "backfill:v1" });
const run = await gg.jobs.wait("shop", "backfill", sub.revision);
const { logs } = await gg.jobs.logs("shop", "backfill", { since: "30m" });
```

A scheduled job (runs nothing now):

```ts
await gg.jobs.run("shop", "nightly", { image: "report:v2", schedule: "0 3 * * *", timeZone: "Europe/Berlin" });
```

API refusals throw `GagarinError` with `status`, `code`, `message` and `fixHint`.

```sh
npm install && npm test
```
