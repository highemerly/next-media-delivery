# 開発の進め方

## ブランチと PR

- **main へは直接コミット・push しない**。変更は必ずブランチを切り、PR → CI 緑 → squash マージで取り込む（依存更新を Renovate の PR で回すため、人の変更も同じ経路に揃える）。
- PR のタイトルは英語で、中身が分かるように付ける。squash のコミットメッセージと GitHub Release のノートにそのまま使われる。
- 手元：[.githooks/pre-commit](../.githooks/pre-commit) が main 上のコミットを拒否する。clone ごとに1回有効化する：
  ```bash
  git config core.hooksPath .githooks
  ```
- サーバー側：GitHub の ruleset（定義は [.github/rulesets/main.json](../.github/rulesets/main.json)）が main への push を拒否する。タグは ruleset の対象外なので、リリースのタグは手元から push できる。

### ruleset の中身と理由

| ルール | 設定 | 理由 |
|---|---|---|
| PR 必須 | 承認数 0 | 一人運用で、自分の PR は自分で承認できないため |
| アカウントに紐づかない作者のコミット | 追加の承認を求めない（`require_extra_approval_for_unattributed_changes: false`） | 省略すると true が入り、そうしたコミットを含む PR を自分では承認できず、マージできなくなるため |
| マージ方式 | squash のみ＋linear history | 「1 PR＝1 コミット」の履歴にする。Release のノートは PR 単位なので、履歴と揃う |
| 必須チェック | `ci-result` だけ | ジョブは差分で間引く（下の「CI」）ので、個別に必須にするとスキップされたジョブに結果が付かずマージできなくなる |
| 最新化の強制 | しない（strict=false） | Renovate の PR が複数並んだとき、1件マージするたびに全件を rebase・再 CI させないため |
| force push・削除 | 禁止 | |
| バイパス | なし（管理者も対象） | |

必須でないチェックは、auto-merge も ruleset も待たない。マージ前に見たい検査は全部 `ci-result` にまとめる。

ruleset を変えたら JSON を直して反映する（作成時は POST、以降は `gh api repos/highemerly/next-media-delivery/rulesets` で id を引いて PUT）：
```bash
gh api -X POST repos/highemerly/next-media-delivery/rulesets --input .github/rulesets/main.json
gh api -X PUT repos/highemerly/next-media-delivery/rulesets/<id> --input .github/rulesets/main.json
```

### リポジトリの設定

| 設定 | 値 | 理由 |
|---|---|---|
| マージ方式 | squash だけ有効 | ruleset と揃える（ほかの方式のボタンを出さない） |
| squash のコミット | タイトル＝PR タイトル、本文は空 | ブランチ上の途中のコミットメッセージを main に持ち込まない |
| マージ後のブランチ | 自動で削除 | |
| auto-merge | 有効 | Renovate の PR などを、CI が緑になったら取り込めるように |

## 依存更新（Renovate）

依存は Renovate（GitHub App）が出す PR で上げる。設定は [.github/renovate.json5](../.github/renovate.json5)（各ルールの理由はコメント参照）。

- **毎週月曜・木曜の 9時前（JST）** にまとめて PR が来る（Go の間接依存の一括更新は月曜のみ）。自動マージはしない。
- **メジャー更新**は PR が自動では来ず、Issue「Dependency Dashboard」の Pending Approval に並ぶ（コード側の対応が要るため）。
- 1つの PR にまとまるもの：
  - **Go**：go.mod の `go` ディレクティブ・Dockerfile の `golang:X.Y.Z-bookworm`・`.tool-versions`（CI の setup-go は go.mod を読むので、揃えないと CI とイメージで版がずれる）
  - **Go indirect dependencies**：go.mod の `// indirect`（`golang.org/x/sys` など。gomod は lockFileMaintenance に対応していないので、その代わり）
  - **test origin images**：`docker-compose.yml` の httpbin・nginx（テスト用。本番には入らない）
- Go モジュールは公開から3日待ってから PR になる（乗っ取られて公開された版は数日以内に取り下げられることが多いため）。
- ベースイメージ（`golang`・`debian`）とテスト用イメージは digest で固定。Debian のパッケージ（libvips など）の更新は digest 更新の PR として来る。
- **Debian のリリース（bookworm → trixie）は Renovate が提案しない**。builder（`golang:*-bookworm` の libvips-dev）と runtime（`debian:bookworm-slim` の libvips42）は同じリリースでないとビルド時と実行時の libvips がずれるので、両方を手で揃えて上げる。
- Dockerfile の `apt-get install`（libvips など）は版を指定していないので Renovate の対象外。ベースイメージの digest 更新とビルドの時点で入る版が決まる。

