#!/usr/bin/env bash
# Test 07: Non-image origin response → 422 / debug key bypass
#
# httpbin container (/headers) returns application/json.
# Without ?debug, the proxy must reject it with 422.
# With ?debug=<DEBUG_KEY> (correct key), the proxy must pass it through (200).
# With ?debug=wrongkey (wrong key), the proxy must still reject it with 422.
#
# Expected (no debug):
#   HTTP status    : 422
#   Nmd-Cache      : L1=MISS, ORI=200, L1=DENY/BAD_CONTENT
#   Cache-Control  : max-age=86400
#
# Expected (debug=<DEBUG_KEY>):
#   HTTP status    : 200
#   Nmd-Cache      : L1=MISS, ORI=200
#   Cache-Control  : no-store
#
# Expected (debug=wrongkey):
#   HTTP status    : 422
#   Nmd-Cache      : L1=MISS, ORI=200, L1=DENY/BAD_CONTENT
#   Cache-Control  : max-age=86400
source "$(dirname "$0")/lib.sh"

HTTPBIN_BASE="${HTTPBIN_BASE:-http://httpbin}"
ORIGIN_URL="${HTTPBIN_BASE}/headers"
DEBUG_KEY="${DEBUG_KEY:-}"

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

  assert_http_status "422"                                      "$RESP_STATUS" "HTTP status"   || ok=1
  assert_eq          "L1=MISS, ORI=200, L1=DENY/BAD_CONTENT"  "$nc"          "Nmd-Cache"     || ok=1
  assert_eq          "max-age=86400"                            "$cc"          "Cache-Control" || ok=1

  return $ok
}

test_debug_correct_key() {
  if [ -z "$DEBUG_KEY" ]; then
    echo "  [SKIP] DEBUG_KEY not set — skipping debug bypass test"
    return 0
  fi

  local encoded url
  encoded=$(encode_url "$ORIGIN_URL")
  url=$(proxy_url "07-bad-ct-debug.json" "$encoded" "emoji" "debug=${DEBUG_KEY}")

  # Use GET: httpbin /headers returns 405 for HEAD requests
  get_response_get "$url"

  local ok=0
  local nc cc

  nc=$(extract_header "nmd-cache"     "$RESP_HEADERS")
  cc=$(extract_header "cache-control" "$RESP_HEADERS")

  assert_http_status "200"              "$RESP_STATUS" "HTTP status (debug=correct)"   || ok=1
  assert_eq          "L1=MISS, ORI=200" "$nc"          "Nmd-Cache (debug=correct)"     || ok=1
  assert_eq          "no-store"          "$cc"          "Cache-Control (debug=correct)" || ok=1

  return $ok
}

test_debug_wrong_key() {
  local encoded url
  encoded=$(encode_url "$ORIGIN_URL")
  url=$(proxy_url "07-bad-ct-wrongkey.json" "$encoded" "emoji" "debug=wrongkey")

  get_response "$url"

  local ok=0
  local nc cc

  nc=$(extract_header "nmd-cache"     "$RESP_HEADERS")
  cc=$(extract_header "cache-control" "$RESP_HEADERS")

  assert_http_status "422"                                      "$RESP_STATUS" "HTTP status (debug=wrongkey)"   || ok=1
  assert_eq          "L1=MISS, ORI=200, L1=DENY/BAD_CONTENT"  "$nc"          "Nmd-Cache (debug=wrongkey)"     || ok=1
  assert_eq          "max-age=86400"                            "$cc"          "Cache-Control (debug=wrongkey)" || ok=1

  return $ok
}

run_test "07a: Non-image origin response → 422 Unprocessable Entity" test_bad_content_type
run_test "07b: debug=<correct key> bypasses Content-Type check → 200" test_debug_correct_key
run_test "07c: debug=<wrong key> → still 422 (no bypass)" test_debug_wrong_key
print_summary
