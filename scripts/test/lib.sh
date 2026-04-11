#!/usr/bin/env bash
# lib.sh — shared helpers for integration tests
# Source this file from each test script: source "$(dirname "$0")/lib.sh"

set -euo pipefail

PROXY_BASE="${PROXY_BASE:-http://localhost:3000}"
PASS=0
FAIL=0

# encode_url <raw_url>
encode_url() {
  python3 -c "import urllib.parse, sys; print(urllib.parse.quote(sys.argv[1], safe=''))" "$1"
}

# proxy_url <filename> <encoded_url> [flag ...]
# e.g. proxy_url "img.png" "$enc" "avatar"
#      proxy_url "img.png" "$enc" "avatar" "fallback"
proxy_url() {
  local filename="$1" encoded="$2"
  shift 2
  local qs="${PROXY_BASE}/proxy/${filename}?url=${encoded}"
  for flag in "$@"; do
    qs="${qs}&${flag}"
  done
  echo "$qs"
}

# get_response <url>
# Fetches headers and status code in a single request.
# Stores results in: RESP_STATUS, RESP_HEADERS
get_response() {
  local url="$1" tmpfile
  tmpfile=$(mktemp)
  RESP_STATUS=$(curl -sI -o "$tmpfile" -w "%{http_code}" "$url")
  RESP_HEADERS=$(tr -d '\r' < "$tmpfile")
  rm -f "$tmpfile"
}

# get_http_status <url>  (single request; use get_response when headers are also needed)
get_http_status() {
  curl -o /dev/null -s -w "%{http_code}" "$1"
}

# get_all_headers <url>  (full header block, \r stripped)
get_all_headers() {
  curl -sI "$1" | tr -d '\r'
}

# extract_header <header_name> <header_block>  (case-insensitive)
extract_header() {
  local name="$1" block="$2"
  echo "$block" | grep -i "^${name}:" | head -1 | cut -d' ' -f2-
}

# assert_http_status <expected> <actual> <label>
assert_http_status() {
  local expected="$1" actual="$2" label="$3"
  if [ "$actual" = "$expected" ]; then
    echo "  [OK] ${label}: HTTP ${actual}"
    return 0
  else
    echo "  [FAIL] ${label}: expected HTTP ${expected}, got ${actual}"
    return 1
  fi
}

# assert_eq <expected> <actual> <label>
assert_eq() {
  local expected="$1" actual="$2" label="$3"
  if [ "$actual" = "$expected" ]; then
    echo "  [OK] ${label}: '${actual}'"
    return 0
  else
    echo "  [FAIL] ${label}: expected '${expected}', got '${actual}'"
    return 1
  fi
}

# assert_header_absent <header_name> <header_block> <label>
# Checks that a header does NOT appear in the response.
assert_header_absent() {
  local name="$1" block="$2" label="$3"
  if echo "$block" | grep -qi "^${name}:"; then
    local value
    value=$(echo "$block" | grep -i "^${name}:" | head -1 | cut -d' ' -f2-)
    echo "  [FAIL] ${label}: header '${name}' must be absent, got '${value}'"
    return 1
  else
    echo "  [OK] ${label}: header '${name}' is absent"
    return 0
  fi
}

# assert_match <pattern> <actual> <label>  (grep -qE extended regex)
assert_match() {
  local pattern="$1" actual="$2" label="$3"
  if echo "$actual" | grep -qE "$pattern"; then
    echo "  [OK] ${label}: '${actual}' matches '${pattern}'"
    return 0
  else
    echo "  [FAIL] ${label}: '${actual}' does not match '${pattern}'"
    return 1
  fi
}

# assert_server_timing_fetch_ge1 <server_timing_value> <label>
# Checks fetch;dur >= 1 (origin was actually contacted)
assert_server_timing_fetch_ge1() {
  local st="$1" label="$2"
  local fetch_dur
  fetch_dur=$(echo "$st" | sed -E 's/.*fetch;dur=([0-9]+).*/\1/')
  if [ -n "$fetch_dur" ] && [ "$fetch_dur" -ge 1 ]; then
    echo "  [OK] ${label}: fetch;dur=${fetch_dur} (>= 1)"
    return 0
  else
    echo "  [FAIL] ${label}: expected fetch;dur >= 1, got '${st}'"
    return 1
  fi
}

# assert_server_timing_fetch_zero <server_timing_value> <label>
# Checks fetch;dur = 0 (cache hit, no origin contact)
assert_server_timing_fetch_zero() {
  local st="$1" label="$2"
  local fetch_dur
  fetch_dur=$(echo "$st" | sed -E 's/.*fetch;dur=([0-9]+).*/\1/')
  if [ -n "$fetch_dur" ] && [ "$fetch_dur" -eq 0 ]; then
    echo "  [OK] ${label}: fetch;dur=0 (cache hit)"
    return 0
  else
    echo "  [FAIL] ${label}: expected fetch;dur=0, got '${st}'"
    return 1
  fi
}

# assert_last_modified_not_after <before_epoch> <http_date_value> <label>
# Checks Last-Modified <= before_epoch (reflects cache-write time, not current time)
assert_last_modified_not_after() {
  local before_epoch="$1" lm_value="$2" label="$3"
  if [ -z "$lm_value" ]; then
    echo "  [FAIL] ${label}: Last-Modified header is absent"
    return 1
  fi
  local lm_epoch
  lm_epoch=$(python3 -c "
from email.utils import parsedate_to_datetime
import sys
print(int(parsedate_to_datetime(sys.argv[1]).timestamp()))
" "$lm_value" 2>/dev/null || echo 0)

  if [ "$lm_epoch" -le "$before_epoch" ]; then
    echo "  [OK] ${label}: Last-Modified(${lm_value}) <= cache-write time"
    return 0
  else
    echo "  [FAIL] ${label}: Last-Modified(epoch=${lm_epoch}) is AFTER cache-write time(${before_epoch})"
    return 1
  fi
}

# run_test <name> <function>
run_test() {
  local name="$1" fn="$2"
  echo ""
  echo "==> ${name}"
  if $fn; then
    PASS=$((PASS + 1))
  else
    FAIL=$((FAIL + 1))
  fi
}

# print_summary — call at end of each test script
print_summary() {
  echo ""
  echo "========================================"
  echo "Results: ${PASS} passed, ${FAIL} failed"
  echo "========================================"
  [ "$FAIL" -eq 0 ]
}
