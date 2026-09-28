# 上級(76–100): 並行処理、セキュリティ、可観測性、性能、総合実装

> `docs/...` と書かれたパスはリポジトリルートの `docs/`(このファイルから見て `../../docs/`)を指します。`docs/notes/NN.md` などはそこへ作ってください。

## この範囲のゴール

- goroutine・channel・同期プリミティブを「リークせず、race せず、止められる」形で使える
- 認証・入力・監査・鍵管理を攻撃者の視点で検査し、回帰テストとして固定できる
- トレース・メトリクス・ログを相関させ、性能問題を計測 → 仮説 → 修正 → 再計測で扱える
- 仕様書 1 枚から縦に 1 本、設計・実装・テスト・運用手順まで自力で完遂できる

## 事前準備

```bash
make obs-up            # Jaeger / Prometheus(問題 85 以降)
brew install hey       # 負荷試験(問題 91)
```

## 100 問終了時点の追加ディレクトリ

```
pkg/otel pkg/metrics pkg/ratelimit pkg/cache pkg/jwt
gateway/http/admin/     pprof, metrics
docs/adr/               ADR-001 〜
docs/perf/              ベンチマーク・負荷試験の記録
docs/runbook/           運用手順
openapi.yaml
scripts/e2e.sh
```

---

## 76. 並行処理①: errgroup による fan-out

**主題**: `golang.org/x/sync/errgroup`、並列クエリ、最初のエラーで中断、同時実行数の制限 **TDD 必須**

**実装**

- 問題 59 の `ProjectDashboard` を、独立した 4 つのクエリ(ステータス別、期限順、担当者別、日別)に分割し、`errgroup.WithContext` で並列に実行して結果を組み立てる `usecase/query/dashboard.go` を作る(SQL 1 本で済んでいたものを、あえて分けて「並列化の作法」を学ぶ。どちらが良いかは問題 91 で計測して判断する)。
- `g.SetLimit(3)` で同時実行数を制限する(接続プールを食い潰さないため)。
- 1 つのクエリが失敗したら残りは context キャンセルで中断し、最初のエラーを返す。
- 結果の書き込みは goroutine ごとに別の変数へ行い、共有スライスへの `append` をしない。

**完了条件**

- [ ] 4 つの Query をモックにし、1 つがエラーを返すと他の 3 つに渡された context がキャンセルされる(モック内で `<-ctx.Done()` を確認)
- [ ] 同時実行数 3 の制限をモックで検証(同時に `Execute` 中の数をカウントし最大 3)
- [ ] `-race` で通る
- [ ] Tx の中で `errgroup` を使っていない(`pgx.Tx` は並行利用不可。理由を doc コメントに書く)

**ヒント**: 「並列化してよいのは互いに独立で、失敗時にまとめて捨ててよい処理」だけ。

---

## 77. 並行処理②: Worker Pool と goroutine リーク検査

**主題**: 有界チャネル、`sync.WaitGroup`、`go.uber.org/goleak`、停止の順序 **TDD 必須**

**実装**

- 問題 69 の `Executor` を「1 ループ 1 ジョブ」から「Dequeue する 1 goroutine + `Concurrency` 個のワーカー goroutine」の Worker Pool に変更する。ジョブは容量 `Concurrency` のチャネルで渡す。
- 停止手順: context キャンセル → Dequeue ループ終了 → チャネルを close → ワーカーが残りを処理して終了 → `WaitGroup.Wait`。この順序をコメントに書く。
- `go get go.uber.org/goleak` を導入し、`Executor` のテストに `defer goleak.VerifyNone(t)` を付ける。
- ジョブ 1 件のタイムアウトは `context.WithTimeout` で付け、`Execute` がタイムアウトを無視して走り続けた場合でも `Executor` の停止が塞がれないようにする(タイムアウト後は `Fail` を記録して次へ進む。走り続ける goroutine はリークとして検出されるので、Handler 側が context を尊重する契約をテストで固定する)。

**完了条件**

- [ ] `goleak` 付きのテストで、`Start` → `Stop` 後に goroutine が残らない
- [ ] 100 ジョブを `Concurrency=4` で処理し、同時実行数が 4 を超えない(カウンタで検証)
- [ ] `Stop` 中に受け取ったジョブが必ず `Complete` か `Fail` される(取りこぼしゼロ)
- [ ] `Dequeue` が `nil, nil` を返し続けるときに CPU を回さない(`PollInterval` の `select` に `ctx.Done()` がある)

**ヒント**: 「チャネルを close するのは送り手だけ」「`WaitGroup.Add` は goroutine の外で」を守る。

---

## 78. 並行処理③: singleflight とキャッシュ

**主題**: `golang.org/x/sync/singleflight`、TTL キャッシュ、書き込み時の無効化、キャッシュスタンピード **TDD 必須**

**実装**

- `pkg/cache` に `Cache[K comparable, V any]` を作る。`Get(ctx, key, ttl, loader func(ctx) (V, error)) (V, error)`、`Invalidate(key)`。内部は `map` + `sync.RWMutex` + `singleflight.Group`。期限切れ判定は `clock.Clock`。
- `ProjectDashboard` を 10 秒 TTL でキャッシュする(キーは `tenant + project`)。Task の書き込み usecase は `saveAndPublish` で該当 Project のキャッシュを `Invalidate` する(port `DashboardCache` として抽象化し、memory 実装のみ)。
- 期限切れの掃除: `Get` のたびに全走査しない。`janitor` goroutine を `Start(ctx)` で起動し、1 分ごとに掃除する(停止可能)。

**完了条件**

- [ ] 100 goroutine が同時に同じキーを `Get` しても `loader` は 1 回しか呼ばれない
- [ ] `Fake` Clock を TTL 分進めると `loader` が再度呼ばれる
- [ ] `Invalidate` 後の `Get` で `loader` が呼ばれる
- [ ] `loader` がエラーを返したとき、エラーはキャッシュされない(次の `Get` で再試行)
- [ ] `goleak` で janitor が停止することを確認

