#!/usr/bin/env bash
# Tests for check-version.sh: it passes when the versions agree with the tag,
# and refuses every way they can disagree.
set -euo pipefail
cd "$(dirname "$0")"

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
fail() { echo "FAIL: $*" >&2; exit 1; }

tree() { # tree NODE_VERSION PYTHON_VERSION
  rm -rf "$tmp/repo"; mkdir -p "$tmp/repo/node" "$tmp/repo/python"
  printf '{\n  "name": "@gagarin-cloud/sdk",\n  "version": "%s",\n  "devDependencies": {\n    "typescript": "^5.6.0"\n  }\n}\n' "$1" > "$tmp/repo/node/package.json"
  printf '[project]\nname = "gagarin"\nversion = "%s"\nrequires-python = ">=3.9"\n' "$2" > "$tmp/repo/python/pyproject.toml"
}
check() { ROOT="$tmp/repo" ./check-version.sh "$@" >/dev/null 2>&1; }

tree 0.2.0 0.2.0
check v0.2.0 || fail "matching versions were refused"

tree 0.1.0 0.2.0
check v0.2.0 && fail "a stale node version was accepted"

tree 0.2.0 0.1.0
check v0.2.0 && fail "a stale python version was accepted"

tree 0.2.0 0.2.0
check v0.3.0 && fail "a tag ahead of both was accepted"
check 0.2.0 && fail "a tag without its v was accepted"
check v0.2 && fail "a two-part tag was accepted"

# A dependency's version must not be mistaken for the package's own.
tree "" 0.2.0
check v0.2.0 && fail "a package.json with no version was accepted"

# And the real repository agrees with itself.
ROOT=.. ./check-version.sh "v$(sed -nE 's/^version = "([^"]+)".*/\1/p' ../python/pyproject.toml)" >/dev/null \
  || fail "the repository's own versions disagree"

echo "check-version: all cases passed"
