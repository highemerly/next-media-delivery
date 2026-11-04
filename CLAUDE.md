# CLAUDE.md

## 基本ルール
- 応答は日本語で行うこと
- 仕様・動作変更を伴うコード修正は README.md の該当箇所も同時に更新する
- ビルド確認は `go build ./...`
- main へは直接コミットしない。作業を始めるときにブランチを切り、PR（squash マージ）で取り込む（docs/development.md「ブランチと PR」）

## テスト
- インテグレーションテストは `scripts/test/NN-<name>.sh` に追加する。`lib.sh` の共通関数（`get_response`・`assert_eq` 等）を使い、Nmd-Cache・Cache-Control・Server-Timing の期待値は README.md の仕様表に従って厳密に指定すること。画像の取得元は `ORIGIN_BASE`（compose 内の nginx が `assets/` を配る）を使い、インターネット上の URL は使わない。

## CI・リリース
- CI の構成・差分での間引き・リリースの流れは docs/development.md を参照すること。ジョブの定義は `.github/workflows/checks.yml`・`e2e.yml` の1か所に置き、`ci.yml`（ブランチ）と `docker-build.yml`（タグ）の両方から呼ぶ。
- 新しい種類のファイルを足したら `.github/scripts/ci-targets.sh` の判定も見直す。

## 設計・仕様
アーキテクチャ、エンドポイント仕様、X-Cache 値一覧、環境変数一覧などの詳細は README.md を参照すること。

## 開発状況・未実装メモ

### 実装済み
- L1 ディスクキャッシュ（内部パッケージ: `internal/cache/l1`）
- in-memory NegativeCache・CircuitBreaker（`internal/store`）
- SSRF ガード、ブラックリスト（ドメイン・IP・ファイルホットリロード）
- singleflight によるオリジンフェッチ重複排除
- fallback 画像
- graceful shutdown、panic recovery
- admin HTTP ポート（3001、localhost only）: stats / purge エンドポイント
- stats / purge CLI（admin エンドポイントの HTTP ラッパー）

### 制約（ライブラリ由来・回避策で対応済み）
- **アニメーション変換不可**: bimg は libvips のローダに `n=-1` を渡さないため常に先頭フレームのみをデコードする。さらに `pngload` にはページ指定プロパティが存在せず、APNG はアニメーションを読めない。対応として `emoji=1` / `avatar=1` はアニメーション入力（`internal/imageinfo` で判定）を変換せず原本のまま返す。アニメーションを保ったままの縮小・WebP 化を実装するには bimg → govips 移行（GIF/WebP のみ）＋ APNG は別デコーダが必要

### 未実装（Phase 3）
- **L2 キャッシュ（S3）**: インターフェース (`internal/cache/l2/l2.go`) と NoopStore のみ実装済み。`S3_ENABLED=true` 時の実際の S3 実装（`internal/cache/l2/s3.go`）が未実装
- **Redis バックエンド**: 設定（`STORE_BACKEND=redis`、`REDIS_ADDR` 等）は実装済みだが、NegativeCache・CircuitBreaker の Redis 実装（`internal/store/redis/`）が未実装。現状は `STORE_BACKEND=memory` のみ動作する