**ヒント**: キャッシュは「一貫性をどこまで諦めるか」の道具。TTL と無効化の両方を持つと、諦める範囲を小さくできる。

---

## 79. 並行処理④: テナント別レートリミット

**主題**: トークンバケット、`golang.org/x/time/rate`、ミドルウェア、429 と `Retry-After` **TDD 必須**

**実装**

- `pkg/ratelimit` に `Limiter` を作る。テナントごとに `*rate.Limiter` を `map` に持ち(`sync.Mutex`)、`Allow(tenant string) (ok bool, retryAfter time.Duration)`。既定は 10 req/s、バースト 20。`Config` で変更可能。
- 長時間使われないテナントのエントリを削除する(最終アクセス時刻を持ち、janitor で掃除)。
- HTTP ミドルウェアと gRPC インターセプタの両方に組み込む(認証の後、`Principal` からテナントを取る)。超過は 429 / `ResourceExhausted`、`Retry-After` ヘッダ(秒、切り上げ)。
- `/healthz`、`/readyz`、`/metrics` は対象外。

**完了条件**

- [ ] 21 回連続で叩くと 21 回目が 429、`Retry-After` が 1 以上
- [ ] テナント A の超過がテナント B に影響しない
- [ ] 1000 goroutine から `Allow` を呼んで race が無い
- [ ] janitor で古いエントリが消える(`Fake` Clock)
- [ ] `rate.Limiter` の「時間」はライブラリ内部の `time.Now` に依存するため `Fake` Clock で制御できない。テストではどう扱ったかを doc コメントに書く(`rate.NewLimiter` の `Limit` と `Burst` を小さくして実時間で確認する、など)

**ヒント**: レートリミットは「守るべきもの(DB、下流)」を決めてから数値を決める。

---

## 80. セキュリティ①: JWT 認証

**主題**: 署名アルゴリズム、`exp` / `aud` / `iss`、時計のずれ、鍵の管理 **TDD 必須**

**実装**

- `go get github.com/golang-jwt/jwt/v5` を導入し、`pkg/jwt` に `Verifier` を作る。アルゴリズムは `EdDSA`(Ed25519)。公開鍵は `Config.JWTPublicKeys`(PEM、複数可。`kid` で選択)。
- 検証: `alg` の固定(`WithValidMethods`)、`exp`、`nbf`、`iss`、`aud`、`kid`、時計ずれ許容 30 秒(`WithLeeway`)。時刻は `clock.Clock` から(`jwt.WithTimeFunc`)。
- クレーム `sub`(user_id)、`tenant_id`、`role` から `Principal{Kind: user}` を作る。
- 認証ミドルウェア / インターセプタは「`Bearer` の値が JWT 形式(`.` が 2 つ)なら JWT、そうでなければ API キー」で分岐する。API キーは `Kind: api_key` のまま残す。
- `cmd/admin token --user u1 --tenant t1 --role member --ttl 1h` で開発用トークンを発行する(秘密鍵は `JWT_PRIVATE_KEY` から)。

**完了条件**

- [ ] テストで鍵ペアを生成し、有効 / 期限切れ / `nbf` 未来 / 別鍵で署名 / `alg=none` / `alg=HS256` / `aud` 不一致 / `kid` 不明の 8 ケースが期待どおり
- [ ] 時計ずれ 29 秒は許容、31 秒は拒否
- [ ] トークンの値がログに出ない
- [ ] `Config.String()` に秘密鍵が出ない

**ヒント**: `alg=none` と「鍵の取り違え(RS256 の公開鍵を HS256 の共有鍵として使わせる)」は JWT の典型的な攻撃。ライブラリ任せにせず、テストで固定する。

---

## 81. セキュリティ②: 入力ハードニング

**主題**: 境界での防御、リソース枯渇、インジェクションの多様な形 **TDD 必須**

**実装**

- HTTP 全体に `http.MaxBytesReader`(1MB。CSV アップロードは個別に 10MB)を付けるミドルウェア。超過は 413。
- JSON デコード: `DisallowUnknownFields`、ネスト深さ制限(`json.Decoder` には無いので、`Token()` で深さを数える簡易チェッカーを `pkg/jsonx` に作る。上限 32)、数値の桁数。
- CSV エクスポート(問題 64)の各セルで、先頭が `=`、`+`、`-`、`@`、タブ、CR のときは `'` を前置する(CSV インジェクション対策)。インポート側では逆に `'` を剥がさない(仕様として明記)。
- 添付ファイル(問題 67)の `filename` は `filepath.Base` で正規化し、`..`、`/`、制御文字、200 文字超を拒否。`Content-Disposition` で返すときは `mime.FormatMediaType` でエスケープ。
- `Content-Type` の強制: JSON エンドポイントは `application/json` 以外を 415。
- ヘッダインジェクション: `Location` などに入れる値に CR/LF が含まれないことを保証(`net/http` が防ぐが、テストで固定)。
- 時間ベースの比較(`hmac.Equal`、`subtle.ConstantTimeCompare`)を API キーの比較でも使う(ハッシュ検索後の再比較)。

**完了条件**

- [ ] 上記各項目に対応する攻撃入力のテーブル駆動テスト(最低 12 ケース)
- [ ] `=1+1` で始まるタイトルが CSV で `'=1+1` になり、インポートしても `'=1+1` のまま
- [ ] 深さ 33 の JSON で 400、32 で通る
- [ ] `gosec` の指摘が 0

**ヒント**: 「入力はすべて敵意がある」前提で境界(handler / adapter)に集める。domain は正常系の不変条件に集中させる。

---

## 82. セキュリティ③: 監査ログ

