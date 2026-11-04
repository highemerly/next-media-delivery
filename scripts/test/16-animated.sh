#!/usr/bin/env bash
# Test 16: アニメーション画像の扱い
#
# 素材（いずれも 240x180・3フレーム）:
#   test-anim.png  … APNG
#   test-anim.gif  … アニメーションGIF
#   test-anim.webp … アニメーションWebP
#
# 仕様（README「アニメーション画像の扱い」）:
#   emoji=1 / avatar=1 + アニメーション入力 → 原本をそのまま返す（縮小・変換なし）
#   emoji=1 / avatar=1 + static=1           → 従来通り縮小＋WebP変換（静止画）
#   preview=1 / badge=1 / static=1 / ogp=1  → 常に静止画
#   変換なし（rawアクセス）                  → 原本をそのまま返す
#   キャッシュキーの variant は emoji / emoji-static に分かれる
source "$(dirname "$0")/lib.sh"

BASE_URL="${ORIGIN_BASE}"

# ---- emoji + APNG → 原本パススルー ----
test_emoji_apng() {
  local encoded url body_file
  encoded=$(encode_url "${BASE_URL}/test-anim.png")
  url=$(proxy_url "16-emoji-apng.webp" "$encoded" "emoji")

  get_response "$url"
  local ct nc cc
  ct=$(extract_header "content-type"  "$RESP_HEADERS")
  nc=$(extract_header "nmd-cache"     "$RESP_HEADERS")
  cc=$(extract_header "cache-control" "$RESP_HEADERS")

  local ok=0
  assert_http_status "200"                         "$RESP_STATUS" "HTTP status"   || ok=1
  assert_match       "^image/png"                  "$ct"          "Content-Type"  || ok=1
  assert_eq          "L1=MISS, ORI=200"            "$nc"          "Nmd-Cache"     || ok=1
  assert_eq          "max-age=31536000, immutable" "$cc"          "Cache-Control" || ok=1

  body_file=$(mktemp).png
  curl -sf "$url" -o "$body_file"
  assert_image_dimensions "240x180" "$body_file" "dimensions (原本のまま)" || ok=1
  assert_animated                   "$body_file" "APNG"                    || ok=1
  rm -f "$body_file"
  return $ok
}

# ---- emoji + static=1 + APNG → 128x96 WebP 静止画 ----
test_emoji_static_apng() {
  local encoded url body_file
  encoded=$(encode_url "${BASE_URL}/test-anim.png")
  url=$(proxy_url "16-emoji-static-apng.webp" "$encoded" "emoji" "static=1")

  get_response "$url"
  local ct; ct=$(extract_header "content-type" "$RESP_HEADERS")

  local ok=0
  assert_http_status "200"         "$RESP_STATUS" "HTTP status"  || ok=1
  assert_match       "^image/webp" "$ct"          "Content-Type" || ok=1

  body_file=$(mktemp).webp
  curl -sf "$url" -o "$body_file"
  assert_image_dimensions "128x96" "$body_file" "dimensions (128x96)" || ok=1
  assert_not_animated              "$body_file" "静止画化"            || ok=1
  rm -f "$body_file"
  return $ok
}

# ---- avatar + アニメーションGIF → 原本パススルー ----
test_avatar_gif() {
  local encoded url body_file
  encoded=$(encode_url "${BASE_URL}/test-anim.gif")
  url=$(proxy_url "16-avatar-gif.webp" "$encoded" "avatar")

  get_response "$url"
  local ct cd
  ct=$(extract_header "content-type"        "$RESP_HEADERS")
  cd=$(extract_header "content-disposition" "$RESP_HEADERS")

  local ok=0
  assert_http_status "200"                            "$RESP_STATUS" "HTTP status"         || ok=1
  assert_match       "^image/gif"                     "$ct"          "Content-Type"        || ok=1
  assert_eq          'inline; filename="test-anim.gif"' "$cd"        "Content-Disposition" || ok=1

  body_file=$(mktemp).gif
  curl -sf "$url" -o "$body_file"
  assert_image_dimensions "240x180" "$body_file" "dimensions (原本のまま)" || ok=1
  assert_animated                   "$body_file" "アニメーションGIF"       || ok=1
  rm -f "$body_file"
  return $ok
}

