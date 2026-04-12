#!/usr/bin/env bash
# Test 07: Non-image origin response → 422
#
# https://httpbin.org/headers returns application/json.
# Without ?debug, the proxy must reject it with 422.
#
# Expected:
#   HTTP status    : 422
#   Nmd-Cache      : L1=MISS, ORI, L1=DENY/BAD_CONTENT
#   Cache-Control  : max-age=86400
source "$(dirname "$0")/lib.sh"

ORIGIN_URL="https://httpbin.org/headers"

test_bad_content_type() {
  local encoded url
  encoded=$(encode_url "$ORIGIN_URL")
  # debug なし — content-type チェックが有効
  url=$(proxy_url "07-bad-ct.json" "$encoded" "emoji")

  get_response "$url"

  local ok=0
  local nc cc

  nc=$(extract_header "nmd-cache"     "$RESP_HEADERS")
  cc=$(extract_header "cache-control" "$RESP_HEADERS")

  assert_http_status "422"                                  "$RESP_STATUS" "HTTP status"   || ok=1
  assert_eq          "L1=MISS, ORI, L1=DENY/BAD_CONTENT"  "$nc"          "Nmd-Cache"     || ok=1
  assert_eq          "max-age=86400"                        "$cc"          "Cache-Control" || ok=1

  return $ok
}

run_test "07: Non-image origin response → 422 Unprocessable Entity" test_bad_content_type
print_summary
