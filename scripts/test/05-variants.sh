#!/usr/bin/env bash
# Test 05: Variant conversion — resize, format, aspect ratio
#
# Source image: 640x480 (横長, アスペクト比 4:3)
# Variants tested: avatar, emoji, preview, badge, static
#
# fitDimensions rules (avatar/emoji/preview):
#   ratio = min(maxW/640, maxH/480)
#   avatar  (320x320): ratio = min(320/640, 320/480) = min(0.5, 0.666) = 0.5 → 320x240
#   emoji   (128x128): ratio = min(128/640, 128/480) = min(0.2, 0.266) = 0.2 → 128x96
#   preview (200x200): ratio = min(200/640, 200/480) = min(0.312, 0.416) = 0.312 → 200x150 (rounded)
#   badge   (96x96):   embed=true → exact 96x96
#   static:            WebP, original dimensions (640x480)
#
# Format conversion (JPEG/WebP/AVIF → WebP) tested for emoji variant.
source "$(dirname "$0")/lib.sh"

BASE_URL="https://raw.githubusercontent.com/highemerly/next-media-delivery/main/assets"

# helper: fetch image body, run assertions, clean up
# Usage: test_variant <variant_flag> <filename_suffix> <origin_url> <expected_content_type_pattern> \
#                     <dim_check_fn> [args...]
# We inline each test for clarity.

# ---- avatar (WebP, 320x240) ----
test_avatar() {
  local encoded url body_file
  encoded=$(encode_url "${BASE_URL}/test-large.png")
  url=$(proxy_url "05-avatar.png" "$encoded" "avatar")

  get_response "$url"
  local ct nc cc
  ct=$(extract_header "content-type"  "$RESP_HEADERS")
  nc=$(extract_header "nmd-cache"     "$RESP_HEADERS")
  cc=$(extract_header "cache-control" "$RESP_HEADERS")

  local ok=0
  assert_http_status "200"                         "$RESP_STATUS" "HTTP status"    || ok=1
  assert_match       "^image/webp"                 "$ct"          "Content-Type"   || ok=1
  assert_eq          "L1=MISS, ORI=200"            "$nc"          "Nmd-Cache"      || ok=1
  assert_eq          "max-age=31536000, immutable" "$cc"          "Cache-Control"  || ok=1

  body_file=$(mktemp).webp
  curl -sf "$url" -o "$body_file"
  assert_image_dimensions          "320x240" "$body_file" "dimensions (320x240)"         || ok=1
  assert_aspect_ratio_preserved    640 480   "$body_file" "aspect ratio preserved (4:3)" || ok=1
  rm -f "$body_file"
  return $ok
}

# ---- emoji (WebP, 128x96) ----
test_emoji() {
  local encoded url body_file
  encoded=$(encode_url "${BASE_URL}/test-large.png")
  url=$(proxy_url "05-emoji.png" "$encoded" "emoji")

  get_response "$url"
  local ct; ct=$(extract_header "content-type" "$RESP_HEADERS")

  local ok=0
  assert_http_status "200"       "$RESP_STATUS" "HTTP status"  || ok=1
  assert_match       "^image/webp" "$ct"        "Content-Type" || ok=1

  body_file=$(mktemp).webp
  curl -sf "$url" -o "$body_file"
  assert_image_dimensions       "128x96" "$body_file" "dimensions (128x96)"          || ok=1
  assert_aspect_ratio_preserved 640 480  "$body_file" "aspect ratio preserved (4:3)" || ok=1
  rm -f "$body_file"
  return $ok
}

# ---- preview (WebP, 200x150) ----
test_preview() {
  local encoded url body_file
  encoded=$(encode_url "${BASE_URL}/test-large.png")
  url=$(proxy_url "05-preview.png" "$encoded" "preview")

  get_response "$url"
  local ct; ct=$(extract_header "content-type" "$RESP_HEADERS")

  local ok=0
  assert_http_status "200"         "$RESP_STATUS" "HTTP status"  || ok=1
  assert_match       "^image/webp" "$ct"          "Content-Type" || ok=1

  body_file=$(mktemp).webp
  curl -sf "$url" -o "$body_file"
  assert_image_dimensions       "200x150" "$body_file" "dimensions (200x150)"         || ok=1
  assert_aspect_ratio_preserved 640 480   "$body_file" "aspect ratio preserved (4:3)" || ok=1
  rm -f "$body_file"
  return $ok
}

