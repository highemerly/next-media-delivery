#!/usr/bin/env bash
# Test 14: AVIF output — filename extension controls output format
#
# Spec:
#   .avif extension + emoji/avatar/preview variant → Content-Type: image/avif
#   .avif extension + badge/static/raw variant     → WebP fallback (AVIF unsupported)
#   .webp / other extension + any variant          → WebP (unchanged behaviour)
#
# Cache key includes format: SHA256(url + "|" + variant + "|" + format)
# so avatar.avif and avatar.webp are stored separately.
source "$(dirname "$0")/lib.sh"

BASE_URL="https://raw.githubusercontent.com/highemerly/next-media-delivery/main/assets"

# ---- avatar + .avif → image/avif ----
test_avatar_avif() {
  local encoded url
  encoded=$(encode_url "${BASE_URL}/test-large.png")
  url=$(proxy_url "14-avatar.avif" "$encoded" "avatar")

  get_response "$url"
  local ct nc cc
  ct=$(extract_header "content-type"  "$RESP_HEADERS")
  nc=$(extract_header "nmd-cache"     "$RESP_HEADERS")
  cc=$(extract_header "cache-control" "$RESP_HEADERS")

  local ok=0
  assert_http_status "200"                         "$RESP_STATUS" "HTTP status"   || ok=1
  assert_match       "^image/avif"                 "$ct"          "Content-Type"  || ok=1
  assert_eq          "L1=MISS, ORI=200"            "$nc"          "Nmd-Cache"     || ok=1
  assert_eq          "max-age=31536000, immutable" "$cc"          "Cache-Control" || ok=1
  return $ok
}

# ---- emoji + .avif → image/avif ----
test_emoji_avif() {
  local encoded url
  encoded=$(encode_url "${BASE_URL}/test-large.png")
  url=$(proxy_url "14-emoji.avif" "$encoded" "emoji")

  get_response "$url"
  local ct; ct=$(extract_header "content-type" "$RESP_HEADERS")

  local ok=0
  assert_http_status "200"         "$RESP_STATUS" "HTTP status"  || ok=1
  assert_match       "^image/avif" "$ct"          "Content-Type" || ok=1
  return $ok
}

# ---- preview + .avif → image/avif ----
test_preview_avif() {
  local encoded url
  encoded=$(encode_url "${BASE_URL}/test-large.png")
  url=$(proxy_url "14-preview.avif" "$encoded" "preview")

  get_response "$url"
  local ct; ct=$(extract_header "content-type" "$RESP_HEADERS")

  local ok=0
  assert_http_status "200"         "$RESP_STATUS" "HTTP status"  || ok=1
  assert_match       "^image/avif" "$ct"          "Content-Type" || ok=1
  return $ok
}

# ---- badge + .avif → PNG (AVIF fallback to PNG固定) ----
test_badge_avif_fallback() {
  local encoded url
  encoded=$(encode_url "${BASE_URL}/test-large.png")
  url=$(proxy_url "14-badge.avif" "$encoded" "badge")

  get_response "$url"
  local ct; ct=$(extract_header "content-type" "$RESP_HEADERS")

  local ok=0
  assert_http_status "200"        "$RESP_STATUS" "HTTP status"             || ok=1
  assert_match       "^image/png" "$ct"          "Content-Type (PNG固定)" || ok=1
  return $ok
}

# ---- static + .avif → WebP fallback ----
test_static_avif_fallback() {
  local encoded url
  encoded=$(encode_url "${BASE_URL}/test-large.png")
  url=$(proxy_url "14-static.avif" "$encoded" "static")

  get_response "$url"
  local ct; ct=$(extract_header "content-type" "$RESP_HEADERS")

  local ok=0
  assert_http_status "200"         "$RESP_STATUS" "HTTP status"              || ok=1
  assert_match       "^image/webp" "$ct"          "Content-Type (WebP fallback)" || ok=1
  return $ok
}

# ---- avatar + .webp → WebP (既存動作が変わっていないこと) ----
test_avatar_webp_unchanged() {
  local encoded url
  encoded=$(encode_url "${BASE_URL}/test-large.png")
  url=$(proxy_url "14-avatar.webp" "$encoded" "avatar")

  get_response "$url"
  local ct; ct=$(extract_header "content-type" "$RESP_HEADERS")

  local ok=0
  assert_http_status "200"         "$RESP_STATUS" "HTTP status"  || ok=1
  assert_match       "^image/webp" "$ct"          "Content-Type" || ok=1
  return $ok
}

# ---- キャッシュキー分離: avatar.avif と avatar.webp が別エントリであること ----
# avatar.avif を先にフェッチ済みの状態で avatar.webp を叩き、L1=MISS になることを確認。
test_cache_key_isolation() {
  local encoded url_avif url_webp
  encoded=$(encode_url "${BASE_URL}/test-large.png")
  url_avif=$(proxy_url "14-isolation.avif" "$encoded" "avatar")
  url_webp=$(proxy_url "14-isolation.webp" "$encoded" "avatar")

  # avif を先にフェッチ（キャッシュ書き込み）
  get_response "$url_avif"

  # webp は別キーなので L1 MISS になるはず
  get_response "$url_webp"
  local nc; nc=$(extract_header "nmd-cache" "$RESP_HEADERS")

  local ok=0
  assert_eq "L1=MISS, ORI=200" "$nc" "Nmd-Cache (cache key isolated from avif)" || ok=1
  return $ok
}

run_test "14a: avatar + .avif → image/avif"                  test_avatar_avif
run_test "14b: emoji  + .avif → image/avif"                  test_emoji_avif
run_test "14c: preview + .avif → image/avif"                 test_preview_avif
run_test "14d: badge + .avif → PNG (AVIF非対応、固定フォールバック)" test_badge_avif_fallback
run_test "14e: static + .avif → WebP (AVIF非対応フォールバック)"    test_static_avif_fallback
run_test "14f: avatar + .webp → WebP (既存動作不変)"               test_avatar_webp_unchanged
run_test "14g: cache key isolation (avif vs webp)"           test_cache_key_isolation
print_summary