**主題**: 誰が・いつ・何を、改竄耐性、PII 最小化、同一 Tx **TDD 必須(DB テスト)**

**実装**

- マイグレーション: `audit_logs(id, tenant_id, actor_id, actor_kind, action, target_type, target_id, request_id, occurred_at, metadata JSONB)`。`(tenant_id, occurred_at)` インデックス。`UPDATE` / `DELETE` を禁止する trigger(または DB ロールの権限)を追加し、`docs/db.md` に理由を書く。
- `port.AuditLogger.Record(ctx, entry)` を作り、`saveAndPublish` と削除・割り当て・アーカイブなどの書き込み usecase から同じ Tx 内で呼ぶ。`actor` は `Principal` から、`request_id` は context から。
- `metadata` には「何が変わったか」の最小限(例: `{"from":"todo","to":"doing"}`)を入れ、タイトルやコメント本文などの内容は入れない。
- `GET /audit-logs?target_id=&cursor=` を `owner` のみに開放する(キーセット)。

**完了条件**

- [ ] usecase のロールバックで監査ログも消える(同一 Tx)DB テスト
- [ ] `UPDATE audit_logs` が DB エラーになる DB テスト
- [ ] `metadata` にタイトルが含まれないことをテストで固定
- [ ] `member` が `GET /audit-logs` で 403

**ヒント**: 監査ログは「あとで疑われたときの証拠」。書き換えられないことと、機微情報を持たないことの両立が要る。

---

## 83. セキュリティ④: 攻撃的テスト(IDOR / SQLi / カーソル改竄)

**主題**: 攻撃者視点の回帰テスト、署名付きカーソル、脅威の一覧化 **TDD 必須**

**実装**

- カーソル(問題 15)を HMAC 署名付きにする: `base64url(json) + "." + base64url(hmac)`。鍵は `Config.CursorSecret`。改竄は 400。テナント ID をカーソルに含め、別テナントのカーソルを使うと 400。
- `gateway/http/security_test.go`(統合テスト、DB 使用)に攻撃シナリオを書く。
  - IDOR: テナント B のキーでテナント A の Task / Project / Comment / Attachment / Import / AuditLog へ `GET` / `PATCH` / `DELETE` → すべて 404(存在を知らせない)
  - SQLi: `q`、`status`、`tag`、`cursor`、`id` に `' OR 1=1 --`、`'; DROP TABLE tasks; --`、`\x00` を入れても 400 か 0 件で、テーブルが残っている
  - 権限昇格: `viewer` の JWT で `role` クレームを `owner` に書き換えたトークン(署名不正)→ 401
  - 大量入力: 10 個超のタグ、200 文字超のタイトル、1MB 超の本文 → 400 / 413
  - Enum: `status=DONE`(大文字)、`priority=3`、`status=todo,doing` → 仕様どおり(400 か正規化。決めて固定)
- `docs/security/threats.md` に STRIDE 風の表(脅威 / 対策 / テスト名)を書く。

**完了条件**

- [ ] 攻撃シナリオが 20 ケース以上あり、すべて期待どおり
- [ ] 改竄カーソルで 400、別テナントのカーソルで 400
- [ ] `docs/security/threats.md` の各行がテスト名を参照している
- [ ] テスト実行後にテーブルが残っている(`DROP TABLE` の確認)

**ヒント**: 「テストが通っているから安全」ではなく「この攻撃はこのテストが防いでいる」と言える状態を目指す。

---

## 84. セキュリティ⑤: 鍵のローテーションと失効

**主題**: 複数鍵の同時有効、猶予期間、失効の即時反映、運用手順 **TDD 必須**

**実装**

- Webhook 署名鍵(問題 43)を `webhook_secrets(id, tenant_id, secret_hash, kid, created_at, retired_at NULL)` で管理し、送信時は最新の鍵で署名しつつ `X-Signature-Kid` を付ける。受信側の検証関数は複数鍵を受け取り、いずれかで一致すれば OK(猶予期間 24 時間で `retired_at` を過ぎた鍵は無効)。
- API キー: `POST /api-keys/{id}/revoke`(`owner`)で即時失効。認証ミドルウェアは DB を毎回引くので即時反映される。問題 78 のキャッシュを API キーに適用するかを検討し、「失効の即時性」とのトレードオフを `docs/adr/` に ADR として書く(結論: キャッシュするなら TTL 60 秒以内、失効時に `Invalidate`)。
- JWT の公開鍵: `Config.JWTPublicKeys` に複数、`kid` で選択(問題 80 で対応済みなら確認のみ)。
- `cmd/admin secret rotate --tenant t1` で新しい鍵を発行し、古い鍵に `retired_at = now + 24h` を設定する。
- `docs/runbook/rotate-secrets.md` に手順(発行 → 受信側へ配布 → 猶予 → 失効)を書く。

**完了条件**

- [ ] 新旧 2 鍵の期間に、どちらの鍵で署名しても受信側で検証できるテスト
- [ ] `retired_at` を過ぎた鍵で署名した場合は失敗
- [ ] `revoke` 直後のリクエストが 401(キャッシュを入れた場合は `Invalidate` 経由で)
- [ ] ADR と runbook がある

**ヒント**: 「鍵を変える」は必ず起きる運用。変えられない設計は、漏洩したときに全停止になる。

---

## 85. 可観測性①: OpenTelemetry トレーシング

**主題**: span、context 伝搬、自動計装、trace_id とログの結び付け

**実装**

