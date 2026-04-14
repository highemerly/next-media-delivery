#!/usr/bin/env bash
# Test 09: Admin HTTP API (port 3001, localhost-only)
#
# admin サーバーは 127.0.0.1 にのみバインドされているため、
# docker compose exec 経由でコンテナ内から curl を呼ぶ。
#
# 09a: GET /stats          → 200 JSON (l1.file_count フィールドが存在する)
# 09b: DELETE /cache/{key} → キャッシュに入れてから個別削除 → 204、その後 L1=MISS
# 09c: DELETE /cache       → 全削除 → file_count=0
source "$(dirname "$0")/lib.sh"

ADMIN_BASE="http://127.0.0.1:${ADMIN_PORT:-3001}"
ORIGIN_URL="https://raw.githubusercontent.com/highemerly/next-media-delivery/main/assets/test-image.png"

# コンテナ内で curl を実行するラッパー
admin_curl() {
  docker compose exec -T media-delivery curl -s "$@"
}

# ---- 09a: GET /stats → 200 JSON ----
test_stats() {
  local body
  body=$(admin_curl "${ADMIN_BASE}/stats")
  if [ -z "$body" ]; then
    echo "  [FAIL] stats: empty response"
    return 1
  fi

  # l1.file_count フィールドが存在すること
  local count
  count=$(python3 -c "import json,sys; d=json.loads(sys.stdin.read()); print(d['l1']['file_count'])" <<< "$body" 2>/dev/null)
  if [ $? -eq 0 ] && [ -n "$count" ]; then
    echo "  [OK] stats: JSON valid, l1.file_count=${count}"
    return 0
  else
    echo "  [FAIL] stats: cannot parse l1.file_count from response"
    return 1
  fi
}

# ---- 09b: DELETE /cache/{key} → 204、その後 L1=MISS ----
test_purge_key() {
  local encoded url key status nc

  # キャッシュに載せる
  encoded=$(encode_url "$ORIGIN_URL")
  url=$(proxy_url "09b-purge.png" "$encoded" "emoji")
  curl -sf "$url" -o /dev/null  # warm up

  # Nmd-Cache-Key を取得（sha256 部分のみ抽出）
  get_response "$url"
  local raw_key
  raw_key=$(extract_header "nmd-cache-key" "$RESP_HEADERS")
  key=$(echo "$raw_key" | cut -d',' -f1 | tr -d ' ')
  if [ -z "$key" ]; then
    echo "  [FAIL] purge-key: could not get Nmd-Cache-Key"
    return 1
  fi

  # DELETE /cache/{key}
  status=$(admin_curl -o /dev/null -w "%{http_code}" -X DELETE "${ADMIN_BASE}/cache/${key}")
  if [ "$status" != "204" ]; then
    echo "  [FAIL] purge-key: DELETE returned ${status}, expected 204"
    return 1
  fi
  echo "  [OK] purge-key: DELETE /cache/${key} → 204"

  # 削除後は L1=MISS になること
  get_response "$url"
  nc=$(extract_header "nmd-cache" "$RESP_HEADERS")
  if echo "$nc" | grep -q "L1=MISS"; then
    echo "  [OK] purge-key: after purge, Nmd-Cache contains L1=MISS ('${nc}')"
    return 0
  else
    echo "  [FAIL] purge-key: expected L1=MISS after purge, got '${nc}'"
    return 1
  fi
}

# ---- 09c: DELETE /cache → file_count=0 ----
test_purge_all() {
  local encoded url status count body

  # キャッシュに何か入れる
  encoded=$(encode_url "$ORIGIN_URL")
  url=$(proxy_url "09c-purge-all.png" "$encoded" "avatar")
  curl -sf "$url" -o /dev/null  # warm up

  # DELETE /cache → {"deleted": N} を返す
  local resp deleted
  resp=$(admin_curl -w "\n%{http_code}" -X DELETE "${ADMIN_BASE}/cache")
  status=$(echo "$resp" | tail -1)
  body=$(echo "$resp" | head -1)
  if [ "$status" != "200" ]; then
    echo "  [FAIL] purge-all: DELETE returned ${status}, expected 200"
    return 1
  fi
  deleted=$(python3 -c "import json,sys; print(json.loads(sys.stdin.read())['deleted'])" <<< "$body" 2>/dev/null)
  echo "  [OK] purge-all: DELETE /cache → 200, deleted=${deleted}"

  # file_count=0 を確認
  body=$(admin_curl "${ADMIN_BASE}/stats")
  count=$(python3 -c "import json,sys; d=json.loads(sys.stdin.read()); print(d['l1']['file_count'])" <<< "$body" 2>/dev/null)
  if [ "$count" = "0" ]; then
    echo "  [OK] purge-all: l1.file_count=0 after purge"
    return 0
  else
    echo "  [FAIL] purge-all: expected file_count=0, got '${count}'"
    return 1
  fi
}

run_test "09a: GET /stats → JSON with l1.file_count"    test_stats
run_test "09b: DELETE /cache/{key} → 204, then L1=MISS" test_purge_key
run_test "09c: DELETE /cache (all) → 204, file_count=0" test_purge_all
print_summary
