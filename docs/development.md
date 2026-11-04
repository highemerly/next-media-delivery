# 開発の進め方

## CI

### ワークフロー

| ファイル | いつ動くか | 中身 |
|---|---|---|
| `.github/workflows/ci.yml` | main 以外のブランチへの push | 差分で回すジョブを決め、結果を `ci-result` にまとめる |
| `.github/workflows/checks.yml` | 再利用（ci.yml・docker-build.yml から呼ぶ） | `go vet ./...`・`go test ./...` |
| `.github/workflows/e2e.yml` | 再利用（ci.yml・docker-build.yml から呼ぶ） | 本番イメージを `docker-compose.yml` で起動し、`scripts/test/run-all.sh` を回す |
| `.github/workflows/docker-build.yml` | `v*` タグの push | リリース（下の「リリース」） |

ジョブの定義は `checks.yml`・`e2e.yml` の1か所だけにあり、ブランチの CI とタグのリリースの両方がそれを呼ぶ。

### ブランチでは差分が影響しうるジョブだけ回す

どれを回すかは `.github/scripts/ci-targets.sh` が main との merge-base からの差分で決める。

| 変更したファイル | checks | e2e |
|---|---|---|
| Go のソース・`go.mod`・`go.sum`・`Dockerfile`・`assets/` など（下のどれにも当たらないもの） | ○ | ○ |
| `scripts/test/*`・`docker-compose.yml`・`.env.ci` | | ○ |
| `.github/workflows/checks.yml` | ○ | |
| `.github/workflows/e2e.yml` | | ○ |
| ドキュメント・`.claude/`・`.githooks/`・ruleset・Renovate 設定・`.github/release.yml` など | | |
| `.github/workflows/docker-build.yml`（タグでしか動かないので確かめようがない） | | |

- 一覧に無い種類のファイルは両方回す（知らない種類の変更を素通しさせない）。
- `release/*` ブランチ（版の bump だけ）は何も回さない。タグの push で全部回る。
- 手元でも試せる：`.github/scripts/ci-targets.sh "$(git branch --show-current)"`（`GITHUB_OUTPUT` が無ければ標準出力に書く）。

main への push では回さない。マージ前に同じ中身をブランチで通しているので、main でも回すとほぼ二重実行になる。リリースのタグでは全ジョブを必ず回すので、ブランチで省いた検査で壊れていればそこで止まる。

### 必須チェックは `ci-result` の1つだけ

`ci-result` は全ジョブの結果をまとめ、成功とスキップは通し、失敗とキャンセルは落とす。ジョブを個別に必須にすると、差分でスキップしたジョブには結果が付かず、PR がマージできなくなる。

### インテグレーションテストを手元で回す

CI（e2e.yml）と同じ手順：

```bash
cp .env.ci .env
docker compose up -d --wait --build
CDN_NAME=ci-delivery.piyo.me DEBUG_KEY=abc123 bash scripts/test/run-all.sh
docker compose down
```

`docker-compose.yml` の media-delivery のイメージは `E2E_IMAGE`（既定 `next-media-delivery:e2e`）。CI はビルド済み・push 済みのイメージを `E2E_IMAGE` で渡し、`--no-build` で起動する。