- `go get go.opentelemetry.io/otel go.opentelemetry.io/otel/sdk go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc` を導入する。
- `pkg/otel/tracing.go` に `Setup(ctx, cfg) (shutdown func(ctx) error, err error)`。`OTEL_EXPORTER_OTLP_ENDPOINT` が無ければ no-op(`local` では stdout exporter を選択可)。`service.name`、`service.version`、`deployment.environment` をリソース属性に。
- HTTP は `otelhttp.NewHandler`、gRPC は `otelgrpc.NewServerHandler`、外向き HTTP(Webhook)は `otelhttp.NewTransport`。DB は `pgx` の `QueryTracer` を実装して span を作る(SQL 文の先頭 80 文字を属性に。引数は入れない)。
- usecase の `Execute` に span を張る共通ラッパー `usecase.Traced(name, next)`(問題 28 の `Validated` と同じ形)。
- worker のジョブ実行にも span(`job.kind`、`job.attempt`)。enqueue 時に trace context を `payload` の横(`jobs.trace_context JSONB`)に保存し、実行時に `Link` として繋ぐ。
- ログに `trace_id`、`span_id` を自動で付ける(`slog.Handler` をラップ)。

**完了条件**

- [ ] `make obs-up` → `make run` → リクエスト → Jaeger UI で「HTTP → usecase → DB」の親子 span が見える(スクリーンショットのパスをメモ)
- [ ] worker のジョブ span が enqueue 元のトレースに Link で繋がる
- [ ] SQL の引数(タイトルなど)が span 属性に無い
- [ ] `tracetest.SpanRecorder` を使った単体テストで、usecase の span 名とエラー時の `Status` を検証
- [ ] `OTEL_EXPORTER_OTLP_ENDPOINT` 未設定でも起動し、オーバーヘッドが無視できる

**ヒント**: トレースは「1 リクエストの旅路」。context を切ったところで旅路も切れる。

---

## 86. 可観測性②: Prometheus メトリクス

**主題**: RED メトリクス、ヒストグラムのバケット、ラベルの爆発、プールとキューの計測

**実装**

- `go get github.com/prometheus/client_golang` を導入し、`pkg/metrics` に登録関数をまとめる。
  - `http_requests_total{method, route, status}`、`http_request_duration_seconds{method, route}`(ヒストグラム。バケットは 5ms〜10s の対数)。`route` はパスパターン(`/projects/{pid}/tasks`)であり実 URL ではない(ラベル爆発防止)。
  - `grpc_server_handled_total{method, code}`、`grpc_server_handling_seconds`。
  - `jobs_queue_depth{kind, status}`(gauge。30 秒ごとに SQL で数える Collector)、`jobs_processed_total{kind, result}`、`jobs_duration_seconds{kind}`。
  - `db_pool_*`(`pgxpool.Stat` から Collector で)。
  - `webhook_deliveries_total{result}`、`breaker_state{target}`。
- 管理ポート(`ADMIN_PORT=6060`)に `/metrics` を出す(認証は無いが、外部へ公開しない前提を doc に書く)。
- `make obs-up` の Prometheus でスクレイプされていることを確認する。

**完了条件**

- [ ] `promhttp` のテストで、リクエスト後に `http_requests_total` が増える(`testutil.ToFloat64`)
- [ ] `route` ラベルに ULID が含まれない(`/tasks/01H...` が `/tasks/{id}` になる)
- [ ] `jobs_queue_depth` が Prometheus UI で見える(スクリーンショットのパス)
- [ ] `docs/observability.md` に「何を見れば何が分かるか」を 10 行

**ヒント**: メトリクスは「集計された事実」、トレースは「個別の事実」。両方あると障害調査が速い。

---

## 87. 可観測性③: ログ相関とエラー分類

**主題**: request_id / trace_id / tenant / actor の一貫した付与、エラーの重要度、ノイズの削減

**実装**

- `pkg/log` の context ロガーに、ミドルウェア / インターセプタ / worker で `request_id`、`trace_id`、`tenant_id`、`actor_id`、`job_id` を積み、以降のログにすべて付く状態にする。
- `apperr.Error` に `Op string`(例: `usecase.task.Create`)を追加し、ラップの度に `Op` が連なる(`apperr.Ops(err) []string`)。500 のログにこの経路を出す。
- エラーの重要度: `KindInvalid` / `NotFound` / `Conflict` / `Unauthorized` / `Forbidden` は Info(利用者起因)、`Unavailable` は Warn(依存先)、`Unknown` は Error(バグ)。`writeError` と `toStatus` はこの規則でログレベルを決める。ヘルスチェックのアクセスログは Debug。
- 同じエラーが短時間に大量に出るときに間引く `pkg/log/sampler.go`(1 秒に同じ `Op` は 10 件まで)を作る。

**完了条件**

- [ ] 1 リクエストの全ログ行に同じ `request_id` と `trace_id` が付く(バッファへ書いて検証)
- [ ] `KindNotFound` が Error レベルで出ない
- [ ] `Ops` が `[usecase.task.Create, adapter.db.TaskRepository.Save]` のように並ぶテスト
- [ ] サンプラーのテスト(`Fake` Clock で 1 秒に 11 件目が捨てられる)

**ヒント**: ログの価値は「絞り込めること」。相関 ID が無いログは大量にあっても読めない。

---

## 88. 可観測性④: pprof と管理ポート

**主題**: `net/http/pprof`、管理系エンドポイントの分離、実行時の内省

**実装**

- 管理ポート(`ADMIN_PORT`)に `/debug/pprof/*`、`/metrics`、`/healthz`、`/readyz`、`/debug/vars`(`expvar`。ビルド情報、起動時刻、`Config` の非機密部分)を集約し、アプリポートからは `healthz` / `readyz` だけを残す。
- `pprof` は `ENV=production` では `ADMIN_PPROF=true` のときだけ有効にする。
- `readyz` を「DB」「オブジェクトストレージ(`HeadBucket`)」「worker の advisory lock 保持状況(worker のみ)」の複数チェックに拡張し、`{"status":"ok","checks":{"db":"ok","storage":"fail: ..."}}` を返す(1 つでも失敗なら 503)。
- `go tool pprof` で CPU / heap / goroutine を取る手順を `docs/runbook/pprof.md` に書く。

**完了条件**

