#!/usr/bin/env bash
# Test 06: Origin request headers
#
# Uses httpbin container (/headers) which echoes received request headers as JSON.
# Must use ?debug to bypass content-type check (response is application/json).
#
# Verified in one request:
# 1. User-Agent は所定の値であること
# 2. Accept は所定の値であること
# 3. Cookie / Authorization / X-Forwarded-For / Referer が送信されないこと
# 4. CDN-Loop ヘッダが付与されること
source "$(dirname "$0")/lib.sh"

# docker compose ネットワーク内のサービス名で参照する
HTTPBIN_BASE="${HTTPBIN_BASE:-http://httpbin}"
ORIGIN_URL="${HTTPBIN_BASE}/headers"
# CDN_NAME は起動時の環境変数に合わせる（未設定時は localhost）
CDN_NAME="${CDN_NAME:-localhost}"
EXPECTED_UA="NextMediaDelivery/1.0 (+https://github.com/highemerly/media-delivery; misskey compatible media proxy; instance=${CDN_NAME})"
EXPECTED_ACCEPT="image/*, video/*, audio/*, */*;q=0.8"
EXPECTED_CDN_LOOP="${CDN_NAME}; v=1.0"

test_origin_request_headers() {
  local encoded url body
  encoded=$(encode_url "$ORIGIN_URL")
  # debug フラグ必須 (application/json はそのままでは 422 になる)
  url=$(proxy_url "06-headers.json" "$encoded" "debug")

  # ボディ（JSON）を取得
  body=$(curl -sf "$url")
  if [ -z "$body" ]; then
    echo "  [FAIL] empty response body"
    return 1
  fi

  local ok=0

  # JSON から各ヘッダ値を抽出 (python3)
  extract_json_header() {
    local key="$1"
    python3 -c "
import json, sys
data = json.loads(sys.stdin.read())
headers = {k.lower(): v for k, v in data.get('headers', {}).items()}
print(headers.get('$key', ''))
" <<< "$body"
  }

  local actual_ua actual_accept actual_cdn_loop
  local actual_cookie actual_auth actual_xff actual_referer
  actual_ua=$(extract_json_header "user-agent")
  actual_accept=$(extract_json_header "accept")
  actual_cdn_loop=$(extract_json_header "cdn-loop")
  actual_cookie=$(extract_json_header "cookie")
  actual_auth=$(extract_json_header "authorization")
  actual_xff=$(extract_json_header "x-forwarded-for")
  actual_referer=$(extract_json_header "referer")

  # 1. User-Agent
  assert_eq "$EXPECTED_UA"      "$actual_ua"       "User-Agent"         || ok=1
  # 2. Accept
  assert_eq "$EXPECTED_ACCEPT"  "$actual_accept"   "Accept"             || ok=1
  # 3. 送信されないべきヘッダ
  assert_eq "" "$actual_cookie"  "Cookie absent at origin"            || ok=1
  assert_eq "" "$actual_auth"    "Authorization absent at origin"     || ok=1
  assert_eq "" "$actual_xff"     "X-Forwarded-For absent at origin"   || ok=1
  assert_eq "" "$actual_referer" "Referer absent at origin"           || ok=1
  # 4. CDN-Loop
  assert_eq "$EXPECTED_CDN_LOOP" "$actual_cdn_loop" "CDN-Loop"         || ok=1

  return $ok
}

run_test "06: Origin request headers (User-Agent / Accept / CDN-Loop / no sensitive headers)" test_origin_request_headers
print_summary
