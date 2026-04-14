#!/usr/bin/env bash
# Test 04: Negative cache — 404 from origin is stored; 2nd request served from negative cache
#
# 04a — 1st request (origin fetch):
#   HTTP status    : 404
#   Nmd-Cache      : L1=MISS, ORI=ERR
#   Cache-Control  : max-age=3600
#   Server-Timing  : nmdFetch;dur >= 1  (origin contacted)
#
# 04b — 2nd request (negative cache hit):
#   HTTP status    : 404
#   Nmd-Cache      : L1=HIT/NEGATIVE4XX
#   Cache-Control  : max-age=3600
#   Server-Timing  : nmdFetch;dur = 0   (origin NOT contacted)
source "$(dirname "$0")/lib.sh"

# Unique URL — must not collide with test 03b's negative cache entry
ORIGIN_URL="https://raw.githubusercontent.com/highemerly/next-media-delivery/main/assets/04-does-not-exist.png"

test_negative_cache_first_request() {
  local encoded url
  encoded=$(encode_url "$ORIGIN_URL")
  url=$(proxy_url "04-missing.png" "$encoded" "avatar")

  get_response "$url"  # sets RESP_STATUS, RESP_HEADERS

  local ok=0
  local nc cc st

  nc=$(extract_header "nmd-cache"     "$RESP_HEADERS")
  cc=$(extract_header "cache-control" "$RESP_HEADERS")
  st=$(extract_header "server-timing" "$RESP_HEADERS")

  assert_http_status "404"           "$RESP_STATUS" "HTTP status (origin 404)"    || ok=1
  assert_eq          "L1=MISS, ORI"  "$nc"          "Nmd-Cache"                   || ok=1
  assert_eq          "max-age=3600"  "$cc"          "Cache-Control"               || ok=1
  assert_server_timing_fetch_ge1     "$st"          "Server-Timing fetch"         || ok=1

  return $ok
}

test_negative_cache_second_request() {
  local encoded url
  encoded=$(encode_url "$ORIGIN_URL")
  url=$(proxy_url "04-missing.png" "$encoded" "avatar")

  get_response "$url"  # sets RESP_STATUS, RESP_HEADERS

  local ok=0
  local nc cc st

  nc=$(extract_header "nmd-cache"     "$RESP_HEADERS")
  cc=$(extract_header "cache-control" "$RESP_HEADERS")
  st=$(extract_header "server-timing" "$RESP_HEADERS")

  assert_http_status "404"                 "$RESP_STATUS" "HTTP status (negative cache hit)" || ok=1
  assert_eq          "L1=HIT/NEGATIVE4XX"  "$nc"          "Nmd-Cache"                        || ok=1
  assert_eq          "max-age=3600"         "$cc"          "Cache-Control"                    || ok=1
  assert_server_timing_fetch_zero           "$st"          "Server-Timing fetch"              || ok=1

  return $ok
}

run_test "04a: Negative cache — 1st request hits origin (L1=MISS, ORI)"             test_negative_cache_first_request
run_test "04b: Negative cache — 2nd request served from cache (L1=HIT/NEGATIVE4XX)" test_negative_cache_second_request
print_summary
