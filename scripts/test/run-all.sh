#!/usr/bin/env bash
# run-all.sh — discover and run all test scripts in scripts/test/NN-*.sh order
# Usage: PROXY_BASE=http://localhost:3000 ./scripts/test/run-all.sh

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
export PROXY_BASE="${PROXY_BASE:-http://localhost:3000}"

TOTAL_PASS=0
TOTAL_FAIL=0

echo "Running integration tests against ${PROXY_BASE}"

for test_file in "${SCRIPT_DIR}"/[0-9][0-9]-*.sh; do
  [ -f "$test_file" ] || continue

  # Run each test in a subshell; capture output and exit code
  output=$(bash "$test_file" 2>&1)
  exit_code=$?
  echo "$output"

  pass=$(echo "$output" | grep "^Results:" | sed -E 's/.*: ([0-9]+) passed.*/\1/' || echo 0)
  fail=$(echo "$output" | grep "^Results:" | sed -E 's/.*, ([0-9]+) failed.*/\1/' || echo 0)
  TOTAL_PASS=$((TOTAL_PASS + pass))
  TOTAL_FAIL=$((TOTAL_FAIL + fail))
done

echo ""
echo "========================================"
echo "TOTAL: ${TOTAL_PASS} passed, ${TOTAL_FAIL} failed"
echo "========================================"

[ "$TOTAL_FAIL" -eq 0 ]
