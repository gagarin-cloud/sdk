#!/usr/bin/env bash
# Runs every SDK's integration test against a real gagarin API.
#
# Needs, in the environment:
#   GAGARIN_API, GAGARIN_TOKEN    the API and a credential with the deploy scope
#   GAGARIN_IT_PROJECT            a project to work in; created if missing
#   GAGARIN_IT_IMAGE_OK           an image in that project's registry space that
#                                 prints "hello" and exits 0
#   GAGARIN_IT_IMAGE_FAIL         one that exits 3
#
# Point it at a local engine (gagarin-cloud/core: engine/Taskfile.yml, `task
# dev-up` then `task run`) rather than production: every run is a job, and a
# job is billed. The tests leave their jobs (it-go-*, it-node-*, it-py-*)
# behind; destroy the project to clean up.
set -euo pipefail
cd "$(dirname "$0")/.."

for v in GAGARIN_API GAGARIN_TOKEN GAGARIN_IT_PROJECT GAGARIN_IT_IMAGE_OK GAGARIN_IT_IMAGE_FAIL; do
  [ -n "${!v:-}" ] || { echo "$v is not set" >&2; exit 2; }
done
export GAGARIN_INTEGRATION=1

echo "== go"
(cd go && go test -count=1 -run TestIntegration -v ./...)

echo "== node"
(cd node && npm run build --silent && node --test test/integration.test.js)

echo "== python"
(cd python && PYTHONPATH=src python3 -m unittest -v tests/test_integration.py)
