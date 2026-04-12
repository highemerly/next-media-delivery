#!/usr/bin/env bash
# Test 10: Admin CLI (media-delivery stats)
#
# docker compose exec 経由でコンテナ内のバイナリを呼ぶ。
# CLI は admin HTTP API の薄いラッパーなので、
# 「コマンドが成功し JSON を返すこと」だけ確認する。
source "$(dirname "$0")/lib.sh"

# ---- 10a: media-delivery stats → JSON 出力 ----
test_stats_cli() {
  local output
  output=$(docker compose exec -T media-delivery \
    media-delivery stats --admin-port "${ADMIN_PORT:-3001}" 2>&1)

  if [ -z "$output" ]; then
    echo "  [FAIL] stats CLI: empty output"
    return 1
  fi

  # JSON として解析できること
  if python3 -c "import json,sys; json.loads(sys.stdin.read())" <<< "$output" 2>/dev/null; then
    echo "  [OK] stats CLI: valid JSON returned"
    return 0
  else
    echo "  [FAIL] stats CLI: output is not valid JSON"
    echo "  output: ${output}"
    return 1
  fi
}

run_test "10a: media-delivery stats → valid JSON" test_stats_cli
print_summary
