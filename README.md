# sdk

Programmatic integration conveniences for [Gagarin](https://gagarin.cloud):
client libraries for the Gagarin API.

| Folder | Language | Package |
| --- | --- | --- |
| [`node/`](node) | Node.js / TypeScript (≥18, no runtime deps) | `@gagarin-cloud/sdk` |
| [`go/`](go) | Go (≥1.22, stdlib only) | `github.com/gagarin-cloud/sdk/go` |
| [`python/`](python) | Python (≥3.9, stdlib only) | `gagarin` |

## What they do so far: run jobs

The first thing each library does is what `gg run` does — start a **job** (an
image that runs to completion and listens on nothing: a migration, a backfill,
a one-off script) and, if you like, wait for it to finish and get its exit code.

Like `gg run`, this **assumes the image is already in Gagarin's registry**
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
| size | `size` | `s`, `m`, `l` or `g`. `g` is one L4 GPU (24 GB, 6 vCPU, 40 GiB), **jobs only** (the API refuses it on services and resources with `invalid_size`): $3 an hour, billed by the minute like every other size, and it waits pending three to six minutes while the GPU node boots. Absent keeps the job's current size. |
| timeout | `timeout_seconds` | 1–3600, counted from when the run gets a machine (image pull included, waiting for a machine not). Absent keeps the current one; a new job gets 3600. |
| ttl | `ttl_seconds` | 60–28800, how long a finished run is kept, counted from when it ended. Then a one-shot job is deleted entirely (history and runs too) and leaves status; a scheduled job stays and loses only that firing. Absent keeps the current one; a new job gets 28800 (8 hours). Reads omit it when it is that default. |
| schedule | `schedule` | Cron (`0 3 * * *`) or `@daily`-style. Nothing runs now; each firing is a run. Cannot be removed except by destroying the job. |
| time zone | `time_zone` | IANA zone for the schedule. Default UTC. |

A scheduled job's latest run may carry `expired: true`: its TTL ran out and it is
no longer in the cluster. What is shown is the platform's record of it; the
phase, exit code and times are still accurate.

Each library offers the same four operations:

- **run** — submit a run and return immediately with its `revision` (the run's name). Every call is a new run.
- **wait** — poll the project's status until that revision is `done` or `failed`, and return its phase and exit code.
- **runAndWait** — both, which is exactly `gg run`; the result carries the exit code `gg run` would exit with.
- **logs** — read the job's logs.

## The basics around it

Each library also covers the reads you need around a job, and creating a project:

| | Go | Node | Python | API |
| --- | --- | --- | --- | --- |
| Who am I, what may I do | `Account.Whoami` | `account.whoami` | `account.whoami` | `GET /v1/whoami` |
| Balance, burn, runway | `Account.Billing` | `account.billing` | `account.billing` | `GET /v1/billing` |
| List projects | `Projects.List` | `projects.list` | `projects.list` | `GET /v1/projects` |
| A project by name or id | `Projects.Get` | `projects.get` | `projects.get` | `GET /v1/projects/{p}` |
| Create a project | `Projects.Create` | `projects.create` | `projects.create` | `POST /v1/projects` |
| Asked-for vs. running | `Projects.Status` | `projects.status` | `projects.status` | `GET /v1/projects/{p}/status` |
| One row of that | `Projects.ServiceStatus` | `projects.serviceStatus` | `projects.service_status` | (same) |
| Logs of a service or job | `Services.Logs` | `services.logs` | `services.logs` | `GET …/services/{s}/logs` |
| Revision history | `Services.Deployments` | `services.deployments` | `services.deployments` | `GET …/services/{s}/deployments` |
| Is Gagarin itself healthy | `Platform.Health` | `platform.health` | `platform.health` | `GET /healthz/platform` |

Nothing here destroys anything. Destroying is something the API asks a human to approve, so the libraries leave it out.

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

## Tests

Every library is tested the same way, and every test makes real HTTP requests:

- **Unit tests** run each SDK against a local fake of the API. The fake answers with
  the shared fixtures in [`testdata/api/`](testdata/api), whose shapes come from the
  engine's own handlers, and records every request. The tests then check the method,
  path, query, headers and body that went out, and what came back. They need no
  network or credentials:

  ```sh
  (cd go && go test ./...)
  (cd node && npm install && npm test)
  (cd python && PYTHONPATH=src python3 -m unittest discover -s tests)
  ```

- **Integration tests** run one scenario against a real Gagarin engine: health,
  whoami, creating and reading a project, running a job that exits 0 and one that
  exits 3, reading logs and history, and checking that refusals come back as typed
  errors. They are skipped unless `GAGARIN_INTEGRATION=1`, and
  [`scripts/integration.sh`](scripts/integration.sh) runs all three. Point them at a
  local engine, not production: every run is a billed job.

## Releasing

All three libraries share one version. To release `0.2.0`:

1. Set `"version": "0.2.0"` in `node/package.json` and `version = "0.2.0"` in
   `python/pyproject.toml`, and merge that to `main`.
2. Tag the merge commit and push the tag:

   ```sh
   git tag -a v0.2.0 <sha> -m "v0.2.0: <what>" && git push origin v0.2.0
   ```

The [`release`](.github/workflows/release.yml) workflow runs the unit tests,
refuses the tag if the versions in step 1 disagree with it
([`scripts/check-version.sh`](scripts/check-version.sh)), publishes
`@gagarin-cloud/sdk` to npm and `gagarin` to PyPI through trusted publishing,
pushes `go/v0.2.0` (the tag Go resolves for a module in a subdirectory), and
creates the GitHub release.