- [ ] `curl localhost:6060/debug/pprof/goroutine?debug=1` で goroutine 一覧が見える
- [ ] `ENV=production` で `ADMIN_PPROF` 未設定なら 404
- [ ] `readyz` の複数チェックのテスト(1 つ失敗で 503、本文に失敗した項目)
- [ ] アプリポートに `/debug/pprof` が無い

**ヒント**: pprof は「本番で何が起きているか」を見る最強の道具。ただし公開すると情報漏洩になる。

---

## 89. 性能①: ベンチマークとアロケーション削減

**主題**: `testing.B`、`b.ReportAllocs`、`benchstat`、ホットパスの最適化

**実装**

- `go install golang.org/x/perf/cmd/benchstat@latest` を導入する(`make tools` に追加してよい)。
- 次のベンチマークを書く: カーソルの encode / decode(署名付き)、`TaskResponse` の JSON エンコード、`validate.Validator`、`Tags` の正規化、`convert.TaskToProto`。
- 各ベンチマークで `-benchmem` を取り、`docs/perf/89-before.txt` に保存する。プロファイル(`-cpuprofile`、`-memprofile`)を見て、アロケーションを減らす(例: `strings.Builder` の `Grow`、`sync.Pool` で `bytes.Buffer`、スライスの事前確保、`json.Encoder` の再利用、`fmt.Sprintf` を避ける)。
- `benchstat before.txt after.txt` の結果を `docs/perf/89-result.md` に貼り、変更内容と効果を表にする。
- 「速くしたが読みにくくなった」変更は戻す。判断基準を書く。

**完了条件**

- [ ] 5 つのベンチマークがあり、`make bench` で動く
- [ ] 少なくとも 2 つで `allocs/op` が 30% 以上減っている(benchstat の p 値付き)
- [ ] 最適化前後でテストがすべて通る
- [ ] `-count=10` で計測している(ノイズ対策)

**ヒント**: 計測せずに最適化しない。計測して意味がなければやらない。

---

## 90. 性能②: クエリ回数の検査

**主題**: N+1 の再発防止、テストでクエリ数を固定する、JOIN vs バッチ取得

**実装**

- 問題 38 の `QueryTracer` を使って、`dbtest.AssertQueryCount(t, ctx, max int, fn func())` を作る。
- 主要な読み取り経路にクエリ数の上限テストを追加する: `List`(タグ・担当者・Subtask 件数込みで ≤ 2)、`ProjectDashboard`(≤ 4 または Batch 1)、`GET /me/tasks`(≤ 2)、コメント一覧(≤ 1)、`FindByID`(≤ 3)。
- 意図的に N+1 を仕込んだブランチ(コメント一覧で作者名を 1 件ずつ引く)を作り、テストが失敗することを確認してから直す(確認結果をメモ)。
- `FindByID` の 3 クエリ(tasks、subtasks、tags)を 1 クエリ(`json_agg` で子を配列に)にする案を実装し、ベンチマークで比較して採用可否を決める。

**完了条件**

- [ ] 5 経路にクエリ数テストがある
- [ ] N+1 を仕込んだときにテストが落ちた記録
- [ ] `json_agg` 案の比較結果と決定が `docs/perf/90-findbyid.md` にある
- [ ] `AssertQueryCount` が `t.Parallel()` と共存する(トレーサーが context 単位でカウントする)

**ヒント**: N+1 は書いた瞬間ではなくデータが増えた半年後に発覚する。テストで数を固定しておく。

---

## 91. 性能③: 負荷試験とプロファイル

**主題**: p50 / p95 / p99、ボトルネックの特定、仮説と検証、目標設定

**実装**

- `scripts/load/` に `hey`(または `k6`)のシナリオを置く: `GET /projects/{pid}/tasks?limit=20`、`GET .../dashboard`、`POST .../tasks`(書き込み)。seed 済み DB(問題 58 の 10 万件)を使う。
- 目標を先に決める(例: 読み取り p95 < 50ms @ 200 rps、書き込み p95 < 100ms @ 50 rps)。`docs/perf/91-load.md` に記録する。
- 負荷中に `go tool pprof http://localhost:6060/debug/pprof/profile?seconds=30` で CPU プロファイルを取り、上位 5 つの関数を記録する。
- 見つかった問題を 1 つ以上直す(候補: 接続プールの `MaxConns` 不足、JSON エンコードのアロケーション、ダッシュボードの並列化(問題 76)の効果 or 逆効果、インデックス不足、ログの同期書き込み)。
- 修正後に再計測し、前後の表を作る。問題 76 の「SQL 1 本 vs errgroup 4 本」はここで決着をつける。

**完了条件**

- [ ] 3 シナリオの前後の p50 / p95 / p99 と rps が表になっている
- [ ] CPU プロファイルの上位 5 関数が記録され、修正の根拠になっている
- [ ] 目標を達成した / しなかった理由が書かれている
- [ ] 負荷試験のスクリプトが `make load` で再実行できる

**ヒント**: 負荷試験は「壊すため」にやる。壊れ方を知らないシステムは運用できない。

---

## 92. 性能④: メモリ上限を守るストリーミング

**主題**: ヒープの上限、`runtime.MemStats`、`GOMEMLIMIT`、大きなレスポンスの分割 **TDD 必須**

**実装**

- CSV エクスポート(問題 64)と gRPC ストリーム(問題 56)について、100 万件をエクスポートしてもヒープ増加が 50MB 以下であることをテストで確認する(`testing.Short()` でスキップ。`runtime.ReadMemStats` を前後で比較。Query はモックで 100 万件を生成し、DB は使わない)。
- DB 側は `Iterate` でページングしているが、`pgx` の `Rows` を 1 ページずつ閉じているか、`COPY TO STDOUT` の方が良いかを検討し、`pgx.Conn.CopyTo` を使う実装を追加して比較する。
- HTTP レスポンスの `Flush` 頻度と、`bufio.Writer` のサイズ(64KB)を調整する。
- `GOMEMLIMIT` を `Dockerfile` / compose で設定し(コンテナメモリの 80%)、`docs/runbook/memory.md` に理由を書く。
- 添付ファイルのダウンロードがサーバーを経由しない(302)ことを再確認し、`Put` の経路(インポート CSV)は `io.Copy` でストリーミングしている(全読みしていない)ことをテストで固定する。

