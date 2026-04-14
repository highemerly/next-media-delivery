#!/usr/bin/env bash
# Test 02: L1 cache hit — 2nd request served from disk cache
#
# Expected on 2nd request:
#   HTTP status    : 200
#   Nmd-Cache      : L1=HIT
#   Cache-Control  : max-age=31536000, immutable
#   Server-Timing  : nmdFetch;dur=0, nmdConvert;dur=0
#   Last-Modified  : <= time recorded just before the 1st request (cache-write time)
source "$(dirname "$0")/lib.sh"

ORIGIN_URL="https://raw.githubusercontent.com/highemerly/next-media-delivery/main/assets/test-image.png"

test_cache_hit() {
  local encoded url
  encoded=$(encode_url "$ORIGIN_URL")
  # Use a unique filename to avoid interference with test 01
  url=$(proxy_url "02-test-image.png" "$encoded" "avatar")

  # 1st request — populate L1; record epoch just before so Last-Modified must be <= it
  local before_epoch
  before_epoch=$(date +%s)
  get_response "$url"
  sleep 3

  # 2nd request — must hit L1
  get_response "$url"  # sets RESP_STATUS, RESP_HEADERS

  local ok=0
  local nc cc st lm

  nc=$(extract_header "nmd-cache"     "$RESP_HEADERS")
  cc=$(extract_header "cache-control" "$RESP_HEADERS")
  st=$(extract_header "server-timing" "$RESP_HEADERS")
  lm=$(extract_header "last-modified" "$RESP_HEADERS")

  assert_eq    "L1=HIT"                       "$nc" "Nmd-Cache"           || ok=1
  assert_eq    "max-age=31536000, immutable"   "$cc" "Cache-Control"       || ok=1
  assert_server_timing_fetch_zero              "$st" "Server-Timing fetch" || ok=1
  assert_last_modified_not_after "$before_epoch" "$lm" "Last-Modified <= cache-write time" || ok=1

  return $ok
}

run_test "02: L1 cache hit — Nmd-Cache: L1=HIT, nmdFetch;dur=0, Last-Modified at store time" test_cache_hit
print_summary
