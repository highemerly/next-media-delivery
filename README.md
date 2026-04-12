# Media Delivery

## 概要

Go言語ベースで開発された Misskey 互換メディアプロキシ

- Kubernetes用に最適化
- 階層化されたキャッシュを持ち一定期間はオリジンへのアクセスを控える

## エンドポイント仕様（Misskey互換）

```
GET /proxy/{filename}?url={encoded_url}[&emoji=1][&avatar=1][&static=1][&preview=1][&badge=1][&fallback][&debug]
```

`filename` は CDN キャッシュ制御のための飾り（`image.webp`, `avatar.webp` など）。

| パラメータ | 効果 | 出力 | Misskey互換 |
|---|---|---|---|
| `url=` | プロキシ対象URL（必須） | — | ✅ |
| `emoji=1` | 絵文字用縮小 | 128×128以下 WebP | ✅ |
| `avatar=1` | アバター用縮小 | 320×320以下 WebP | ✅ |
| `static=1` | アニメーション先頭フレームのみ | WebP静止画 | ✅ |
| `preview=1` | プレビュー用縮小 | 200×200以下 WebP | ✅ |
| `badge=1` | Webプッシュ通知バッジ | 96×96 PNG | ✅ |
| `fallback` | エラー時にダミー画像を200で返す | variant に応じた画像 | ✅ |
| `debug` | オリジンの Content-Type チェックをスキップし画像以外も通す。キャッシュは行わない。`Cache-Control: no-store` / `Nmd-Cacheable: false` を返す | — | ❌ 独自拡張 |

レスポンスヘッダー:

| ヘッダ | 値 | 備考 |
|---|---|---|
| `Cache-Control` | エラー内容による（下記参照） | オリジンの値は無視。 |
| `Access-Control-Allow-Origin` | `*` | 固定 |
| `Content-Security-Policy` | `default-src 'none'; img-src 'self'; media-src 'self'; style-src 'unsafe-inline'` | 固定 |
| `Content-Type` | 変換後の実際のMIMEタイプ | オリジンの値を上書き |
| `Content-Disposition` | `inline; filename=...` | 元ファイル名ベース、拡張子は変換後に合わせる |
| `Last-Modified` | 変換処理を行った時刻 | 変換済みファイルの生成時刻。変換なし（rawアクセス）のみフェッチ時刻とする。いかなる場合もオリジンの値は使わない。 |
| `Age` | 削除 | オリジンの Age はプロキシ再配信時点で意味をなさない |
| `Set-Cookie` / `Server` / `X-Powered-By` / `HSTS` 等 | 削除 | オリジンのセキュリティポリシー・情報を引き継がない |
| `Nmd-Cache-Key` | SHA256ハッシュ | デバッグ用。常に出力。purge CLI との連携用 |
| `Nmd-Cache` | キャッシュヒット有無、エラー内容による（下記参照） | デバッグ用。常に出力。キャッシュ状況の確認用 |
| `Server-Timing` | `fetch;dur=143, convert;dur=28` | デバッグ用。フェッチ時間・変換時間（ms）。L1ヒット時は両方0 |
| `Timing-Allow-Origin` | `*` | デバッグ用。クロスオリジンでも Server-Timing を参照可能にする |

レスポンスステータスコード、`Nmd-Cache`、`Cache-Control` の値:

| L1 Cache | L2 Cache | Originへ送信 | Origin 応答 | その他・説明 | Nmd-Cache | Response | Cache-Control |
|---|---|---|---|---|---|---|---|
| Y | - | N | - | L1ディスクキャッシュヒット | `L1=HIT` | 200 | `max-age=31536000, immutable` |
| N | Y | N | - | L2オブジェクトストレージヒット | `L1=MISS, L2=HIT` | 200 | `max-age=31536000, immutable` |
| N | N | Y | 200 | オリジンで解決 | `L1=MISS, L2=MISS, ORI` | 200 | `max-age=31536000, immutable` |
| N | N | Y | 200 | オリジン取得成功だが画像以外の Content-Type 応答 | `L1=MISS, L2=MISS, ORI, L1=DENY/BAD_CONTENT` | 422 | `max-age=86400` |
| N | N | Y | 410 |（fallbackなし）410 Gone | `L1=MISS, L2=MISS, ORI` | 410 | `max-age=31536000, immutable` |
| N | N | Y | 4xx (410以外) |（fallbackなし）オリジンエラー | `L1=MISS, L2=MISS, ORI` | オリジンの値を引き継ぐ | `max-age=1800` |
| N | N | Y | - | オリジン応答が MAX_FILE_SIZE 超過 | `L1=MISS, L2=MISS, ORI` | 413 | `max-age=86400` |
| N | N | Y | timeout | （fallbackなし）タイムアウト・ネットワークエラー | `L1=MISS, L2=MISS, ORI=TIMEOUT` | 502 | `max-age=120, must-revalidate` |
| N | N | Y | 5xx | （fallbackなし）オリジンエラー | `L1=MISS, L2=MISS, ORI` | 502 | `max-age=120, must-revalidate` |
| N | N | Y | timeout | （fallbackあり）オリジンエラー | `L1=MISS, L2=MISS, ORI=TIMEOUT, L1=FALLBACK`| 200 | `max-age=86400` |
| N | N | Y | 4xx or 5xx | （fallbackあり）オリジンエラー | `L1=MISS, L2=MISS, ORI, L1=FALLBACK` | 200 | `max-age=86400` |
| Y (Negative/410) | - | N | - | L1 Negativeヒット（410） | `L1=HIT/NEGATIVE4XX` | 410 | `max-age=31536000, immutable` |
| Y (Negative/4xx) | - | N | - | L1 Negativeヒット（4xx、410以外） | `L1=HIT/NEGATIVE4XX` | オリジンの値を引き継ぐ | `max-age=1800` |
| Y (Negative/5xx) | - | N | - | L1 Negativeヒット（5xx） | `L1=HIT/NEGATIVE5XX` | 502 | `max-age=120, must-revalidate` |
| - | - | N | - | リクエストをブロック（IPブロック・スキームNG・CDN-Loop等） | `L1=DENY/BAD_REQ` | 403 (CDN-Loopのみ508) | `max-age=86400` |
| - | - | N | - | ブラックリストによるリクエストブロック | `L1=DENY/BAD_DOMAIN` | 403 | `max-age=86400` |
| - | - | N | - | Circuit Breaker によるドメインを遮断 | `L1=DENY/WAIT` | 503 | `max-age=1800` |

