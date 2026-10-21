# Gagarin — Python

```sh
pip install gagarin
```

Python 3.9+, standard library only.

```python
from gagarin import Gagarin, GagarinError

gg = Gagarin()  # GAGARIN_TOKEN, GAGARIN_API; or Gagarin(token=...)

# What `gg run` does: submit, wait, exit with the script's code.
res = gg.jobs.run_and_wait(
    "shop", "migrate",
    image="migrate:v3",  # or registry.gagarin.cloud/<project-id>/migrate:v3
    env={"LOG_LEVEL": "debug"},
    deps=["db"],
    size="m",
    timeout_seconds=900,
    on_update=lambda run: print(run.phase),
)
raise SystemExit(res.exit_code)
```

Fire and forget, then check later:

```python
sub = gg.jobs.run("shop", "backfill", image="backfill:v1")
run = gg.jobs.wait("shop", "backfill", sub.revision)
print(gg.jobs.logs("shop", "backfill", since="30m")["logs"])
```

A scheduled job (runs nothing now):

```python
gg.jobs.run("shop", "nightly", image="report:v2", schedule="0 3 * * *", time_zone="Europe/Berlin")
```

The basics (answers are plain dicts, typed with `TypedDict`):

```python
me = gg.account.whoami()                 # me["account"], me["can"], me["registry"]
bill = gg.account.billing()              # bill["balance"], bill["runway_hours"]
projects = gg.projects.list()
p = gg.projects.create("shop")           # raises project_exists if you have one
st = gg.projects.status("shop")          # st["services"], st["notices"], st["usage_today"]
web = gg.projects.service_status("shop", "web")
logs = gg.services.logs("shop", "web", since="1h", contains="error")
history = gg.services.deployments("shop", "web")
health = gg.platform.health()            # an unhealthy platform is an answer, not an exception
```

API refusals raise `GagarinError` with `status`, `code`, `message` and `fix_hint`.

```sh
PYTHONPATH=src python -m unittest discover -s tests
```
