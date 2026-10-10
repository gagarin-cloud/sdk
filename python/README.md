# gagarin — Python

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

API refusals raise `GagarinError` with `status`, `code`, `message` and `fix_hint`.

```sh
PYTHONPATH=src python -m unittest discover -s tests
```