`Nmd-Cache` は 経路を `L1=結果, L2=結果, ORI` の形式で表すデバック用。L2無効時（`S3_ENABLED=false`）は L2 を省略。

L2省略例：
```
L1=MISS, ORI
L1=MISS, ORI, L1=FALLBACK
```

`Cache-Control` は、オリジンからの応答に依らず、一律で長いTTL＋immutableで設定する。ただしCDNをフロントに置いた場合も考慮するため、エラー系応答は短いTTL＋immutableなしで返す。表のmax-ageの値はデフォルト値で、実際には環境変数で設定変更可能とする。

### fallback 画像

`fallback` 指定時、オリジンエラー（404・タイムアウト等）の場合にダミー画像を200で返す。
variant ごとに返す画像を環境変数で上書き可能（未設定時はデフォルト）。

| variant | デフォルト | 環境変数 |
|---|---|---|
| `avatar=1` | 人型シルエット画像 | `FALLBACK_AVATAR` |
| `emoji=1` | カラーバー画像 | `FALLBACK_EMOJI` |
| `badge=1` | カラーバー画像（PNG） | `FALLBACK_BADGE` |
| その他 | カラーバー画像 | `FALLBACK_DEFAULT` |

環境変数にはファイルパスまたは base64 エンコード済み画像データを指定する。

Misskey への設定: `default.yml` に `mediaProxy: https://your-proxy.example.com` を追記するだけ。

## アーキテクチャ

Client→L1 Cache→L2 Cache→Origin 構成とする。L1・L2ともにlast-accessed TTLを採用する。L1はほぼ厳密なlast-accessed TTLとするが、並列実行などの場合にキャッシュは共有されない。また永続化せず、ディスク容量が増えてきたら直ぐにキャッシュを消すものとする。L2は疑似last-accessed TTLを採用し、90日など長いTTLを実現するようにする。

```
リクエスト
    ↓
[1] L1: ディスクキャッシュ（emptyDir）
    ヒット → （同期）レスポンス
    ミス   ↓
[2] L2: オブジェクトストレージ（S3互換）※ S3_ENABLED=true の場合のみ
    ヒット → （同期）レスポンス
            + （非同期）L1 に書き込み
    ミス   ↓
[3] オリジンフェッチ
         → （同期）レスポンス
           + （非同期）L1 に書き込み
           + （非同期）L2 に書き込み（S3_ENABLED=true の場合のみ）
```

### リクエスト処理パイプライン（handler 内の処理順序）

```
GET /proxy/{filename}?url=...&avatar=1
        │
        ▼
[middleware] REQUEST_TIMEOUT でコンテキストをラップ
[middleware] CDN-Loop ヘッダ検出（RFC 8586）→ ループなら 508
[middleware] panic recovery → 500
        │
        ▼
[handler/proxy.go]
  1.  クエリパース   → Variant（emoji/avatar/preview/badge/static/raw）+ wantFallback + debug フラグ
  2.  スキーム検証   → http/https 以外は即 403
  3.  キャッシュキー → SHA256(url + "|" + variant)
  4.  Nmd-Cache-Key ヘッダをセット（常に出力。値はキャッシュキーの SHA-256）
  5.  NegativeCache  → HIT: Nmd-Cache="L1=HIT/NEGATIVE4XX|5XX"、404/503 を返す
  6.  CircuitBreaker → DENY: Nmd-Cache="L1=DENY/WAIT"、503 を返す
  7.  Blacklist      → DENY: Nmd-Cache="L1=DENY/BAD_DOMAIN"、403 を返す
  8.  L1 lookup      → HIT: Nmd-Cache="L1=HIT"、AccessTracker 更新（非同期）、レスポンス
  9.  L2 lookup      → HIT: Nmd-Cache="L1=MISS, L2=HIT"
                           L1 書き込み（非同期）、AccessTracker 更新（非同期）、レスポンス
 10.  singleflight   → 同一キーへの並列リクエストをまとめ、フェッチ・変換を 1 回だけ実行
       a. SSRF guard: DNS 解決 → プライベート IP チェック → TCP 接続先 IP を再検証
       b. fetcher.Fetch() → 取得失敗（40x/5xx）の場合:
              NegativeCache.Set + CircuitBreaker.RecordFailure
              wantFallback=true → fallback 画像で 200 返却
              wantFallback=false → 404/502 返却
       c. CircuitBreaker.RecordSuccess
       d. Content-Type チェック（`?debug` フラグなしの場合のみ）
              image/* / video/* / audio/* 以外 → Nmd-Cache="...DENY/BAD_CONTENT"、422 を返す
              キャッシュは行わない（オリジンのエラーではなくプロキシ側の判断のため Negative Cache にも記録しない）
       e. converter.Convert() → variant に応じた WebP/PNG 変換（セマフォで同時実行数制限）
       f. L1 書き込み（非同期）、L2 書き込み（非同期・fire-and-forget）、AccessTracker 更新（非同期）
       f. singleflight 共有値: {data, contentType, fetchDur, convertDur}
          → 待機していた全リクエストが同じ値を受け取り、それぞれ独立してレスポンスを書く
 11.  response.Write() → Nmd-Cache・Server-Timing・Cache-Control・CSP 等の固定ヘッダを一括セット
                         Age/Set-Cookie/Server/X-Powered-By/HSTS 等を削除
 12.  ボディ書き込み
```