# ---- avatar + static=1 + アニメーションGIF → WebP 静止画 ----
test_avatar_static_gif() {
  local encoded url body_file
  encoded=$(encode_url "${BASE_URL}/test-anim.gif")
  url=$(proxy_url "16-avatar-static-gif.webp" "$encoded" "avatar" "static=1")

  get_response "$url"
  local ct; ct=$(extract_header "content-type" "$RESP_HEADERS")

  local ok=0
  assert_http_status "200"         "$RESP_STATUS" "HTTP status"  || ok=1
  assert_match       "^image/webp" "$ct"          "Content-Type" || ok=1

  body_file=$(mktemp).webp
  curl -sf "$url" -o "$body_file"
  assert_not_animated "$body_file" "静止画化" || ok=1
  rm -f "$body_file"
  return $ok
}

# ---- emoji + アニメーションWebP → 原本パススルー（縮小されない） ----
test_emoji_animated_webp() {
  local encoded url body_file
  encoded=$(encode_url "${BASE_URL}/test-anim.webp")
  url=$(proxy_url "16-emoji-anim.webp" "$encoded" "emoji")

  get_response "$url"
  local ct; ct=$(extract_header "content-type" "$RESP_HEADERS")

  local ok=0
  assert_http_status "200"         "$RESP_STATUS" "HTTP status"  || ok=1
  assert_match       "^image/webp" "$ct"          "Content-Type" || ok=1

  body_file=$(mktemp).webp
  curl -sf "$url" -o "$body_file"
  assert_image_dimensions "240x180" "$body_file" "dimensions (原本のまま)" || ok=1
  assert_animated                   "$body_file" "アニメーションWebP"      || ok=1
  rm -f "$body_file"
  return $ok
}

# ---- emoji + .avif 拡張子 + APNG → 拡張子より原本維持を優先 ----
test_emoji_avif_apng() {
  local encoded url body_file
  encoded=$(encode_url "${BASE_URL}/test-anim.png")
  url=$(proxy_url "16-emoji-apng.avif" "$encoded" "emoji")

  get_response "$url"
  local ct; ct=$(extract_header "content-type" "$RESP_HEADERS")

  local ok=0
  assert_http_status "200"        "$RESP_STATUS" "HTTP status"          || ok=1
  assert_match       "^image/png" "$ct"          "Content-Type（原本）" || ok=1

  body_file=$(mktemp).png
  curl -sf "$url" -o "$body_file"
  assert_animated "$body_file" "APNG" || ok=1
  rm -f "$body_file"
  return $ok
}

# ---- preview / ogp / static は常に静止画 ----
test_still_variants() {
  local encoded ok=0
  encoded=$(encode_url "${BASE_URL}/test-anim.png")

  local url body_file ct
  # preview: 200x150 WebP
  url=$(proxy_url "16-preview-apng.webp" "$encoded" "preview")
  get_response "$url"
  ct=$(extract_header "content-type" "$RESP_HEADERS")
  assert_match "^image/webp" "$ct" "preview Content-Type" || ok=1
  body_file=$(mktemp).webp
  curl -sf "$url" -o "$body_file"
  assert_image_dimensions "200x150" "$body_file" "preview dimensions" || ok=1
  assert_not_animated               "$body_file" "preview 静止画"     || ok=1
  rm -f "$body_file"

  # static: 原寸 WebP
  url=$(proxy_url "16-static-apng.webp" "$encoded" "static")
  body_file=$(mktemp).webp
  curl -sf "$url" -o "$body_file"
  assert_image_dimensions "240x180" "$body_file" "static dimensions" || ok=1
  assert_not_animated               "$body_file" "static 静止画"     || ok=1
  rm -f "$body_file"

  # ogp: WebP（240x180 は上限1200px以下なので原寸）
  url=$(proxy_url "16-ogp-apng.webp" "$encoded" "ogp")
  get_response "$url"
  ct=$(extract_header "content-type" "$RESP_HEADERS")
  assert_match "^image/webp" "$ct" "ogp Content-Type" || ok=1
  body_file=$(mktemp).webp
  curl -sf "$url" -o "$body_file"
  assert_not_animated "$body_file" "ogp 静止画" || ok=1
  rm -f "$body_file"

  return $ok
}

