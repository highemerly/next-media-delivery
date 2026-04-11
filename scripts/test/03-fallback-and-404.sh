#!/usr/bin/env bash
# Test 03: Fallback behaviour and 404 propagation
#
# 03a — WITH ?fallback: origin 404 → fallback image returned
#   HTTP status    : 200
#   Nmd-Cache      : L1=MISS, ORI, L1=FALLBACK
#   Cache-Control  : max-age=86400
#   Server-Timing  : fetch;dur>=1
#
# 03b — WITHOUT ?fallback: origin 404 propagated as-is
#   HTTP status    : 404
#   Nmd-Cache      : L1=MISS, ORI
#   Cache-Control  : max-age=3600
#   Server-Timing  : fetch;dur>=1
source "$(dirname "$0")/lib.sh"

# Use distinct filenames so negative-cache from 03a does not affect 03b
ORIGIN_URL_A="https://raw.githubusercontent.com/highemerly/media-delivery/main/assets/03a-does-not-exist.png"
ORIGIN_URL_B="https://raw.githubusercontent.com/highemerly/media-delivery/main/assets/03b-does-not-exist.png"

test_fallback_with_flag() {
  local encoded url status headers
  encoded=$(encode_url "$ORIGIN_URL_A")
  url=$(proxy_url "03a-missing.png" "$encoded" "avatar" "fallback")

  status=$(get_http_status "$url")
  headers=$(get_all_headers "$url")

  local ok=0
  local ct nc cc st

  ct=$(extract_header "content-type"  "$headers")
  nc=$(extract_header "nmd-cache"     "$headers")
  cc=$(extract_header "cache-control" "$headers")
  st=$(extract_header "server-timing" "$headers")

  assert_http_status "200"                         "$status" "HTTP status"          || ok=1
  assert_match       "^image/"                     "$ct"     "Content-Type"         || ok=1
  assert_eq          "L1=MISS, ORI, L1=FALLBACK"  "$nc"     "Nmd-Cache"            || ok=1
  assert_eq          "max-age=86400"               "$cc"     "Cache-Control"        || ok=1
  assert_server_timing_fetch_ge1                   "$st"     "Server-Timing fetch"  || ok=1

  return $ok
}

test_no_fallback_404() {
  local encoded url status headers
  encoded=$(encode_url "$ORIGIN_URL_B")
  url=$(proxy_url "03b-missing.png" "$encoded" "avatar")
  # No "fallback" flag — origin 404 must propagate

  status=$(get_http_status "$url")
  headers=$(get_all_headers "$url")

  local ok=0
  local nc cc st

  nc=$(extract_header "nmd-cache"     "$headers")
  cc=$(extract_header "cache-control" "$headers")
  st=$(extract_header "server-timing" "$headers")

  assert_http_status "404"             "$status" "HTTP status"          || ok=1
  assert_eq          "L1=MISS, ORI"   "$nc"     "Nmd-Cache"            || ok=1
  assert_eq          "max-age=3600"   "$cc"     "Cache-Control"        || ok=1
  assert_server_timing_fetch_ge1      "$st"     "Server-Timing fetch"  || ok=1

  return $ok
}

run_test "03a: Nonexistent origin WITH ?fallback → 200 + fallback image" test_fallback_with_flag
run_test "03b: Nonexistent origin WITHOUT ?fallback → 404 propagated"    test_no_fallback_404
print_summary