### リダイレクト追従

オリジンが 301/302 を返した場合、リダイレクト先を自動的に追いかける。ただし無限ループ防止のため最大 `FETCH_MAX_REDIRECTS` 回までとする。

## キャッシュ設計（正常系）

### L1: ディスクキャッシュ

- `CACHE_DIR` 環境変数で指定されたパスにファイルを置く
- k8s では emptyDir をマウント（Pod 再起動で消えるが意図どおりOK）
- アプリ内の cleanup goroutine が定期的に容量チェック
  - `CACHE_MAX_BYTES` 超過時、アクセス時刻が古いものから削除
  - `CACHE_TARGET_BYTES` まで削除してバタつきを防止する

### L2: オブジェクトストレージ（S3互換）

- `S3_ENABLED=true` の場合のみ利用する（L1のみ運用も可とする）
- S3 互換であれば移植可能（`S3_ENDPOINT` 環境変数で切り替え）
- Lifecycle Rule でオブジェクトを自動削除（`S3_LIFECYCLE_DAYS` に合わせてバケット側の事前設定を想定）
- L2 への書き込みは必ず非同期とし、gorouting で実行。レスポンスタイムに影響しないようにする（fire-and-forget）

### L2 オブジェクトの延命（擬似 last-accessed TTL）

厳密でない「最終アクセスから N 日」を以下で実現する:

- インメモリに `key → 最終アクセス時刻` マップを保持（画像データは持たない。軽量）
- アクセス時にマップを更新（syscall なし）
- L1 ヒット時、L2 アップロードから `S3_RENEW_AFTER_DAYS` 以上経過していたら非同期で L2 を再 PUT（Lifecycle をリセット）
- L1 から追い出された（= しばらくアクセスなし）エントリは延命されず、L2 も `S3_LIFECYCLE_DAYS` 日で自然消滅

**制約**: `S3_RENEW_AFTER_DAYS` < `S3_LIFECYCLE_DAYS` となるよう設定すること（デフォルト: 28 < 42）。

Pod 再起動でマップは消えるが許容範囲（再起動後もアクセスが続けば次の更新タイミングで延命される）。

### キャッシュキー

URL + 変換パラメータの組み合わせで SHA-256 を取る。

```
key = SHA256(url + "|" + "avatar")  // avatar 用
key = SHA256(url + "|" + "emoji")   // emoji 用
key = SHA256(url + "|" + "raw")     // 無変換
```

### 同時リクエストの重複排除（singleflight）

同じキーに対して同時に複数リクエストが来た場合、オリジンフェッチ・変換処理を **1回だけ実行**して結果を全員に返す。
`golang.org/x/sync/singleflight` をそのまま使用。

```
t=0ms   リクエストA: L1ミス → オリジンフェッチ開始
t=10ms  リクエストB: 同じURL → Aの処理完了を待つ
t=20ms  リクエストC: 同じURL → Aの処理完了を待つ
t=200ms Aの処理完了 → A・B・C 全員に同じ結果を返す
        オリジンフェッチ1回・変換1回で済む
```

バズった投稿の画像など、キャッシュが温まる前に同時アクセスが集中するケースで特に効果がある。

## キャッシュ設計（異常系）

### Negative Cache

オリジンが 40x/5xx を返した URL を記録し、TTL 内は L2 GET・オリジンフェッチをスキップして即エラーを返す。
主な目的は Mastodon アバター変更後の無効 URL など、恒久的に消失したリソースへの無駄なアクセスを削減すること。

| ステータス | TTL |
|---|---|
| 40x | 1800秒 |
| 5xx | 120秒 |

インメモリに保持（Pod 再起動で消えるが許容範囲）。

### Circuit Breaker

ドメインごとに連続失敗を追跡し、閾値を超えたら一定時間そのドメインへのリクエストを遮断する。
状態はインメモリで管理（Pod 再起動でリセット。許容範囲）。

```
CLOSED（正常）
    ↓ 連続失敗が CIRCUIT_BREAKER_THRESHOLD を超えた
OPEN（遮断中） → 即座に503を返す
    ↓ CIRCUIT_BREAKER_TIMEOUT 経過
HALF-OPEN（試行中） → リクエストを通す
    ↓ 成功              ↓ 失敗
CLOSED（復帰）      OPEN（再遮断）
```

