#!/usr/bin/env bash
# Test 07: Non-image origin response → 422 / debug key bypass / CT sniffing
#
# httpbin container (/headers) returns application/json.
# Without ?debug, the proxy must reject it with 422.
# With ?debug=<DEBUG_KEY> (correct key), the proxy must pass it through (200).
# With ?debug=wrongkey (wrong key), the proxy must still reject it with 422.
#
# The octet-origin container serves ./assets as binary/octet-stream (as B2/S3
# origins do). A disallowed Content-Type must therefore not be decisive on its
# own: media is recognised by sniffing the body, non-media is still rejected.
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
#
# Expected (binary/octet-stream + 画像バイト):
#   HTTP status    : 200
#   Nmd-Cache      : L1=MISS, ORI=200
#   Cache-Control  : max-age=31536000, immutable
#   Nmd-Original   : s=<size>, f=image/png  ← 宣言値ではなくスニッフィング結果
#
# Expected (binary/octet-stream + 非画像バイト):
#   HTTP status    : 422
#   Nmd-Cache      : L1=MISS, ORI=200, L1=DENY/BAD_CONTENT
#   Cache-Control  : max-age=86400
source "$(dirname "$0")/lib.sh"

HTTPBIN_BASE="${HTTPBIN_BASE:-http://httpbin}"
ORIGIN_URL="${HTTPBIN_BASE}/headers"
OCTET_BASE="${OCTET_BASE:-http://octet-origin}"
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
  # raw variant only — debug is not intended for use with conversion variants
  url=$(proxy_url "07-bad-ct-debug.json" "$encoded" "debug=${DEBUG_KEY}")

  get_response "$url"

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
  # raw variant only — debug is not intended for use with conversion variants
  url=$(proxy_url "07-bad-ct-wrongkey.json" "$encoded" "debug=wrongkey")

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

# ---- binary/octet-stream で配信される PNG → スニッフィングで受理・変換 ----
test_octet_stream_image() {
  local encoded url
  encoded=$(encode_url "${OCTET_BASE}/test-image.png")
  url=$(proxy_url "07-octet-image.webp" "$encoded" "avatar")

  get_response "$url"

  local ok=0
  local ct nc cc no

  ct=$(extract_header "content-type"  "$RESP_HEADERS")
  nc=$(extract_header "nmd-cache"     "$RESP_HEADERS")
  cc=$(extract_header "cache-control" "$RESP_HEADERS")
  no=$(extract_header "nmd-original"  "$RESP_HEADERS")

  assert_http_status "200"                         "$RESP_STATUS" "HTTP status"   || ok=1
  assert_match       "^image/webp"                 "$ct"          "Content-Type"  || ok=1
  assert_eq          "L1=MISS, ORI=200"            "$nc"          "Nmd-Cache"     || ok=1
  assert_eq          "max-age=31536000, immutable" "$cc"          "Cache-Control" || ok=1
  # 宣言値 binary/octet-stream ではなくスニッフィング結果が記録される
  assert_match       "^s=[1-9][0-9]*, f=image/png$" "$no"         "Nmd-Original"  || ok=1

  return $ok
}

# ---- 変換なし（rawアクセス）→ スニッフィング結果を Content-Type として返す ----
test_octet_stream_raw() {
  local encoded url
  encoded=$(encode_url "${OCTET_BASE}/test-anim.gif")
  url=$(proxy_url "07-octet-raw.gif" "$encoded")

  get_response "$url"

  local ok=0
  local ct nc

  ct=$(extract_header "content-type" "$RESP_HEADERS")
  nc=$(extract_header "nmd-cache"    "$RESP_HEADERS")

  assert_http_status "200"              "$RESP_STATUS" "HTTP status"   || ok=1
  assert_match       "^image/gif"       "$ct"          "Content-Type"  || ok=1
  assert_eq          "L1=MISS, ORI=200" "$nc"          "Nmd-Cache"     || ok=1

  return $ok
}

# ---- binary/octet-stream だが中身が非画像 → 従来通り 422 ----
test_octet_stream_non_image() {
  local encoded url
  encoded=$(encode_url "${OCTET_BASE}/robots.txt")
  url=$(proxy_url "07-octet-text.webp" "$encoded" "avatar")

  get_response "$url"

  local ok=0
  local nc cc

  nc=$(extract_header "nmd-cache"     "$RESP_HEADERS")
  cc=$(extract_header "cache-control" "$RESP_HEADERS")

  assert_http_status "422"                                     "$RESP_STATUS" "HTTP status"   || ok=1
  assert_eq          "L1=MISS, ORI=200, L1=DENY/BAD_CONTENT" "$nc"          "Nmd-Cache"     || ok=1
  assert_eq          "max-age=86400"                           "$cc"          "Cache-Control" || ok=1

  return $ok
}

run_test "07a: Non-image origin response → 422 Unprocessable Entity" test_bad_content_type
run_test "07b: debug=<correct key> bypasses Content-Type check → 200" test_debug_correct_key
run_test "07c: debug=<wrong key> → still 422 (no bypass)" test_debug_wrong_key
run_test "07d: binary/octet-stream の画像 → スニッフィングで受理 (200)" test_octet_stream_image
run_test "07e: binary/octet-stream + 変換なし → Content-Type はスニッフィング結果" test_octet_stream_raw
run_test "07f: binary/octet-stream の非画像 → 422" test_octet_stream_non_image
print_summary
