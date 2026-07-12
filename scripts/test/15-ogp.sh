#!/usr/bin/env bash
# Test 15: OGP card preset (ogp=1) — WebP, width ≤1200, aspect ratio preserved, no crop/upscale
#
# Spec (README「OGカード用プリセット」):
#   ogp=1 → 出力は常に WebP（.avif 拡張子を指定しても WebP）
#   幅の上限 1200px、高さの上限 4096px、アスペクト比維持・クロップなし
#   元画像が上限より小さい場合は拡大せず WebP 化のみ
#
# fitDimensions rules (ogp, maxW=1200, maxH=4096):
#   test-xlarge.png (2400x1600): ratio = min(1200/2400, 4096/1600) = 0.5 → 1200x800
#   test-large.*    (640x480):   すでに上限以下 → 拡大せず 640x480
source "$(dirname "$0")/lib.sh"

BASE_URL="https://raw.githubusercontent.com/highemerly/next-media-delivery/main/assets"

# ---- 大きい画像 → WebP 1200x800（幅上限、アスペクト比維持） ----
test_ogp_downscale() {
  local encoded url body_file
  encoded=$(encode_url "${BASE_URL}/test-xlarge.png")
  url=$(proxy_url "15-ogp.webp" "$encoded" "ogp")

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

  body_file=$(mktemp).webp
  curl -sf "$url" -o "$body_file"
  assert_image_dimensions       "1200x800" "$body_file" "dimensions (1200x800)"        || ok=1
  assert_image_dimensions_le    1200 4096  "$body_file" "fits within 1200x4096"        || ok=1
  assert_aspect_ratio_preserved 2400 1600  "$body_file" "aspect ratio preserved (3:2)" || ok=1
  rm -f "$body_file"
  return $ok
}

# ---- AVIF 入力 → WebP（SHAMEZO の実ユースケース。上限以下なので拡大なし） ----
test_ogp_avif_input() {
  local encoded url body_file
  encoded=$(encode_url "${BASE_URL}/test-large.avif")
  url=$(proxy_url "15-ogp-avif.webp" "$encoded" "ogp")

  get_response "$url"
  local ct; ct=$(extract_header "content-type" "$RESP_HEADERS")

  local ok=0
  assert_http_status "200"         "$RESP_STATUS" "HTTP status"          || ok=1
  assert_match       "^image/webp" "$ct"          "Content-Type (WebP)"  || ok=1

  body_file=$(mktemp).webp
  curl -sf "$url" -o "$body_file"
  assert_image_dimensions "640x480" "$body_file" "no upscale (640x480)" || ok=1
  rm -f "$body_file"
  return $ok
}

# ---- 上限以下の画像は拡大しない（640x480 のまま WebP 化） ----
test_ogp_no_upscale() {
  local encoded url body_file
  encoded=$(encode_url "${BASE_URL}/test-large.png")
  url=$(proxy_url "15-ogp-small.webp" "$encoded" "ogp")

  get_response "$url"
  local ct; ct=$(extract_header "content-type" "$RESP_HEADERS")

  local ok=0
  assert_http_status "200"         "$RESP_STATUS" "HTTP status"  || ok=1
  assert_match       "^image/webp" "$ct"          "Content-Type" || ok=1

  body_file=$(mktemp).webp
  curl -sf "$url" -o "$body_file"
  assert_image_dimensions       "640x480" "$body_file" "no upscale (640x480)"         || ok=1
  assert_aspect_ratio_preserved 640 480   "$body_file" "aspect ratio preserved (4:3)" || ok=1
  rm -f "$body_file"
  return $ok
}

# ---- .avif 拡張子でも ogp は WebP（AVIF 非対応 variant） ----
test_ogp_avif_ext_is_webp() {
  local encoded url
  encoded=$(encode_url "${BASE_URL}/test-large.png")
  url=$(proxy_url "15-ogp.avif" "$encoded" "ogp")

  get_response "$url"
  local ct; ct=$(extract_header "content-type" "$RESP_HEADERS")

  local ok=0
  assert_http_status "200"         "$RESP_STATUS" "HTTP status"                     || ok=1
  assert_match       "^image/webp" "$ct"          "Content-Type (WebP, AVIF非対応)" || ok=1
  return $ok
}

run_test "15a: ogp — 2400x1600 → WebP 1200x800（幅上限・アスペクト比維持）" test_ogp_downscale
run_test "15b: ogp — AVIF入力 → WebP（実ユースケース）"                     test_ogp_avif_input
run_test "15c: ogp — 640x480 → WebP 640x480（拡大なし）"                     test_ogp_no_upscale
run_test "15d: ogp + .avif → WebP（AVIF非対応 variant）"                     test_ogp_avif_ext_is_webp
print_summary