Negative Cache との使い分け:

| | Negative Cache | Circuit Breaker |
|---|---|---|
| 対象 | 特定のURL | ドメイン全体 |
| トリガー | そのURLが40x/5xx | そのドメインへの連続失敗 |
| 用途 | 消えたコンテンツ | 落ちているサーバー |

## インメモリ状態の設計

### 状態一覧

全てのインメモリ状態はインターフェースで抽象化し、`STORE_BACKEND` 環境変数でインメモリ実装と Redis 実装を切り替え可能にする。Redis が落ちた場合はエラーを返す（フォールバックなし）。なお、Redis実装は初期は行わない。

#### AccessTracker（`map[string]time.Time`）

キャッシュキー → 最終アクセス時刻のマップ。

| 操作 | タイミング |
|---|---|
| 追加 / 更新 | L1・L2 ヒット時、オリジンフェッチ成功時 |
| 削除 | L1 cleanup goroutine がファイルを削除するとき同時に削除。定期 GC で孤立エントリ（ファイルが存在しないキー）を削除 |

Redis 実装: `SET key timestamp EX {S3_LIFECYCLE_DAYS in seconds}`。TTL を L2 Lifecycle に合わせることで GC が不要になる。

#### Negative Cache（`map[string]negativeEntry`）

```go
type negativeEntry struct {
    expireAt time.Time
    status   int  // 40x or 5xx
}
```

| 操作 | タイミング |
|---|---|
| 追加 | オリジンフェッチが 40x/5xx を返したとき |
| 削除 | 定期 GC で `expireAt` 超過エントリを削除 |

Redis 実装: `SET key status EX {TTL}`。Redis の TTL 自動削除により GC が不要になる。

#### Circuit Breaker（`map[string]circuitEntry`）

```go
type circuitEntry struct {
    state      string    // CLOSED / OPEN / HALF-OPEN
    failures   int
    lastFailed time.Time
    openUntil  time.Time
}
```

| 操作 | タイミング |
|---|---|
| 追加 | そのドメインへの初回失敗時 |
| 更新 | 失敗カウント増加・状態遷移時 |
| 削除（CLOSED復帰時） | HALF-OPEN で成功したとき即削除 |
| 削除（定期GC） | CLOSED 状態かつ `lastFailed` が一定期間以上前のエントリを削除 |

Redis 実装: エントリを JSON でシリアライズして保存。CLOSED 復帰時に DEL。定期 GC の代わりに TTL を設定して自動削除。

#### singleflight（`singleflight.Group`）

| 操作 | タイミング |
|---|---|
| 追加 | 同じキーへの2リクエスト目以降が来たとき |
| 削除 | 処理完了時に自動解放（ライブラリが管理） |

**singleflight のみ Redis 化しない。** 処理の合流はプロセス内で完結する必要があり、Redis を介すると複数 Pod 間でリクエストをブロックし合う複雑な実装になるため。複数 Pod 構成では各 Pod が独立して singleflight を持つ（同じ URL が複数 Pod に届いた場合、各 Pod で1回ずつフェッチが走る。許容範囲）。

---

### goroutine 一覧

インメモリ実装時のみ必要。Redis 実装時は TTL による自動削除で代替されるため、GC goroutine は不要になる（L1 cleanup は Redis 使用時も必要）。

| goroutine | 実行間隔 | 処理内容 | Redis時 |
|---|---|---|---|
| **L1 cleanup + AccessTracker GC** | 5分 | ディスク容量チェック・古いファイル削除・削除ファイルのTrackerエントリ削除・孤立エントリ削除 | L1 cleanup のみ実行（Tracker GC は不要） |
| **Negative Cache GC** | 5分 | `expireAt` 超過エントリを全走査して削除 | 不要 |
| **Circuit Breaker GC** | 10分 | CLOSED 状態かつ長期間アクセスのないエントリを削除 | 不要 |

L1 cleanup と Negative Cache GC は同じ間隔なので1つの goroutine にまとめてもよい。

---

### メモリ試算

| データ | 1エントリ | 最大エントリ数 | 最大サイズ |
|---|---|---|---|
| AccessTracker | ~56byte | ディスクキャッシュ数 ≈ 50,000 | ~2.7MB |
| Negative Cache | ~230byte | 実運用で数千 | ~数百KB |
| Circuit Breaker | ~100byte | 連合ドメイン数 ≈ 数千 | ~数百KB |
| singleflight | ~32byte | 同時リクエスト数のみ | 無視できる |

合計 **数MB程度**。libvips 変換バッファ（同時変換数 × 数十〜百MB）と比べると誤差レベル。

libvips は cgo 経由で C の malloc を使うため Go ヒープとは別領域。GoのマップがlibvipsにEvictされることはない。ただし Pod のメモリ上限は両方の合算なので、`CONVERT_CONCURRENCY` でピーク時の総使用量を抑える。

```yaml
# k8s resources の目安
resources:
  requests:
    memory: "256Mi"
  limits:
    memory: "1Gi"  # Goヒープ(数十MB) + libvipsバッファ(コア数×百MB) + 余裕
```

---

### AccessTracker インターフェース

アクセス時刻管理をインターフェースで抽象化し、起動時設定で切り替え可能にする。

