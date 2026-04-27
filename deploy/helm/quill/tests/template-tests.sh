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

# Scenario 2: IRSA — sidecar present, ServiceAccount carries IRSA annotation, no inline Secret
render "irsa" "$TESTS_DIR/values-irsa.yaml"
grep -q 'name: s3-sync' "$TMP/irsa.yaml" \
    || fail "scenario 2: sidecar missing"
grep -q 'restartPolicy: Always' "$TMP/irsa.yaml" \
    || fail "scenario 2: sidecar missing restartPolicy: Always"
grep -q 'image: "rclone/rclone:1.66"' "$TMP/irsa.yaml" \
    || fail "scenario 2: sidecar wrong image"
grep -q 'kind: ServiceAccount' "$TMP/irsa.yaml" \
    || fail "scenario 2: ServiceAccount missing"
grep -q 'eks.amazonaws.com/role-arn: "arn:aws:iam::123456789012:role/quill-s3-backup"' "$TMP/irsa.yaml" \
    || fail "scenario 2: IRSA role annotation missing or wrong"
grep -q 'serviceAccountName: r-quill-kiro' "$TMP/irsa.yaml" \
    || fail "scenario 2: pod missing serviceAccountName"
grep -q 'name: PREFIX' "$TMP/irsa.yaml" \
    || fail "scenario 2: PREFIX env missing"
grep -q 'value: "prod/quill/kiro"' "$TMP/irsa.yaml" \
    || fail "scenario 2: PREFIX env wrong value"
grep -q 'name: BUCKET' "$TMP/irsa.yaml" \
    || fail "scenario 2: BUCKET env missing"
grep -q 'value: "my-quill-backups"' "$TMP/irsa.yaml" \
    || fail "scenario 2: BUCKET env wrong value"
grep -q 'terminationGracePeriodSeconds: 60' "$TMP/irsa.yaml" \
    || fail "scenario 2: terminationGracePeriodSeconds not bumped to 60"
# IRSA mode must NOT inject AWS_ACCESS_KEY_ID env var
grep -q 'name: AWS_ACCESS_KEY_ID' "$TMP/irsa.yaml" \
    && fail "scenario 2: IRSA mode should not inject AWS_ACCESS_KEY_ID env"
# IRSA mode must NOT render a chart-managed s3-creds Secret
grep -q '\-s3-creds' "$TMP/irsa.yaml" \
    && fail "scenario 2: IRSA mode should not render s3-creds Secret"
pass "scenario 2: IRSA mode renders sidecar + SA + IRSA annotation"

echo
echo "All scenarios passed."