### PR が来たら

1. CI（`ci-result`）が緑か見る。赤なら ruleset でマージできない。Go のソース・go.mod・Dockerfile の変更は checks と e2e が両方走る。
2. PR 本文の変更履歴（Renovate が載せる）を読む。go.mod / go.sum の差分で、実際に入る版が変わっているかを見る（`go mod tidy` で間接依存も動くことがある）。
3. 種類に応じて確認を足す。CI が見ているのは `go vet`・unit テストと、本番イメージを起動してのインテグレーションテスト（`scripts/test/`：変換・キャッシュ・ネガティブキャッシュ・SSRF ガード・ブラックリスト・admin API / CLI・条件付きリクエスト）まで：

   | 種類 | 例 | マージ前の確認 |
   |---|---|---|
   | 画像処理 | `h2non/bimg`・`debian` / `golang` の digest（libvips の版が変わり得る） | e2e が実際の libvips で変換を通すので、緑ならマージ。アニメーションの判定（`internal/imageinfo`）も unit テストで見ている |
   | Go の版 | Go グループ | checks・e2e が緑ならマージ。マイナー（1.26 → 1.27）はリリースノートで TLS・net/http の挙動の変更を見る |
   | 実行時に効くが CI で直接見ていないもの | `go.uber.org/automaxprocs`（k8s の CPU 制限から GOMAXPROCS を決める）・`golang.org/x/sync`（singleflight の同時実行は CI で再現していない） | e2e で起動と通常のリクエストまでは確認済み。変更履歴を読む |
   | 本番で使っていない機能の依存 | `fsnotify`（`ORIGIN_BLACKLIST_FILE` のホットリロード。本番は未設定） | CI 緑ならマージ |
   | CLI | `spf13/cobra` | admin CLI のテスト（`10-admin-cli.sh`）が通れば可 |
   | 0.x のもの | `golang.org/x/*` | 0.x はマイナーでも互換を崩し得るのに Renovate はメジャー扱いしない。変更履歴を読む |
   | テスト用イメージ | test origin images | e2e が緑ならマージ |
   | タグ時にしか動かない Actions | docker/login-action・docker/metadata-action・peter-evans/create-pull-request | PR の CI では実行されない。マージ後、次のリリースの docker-build.yml で初めて動く |
4. squash マージする。
5. 手元で取り込む：
   ```bash
   git switch main && git pull
   go mod download
   ```
   `.tool-versions` の golang が上がったときは `asdf install` も行う。

### 承認待ち（メジャー）の片付け方

- 着手するときに Dashboard のチェックを入れる → PR が作られる。一度に多く承認せず1つずつ。
- CI が落ちたら、その Renovate のブランチ（`renovate/...`）に直接コミットを足して直す。人のコミットが入ったブランチは Renovate が以後書き換えない。

### その他

- 今は要らない更新：PR を閉じる（その版は作り直されない。次の版が出れば再び PR になる）。恒久的に止めるなら renovate.json5 にルールを足す。
- 急ぐ更新（セキュリティ修正など）：Dashboard の Awaiting Schedule でチェックを入れると、スケジュールを待たずに PR が作られる。
- 衝突した PR は Renovate が自動で rebase する。急ぐときは PR 本文の rebase チェックボックスを入れる。
- 手元で `go get -u` はしない（Renovate の PR とぶつかる）。依存の追加はブランチを切って PR で。

## CI

### ワークフロー

| ファイル | いつ動くか | 中身 |
|---|---|---|
| `.github/workflows/ci.yml` | main 以外のブランチへの push | 差分で回すジョブを決め、結果を `ci-result` にまとめる |
| `.github/workflows/checks.yml` | 再利用（ci.yml・docker-build.yml から呼ぶ） | `go vet ./...`・`go test ./...` |
| `.github/workflows/e2e.yml` | 再利用（ci.yml・docker-build.yml から呼ぶ） | 本番イメージを `docker-compose.yml` で起動し、`scripts/test/run-all.sh` を回す |
| `.github/workflows/docker-build.yml` | `v*` タグの push・手動 | リリース（下の「リリース」） |

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