```go
type AccessTracker interface {
    Set(key string, t time.Time) error
    Get(key string) (time.Time, bool)
    OldestFirst(limit int) ([]string, error)
}
```

| 実装 | 用途 |
|---|---|
| `MemoryTracker`（デフォルト） | 単一 Pod 構成。軽量でシンプル |
| `RedisTracker`（オプション） | 複数 Pod 構成時。アクセス記録を Pod 間で共有 |

`STORE_BACKEND=redis` 環境変数で切り替え（AccessTracker だけでなく Negative Cache・Circuit Breaker も同時に切り替わる）。Redis 使用時は AccessTracker の Set を非同期（fire-and-forget）にしてボトルネックを回避する。

## 運用CLI

CLI は `ADMIN_PORT`（デフォルト: `3001`）で待ち受ける管理用 HTTP エンドポイントのラッパーとして実装する。
インメモリ状態（NegativeCache・CircuitBreaker 等）を外部から参照するためにエンドポイントが必要なためで、
外部露出は `ADMIN_PORT` を k8s Service に含めないことで防ぐ。

管理エンドポイント（`localhost:ADMIN_PORT` のみ）:

```
GET    /stats                 全体サマリー
GET    /stats/circuit-breaker CircuitBreaker 詳細
GET    /stats/negative-cache  NegativeCache 詳細
DELETE /cache/{key}           指定キーを L1・L2 から削除
DELETE /cache                 全キャッシュ削除
```

キャッシュキーの計算も CLI が行うので SHA256 を手動計算する必要はない。

```bash
# 特定URL・バリアントのキャッシュを削除（L1・L2両方）
media-delivery purge --url "https://example.com/image.png" --variant avatar

# 特定URLの全バリアントを削除
media-delivery purge --url "https://example.com/image.png" --all-variants

# キャッシュを全削除
media-delivery purge --all
```

`Nmd-Cache-Key` レスポンスヘッダでキャッシュキーを確認し、そのまま purge コマンドに渡すことができる。

### stats サブコマンド

インメモリ状態とディスク使用量の統計を表示する。

```bash
# 全体サマリー
media-delivery stats

# Circuit Breaker の詳細（OPEN ドメイン一覧）
media-delivery stats --circuit-breaker

# Negative Cache の詳細（エントリ一覧）
media-delivery stats --negative-cache
```

**`media-proxy stats` の出力例:**

```
=== Cache Stats ===
L1 (Disk)
  Used:      2.3 GiB / 4.0 GiB (57%)
  Files:     23,841
  Oldest:    2025-02-14 (30 days ago)

L2 (S3)
  Enabled:   true
  Endpoint:  https://...r2.cloudflarestorage.com

AccessTracker
  Entries:   23,841
  Backend:   memory

Negative Cache
  Entries:   142
  Backend:   memory

Circuit Breaker
  CLOSED:    1,204 domains
  OPEN:      3 domains
  HALF-OPEN: 0 domains
```

**`media-proxy stats --circuit-breaker` の出力例:**

```
OPEN domains (3):
  suspicious.example.com  failures=8  open_until=2026-03-16T12:34:56
  broken.fediverse.jp     failures=5  open_until=2026-03-16T12:30:00
  slow-server.social      failures=6  open_until=2026-03-16T12:31:00
```

**`media-proxy stats --negative-cache` の出力例:**

```
Negative Cache (142 entries):
  [40x] https://mastodon.social/avatars/...  expires=2026-03-17T10:00:00
  [5xx] https://broken.example.com/img/...   expires=2026-03-16T12:35:00
```

## 環境変数

