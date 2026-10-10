# sdk

Programmatic integration conveniences for [gagarin](https://gagarin.cloud):
client libraries for the gagarin API.

| Folder | Language | Package |
| --- | --- | --- |
| [`node/`](node) | Node.js / TypeScript (≥18, no runtime deps) | `@gagarin-cloud/sdk` |
| [`go/`](go) | Go (≥1.22, stdlib only) | `github.com/gagarin-cloud/sdk/go` |
| [`python/`](python) | Python (≥3.9, stdlib only) | `gagarin` |

## What they do so far: run jobs

The first thing each library does is what `gg run` does — start a **job** (an
image that runs to completion and listens on nothing: a migration, a backfill,
a one-off script) and, if you like, wait for it to finish and get its exit code.

Like `gg run`, this **assumes the image is already in gagarin's registry**
(`registry.gagarin.cloud/<project-id>/<repo>:<tag>`, pushed with `gg ship` or
`docker push`). The libraries do not build or push images. You may pass the
full reference, or just `<repo>:<tag>` and the client expands it for the
project.

Every setting the API accepts for a job is exposed:

| Setting | API field | Meaning |
| --- | --- | --- |
| image | `image` | Required. Full registry reference, or bare `repo:tag`. |
| digest | `digest` | `sha256:…` as `docker push` reported it, pinning the exact image. |
| env | `env` | The **complete** environment for this run. Replaces the job's; not merged. Absent empties it. |
| deps | `deps` | Resources/services the run may reach, **added** to what the job already declares. A migration must name its database, or its connection hangs. |
| size | `size` | `s`, `m` or `l`. Absent keeps the job's current size. |
| timeout | `timeout_seconds` | 1–3600, counted from submission (image pull included). Absent keeps the current one; a new job gets 3600. |
| schedule | `schedule` | Cron (`0 3 * * *`) or `@daily`-style. Nothing runs now; each firing is a run. Cannot be removed except by destroying the job. |
| time zone | `time_zone` | IANA zone for the schedule. Default UTC. |

Each library offers the same four operations:

- **run** — submit a run and return immediately with its `revision` (the run's name). Every call is a new run.
- **wait** — poll the project's status until that revision is `done` or `failed`, and return its phase and exit code.
- **runAndWait** — both, which is exactly `gg run`; the result carries the exit code `gg run` would exit with.
- **logs** — read the job's logs.

## Configuration

All three read the same environment variables `gg` does:

| Variable | Default |
| --- | --- |
| `GAGARIN_TOKEN` | — (required; a credential with the `deploy` scope to run) |
| `GAGARIN_API` | `https://api.gagarin.cloud` |
| `GAGARIN_REGISTRY` | `registry.gagarin.cloud` |

Errors from the API surface as a typed error carrying the API's stable `code`
(e.g. `invalid_timeout`, `not_a_job`, `payment_required`), its `message`, and
its `fix_hint`. Branch on the code.
