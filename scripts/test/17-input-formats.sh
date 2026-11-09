#!/usr/bin/env bash
# Test 17: 変換に渡す入力の制限（形式の許可リスト・画素数の上限）
#
# 素材:
#   test-pdf-as-png.png … 中身は PDF。拡張子 .png なので origin は image/png で配る
#   test-emoji.svg      … 40x30 の SVG
#   test-bomb.svg       … 20000x20000 の SVG（ファイルは小さいが CONVERT_MAX_PIXELS を超える）
#
# 仕様（README「変換に渡す入力の制限」）:
#   変換する variant は JPEG/PNG/GIF/WebP/HEIF/AVIF/SVG 以外を libvips に渡さない → 変換失敗の扱い
#   画素数が CONVERT_MAX_PIXELS（既定 1億）を超える入力も変換失敗の扱い
#
# 17a: PDF を image/png と偽って preview=1            → 502, L1=MISS, ORI=200, ORI=ERR, max-age=120, must-revalidate
# 17b: 同上 + fallback                                → 200, 原本のまま, L1=MISS, ORI=200, L1=FALLBACK, max-age=86400
# 17c: SVG + emoji=1                                  → 200, image/webp 40x30, L1=MISS, ORI=200, max-age=31536000, immutable
# 17d: 画素数の上限を超える SVG + emoji=1              → 502, L1=MISS, ORI=200, ORI=ERR, max-age=120, must-revalidate
source "$(dirname "$0")/lib.sh"

BASE_URL="${ORIGIN_BASE}"

# ---- 17a: PDF を image/png と偽る（fallback なし） ----
test_disguised_pdf() {
  local encoded url
  encoded=$(encode_url "${BASE_URL}/test-pdf-as-png.png")
  url=$(proxy_url "17a-pdf.webp" "$encoded" "preview")

  get_response "$url"
  local nc cc
  nc=$(extract_header "nmd-cache"     "$RESP_HEADERS")
  cc=$(extract_header "cache-control" "$RESP_HEADERS")

  local ok=0
  assert_http_status "502"                          "$RESP_STATUS" "HTTP status"   || ok=1
  assert_eq          "L1=MISS, ORI=200, ORI=ERR"    "$nc"          "Nmd-Cache"     || ok=1
  assert_eq          "max-age=120, must-revalidate" "$cc"          "Cache-Control" || ok=1
  return $ok
}

# ---- 17b: PDF を image/png と偽る（fallback あり → 原本をそのまま返す） ----
test_disguised_pdf_fallback() {
  local encoded url body_file
  encoded=$(encode_url "${BASE_URL}/test-pdf-as-png.png")
  url=$(proxy_url "17b-pdf-fallback.webp" "$encoded" "preview" "fallback")

  get_response "$url"
  local nc cc
  nc=$(extract_header "nmd-cache"     "$RESP_HEADERS")
  cc=$(extract_header "cache-control" "$RESP_HEADERS")

  local ok=0
  assert_http_status "200"                              "$RESP_STATUS" "HTTP status"   || ok=1
  assert_eq          "L1=MISS, ORI=200, L1=FALLBACK"    "$nc"          "Nmd-Cache"     || ok=1
  assert_eq          "max-age=86400"                    "$cc"          "Cache-Control" || ok=1

  body_file=$(mktemp)
  curl -s "$url" -o "$body_file"
  if cmp -s "$body_file" "$(dirname "$0")/../../assets/test-pdf-as-png.png"; then
    echo "  [OK] body: original bytes"
  else
    echo "  [FAIL] body: expected the original bytes"
    ok=1
  fi
  rm -f "$body_file"
  return $ok
}

# ---- 17c: SVG は引き続き変換する ----
test_svg_emoji() {
  local encoded url body_file
  encoded=$(encode_url "${BASE_URL}/test-emoji.svg")
  url=$(proxy_url "17c-svg.webp" "$encoded" "emoji")

  get_response "$url"
  local ct nc cc
  ct=$(extract_header "content-type"  "$RESP_HEADERS")
  nc=$(extract_header "nmd-cache"     "$RESP_HEADERS")
  cc=$(extract_header "cache-control" "$RESP_HEADERS")

  local ok=0
  assert_http_status "200"                         "$RESP_STATUS" "HTTP status"   || ok=1
  assert_match       "^image/webp"                 "$ct"          "Content-Type"  || ok=1
  assert_eq          "L1=MISS, ORI=200"            "$nc"          "Nmd-Cache"     || ok=1
  assert_eq          "max-age=31536000, immutable" "$cc"          "Cache-Control" || ok=1

  get_image_body "$url"
  body_file="$RESP_BODY_FILE"
  assert_image_dimensions "40x30" "$body_file" "dimensions" || ok=1
  rm -f "$body_file"
  return $ok
}

# ---- 17d: 画素数の上限を超える SVG ----
test_pixel_bomb() {
  local encoded url
  encoded=$(encode_url "${BASE_URL}/test-bomb.svg")
  url=$(proxy_url "17d-bomb.webp" "$encoded" "emoji")

  get_response "$url"
  local nc cc
  nc=$(extract_header "nmd-cache"     "$RESP_HEADERS")
  cc=$(extract_header "cache-control" "$RESP_HEADERS")

  local ok=0
  assert_http_status "502"                          "$RESP_STATUS" "HTTP status"   || ok=1
  assert_eq          "L1=MISS, ORI=200, ORI=ERR"    "$nc"          "Nmd-Cache"     || ok=1
  assert_eq          "max-age=120, must-revalidate" "$cc"          "Cache-Control" || ok=1
  return $ok
}

run_test "17a: PDF disguised as image/png + preview → 502"          test_disguised_pdf
run_test "17b: PDF disguised as image/png + fallback → original"    test_disguised_pdf_fallback
run_test "17c: SVG + emoji → WebP 40x30"                             test_svg_emoji
run_test "17d: SVG over CONVERT_MAX_PIXELS + emoji → 502"            test_pixel_bomb
print_summary