| 変数 | デフォルト | 説明 |
|---|---|---|
| `HOST` | `0.0.0.0` | 受け付けるホスト名/IPアドレス（例: `127.0.0.1` でローカルのみ） |
| `PORT` | `3000` | リッスンポート |
| `CACHE_DIR` | `/cache` | ディスクキャッシュのパス |
| `CACHE_MAX_BYTES` | `4GiB` | L1 削除開始閾値 |
| `CACHE_TARGET_BYTES` | `3GiB` | L1 削除目標 |
| `CACHE_CONTROL_SUCCESS` | `max-age=31536000, immutable` | 成功時のレスポンス Cache-Control |
| `CACHE_CONTROL_4XXERROR` | `max-age=3600` | エラー（4xx）時のレスポンス Cache-Control |
| `CACHE_CONTROL_5XXERROR` | `max-age=120, must-revalidate` | エラー（5xx）時のレスポンス Cache-Control |
| `CACHE_CONTROL_FAILOVER` | `max-age=86400` | エラー時、Failover画像を200 OKで返したときのレスポンス Cache-Control |
| `CACHE_CONTROL_DENY` | `max-age=86400` | リクエストブロック時（DENY/BAD_REQ・DENY/BAD_CONTENT・DENY/BAD_DOMAIN）のレスポンス Cache-Control |
| `S3_ENABLED` | `true` | L2 オブジェクトストレージの有効/無効 |
| `S3_ENDPOINT` | — | オブジェクトストレージエンドポイント |
| `S3_BUCKET` | — | バケット名 |
| `S3_ACCESS_KEY` | — | アクセスキー |
| `S3_SECRET_KEY` | — | シークレットキー |
| `S3_LIFECYCLE_DAYS` | `42` | L2 オブジェクトの Lifecycle Rule 日数（バケット側と合わせること） |
| `S3_RENEW_AFTER_DAYS` | `28` | L2 再 PUT するまでの経過日数（`S3_LIFECYCLE_DAYS` より小さくすること） |
| `STORE_BACKEND` | `memory` | `memory` または `redis`（Negative Cache・Circuit Breaker・AccessTracker すべてに適用） |
| `REDIS_ADDR` | — | Redis ホスト:ポート（例: `localhost:6379`） |
| `REDIS_PASSWORD` | — | Redis パスワード |
| `REDIS_DB` | `0` | Redis DB 番号 |
| `MAX_FILE_SIZE` | `250MiB` | オリジンフェッチの最大サイズ |
| `FETCH_TIMEOUT` | `30s` | オリジンフェッチのタイムアウト |
| `FETCH_MAX_REDIRECTS` | `3` | オリジンフェッチ時のリダイレクト追従上限 |
| `REQUEST_TIMEOUT` | `60s` | リクエスト全体のタイムアウト（フェッチ+変換含む） |
| `FALLBACK_AVATAR` | 人型シルエット（組込） | fallback時のアバター画像（ファイルパスまたはbase64） |
| `FALLBACK_EMOJI` | カラーバー（組込） | fallback時の絵文字画像 |
| `FALLBACK_BADGE` | カラーバーPNG（組込） | fallback時のバッジ画像 |
| `FALLBACK_DEFAULT` | カラーバー（組込） | fallback時のデフォルト画像 |
| `CONVERT_CONCURRENCY` | CPU コア数 | 画像変換の同時実行数上限 |
| `CONVERT_WEBP_QUALITY` | `80` | WebP出力品質（1-100） |
| `CONVERT_PNG_COMPRESSION` | `6` | PNG圧縮レベル（0-9） |
| `CONVERT_ANIM_QUALITY` | `75` | アニメーションWebP出力品質（1-100） |
| `NEGATIVE_CACHE_TTL_40X` | `24h` | 40x の Negative Cache TTL |
| `NEGATIVE_CACHE_TTL_5XX` | `5m` | 5xx の Negative Cache TTL |
| `LOG_LEVEL` | `INFO` | ログレベル（`DEBUG` / `INFO` / `WARN` / `ERROR`） |
| `CDN_NAME` | — | CDN-Loop ヘッダに使用する cdn-id（例: `media-proxy.example.com`）。必須 |
| `ORIGIN_ALLOWED_PRIVATE_NETWORKS` | — | オリジンフェッチ時の SSRF 許可 CIDR（通常は空） |
| `ORIGIN_BLACKLIST_DOMAINS` | — | オリジンのブロックするドメイン（カンマ区切り） |
| `ORIGIN_BLACKLIST_IPS` | — | オリジンのブロックする IP/CIDR（カンマ区切り） |
| `ORIGIN_BLACKLIST_FILE` | — | オリジンのブラックリストファイルパス（ホットリロード対応） |
| `CIRCUIT_BREAKER_ENABLED` | `true` | Circuit Breaker の有効/無効 |
| `CIRCUIT_BREAKER_THRESHOLD` | `5` | 連続失敗回数で OPEN にする閾値 |
| `CIRCUIT_BREAKER_TIMEOUT` | `5m` | OPEN 継続時間 |

## k8s マニフェスト構成（概要）

```yaml
# デフォルト構成（emptyDir）
volumes:
  - name: cache
    emptyDir:
      sizeLimit: 5Gi  # アプリの CACHE_MAX_BYTES より大きく設定

# PVC オプション（Pod 再起動後もキャッシュを保持したい場合）
volumes:
  - name: cache
    persistentVolumeClaim:
      claimName: media-proxy-cache
```

アプリコードは `CACHE_DIR` に読み書きするだけなので、**emptyDir → PVC の切り替えにコード変更は不要**。

### コンテナイメージ

- ベースイメージ: `debian:bookworm-slim`（libvipsのAlpine/musl相性問題を避けるため）
- マルチプラットフォーム: `amd64` / `arm64` の両方をビルド

## robots.txt

`GET /robots.txt` に対して静的レスポンスを返す。内容はバイナリに埋め込む。

```
User-agent: *
Disallow: /

User-agent: GPTBot
Disallow: /

User-agent: ClaudeBot
Disallow: /

User-agent: Google-Extended
Disallow: /

User-agent: CCBot
Disallow: /

User-agent: FacebookBot
Disallow: /

User-agent: Applebot-Extended
Disallow: /
```

## ヘルスチェック

```
GET /healthz → 200 OK
```

k8s の liveness/readiness probe に使用。L1ディスクへの書き込み可否・S3接続確認などは行わずシンプルに200を返す（依存サービスの障害でPodが再起動ループに入るのを防ぐ）。

## セキュリティ

### レスポンス Content-Type チェック

オリジンが `image/*` / `video/*` / `audio/*` 以外の Content-Type（例: `text/html`）を返した場合、422 Unprocessable Entity を返してキャッシュも行わない。
HTML エラーページや意図しないリソースを配信・キャッシュしないための対策。

- Negative Cache には記録しない（オリジンのエラーではなくプロキシ側の判断）
- `?debug` クエリパラメータを付与するとチェックをスキップし、そのまま応答する（Misskey互換外の独自拡張）
- `debug` はContent-Typeチェックのバイパスに加え、L1/L2キャッシュの読み書きを行わない（originへ必ずフェッチする）。レスポンスには `Cache-Control: no-store` と `Nmd-Cacheable: false` を付与する

