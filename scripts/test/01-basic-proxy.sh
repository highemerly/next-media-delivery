#!/usr/bin/env bash
# Test 01: Basic proxy — origin fetch path (L1 MISS → ORI)
#
# Expected (S3 disabled, so L2 omitted):
#   HTTP status    : 200
#   Nmd-Cache      : L1=MISS, ORI=200
#   Cache-Control  : max-age=31536000, immutable
#   Server-Timing  : nmdFetch;dur>=1, nmdConvert;dur>=0
#   Nmd-Cache-Key  : <sha256>, v=avatar, c=y
#   Nmd-Info       : NextMediaDelivery/<ver>, <instance>
#   Nmd-Original   : s=<size>, f=image/<format>
#   Set-Cookie     : (absent)
#   Server         : (absent)
#   Nmd-Cacheable  : (absent)
#   Access-Control-Allow-Origin : *
source "$(dirname "$0")/lib.sh"

ORIGIN_URL="https://raw.githubusercontent.com/highemerly/next-media-delivery/main/assets/test-image.png"

test_basic_proxy() {
  local encoded url
  encoded=$(encode_url "$ORIGIN_URL")
  url=$(proxy_url "01-test-image.png" "$encoded" "avatar")

  get_response "$url"  # sets RESP_STATUS, RESP_HEADERS

  local ok=0
  local ct nc cc st ck acao cl ni no

  ct=$(extract_header "content-type"                  "$RESP_HEADERS")
  nc=$(extract_header "nmd-cache"                     "$RESP_HEADERS")
  cc=$(extract_header "cache-control"                 "$RESP_HEADERS")
  st=$(extract_header "server-timing"                 "$RESP_HEADERS")
  ck=$(extract_header "nmd-cache-key"                 "$RESP_HEADERS")
  acao=$(extract_header "access-control-allow-origin" "$RESP_HEADERS")
  cl=$(extract_header "content-length"                "$RESP_HEADERS")
  ni=$(extract_header "nmd-info"                      "$RESP_HEADERS")
  no=$(extract_header "nmd-original"                  "$RESP_HEADERS")

  assert_http_status "200"                         "$RESP_STATUS" "HTTP status"                 || ok=1
  assert_match       "^image/"                     "$ct"          "Content-Type"                || ok=1
  assert_eq          "L1=MISS, ORI=200"            "$nc"          "Nmd-Cache"                   || ok=1
  assert_eq          "max-age=31536000, immutable" "$cc"          "Cache-Control"               || ok=1
  assert_server_timing_fetch_ge1                   "$st"          "Server-Timing fetch"         || ok=1
  assert_match       "^[0-9a-f]{64}, v=avatar, c=y$" "$ck"       "Nmd-Cache-Key"               || ok=1
  assert_eq          "*"                           "$acao"        "Access-Control-Allow-Origin" || ok=1
  assert_match       "^[1-9][0-9]*$"               "$cl"          "Content-Length >= 1"         || ok=1
  assert_match       "^NextMediaDelivery/[^,]+, " "$ni"          "Nmd-Info"                    || ok=1
  assert_match       "^s=[1-9][0-9]*, f=image/"   "$no"          "Nmd-Original"                || ok=1
  assert_header_absent "set-cookie"                "$RESP_HEADERS" "Set-Cookie absent"          || ok=1
  assert_header_absent "server"                    "$RESP_HEADERS" "Server absent"              || ok=1
  assert_header_absent "nmd-cacheable"             "$RESP_HEADERS" "Nmd-Cacheable absent"       || ok=1

  return $ok
}

run_test "01: Basic proxy — L1=MISS, ORI" test_basic_proxy
print_summary