テストの取得元はすべて compose の中にあり、インターネットには取りに行かない（08e の DNS エラーのテストを除く）：

| サービス | 中身 | 使うテスト |
|---|---|---|
| `origin` | `./assets` を nginx の既定の Content-Type で配る。`lib.sh` の `ORIGIN_BASE`（既定 `http://origin`） | ほとんど全部 |
| `octet-origin` | `./assets` を `binary/octet-stream` で配る（Content-Type の推定の確認） | 07 |
| `httpbin` | リクエストヘッダーの確認など | 06 など |

以前は `raw.githubusercontent.com` の main の `assets/` から取っていたが、GitHub の制限（429）で CI が落ち、ブランチで足した画像も使えなかった。そのかわり、本物の HTTPS の取得元への接続（runtime イメージの `ca-certificates` での証明書の検証など）は CI では確かめていない。

## リリース

### 手順

```bash
git switch main && git pull
git tag v0.2.0
git push origin v0.2.0
```

タグは `vX.Y.Z`。イメージのタグ・k8s マニフェストの版は `v` を外した `X.Y.Z` になる。

### タグで何が起きるか（`.github/workflows/docker-build.yml`）

```
tag push ─┬─ checks（go vet / go test）─────────────────────┐
          └─ build（イメージを ghcr へ push）─ e2e（そのイメージで）─┴─ release ─ create-k8s-pr
```

1. **build**：本番イメージを `ghcr.io/highemerly/next-media-delivery` へ push する。タグは `X.Y.Z`・`X.Y`・`latest`。checks と並走し、検査の結果は待たない。
2. **checks・e2e**：ブランチで差分により省いていたかもしれない検査を全部回す。e2e は push したイメージそのもの（＝デプロイするもの）を pull して回す。
3. **release**：全部通ったら GitHub Release を作る（下）。
4. **create-k8s-pr**：k8sg1-repo（`highemerly/k8sg1-repo`）の `manifests/nmd/prd/web.yaml` のイメージを `X.Y.Z` に上げる PR を出す。

**k8sg1 は Flux で同期しているので、この PR のマージがそのまま本番へのデプロイになる**（約1分で反映）。マージはこちらで行う。k8sg1 の CI（yamllint・kubeconform）は必須チェックにできないので、緑であることを目で見てからマージする。

- `latest` は検査の前に動くので、デプロイには使わない。本番は版を固定する。
- 途中で落ちたら Release も k8sg1 の PR もできない。直して次のパッチのタグを打つ。
- PR が出ないときは、Actions の `create-k8s-pr` のログを見る。マニフェストの `image:` の行が書き換わらなかったときは `image line not updated` で落ちる。
- k8sg1 の PR は `app-update` ラベル・タイトル `Update next-media-delivery to X.Y.Z`・ブランチ `update-next-media-delivery-X.Y.Z`（全アプリ共通の書式）。本文はイメージ・ビルドのリンクと、GitHub Release と同じノート。
- k8sg1 への push にはシークレット `K8SG1_PAT`（fine-grained PAT。対象は `highemerly/k8sg1-repo` だけ、Contents と Pull requests の Read and write）を使う。
- DB は持たないので、マイグレーションの手順は無い。

### GitHub Release

- タグごと（パッチも含む）に、全検査が通ったときだけ作る。Release がある版＝デプロイしてよい版。
- ノートは GitHub の自動生成（前回のタグからマージされた PR の一覧と Full Changelog・英語）。PR のタイトルがそのままノートになるので、PR のタイトルは英語で中身が分かるように付ける。
- 分類は `.github/release.yml`：Renovate の PR（`dependencies` ラベル）は Dependencies、それ以外は Changes。
- 前回のタグ（`previous_tag_name`）は明示している。省くと GitHub は前回の「Release」を起点にするが、v0.1.8 以前は Release を作っていないため。

### 作り直し（workflow_dispatch）

Actions の「Release」を手動で実行し、`tag` に `v` を付けずに版（例 `0.2.0`）を入れる。イメージはその版で push し直し、検査を全部回して k8sg1 の PR を出す。Release は作らない（ノートの生成だけして PR 本文に使う）。ベースイメージの更新だけを取り込み直したいときなどに使う。