### SSRF・不正ドメイン対策

`?url=` で任意のURLを指定できてしまうため、いくつかの対策を行う。

**URLスキームの制限**

`http://` と `https://` のみ許可。それ以外は即403。

```
file:///etc/passwd  → 403
gopher://...        → 403
ftp://...           → 403
```

**IPチェック（DNS rebinding対策含む）**

DNS解決を行い、解決されたIPを2段階で検証。

1. IP直指定（IPv4/IPv6） → 403
2. DNS解決が失敗 → 403
2. 解決したIPがプライベート/予約済みアドレス帯 → 403
3. フェッチ時のTCP接続先IPを再検証（DNS rebinding対策）

ブロック対象のアドレス帯:
- `10.0.0.0/8`, `172.16.0.0/12`, `192.168.0.0/16`（プライベート）
- `127.0.0.0/8`, `::1/128`（ループバック）
- `169.254.0.0/16`（リンクローカル・AWSメタデータ等）
- `fd00::/8`（ULAプライベート）

`ORIGIN_ALLOWED_PRIVATE_NETWORKS` 環境変数で例外CIDRを許可可能。

**ブラックリスト**

環境変数またはファイルで管理。ファイルはホットリロード対応（再起動不要）。

```
ORIGIN_BLACKLIST_DOMAINS=malicious.example.com,spam.example.org
ORIGIN_BLACKLIST_IPS=1.2.3.4/32,5.6.7.0/24
ORIGIN_BLACKLIST_FILE=/etc/media-proxy/blacklist.txt
```

### リクエストヘッダの制御

オリジンへのフェッチ時はホワイトリスト方式で、明示的に許可したもの以外は転送しない。

**削除するヘッダ（オリジンに送らない）:**

| ヘッダ | 理由 |
|---|---|
| `Cookie` | オリジンへの不正アクセス・プライバシー漏洩防止 |
| `Authorization` | 同上 |
| `X-Forwarded-For` / `X-Real-IP` | クライアントIPをオリジンに漏らさない |
| `Referer` | 経由サービスをオリジンに漏らさない |

**上書きするヘッダ:**

| ヘッダ | 値 |
|---|---|
| `User-Agent` | `MisskeyMediaProxy/x.x` |
| `Accept` | `image/*, video/*, audio/*, */*;q=0.8` |
| `CDN-Loop` | 既存設計通り（後述） |

### ループ検出（CDN-Loop / RFC 8586）

RFC 8586 で標準化された `CDN-Loop` ヘッダを使用する。

**受信時の処理：**

```
受信リクエストに CDN-Loop ヘッダがある
    → 自分の cdn-id（= CDN_NAME 環境変数）が含まれている → 508 Loop Detected を返す
    → 含まれていない → 自分の cdn-id を追記してオリジンへフェッチ
受信リクエストに CDN-Loop ヘッダがない
    → CDN-Loop: {CDN_NAME} を付けてオリジンへフェッチ
```

- `CDN-Loop` はリクエスト専用ヘッダ。レスポンスには付けない
- `CDN-Loop` ヘッダはクライアントから送られてきても改ざん不可として扱う（RFC 8586 の要件）
- `CDN_NAME` 環境変数で cdn-id を設定する（例: `media-delivery.example.com`）

### その他

- レートリミット: CDN配下での運用を前提とするためプロキシ側では実装しない。必要な場合はCloud WAFで実装

## 画像変換ライブラリ

Go + bimg（libvips wrapper）を使用。

- 対応入力: JPEG, PNG, WebP, GIF, AVIF, HEIC
- 出力: WebP（badge=1 のみ PNG）
- アニメーション: static=1 指定時のみ先頭フレーム抽出
- 変換パラメータなし（rawアクセス）の場合は変換スキップ、オリジナルをそのまま返す

*TODO* Misskeyとの互換性を保ちながらAVIFを出力できるようにする

### 変換の同時実行数制限（セマフォ）

libvips は内部でスレッドを使うが、同時に大量リクエストが来ると CPU を使い切る。
セマフォで同時実行数を制限し、1リクエストあたりの変換時間を安定させる。
デフォルトは CPU コア数。`CONVERT_CONCURRENCY` 環境変数で上書き可能。


## ログ設計

Loki への転送を前提とした JSON 構造化ログを出力する。アクセスログはリバースプロキシ側に任せるため出力しない。

### ログレベル

| レベル | 用途 |
|---|---|
| `ERROR` | 処理継続不可能なエラー（L1書き込み失敗・panic recovery等） |
| `WARN` | 処理は継続したが異常（L2への書き込み失敗） |
| `INFO` | ライフサイクルイベントや準正常系（ガベコレの成功・Circuit Breaker OPEN遷移・Negative Cacheの追加など） |
| `DEBUG` | オリジンフェッチ・変換処理の詳細（通常は無効） |

`LOG_LEVEL` 環境変数で設定（デフォルト: `INFO`）。

### ログフォーマット

```json
{
  "level": "info",
  "ts": "2026-03-16T12:34:56Z",
  "msg": "circuit breaker opened: broken.fediverse.jp",
  "url": "https://broken.fediverse.jp/aaa/bbb.jpeg"
}
```