**完了条件**

- [ ] 100 万件エクスポートのヒープ増加テスト(CSV と gRPC の 2 本)
- [ ] `COPY TO` 実装との比較結果が `docs/perf/92-export.md` にある
- [ ] `io.ReadAll` が adapter / gateway に無い(`rg "io.ReadAll"` の結果を確認し、必要な箇所には上限付き `io.LimitReader` があること)
- [ ] `GOMEMLIMIT` が設定されている

**ヒント**: 「全部メモリに載せてから返す」コードは、データが小さいうちは動く。動いているうちに直す。

---

## 93. 信頼性①: Idempotency-Key

**主題**: 書き込み API の冪等化、レスポンスの保存、同時リクエストの直列化 **TDD 必須(DB テスト)**

**実装**

- マイグレーション: `idempotency_keys(tenant_id, key, request_hash, status(running|done), response_status, response_body, created_at, PRIMARY KEY(tenant_id, key))`。
- ミドルウェア `Idempotent()` を `POST /projects/{pid}/tasks`、`POST .../assign` などの作成系に適用する。
  1. `Idempotency-Key` が無ければそのまま通す(任意ヘッダ。必須にするか検討し ADR に書く)
  2. あれば `INSERT ... ON CONFLICT DO NOTHING` で `running` を確保。確保できなければ既存行を読み、`done` なら保存済みレスポンスを返す(`Idempotent-Replayed: true` ヘッダ)、`running` なら 409
  3. リクエスト本文のハッシュを保存し、同じキーで違う本文なら 422
  4. handler 実行後にレスポンスをキャプチャして `done` で保存(5xx は保存せず行を削除し、再試行を許す)
- 24 時間で期限切れとし、`CleanupIdempotencyKeys` ジョブ(問題 72 のスケジュール)で削除する。
- gRPC は `idempotency-key` メタデータで同様に(インターセプタ)。

**完了条件**

- [ ] 同じキーで 2 回 `POST` して Task が 1 つ、2 回目に `Idempotent-Replayed: true`
- [ ] 同じキーで本文違いは 422
- [ ] 10 goroutine が同時に同じキーで `POST` して、成功 1・409 または replayed 9、Task は 1 つ
- [ ] 1 回目が 500 のとき、2 回目は再実行される
- [ ] 期限切れの掃除ジョブのテスト

**ヒント**: 冪等キーは「クライアントの再送」を安全にする。Outbox は「サーバーの再送」を安全にする。両方で初めて at-least-once が実用になる。

---

## 94. 信頼性②: 全体の graceful shutdown

**主題**: 停止順序、in-flight の完了、readiness の先行切り替え、テスト可能な起動関数 **TDD 必須**

**実装**

- `app.Run(ctx, app) error` に停止手順を集約する。順序:
  1. `readyz` を 503 に切り替える(ロードバランサから外れる時間を稼ぐ。`PreStopDelay` 5 秒)
  2. HTTP `Shutdown`(タイムアウト付き)と gRPC `GracefulStop` を並行
  3. worker: Dequeue 停止 → 実行中ジョブの完了待ち(タイムアウトで `Fail` にして再キュー)
  4. scheduler 停止、advisory lock 解放
  5. OTel の flush、メトリクスの最終出力
  6. DB プール、ストレージクライアントを閉じる
- 各ステップの所要時間をログに出す。
- テスト: `httptest` ではなく実ポート(`:0`)で起動し、遅いリクエスト(2 秒 sleep する専用のテスト用ハンドラ)を投げた直後に `cancel()` → レスポンスが 200 で返り、その後サーバーが閉じる。gRPC も同様。worker は `Fake` の Handler で「実行中のジョブが `Complete` されてから終了」を確認。
- `goleak` で停止後の goroutine 残留がないことを確認する。

**完了条件**

- [ ] 上記テストが `-race` で通り、5 秒以内に終わる(`PreStopDelay` はテストで 0 に)
- [ ] 停止順序が `docs/runbook/shutdown.md` に図で書かれている
- [ ] `ShutdownTimeout` を超えたときに強制終了し、終了コードが非 0
- [ ] `readyz` が停止開始と同時に 503 になる

**ヒント**: 「デプロイのたびにエラーが数件出る」の大半は shutdown 順序の問題。

---

## 95. 信頼性③: 無停止マイグレーション(expand / contract)

**主題**: 後方互換なスキーマ変更、バックフィルのバッチ化、3 段階リリース、ロック時間の意識

**実装**

- 課題: `tasks.description` を `tasks.body` に改名する(実務では「型を変える」「テーブルを分ける」でも同じ手順)。
- 3 段階で実装し、各段階で `make check` と `make test-db` が通り、旧コードと新コードが同時に動ける状態を保つ。
  1. **Expand**: `body` 列を追加(NULL 許容)。アプリは `description` と `body` の両方に書き、読みは `COALESCE(body, description)`。
  2. **Backfill**: `BackfillTaskBody` ジョブで 1000 行ずつ `UPDATE ... WHERE body IS NULL AND id IN (SELECT id ... LIMIT 1000)`。各バッチは別 Tx、バッチ間に `sleep`。進捗を `jobs.metadata` に記録し、再実行可能。
  3. **Contract**: アプリを `body` だけ読み書きするよう変更 → `description` に `NOT NULL` を外す → 列を削除。`Down` も書く。
