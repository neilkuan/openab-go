#!/usr/bin/env bash
# Hermetic chart-template tests. Each scenario renders the chart with a
# values fixture and greps the output for invariants.
#
# Usage: bash deploy/helm/quill/tests/template-tests.sh
set -euo pipefail

CHART_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TESTS_DIR="$CHART_DIR/tests"
TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT

pass() { printf '\033[32mOK\033[0m  %s\n' "$1"; }
fail() { printf '\033[31mFAIL\033[0m %s\n' "$1" >&2; exit 1; }

render() {
    local name="$1" values="$2"
    if [[ -n "$values" ]]; then
        helm template r "$CHART_DIR" -f "$values" > "$TMP/$name.yaml"
    else
        helm template r "$CHART_DIR" > "$TMP/$name.yaml"
    fi
}

# Scenario 1: backup disabled (default values) — no sidecar
render "default" ""
grep -q 'name: s3-sync' "$TMP/default.yaml" \
    && fail "scenario 1: sidecar should NOT render when backup is disabled"
pass "scenario 1: default values produce no s3-sync sidecar"

echo
echo "All scenarios passed."