# ---- badge (PNG, exact 96x96) ----
test_badge() {
  local encoded url body_file
  encoded=$(encode_url "${BASE_URL}/test-large.png")
  url=$(proxy_url "05-badge.png" "$encoded" "badge")

  get_response "$url"
  local ct; ct=$(extract_header "content-type" "$RESP_HEADERS")

  local ok=0
  assert_http_status "200"        "$RESP_STATUS" "HTTP status"  || ok=1
  assert_match       "^image/png" "$ct"          "Content-Type" || ok=1

  body_file=$(mktemp).png
  curl -sf "$url" -o "$body_file"
  assert_image_dimensions "96x96" "$body_file" "dimensions (exact 96x96)" || ok=1
  rm -f "$body_file"
  return $ok
}

# ---- static (WebP, original 640x480) ----
test_static() {
  local encoded url body_file
  encoded=$(encode_url "${BASE_URL}/test-large.png")
  url=$(proxy_url "05-static.png" "$encoded" "static")

  get_response "$url"
  local ct; ct=$(extract_header "content-type" "$RESP_HEADERS")

  local ok=0
  assert_http_status "200"         "$RESP_STATUS" "HTTP status"            || ok=1
  assert_match       "^image/webp" "$ct"          "Content-Type"           || ok=1

  body_file=$(mktemp).webp
  curl -sf "$url" -o "$body_file"
  assert_image_dimensions "640x480" "$body_file" "dimensions (original 640x480)" || ok=1
  rm -f "$body_file"
  return $ok
}

# ---- JPEG → WebP 変換 (emoji) ----
test_format_jpeg() {
  local encoded url body_file
  encoded=$(encode_url "${BASE_URL}/test-large.jpg")
  url=$(proxy_url "05-emoji-jpg.jpg" "$encoded" "emoji")

  get_response "$url"
  local ct; ct=$(extract_header "content-type" "$RESP_HEADERS")

  local ok=0
  assert_http_status "200"         "$RESP_STATUS" "HTTP status"         || ok=1
  assert_match       "^image/webp" "$ct"          "Content-Type (WebP)" || ok=1
  return $ok
}

# ---- WebP → WebP 変換 (emoji) ----
test_format_webp() {
  local encoded url body_file
  encoded=$(encode_url "${BASE_URL}/test-large.webp")
  url=$(proxy_url "05-emoji-webp.webp" "$encoded" "emoji")

  get_response "$url"
  local ct; ct=$(extract_header "content-type" "$RESP_HEADERS")

  local ok=0
  assert_http_status "200"         "$RESP_STATUS" "HTTP status"         || ok=1
  assert_match       "^image/webp" "$ct"          "Content-Type (WebP)" || ok=1
  return $ok
}

# ---- AVIF → WebP 変換 (emoji) ----
test_format_avif() {
  local encoded url
  encoded=$(encode_url "${BASE_URL}/test-large.avif")
  url=$(proxy_url "05-emoji-avif.webp" "$encoded" "emoji")

  get_response "$url"
  local ct; ct=$(extract_header "content-type" "$RESP_HEADERS")

  local ok=0
  assert_http_status "200"         "$RESP_STATUS" "HTTP status"         || ok=1
  assert_match       "^image/webp" "$ct"          "Content-Type (WebP)" || ok=1
  return $ok
}

run_test "05a: avatar  — WebP 320x240, aspect ratio preserved" test_avatar
run_test "05b: emoji   — WebP 128x96,  aspect ratio preserved" test_emoji
run_test "05c: preview — WebP 200x150, aspect ratio preserved" test_preview
run_test "05d: badge   — PNG  96x96,   exact fit"              test_badge
run_test "05e: static  — WebP 640x480, no resize"              test_static
run_test "05f: JPEG → WebP (emoji)"                            test_format_jpeg
run_test "05g: WebP → WebP (emoji)"                            test_format_webp
run_test "05h: AVIF → WebP (emoji)"                            test_format_avif
print_summary
