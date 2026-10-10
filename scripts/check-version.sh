#!/usr/bin/env bash
# Fail unless every published package carries the version a release tag names.
#
#   scripts/check-version.sh v0.2.0
#
# The release workflow runs this before publishing anything, so a tag that
# disagrees with node/package.json or python/pyproject.toml stops the release
# instead of shipping 0.1.0 to npm under a v0.2.0 tag. Go needs no check: its
# version is the go/vX.Y.Z tag the workflow pushes.
#
# ROOT overrides the repository root, which is what the test uses.
set -euo pipefail

tag=${1:?usage: $0 vX.Y.Z}
[[ $tag =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]] || { echo "not a release tag: $tag (want vX.Y.Z)" >&2; exit 2; }
want=${tag#v}
root=${ROOT:-$(cd "$(dirname "$0")/.." && pwd)}

node=$(sed -nE 's/^  "version": "([^"]+)".*/\1/p' "$root/node/package.json" | head -1)
python=$(sed -nE 's/^version = "([^"]+)".*/\1/p' "$root/python/pyproject.toml" | head -1)

bad=0
[ "$node" = "$want" ] || { echo "node/package.json is ${node:-unversioned}, the tag is $want" >&2; bad=1; }
[ "$python" = "$want" ] || { echo "python/pyproject.toml is ${python:-unversioned}, the tag is $want" >&2; bad=1; }
[ $bad -eq 0 ] || exit 1
echo "node and python are both $want"