- `ALTER TABLE ... ADD COLUMN ... DEFAULT` や `NOT NULL` 追加、インデックス作成(`CREATE INDEX CONCURRENTLY`。goose では `-- +goose NO TRANSACTION`)のロック挙動を `docs/runbook/migration.md` にまとめる。
- 危険なマイグレーション(列削除、`NOT NULL` 追加、型変更)を検出する簡易チェック `scripts/check-migration.sh`(`rg` ベースで十分)を CI に追加する。

**完了条件**

- [ ] 3 段階が 3 つ以上のマイグレーションファイルとコミットに分かれている
- [ ] Backfill ジョブが途中で止めても再開できる DB テスト
- [ ] `CREATE INDEX CONCURRENTLY` が `NO TRANSACTION` 付きで書かれている
- [ ] `scripts/check-migration.sh` が `DROP COLUMN` を含むファイルで警告を出す

**ヒント**: 「スキーマ変更とコード変更を同時にデプロイできる」と思わないこと。必ず時間差がある。

---

## 96. API の進化と互換性

**主題**: 後方互換の判定、フィールド追加と削除、非推奨、proto の互換性チェック

**実装**

- HTTP: `TaskResponse` に `labels`(`tags` の別名。将来 `tags` を廃止する想定)を追加し、`tags` を非推奨にする。`Deprecation: true`、`Sunset: <RFC1123 date>`、`Link: <docs>; rel="deprecation"` ヘッダを、`tags` を使うリクエスト(`?fields=tags` または旧クライアントの `User-Agent`)に付ける。
- proto: `Task` に `repeated string labels = N` を追加。`tags` に `[deprecated = true]`。フィールド番号の再利用を防ぐため `reserved` を使う例を 1 つ作る(`ExportTasksRequest` の未使用フィールドを削除して `reserved`)。
- `buf` を導入(`brew install bufbuild/buf/buf`)し、`buf.yaml` と `buf breaking --against '.git#branch=main'` を `make proto-check` に追加する。`make proto` も `buf generate` に置き換えてよい。
- 互換性テスト: 問題 51 時点の proto で生成した古いクライアント(テスト用に `gen/task/v1old` として保持)で `GetTask` を呼んでも動く。
- `docs/adr/ADR-00X-api-versioning.md`: URL バージョン(`/v2`)ではなくフィールド追加で進化する方針と、破壊的変更が必要なときの手順。

**完了条件**

- [ ] `buf breaking` がフィールド番号の変更で失敗し、追加では通る(手動で確認しメモ)
- [ ] 旧クライアントのテストが通る
- [ ] `Deprecation` ヘッダのテスト
- [ ] ADR がある

**ヒント**: 「削除」は最も高コストな変更。追加だけで進化できる設計を先に選ぶ。

---

## 97. OpenAPI と ADR

**主題**: 仕様書としての API 定義、スキーマ検証、意思決定の記録

**実装**

- `openapi.yaml`(OpenAPI 3.1)を手で書く(生成ツールは使わない。書くことで API の一貫性の欠如に気付く)。全エンドポイント、`components/schemas`(`Task`、`Problem`、`Page`)、`securitySchemes`(bearer)。
- `go get github.com/getkin/kin-openapi` を導入し、統合テスト(問題 49)のレスポンスを `openapi3filter.ValidateResponse` でスキーマ検証する共通ヘルパーを `testutil` に作る。全統合テストに適用する。
- `openapi.yaml` を管理ポートの `/openapi.yaml` で配信し、`docs/setup.md` に Swagger UI / Redoc での閲覧方法を書く。
- `docs/adr/` にテンプレートと、これまでの決定から 5 本以上を ADR にする(例: キーセットページネーション、Outbox、DB ベースのジョブキュー、ソフトデリート、テナントの context 伝搬、楽観ロック、JWT の EdDSA)。形式: 文脈 / 決定 / 結果 / 代替案。
- `CHANGELOG.md` を作り、問題 96 の非推奨を記載する。

**完了条件**

- [ ] 全統合テストがスキーマ検証付きで通る(スキーマに無いフィールドを返すと失敗することを確認)
- [ ] `openapi.yaml` が `spectral`(`npx @stoplight/spectral-cli lint openapi.yaml`)で警告 0(導入できない場合は理由をメモ)
- [ ] ADR が 5 本以上
- [ ] `docs/architecture.md` を最終形に更新

**ヒント**: ADR は「なぜそうしたか」を未来の自分とチームに残す。コードは「何をしたか」しか語らない。

---

## 98. 総合実装①: 繰り返しタスク

**主題**: 仕様書から縦に 1 本(ヒント無し)。設計判断を ADR に残す。

**仕様**

- Task に「繰り返しルール」を設定できる。ルールは `daily | weekly(曜日指定) | monthly(日付指定)`、`interval`(1〜52)、`until`(任意)。
- 繰り返し Task が `done` になると、次の期限(`due_at`)を計算して新しい Task を同じ Project に作成する(タイトル・説明・タグ・担当者を引き継ぐ。Subtask は未完了状態で複製)。`until` を過ぎたら作らない。
- 次回作成は同期(`Complete` の Tx 内)ではなく、`SpawnRecurringTask` ジョブで行う。ジョブは冪等(同じ「元 Task + 世代」から 2 つ作らない。`recurrences(source_task_id, generation, spawned_task_id, UNIQUE(source_task_id, generation))`)。
- 月末の扱い(31 日指定で 30 日までの月)、うるう年、タイムゾーン(テナントの `timezone` 設定を追加し、日付計算はそのタイムゾーンで行う)を仕様として決め、テストで固定する。
- HTTP / gRPC / CLI / OpenAPI / proto / 監査ログ / メトリクス(`recurrences_spawned_total`)/ トレースまで揃える。
- 繰り返しの停止(`DELETE .../recurrence`)と、系列の一覧(`GET .../tasks/{id}/recurrences`)。