# ---- rawアクセス（変換なし）→ 原本のまま ----
test_raw_apng() {
  local encoded url body_file
  encoded=$(encode_url "${BASE_URL}/test-anim.png")
  url=$(proxy_url "16-raw-apng.png" "$encoded")

  get_response "$url"
  local ct; ct=$(extract_header "content-type" "$RESP_HEADERS")

  local ok=0
  assert_http_status "200"        "$RESP_STATUS" "HTTP status"  || ok=1
  assert_match       "^image/png" "$ct"          "Content-Type" || ok=1

  body_file=$(mktemp).png
  curl -sf "$url" -o "$body_file"
  assert_animated "$body_file" "APNG" || ok=1
  rm -f "$body_file"
  return $ok
}

# ---- emoji と emoji+static=1 でキャッシュキーが分かれる ----
test_cache_key_separation() {
  local encoded url_anim url_static key_anim key_static v_anim v_static
  encoded=$(encode_url "${BASE_URL}/test-anim.png")
  url_anim=$(proxy_url "16-key-anim.webp" "$encoded" "emoji")
  url_static=$(proxy_url "16-key-static.webp" "$encoded" "emoji" "static=1")

  get_response "$url_anim"
  key_anim=$(extract_header "nmd-cache-key" "$RESP_HEADERS" | cut -d, -f1)
  v_anim=$(extract_header "nmd-cache-key" "$RESP_HEADERS" | sed -E 's/.*v=([^,]+).*/\1/')

  get_response "$url_static"
  key_static=$(extract_header "nmd-cache-key" "$RESP_HEADERS" | cut -d, -f1)
  v_static=$(extract_header "nmd-cache-key" "$RESP_HEADERS" | sed -E 's/.*v=([^,]+).*/\1/')

  local ok=0
  assert_eq "emoji"        "$v_anim"   "variant (emoji)"        || ok=1
  assert_eq "emoji-static" "$v_static" "variant (emoji+static)" || ok=1
  if [ "$key_anim" = "$key_static" ]; then
    echo "  [FAIL] cache key separation: emoji と emoji+static=1 が同じキー '${key_anim}'"
    ok=1
  else
    echo "  [OK] cache key separation: 別キー（${key_anim} / ${key_static}）"
  fi
  return $ok
}

# ---- 非アニメーション入力は従来通り変換される（リグレッション確認） ----
test_still_png_unchanged() {
  local encoded url body_file
  encoded=$(encode_url "${BASE_URL}/test-large.png")
  url=$(proxy_url "16-emoji-still.webp" "$encoded" "emoji")

  get_response "$url"
  local ct; ct=$(extract_header "content-type" "$RESP_HEADERS")

  local ok=0
  assert_http_status "200"         "$RESP_STATUS" "HTTP status"  || ok=1
  assert_match       "^image/webp" "$ct"          "Content-Type" || ok=1

  body_file=$(mktemp).webp
  curl -sf "$url" -o "$body_file"
  assert_image_dimensions "128x96" "$body_file" "dimensions (128x96)" || ok=1
  rm -f "$body_file"
  return $ok
}

run_test "16a: emoji  + APNG          — 原本パススルー（240x180・アニメ維持）" test_emoji_apng
run_test "16b: emoji  + static=1 + APNG — WebP 128x96 静止画"                  test_emoji_static_apng
run_test "16c: avatar + アニメGIF      — 原本パススルー（image/gif）"          test_avatar_gif
run_test "16d: avatar + static=1 + GIF — WebP 静止画"                          test_avatar_static_gif
run_test "16e: emoji  + アニメWebP     — 原本パススルー（縮小されない）"       test_emoji_animated_webp
run_test "16f: emoji  + .avif + APNG   — 拡張子より原本維持を優先"             test_emoji_avif_apng
run_test "16g: preview / static / ogp  — 常に静止画"                           test_still_variants
run_test "16h: rawアクセス + APNG      — 原本のまま"                           test_raw_apng
run_test "16i: emoji と emoji+static=1 — キャッシュキー分離"                   test_cache_key_separation
run_test "16j: 非アニメーションPNG     — 従来通り WebP 変換"                   test_still_png_unchanged
print_summary
