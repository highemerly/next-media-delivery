# CLAUDE.md

## 基本ルール
- 応答は日本語で行うこと
- 仕様・動作変更を伴うコード修正は README.md の該当箇所も同時に更新する
- ビルド確認は `go build ./...`

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

### 未実装（Phase 3）
- **L2 キャッシュ（S3）**: インターフェース (`internal/cache/l2/l2.go`) と NoopStore のみ実装済み。`S3_ENABLED=true` 時の実際の S3 実装（`internal/cache/l2/s3.go`）が未実装
- **Redis バックエンド**: 設定（`STORE_BACKEND=redis`、`REDIS_ADDR` 等）は実装済みだが、NegativeCache・CircuitBreaker の Redis 実装（`internal/store/redis/`）が未実装。現状は `STORE_BACKEND=memory` のみ動作する