フィールドは `level`・`ts`・`msg` を共通として、必ず出力する。
必要に応じて `url` を追加で出力するものとする。

## 実装補足・チューニング

### Goランタイム

- **automaxprocs**: `uber-go/automaxprocs` を使用してk8sのCPU limitsを自動認識させる。未設定だとノード全体のコア数でスレッドを立ててしまう
- **GOGC**: 画像変換でメモリ使用量が激しく変動するためGCのレイテンシスパイクが起きやすい。`GOGC=200` 程度に設定してGC頻度を下げる

### HTTPサーバー（クライアント側）

- **IdleTimeout**: デフォルト無制限のため明示的に設定する（例: 60s）。アイドル接続の蓄積を防ぐ
- **最大同時接続数**: 大量リクエストによるgoroutine爆発を防ぐためセマフォで制限する

### HTTPクライアント（オリジン側）

- **Keep-Alive**: 有効にする。TCPハンドシェイクのオーバーヘッドを削減できる
- **オリジン側の接続数制限**: レートリミット・変換セマフォで十分なため不要

### libvips

- **内部キャッシュの無効化**: プロキシ用途では同じ画像を繰り返し変換しないためlibvips内部キャッシュは不要。メモリの無駄なので無効化する
```go
vips.Startup(&vips.Config{
    ConcurrencyLevel: concurrency,
    CacheSize:        0,
    MaxCacheMem:      0,
})
```

### 信頼性

- **graceful shutdown**: SIGTERM受信時に処理中リクエストを完走させてから終了する。非同期goroutineも `sync.WaitGroup` で追跡して完走を待つ。k8sの `terminationGracePeriodSeconds` と合わせて設定
- **panic recovery**: libvips変換でのpanicでプロセスごと落ちないよう、リクエストハンドラに `recover` を入れる
- **非同期書き込み失敗**: fire-and-forgetのL1/L2書き込みが失敗した場合はエラーログを出力する（サイレント無視しない）
- **L1書き込みエラー**: ディスクfull時など書き込みエラーが発生してもレスポンスは返す。エラーはログに記録

## 判断ログ（設計中の検討事項）

- **インメモリ LRU を使わない理由**: Pod に余分なメモリを割り当てたくない。画像データ自体はディスクに置き、アクセス時刻のみメモリに持つ設計に
- **atime/mtime に頼らない理由**: k8s の emptyDir は `noatime` / `relatime` が多く信頼できない。またアクセスのたびに `os.Chtimes()` syscall を打つのはオーバーヘッドになる
- **SQLite を使わない理由**: AccessTracker はメモリ上のマップで十分。L2 のメタデータに `uploaded_at` を持たせればファイルの対応付けも DB 不要で実現できる
- **L2 Lifecycle が「最終アクセスから」にできない理由**: S3 互換全般の制約。「作成から N 日」のみ。擬似的な延命は L1 ヒット時の再 PUT で対応（`S3_RENEW_AFTER_DAYS` < `S3_LIFECYCLE_DAYS` を維持すること。デフォルト: 28 < 42）
- **singleflight を使う理由**: キャッシュが温まる前の同時アクセスでオリジンフェッチ・変換が多重実行されるのを防ぐ。`golang.org/x/sync/singleflight` で実装コストは低い
- **Negative Cache の TTL をステータスコードで分ける理由**: 40x は恒久的消失が多いので長く、5xx は一時的障害なので短くすることで、無駄なフェッチ削減とキャッシュの鮮度を両立
- **変換あり時に変換前データを先に返さない理由**: `no-cache` で返してもブラウザ汚染は防げるが、`static=1`（アニメーション静止化）などは変換前の生ファイルを返すと動作が壊れる。変換なし（rawアクセス）は変換処理が不要なので即レスポンス可能。変換ありの遅さは singleflight + セマフォで対応
- **Circuit Breaker の HALF-OPEN を厳密にしない理由**: HALF-OPEN 中に数リクエスト通り抜けても実害はない。`sync.Mutex` による排他制御は実装コストに見合わないため省略
- **Via ではなく CDN-Loop を使う理由**: Via はループ検出専用ではなく、一部実装でVia存在時にHTTP/1.1機能が無効化されるため実用的でない。CDN-Loop（RFC 8586）はAkamai・Fastly・Cloudflare共同で策定したループ検出専用の標準ヘッダ
- **Server-Timing の desc を省略する理由**: ヘッダを短く保つため。`fetch` と `convert` という名前で用途は自明
- **Server-Timing のタイミング情報が攻撃に利用されないか**: フェッチ時間・変換時間はオリジンサーバーの機密情報を含まない。処理時間のばらつきからコンテンツの推測は困難でリスクは無視できる
- **オリジンの Cache-Control を無視する理由**: Misskey公式仕様でも固定で `max-age=31536000, immutable` を返すと定められており、オリジンの指示を尊重しない設計。`no-store` を尊重するとMastodon URL無効化問題への対処ができなくなる。`no-transform` も同様に無視する
- **管理エンドポイントを別ポートに分離した理由**: インメモリ状態（NegativeCache・CircuitBreaker）はCLIから直接参照できないため、実行中プロセスへのHTTP経由アクセスが必要。別ポート（ADMIN_PORT）に分離し、k8s Serviceに含めないことで外部露出なく実現する