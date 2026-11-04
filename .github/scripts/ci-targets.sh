#!/usr/bin/env bash
# ブランチ push の CI（ci.yml）で、どのジョブ（checks / e2e）を回すかを決める。
# main との差分ファイルを見て、変更が影響しうるジョブだけ true にする。
#
# 方針（docs/development.md「CI」）:
# - アプリ本体（Go のソース・go.mod / go.sum・Dockerfile・assets）に触れたら両方回す。
# - 下の一覧にないファイルは両方回す（知らない種類の変更を素通しさせない）。
# - release/*（バージョンの bump だけ）は何も回さない。タグ push 時に docker-build.yml が全部回す。
#
# 使い方: ci-targets.sh <ブランチ名>。GITHUB_OUTPUT があればそこへ、無ければ標準出力へ書く（手元で試せる）。
set -euo pipefail

branch="${1:?branch name required}"
checks=false
e2e=false

if [[ "$branch" != release/* ]]; then
  base="$(git merge-base origin/main HEAD)"
  while IFS= read -r file; do
    case "$file" in
      # 動作に影響しないもの（ドキュメント・Claude Code の設定・ローカル hook・GitHub の設定・手元用の設定）
      *.md | docs/* | .claude/* | .githooks/* | .github/rulesets/* | .github/renovate.json5 | .github/release.yml | LICENSE | .gitignore | .env.example | .tool-versions) ;;
      # タグ push でしか動かない（ブランチでは確かめようがない）
      .github/workflows/docker-build.yml) ;;
      # ワークフロー単体の変更（Actions の更新など）は、それを使うジョブだけ
      .github/workflows/checks.yml) checks=true ;;
      .github/workflows/e2e.yml) e2e=true ;;
      # インテグレーションテストのスクリプトと、それを動かす compose・環境変数
      scripts/test/* | docker-compose.yml | .env.ci) e2e=true ;;
      *) checks=true; e2e=true ;;
    esac
  done < <(git diff --name-only "$base" HEAD)
fi

out="${GITHUB_OUTPUT:-/dev/stdout}"
{
  echo "checks=$checks"
  echo "e2e=$e2e"
} >> "$out"
