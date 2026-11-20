#!/usr/bin/env bash
set -euo pipefail
. "$(dirname "$0")/common.sh"

tmpdir="$(mktemp -d)"
trap 'rm -rf "$tmpdir"' EXIT

npx --yes js-yaml docs/req/requirements.yaml > "$tmpdir/requirements.json"
npx --yes --package ajv-cli@5 ajv validate --spec=draft2020 -s docs/req/req-schema.json -d "$tmpdir/requirements.json"
echo "requirements.yaml is valid against req-schema.json"