**完了条件**

- [ ] 日付計算の domain テストが 30 ケース以上(月末、うるう年、DST 切り替え日、`until` 境界)
- [ ] ジョブの冪等性 DB テスト
- [ ] 全レイヤーのテスト、統合テスト、スキーマ検証、`make check`、`make test-db`、`make lint`、生成物の鮮度
- [ ] ADR 1 本(なぜ非同期で作るか、なぜ世代番号か)
- [ ] `docs/notes/98.md` に所要時間と振り返り

**自己採点の目安**: 6 時間以内。日付計算を `time.AddDate` だけで済ませて月末バグを踏んでいたら、仕様を先にテストへ落とす訓練が足りない。

---

## 99. 総合実装②: Webhook サブスクリプション管理

**主題**: 仕様書から縦に 1 本(ヒント無し)。外部連携基盤を「製品」として仕上げる。

**仕様**

- テナントごとに複数の Webhook サブスクリプションを登録できる: `webhook_subscriptions(id, tenant_id, url, events TEXT[], secret_kid, active, created_at)`。`url` は `https` のみ(`local` では `http` 許可)、プライベート IP・`localhost`・リンクローカルへの送信を拒否(SSRF 対策。DNS 解決後の IP も検査)。
- イベントフィルタ: `task.*`、`task.completed`、`project.archived` のようなワイルドカード。
- Outbox リレー(問題 70)を「1 イベント × N サブスクリプション」の配信に拡張。配信ごとに `webhook_deliveries(id, subscription_id, outbox_id, attempt, status, response_status, duration_ms, error, created_at)` を記録。サブスクリプションごとに独立したブレーカ。
- 連続 20 回失敗したサブスクリプションは自動で `active=false` にし、`WebhookSubscriptionDisabled` イベント(監査ログにも)。
- API: CRUD(`owner`)、`POST .../subscriptions/{id}/test`(テストイベント送信)、`GET .../subscriptions/{id}/deliveries?cursor=`、`POST .../deliveries/{id}/redeliver`。
- 秘密鍵は問題 84 のローテーション対象。`secret` はレスポンスに作成時 1 回だけ出す。
- CLI と OpenAPI と proto と メトリクス(`webhook_deliveries_total{subscription, result}` はラベル爆発するので `subscription` を入れない設計にし、理由を ADR に)。

**完了条件**

- [ ] SSRF テスト(`http://169.254.169.254`、`http://localhost`、`http://10.0.0.1`、DNS が private を返すホスト)で 400
- [ ] ワイルドカードのテーブル駆動テスト
- [ ] 2 サブスクリプション × 1 イベントで 2 配信、片方の失敗がもう片方に影響しない DB テスト
- [ ] 自動無効化のテスト
- [ ] `redeliver` の冪等性
- [ ] 全レイヤーのテスト、統合テスト、`make check`、`make test-db`、`make lint`
- [ ] ADR 1 本以上、`docs/notes/99.md`

**自己採点の目安**: 8 時間以内。SSRF 対策を URL の文字列検査だけで済ませていたら、脅威モデル(問題 83)を読み直す。

---

## 100. 最終チェックポイント: フルスタック起動と振り返り

**主題**: 100 問の成果物を「動く製品」として通しで検証し、次に繋げる。

**実装**

1. `make app-up` で `db` / `minio` / `server` / `worker` を起動し、`scripts/e2e.sh` を書いて次を自動で通す(`curl` + `jq` + `taskctl`)。
   - API キー発行 → Project 作成 → Task 作成 × 5(うち 1 つは繰り返し)→ 割り当て → 添付(presigned PUT)→ コメント → 完了 → 繰り返し Task の自動生成を待つ → CSV エクスポート → CSV インポート(非同期)→ Webhook サブスクリプション登録(受信側は `scripts/webhook-sink.go`)→ 配信の確認 → アーカイブ → エクスポートファイルの確認 → 監査ログの確認 → `readyz` / `metrics` の確認
   - 失敗したら非 0 で終了し、どのステップかを出す
2. `make e2e` を Makefile に追加し、CI(問題 74)にも `services` で組み込む(または `docker compose` を CI 内で起動)。
3. `docs/architecture.md` に「1 リクエストの旅路」(HTTP → middleware → handler → usecase → tx → repository → outbox → job → webhook)のシーケンス図(Mermaid)を書く。
4. 品質チェックリストを `docs/checklist.md` として整理し、全項目を確認する: テナント条件、ソフトデリート条件、冪等性、リソースのクローズ、ログの機密情報、shutdown 順序、インデックス、ラベル爆発、SSRF、CSV インジェクション、鍵ローテーション。
5. `PROGRESS.md` の振り返りメモを見返し、`docs/retrospective.md` に「知らなかった / 知っていたが使えなかった / 確認漏れ」の分布と、次に学ぶべき 3 テーマを書く。
6. 親リポジトリの読み歩き(README の 5 ステップ)を行い、各ステップで「この教材のどの問題に対応するか」を `docs/bridge.md` にメモする。

**完了条件**

- [ ] `make e2e` が成功する(所要時間をメモ)
- [ ] `make check`、`make test-db`、`make lint`、`make proto` / `make wire` / `make generate` 後の `git diff --exit-code` がすべて通る
- [ ] `go test -race -count=3 ./...` でフレークがない
- [ ] `docs/checklist.md` の全項目にチェックと根拠(テスト名 or lint ルール)
- [ ] `docs/retrospective.md` と `docs/bridge.md` がある
- [ ] 100 問終了時点のコードにタグ `v1.0.0` を付ける

**おつかれさまでした。** ここから先は [docs/final-app.md](../../docs/final-app.md) にある「作れるアプリ」のいずれかを、同じ縦切りの進め方で実装してみてください。仕様を自分で書き、ADR を残し、テストで固定する。その繰り返しが実務です。
