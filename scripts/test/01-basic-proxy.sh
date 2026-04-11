#!/usr/bin/env bash
# Test 01: Basic proxy — origin fetch path (L1 MISS → ORI)
#
# Expected (S3 disabled, so L2 omitted):
#   HTTP status    : 200
#   Nmd-Cache      : L1=MISS, ORI
#   Cache-Control  : max-age=31536000, immutable
#   Server-Timing  : fetch;dur>=1, convert;dur>=0
#   Nmd-Cache-Key  : 64-char SHA-256 hex
#   Set-Cookie     : (absent)
#   Server         : (absent)
#   Access-Control-Allow-Origin : *
source "$(dirname "$0")/lib.sh"

ORIGIN_URL="https://raw.githubusercontent.com/highemerly/next-media-delivery/main/assets/test-image.png"

test_basic_proxy() {
  local encoded url status headers
  encoded=$(encode_url "$ORIGIN_URL")
  # Use a unique filename so this test is unaffected by cache state from other tests
  url=$(proxy_url "01-test-image.png" "$encoded" "avatar")

  status=$(get_http_status "$url")
  headers=$(get_all_headers "$url")

  local ok=0
  local ct nc cc st ck acao

  ct=$(extract_header "content-type"               "$headers")
  nc=$(extract_header "nmd-cache"                  "$headers")
  cc=$(extract_header "cache-control"              "$headers")
  st=$(extract_header "server-timing"              "$headers")
  ck=$(extract_header "nmd-cache-key"              "$headers")
  acao=$(extract_header "access-control-allow-origin" "$headers")

  assert_http_status "200"                              "$status" "HTTP status"                      || ok=1
  assert_match       "^image/"                          "$ct"     "Content-Type"                     || ok=1
  assert_eq          "L1=MISS, ORI"                    "$nc"     "Nmd-Cache"                        || ok=1
  assert_eq          "max-age=31536000, immutable"      "$cc"     "Cache-Control"                    || ok=1
  assert_server_timing_fetch_ge1                        "$st"     "Server-Timing fetch"              || ok=1
  assert_match       "^[0-9a-f]{64}$"                  "$ck"     "Nmd-Cache-Key"                    || ok=1
  assert_eq          "*"                               "$acao"   "Access-Control-Allow-Origin"      || ok=1
  assert_header_absent "set-cookie"                    "$headers" "Set-Cookie absent"                || ok=1
  assert_header_absent "server"                        "$headers" "Server absent"                    || ok=1

  return $ok
}

run_test "01: Basic proxy — L1=MISS, ORI" test_basic_proxy
print_summary
