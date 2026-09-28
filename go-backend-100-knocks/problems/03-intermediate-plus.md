# 中級プラス(51–75): protobuf/gRPC、SQL、Wire、CSV、非同期ジョブ

> `docs/...` と書かれたパスはリポジトリルートの `docs/`(このファイルから見て `../../docs/`)を指します。`docs/notes/NN.md` などはそこへ作ってください。

## この範囲のゴール

- protobuf で API を定義し、HTTP と同じ usecase を gRPC からも呼べる
- インデックス・集計・行ロック・バルク処理を SQL で正しく書ける
- Wire で依存を組み立て、`main` から手書きの配線を無くせる
- CSV とオブジェクトストレージを扱い、重い処理を非同期ジョブへ逃がせる

## 事前準備

```bash
brew install protobuf          # protoc(未導入なら)
make db-up && make migrate && make migrate-test
```

`protoc-gen-go` / `protoc-gen-go-grpc` / `wire` は `go.mod` の tool として固定済みです。`make proto`、`make wire` を使います。

## 75 問終了時点の追加ディレクトリ

```
proto/task/v1/          task.proto project.proto
gen/task/v1/            生成物
gateway/rpc/            server.go handler_task.go interceptor.go convert/ errors.go
cmd/worker cmd/taskctl
app/wire.go app/wire_gen.go app/providers.go
adapter/gateway/storage/ s3.go memory.go
worker/                 job.go queue.go executor.go jobs/...
Dockerfile
```

---

## 51. protobuf 定義とコード生成

**主題**: `.proto` の書き方、パッケージ・オプション、`make proto`

**実装**

- `proto/task/v1/task.proto` に定義する。`syntax = "proto3"`、`package task.v1`、`option go_package = "github.com/kusa04/go-backend-100-knocks/gen/task/v1;taskv1"`。
  - `message Task`(`id`、`project_id`、`title`、`description`、`Status status`、`Priority priority`、`repeated string tags`、`google.protobuf.Timestamp created_at / updated_at / due_at`、`int32 version`、`repeated Subtask subtasks`、`optional string assignee_id`)
  - `enum Status { STATUS_UNSPECIFIED = 0; STATUS_TODO = 1; ... }`(enum は `_UNSPECIFIED = 0` から始める)
  - `service TaskService { rpc GetTask; rpc CreateTask; rpc ListTasks; rpc UpdateTask; rpc CompleteTask; }` と各 Request / Response
  - `ListTasksRequest` に `page_size`、`page_token`、`filter`(`status`、`tag`、`query`)
- `make proto` で `gen/task/v1/*.pb.go` を生成する。生成物はコミットする。
- `docs/proto-style.md` に「フィールド名は snake_case」「enum の接頭辞」「Request / Response は RPC ごとに専用」「破壊的変更の禁止(番号の再利用禁止)」を書く。

**完了条件**

- [ ] `make proto` が通り、`go build ./...` が通る
- [ ] `gen/` が `.golangci.yml` と `arch_test` の対象外になっている
- [ ] `docs/proto-style.md` がある
- [ ] `git diff` で `make proto` を 2 回実行しても差分が出ない

**ヒント**: `google/protobuf/timestamp.proto` の import は `protoc -I` にインクルードパスが必要。Homebrew の protoc は `/opt/homebrew/include` を自動で見る。

---

## 52. gRPC サーバー起動

**主題**: `google.golang.org/grpc`、生成された Server interface、HTTP と gRPC の同居

**実装**

- `go get google.golang.org/grpc` を導入する。
- `gateway/rpc/handler_task.go` に `taskv1.UnimplementedTaskServiceServer` を埋め込んだ `TaskServer` を作り、`GetTask` と `CreateTask` を実装する。usecase は HTTP と**同じもの**を使う。
- `gateway/rpc/server.go` に `New(opts) *grpc.Server` を作り、`reflection.Register` を有効にする(開発用。`Config.GRPCReflection` で切り替え)。
- `cmd/server` で HTTP(`:8080`)と gRPC(`:9090`)を別 goroutine で起動し、`errgroup` で束ねる。shutdown は gRPC の `GracefulStop`(タイムアウト付きで `Stop` にフォールバック)。
- 認証はこの問題ではまだ無い(次々問)。テナントは暫定でメタデータ `x-tenant-id` から読む。

**完了条件**

- [ ] `grpcurl -plaintext localhost:9090 list` で `task.v1.TaskService` が見える(`brew install grpcurl`。手順を `docs/setup.md` に追記)
- [ ] `bufconn` を使ったテストで `CreateTask` → `GetTask` が通る
- [ ] HTTP と gRPC が同じ usecase インスタンスを共有している(`main` で 1 回だけ生成)
- [ ] `Ctrl-C` で両方のサーバーが止まる

**ヒント**: `google.golang.org/grpc/test/bufconn` はネットワークを使わずに gRPC をテストできる。

---

## 53. gRPC のエラー変換

**主題**: `status.Code`、`errdetails`、HTTP と同じ apperr からの変換

**実装**

- `gateway/rpc/errors.go` に `toStatus(err error) error` を作る。対応表: `KindInvalid → InvalidArgument`、`KindNotFound → NotFound`、`KindConflict → FailedPrecondition`(または `Aborted`。楽観ロック競合は `Aborted` にする、と決めて表に書く)、`KindUnauthorized → Unauthenticated`、`KindForbidden → PermissionDenied`、`KindUnavailable → Unavailable`、その他 `Internal`。
- `validate.Errors` があれば `errdetails.BadRequest{FieldViolations}` を `status.WithDetails` で付ける。
- `Internal` のときはメッセージを固定文言にし、原因はログにだけ出す。
- 対応表は `gateway/http/errors.go` の表と並べて `docs/architecture.md` に載せる。

**完了条件**

- [ ] 7 種の Kind すべてのテーブル駆動テスト(`status.Code(err)` で検証)
- [ ] 検証エラーで `BadRequest.FieldViolations[0].Field == "title"`
- [ ] `errors.New("password=xxx")` を渡したときにクライアントへ返る `status.Message()` に `password` が無い
- [ ] `context.Canceled` が `Canceled`、`DeadlineExceeded` が `DeadlineExceeded` になる(`apperr` のラップを解いて判定)

**ヒント**: HTTP と gRPC で「同じ apperr → 別の表現」。表現の違いは gateway だけが知っている。

---

## 54. gRPC インターセプタ

**主題**: `UnaryServerInterceptor`、メタデータからの認証、チェーン、リカバリ **TDD 必須**

**実装**

- `gateway/rpc/interceptor.go` に次を作る。
  - `Auth(store)`: メタデータ `authorization: Bearer <key>` から HTTP と同じ `APIKeyStore` で `Principal` を解決し、context に載せる。失敗は `Unauthenticated`。`grpc.health.v1.Health` とリフレクションは除外。
  - `RequestID()`: メタデータ `x-request-id` があれば使い、無ければ生成。レスポンスヘッダ(`grpc.SetHeader`)にも返す。
  - `Logging(logger)`: メソッド名、コード、所要時間を 1 行。
  - `Recover()`: panic → `Internal`、スタックをログ。
- 認証ロジック(キーのハッシュ化・失効判定)は `pkg/auth` に共通関数として切り出し、HTTP ミドルウェアと gRPC インターセプタの両方から呼ぶ。
- `grpc.ChainUnaryInterceptor(Recover, RequestID, Logging, Auth)` の順で登録する。順序の理由をコメントに書く。

**完了条件**

- [ ] `bufconn` テストで、認証無しは `Unauthenticated`、失効キーは `Unauthenticated`、正しいキーで成功
- [ ] `x-request-id` の往復テスト
- [ ] panic するダミー RPC で `Internal` が返りサーバーが落ちない
- [ ] `rg "sha256" gateway/` が 0 件(ハッシュ計算が `pkg/auth` にだけある)

**ヒント**: インターセプタは HTTP ミドルウェアの gRPC 版。`handler(ctx, req)` を呼ぶ前後に処理を挟む。

---

## 55. proto ↔ domain 変換の分離

**主題**: 変換コードの置き場所、`timestamppb`、enum の相互変換、往復テスト

**実装**

- `gateway/rpc/convert` パッケージに `TaskToProto(*task.Task) *taskv1.Task`、`StatusToProto`、`StatusFromProto`、`PriorityToProto/FromProto`、`TimeToProto(*time.Time) *timestamppb.Timestamp`(nil 安全)を置く。usecase の Input / Output と proto の Request / Response の変換もここに置く。
- handler は「変換 → Execute → 変換 → `toStatus`」だけになる。各 RPC メソッドが 10 行以内に収まることを目標にする。
- `STATUS_UNSPECIFIED` を受け取ったときの扱い(フィルタ無し / 検証エラー)を決めてテストに書く。
- `ListTasks` の `page_token` は HTTP の cursor と同じエンコーダを使う(`gateway/http/cursor.go` を `gateway/cursor` へ移動し、両方から使う)。

**完了条件**

- [ ] `TaskToProto` → `TaskFromProto`(テスト用に作る)の往復で全フィールドが一致(`go-cmp` の `protocmp.Transform()` を使う)
- [ ] enum の全値がテーブル駆動テストで往復する
- [ ] `due_at` が nil のとき proto でも未設定(`HasDueAt()` が false)
- [ ] `gateway/rpc/handler_task.go` に `time.` と `taskv1.Status_` の直接参照が無い

**ヒント**: proto の型を usecase に持ち込まない。gRPC を剥がしても usecase が変わらないのが正しい状態。

---

## 56. サーバーストリーミング

**主題**: ストリーム RPC、クライアントのキャンセル、バックプレッシャ

**実装**

- `rpc ExportTasks(ExportTasksRequest) returns (stream Task)` を追加する。全 Task をキーセットでページングしながら順に `Send` する(1 ページ 100 件。全件をメモリに載せない)。
- `stream.Context()` のキャンセルを毎ページ確認し、キャンセルされたら `Canceled` で終了する。
- Query 側に `port.TaskQuery.Iterate(ctx, filter, fn func(TaskListItem) error) error` を追加し、SQL 側でページを回す。`fn` がエラーを返したら中断する。

**完了条件**

- [ ] `bufconn` テストで 250 件を送り、クライアントが 250 件受け取る
- [ ] クライアントが 10 件受け取った時点で `cancel()` すると、サーバー側の `Iterate` が 2 ページ目以降で止まる(`Iterate` の呼び出し回数をモックで検証)
- [ ] `Send` のエラーを捨てていない
- [ ] メモリ実装の `Iterate` もある

**ヒント**: ストリーミングは「返す量が事前に分からない」「クライアントが途中でやめる」を前提に書く。

---

## 57. gRPC クライアント CLI

**主題**: `grpc.NewClient`、`flag` / サブコマンド、デッドライン、終了コード

**実装**

- `cmd/taskctl` を作る。サブコマンド: `get <id>`、`create --project <pid> --title <t>`、`list --status todo`、`export > tasks.jsonl`。`flag.NewFlagSet` で十分(cobra は使わない)。
- 接続先は `TASKCTL_ADDR`(既定 `localhost:9090`)、キーは `TASKCTL_API_KEY`。すべての呼び出しに `context.WithTimeout(ctx, 10s)` を付け、`authorization` メタデータを付与する。
- エラーは `status.Code` に応じて終了コードを変える(`NotFound` は 2、`Unauthenticated` は 3、その他 1)。標準エラー出力に人が読めるメッセージ。
- `export` は `ExportTasks` ストリームを JSON Lines で標準出力へ流す。`Ctrl-C` でキャンセルされる。

**完了条件**

- [ ] `bufconn` を差し込めるよう、CLI の本体を `run(ctx, args []string, conn grpc.ClientConnInterface, stdout, stderr io.Writer) int` に分離し、テストしている
- [ ] `taskctl get missing` の終了コードが 2
- [ ] キーが未設定のときにネットワークへ出る前に終了コード 3 とメッセージ
- [ ] `docs/setup.md` に使い方を追記

**ヒント**: CLI も `main` を薄くして `run` をテストする。サーバーと同じ作法。

---

## 58. インデックス設計と EXPLAIN

**主題**: 実行計画の読み方、複合インデックス、部分インデックス、記録の残し方

**実装**

- テスト DB に 10 万件の Task を投入するスクリプト `cmd/admin seed --tasks 100000` を作る(問題 61 のバルク挿入を先取りしてよいが、ここでは `COPY` を `pgx.CopyFrom` で使う)。
- 次のクエリの `EXPLAIN (ANALYZE, BUFFERS)` を取り、`docs/perf/58-explain.md` に「変更前 → インデックス追加 → 変更後」を記録する。
  1. `List`(`tenant_id` + `status` フィルタ + キーセット)
  2. `GET /me/tasks`(`assignee_id`)
  3. `Summary`(`GROUP BY status`)
- 必要なインデックスをマイグレーションで追加する。候補: `(tenant_id, status, created_at, id) WHERE deleted_at IS NULL`、`(tenant_id, assignee_id, created_at, id) WHERE deleted_at IS NULL`。既存の `tasks_created_at_id_idx` が不要になれば削除する。
- 追加したインデックスが**使われている**ことを実行計画で確認する。使われていなければ列の順序を見直す。

**完了条件**

- [ ] 3 クエリの実行計画(前後)が `docs/perf/58-explain.md` にある
- [ ] `Index Scan` / `Index Only Scan` が出ており、`Rows Removed by Filter` が大幅に減っている
- [ ] インデックス追加が「書き込み性能とストレージのコスト」であることを 3 行で書いている
- [ ] seed 用データはテナント `t_seed` に入り、`make db-reset` で消せる

**ヒント**: 「インデックスの列順 = WHERE の等値条件 → 範囲条件 → ORDER BY」が基本形。

---

## 59. 集計クエリ(CTE / window 関数)

**主題**: `WITH`、`RANK() OVER`、`FILTER (WHERE ...)`、読み取りモデルの拡張

**実装**

- `port.TaskQuery.ProjectDashboard(ctx, projectID, now) (Dashboard, error)` を追加する。返す内容:
  - ステータス別件数(`count(*) FILTER (WHERE status = 'todo')` を 1 行で)
  - 期限が近い順トップ 5(`RANK() OVER (ORDER BY due_at)`、期限なしは除外)
  - 担当者別の未完了件数と、その担当者の中で最も古い未完了 Task(`DISTINCT ON` または window)
  - 直近 7 日間の日別完了数(`generate_series` で 0 件の日も出す)
- 1 回の `Query` で複数結果を返す方法(`;` 区切りの `pgx.Batch`、または CTE で 1 結果に寄せる)を選び、理由を書く。
- `GET /projects/{pid}/dashboard` を追加する。

**完了条件**

- [ ] 「0 件の日も出る」テスト(7 要素で埋まっている)
- [ ] `RANK` の同順位の扱いをテストで固定している
- [ ] クエリ数が 1 または `pgx.Batch` 1 往復
- [ ] `now` を引数で渡し、テストで固定している

**ヒント**: 集計は「Go で頑張る」より「SQL で頑張る」ほうが速く、テストもしやすい。

---

## 60. 行ロックと採番

**主題**: `SELECT ... FOR UPDATE`、直列化、並行テスト **TDD 必須(DB テスト)**

**実装**

- Task に人間が読む番号 `Number int`(プロジェクト内で連番、表示は `PROJ-42`)を追加する。マイグレーションで `projects.next_number INT NOT NULL DEFAULT 1` と `tasks.number`、一意制約 `(project_id, number)`。
- `port.ProjectRepository.NextNumber(ctx, projectID) (int, error)` を `UPDATE projects SET next_number = next_number + 1 WHERE id = $1 AND tenant_id = $2 RETURNING next_number - 1` で実装する(`FOR UPDATE` を使う 2 文の実装と比較し、どちらを選んだか書く)。
- `Create` usecase は `WithinTx` 内で `NextNumber` → `NewTask(...)` → `Save`。
- 並行テスト: 20 goroutine が同じ Project へ同時に `Create` し、番号が 1〜20 で重複・欠番がない。

**完了条件**

- [ ] 並行テストが `make test-db` で通る(`Pool` を使い、テナントごとに Cleanup)
- [ ] ロールバックされた Tx の番号が欠番になる(仕様として許容し、doc コメントに書く)
- [ ] `TaskResponse` と proto に `number` と `key`(`PROJ-42`)が出る
- [ ] 一意制約違反が `KindConflict` に変換される

**ヒント**: 「連番の欠番を許さない」要件は非常に高コスト。要件を疑うのも設計の一部。

---

## 61. バルクインサート

**主題**: `unnest` / `COPY`、バッチサイズ、部分失敗の扱い

**実装**

- `port.TaskRepository.SaveAll(ctx, tasks []*task.Task) error` を追加する。実装は `INSERT INTO tasks (...) SELECT * FROM unnest($1::text[], $2::text[], ...)` で 1 文に複数行を入れる。1000 件ごとにチャンク(`collection.Chunk`)。
- タグと Subtask も同様にバルクで入れる。
- 1 件でも CHECK 制約違反があれば全体を失敗させる(Tx 内)。どの行が原因か分かるよう、事前に Go 側で検証し、失敗行のインデックス一覧を `validate.Errors` として返す。
- ベンチマーク `BenchmarkSaveAll` で、1 件ずつ `Save` した場合と 1000 件で比較し、`docs/perf/61-bulk.md` に記録する。

**完了条件**

- [ ] 5000 件の `SaveAll` が DB テストで通り、件数が一致する
- [ ] 途中に不正な行を混ぜると 0 件挿入され、エラーに行番号が含まれる
- [ ] チャンクの境界(999、1000、1001 件)がテストにある
- [ ] ベンチマークの結果が記録されている(数値は環境依存でよい)

**ヒント**: `pgx.CopyFrom` はさらに速いが `ON CONFLICT` が使えない。用途で使い分ける。

---

## 62. Wire 導入①: Provider と Injector

**主題**: DI の考え方、`wire.NewSet`、`wire.Build`、生成コードを読む

**実装**

- `app/providers.go` に既存の生成関数を Provider として並べる。`ProvideConfig`、`ProvideLogger`、`ProvidePool`(`cleanup` 関数を返す)、`ProvideClock`、`ProvideIDGenerator`、各 Repository / Query / Transactor / Publisher / usecase / handler / router / gRPC server。
- `app/wire.go`(`//go:build wireinject`)に `func InitializeApp(ctx context.Context) (*App, func(), error)` を定義し、`wire.Build(...)` で Set を列挙する。`App` は HTTP handler、gRPC server、Config、Logger を持つ構造体。
- `make wire` で `app/wire_gen.go` を生成し、`cmd/server/main.go` は `app.InitializeApp` を呼ぶだけにする。
- Set は層ごとに分ける: `InfraSet`、`RepositorySet`、`UsecaseSet`、`HTTPSet`、`GRPCSet`。

**完了条件**

- [ ] `make wire` が通り、生成された `wire_gen.go` を読んで「Provider の呼び出し順序が依存関係から決まっている」ことを `docs/notes/62.md` に 5 行で説明
- [ ] `cmd/server/main.go` が 60 行以内
- [ ] `wire_gen.go` はコミットされ、`.golangci.yml` の除外に入っている
- [ ] Provider を 1 つ削除するとビルドではなく `make wire` で分かりやすいエラーになることを確認

**ヒント**: Wire は実行時 DI ではなくコード生成。生成物を読めばただの関数呼び出し。

---

## 63. Wire 導入②: Bind / cleanup / テスト用 Injector

**主題**: `wire.Bind`、`wire.Struct`、`wire.Value`、環境ごとの組み立て

**実装**

- `port.TaskRepository` に対して `wire.Bind(new(port.TaskRepository), new(*db.TaskRepository))` で interface と実装を結ぶ。すべての port で同様にする。
- `usecase` の `Deps` 構造体は `wire.Struct(new(task.Deps), "*")` で埋める。
- `cleanup`: `ProvidePool` の cleanup で `Pool.Close`、ロガーの flush など。`main` は終了時に `cleanup()` を呼ぶ。
- テスト用 Injector `InitializeTestApp(cfg Config, clk clock.Clock, gen id.Generator) (*App, func(), error)` を `app/wire_test_inject.go` に作り、`memory` 実装を `wire.Bind` した `MemorySet` を使う。統合テスト(問題 49)はこれで組み立てる。
- `WEBHOOK_URL` の有無で Publisher を切り替えていた分岐は、Outbox 化(問題 45)で不要になっているはず。残っていれば削除する。

**完了条件**

- [ ] 本番用と テスト用の 2 つの Injector が同じ `UsecaseSet` を共有している
- [ ] `cleanup` の呼び出し順(後から作ったものが先に閉じる)を `wire_gen.go` で確認し、メモに書く
- [ ] `main` に `db.New...` などの具象型の呼び出しが無い
- [ ] `make wire` → `make check` が通る

**ヒント**: 「差し替えたい単位 = Set の単位」。環境で変わるのは Infra 層だけ。

---

## 64. CSV エクスポート(ストリーミング)

**主題**: `encoding/csv`、`http.Flusher`、`Content-Disposition`、大量データをメモリに載せない **TDD 必須**

**実装**

- `GET /projects/{pid}/tasks/export.csv?status=` を追加する。`Content-Type: text/csv; charset=utf-8`、`Content-Disposition: attachment; filename="tasks-<pid>-<yyyymmdd>.csv"`。
- `usecase/query/export_tasks.go` は `io.Writer` を受け取り、`port.TaskQuery.Iterate` で 1 件ずつ `csv.Writer` に書く。列: `key,title,status,priority,assignee,tags,due_at,created_at`。`tags` は `;` 区切り。
- 先頭に UTF-8 BOM を付けるかを `?bom=1` で選べるようにする(Excel 対策。既定は付けない)。
- 1000 行ごとに `Flush` し、`http.Flusher` があれば呼ぶ。エラーが途中で起きた場合の扱い(ヘッダ送信後は 200 を変えられない)を doc コメントに書き、ログに残す。
- `csv.Writer` は改行やカンマを含むタイトルを正しくクォートすることをテストする。

**完了条件**

- [ ] 改行・カンマ・ダブルクォートを含むタイトルがラウンドトリップする(`csv.Reader` で読み戻して比較)
- [ ] `?bom=1` で先頭 3 バイトが `EF BB BF`
- [ ] 10 万件のエクスポート中のヒープ増加が一定以下(問題 92 で厳密化。ここでは `Iterate` がページ単位で動いていることをモックで確認)
- [ ] `csv.Writer.Error()` を最後に確認している

**ヒント**: CSV の「セル先頭が `=`、`+`、`-`、`@`」の問題は問題 81 で扱う。ここでは TODO を残す。

---

## 65. CSV インポート①: 解析と検証

**主題**: multipart、`http.MaxBytesReader`、行単位のエラー報告、ドライラン **TDD 必須**

**実装**

- `POST /projects/{pid}/tasks/import?dry_run=1` で `multipart/form-data`(フィールド名 `file`)を受け取る。サイズ上限 10MB(`http.MaxBytesReader`。超過は 413)。
- `usecase/task/import_tasks.go`: `io.Reader` から `csv.Reader` で読み、ヘッダ行を検証(必須列 `title`、任意 `description,status,priority,tags,due_at`)。各行を `CreateInput` 相当に変換して `Validate()`。
- 結果 `ImportResult{Total, Valid, Errors []RowError{Line int; Field, Message string}}`。エラーは最大 100 件まで集め、それ以上は `Truncated: true`。
- ドライランでは DB へ書かず結果だけ返す(200)。ドライランでないときはこの問題では 501 を返し、問題 66 で実装する。
- BOM 付きファイルを受け入れる(先頭 BOM を読み飛ばす)。

**完了条件**

- [ ] ヘッダ欠落、列数不一致(`csv.ErrFieldCount`)、不正な `priority`、200 文字超のタイトルがそれぞれ行番号付きで報告される
- [ ] 10MB + 1 バイトで 413(`MaxBytesError` を `KindInvalid` ではなく専用の Kind か 413 に対応付ける)
- [ ] `multipart.File` を閉じている
- [ ] BOM 付き・無しの両方で同じ結果

**ヒント**: 検証は「全行を見て全部報告」。1 行目で止めるとユーザーは何度も往復する。

---

## 66. CSV インポート②: トランザクションと冪等性

**主題**: 全件成功 or 全件失敗、`import_id` による重複防止、バルク挿入の再利用 **TDD 必須(DB テスト)**

**実装**

- マイグレーション: `imports(id, tenant_id, project_id, idempotency_key TEXT, status, total, created_at, UNIQUE(tenant_id, idempotency_key))`。
- `POST .../import`(ドライランでない)は `Idempotency-Key` ヘッダ必須(無ければ 400)。同じキーで 2 回目は、1 回目の結果をそのまま返す(200)。処理中(`status = running`)なら 409。
- 処理: `WithinTx` 内で `imports` に `running` を INSERT(一意制約で重複を検出)→ 全行検証 → `SaveAll`(問題 61)→ `NextNumber` を件数分まとめて確保(`next_number = next_number + $n RETURNING`)→ `imports.status = done`。検証エラーが 1 件でもあれば全体をロールバックし、`imports` にも残さない(または `failed` として残す。どちらかを選ぶ)。
- イベントは `TasksImported{Count}` を 1 件だけ Outbox に書く(1 行ごとの `TaskCreated` を書くかどうかを検討し、決定を書く)。

**完了条件**

- [ ] 1000 行の CSV が 1 Tx で入り、途中の不正行で 0 件になる
- [ ] 同じ `Idempotency-Key` で 2 回送っても Task が増えない
- [ ] 採番が連続していて重複がない(並行インポートのテスト: 2 つの CSV を同時に投げる)
- [ ] 統合テストにシナリオを 1 本追加

**ヒント**: 冪等性は「同じ要求を何度受けても結果が同じ」。ネットワークの再送は必ず起きる前提。

---

## 67. オブジェクトストレージと添付ファイル

**主題**: S3 互換 API、presigned URL、port による抽象化、MinIO

**実装**

- `go get github.com/aws/aws-sdk-go-v2/{config,service/s3,credentials}` を導入する。
- `port.ObjectStorage` に `Put(ctx, key string, r io.Reader, size int64, contentType string) error`、`PresignGet(ctx, key, ttl) (string, error)`、`PresignPut(ctx, key, contentType, ttl) (string, error)`、`Delete(ctx, key) error`。`adapter/gateway/storage/s3.go`(MinIO 向けに `UsePathStyle: true`)と `storage/memory.go`。
- マイグレーション: `task_attachments(id, tenant_id, task_id, filename, content_type, size, object_key, status(pending|ready), created_at)`。
- フロー(直接アップロード方式):
  1. `POST .../tasks/{id}/attachments {"filename","content_type","size"}` → `pending` で行を作り、`PresignPut` の URL を返す(201)
  2. クライアントが URL へ `PUT`
  3. `POST .../attachments/{aid}/complete` → `HeadObject` で存在とサイズを確認し `ready` に
  4. `GET .../attachments/{aid}` → `PresignGet` の URL へ 302
- `object_key` は `tenants/{tenant}/tasks/{task}/{attachment_id}` の形にし、ファイル名を key に含めない(問題 81 でパストラバーサルとして扱う)。
- サイズ上限 25MB、`content_type` は許可リスト。
- `make db-up` で MinIO が上がっているので、`TEST_S3_ENDPOINT` があるときだけ実 MinIO でテスト、無ければ skip。

**完了条件**

- [ ] メモリ実装で usecase のテストが通る
- [ ] MinIO を使った adapter のテスト(Put → PresignGet で `http.Get` して内容一致)
- [ ] `complete` 前の添付は `GET` で 404
- [ ] シークレット(`S3_SECRET_KEY`)が `Config.String()` に出ない
- [ ] バケット作成は起動時に `CreateBucket`(存在すれば無視)で行い、手順を `docs/setup.md` に追記

**ヒント**: サーバーがファイル本体を中継しない設計にすると、メモリと帯域を守れる。

---

## 68. 非同期ジョブ①: ジョブテーブルと Enqueue

**主題**: DB ベースのキュー、型付きペイロード、Tx 内での enqueue **TDD 必須(DB テスト)**

**実装**

- マイグレーション: `jobs(id, tenant_id NULL, kind TEXT, payload JSONB, status(queued|running|done|failed|dead), run_at, attempts INT, max_attempts INT, last_error TEXT NULL, locked_at NULL, locked_by NULL, created_at, updated_at)`。`(status, run_at)` にインデックス。
- `worker/job.go`:
  ```go
  type Job[P any] struct { Kind string; MaxAttempts int }        // ジョブ定義
  type Envelope struct { ID, Kind string; Payload json.RawMessage; Attempts int; ... }
  type Handler interface { Kind() string; Execute(ctx context.Context, env Envelope) error }
  type Registry struct{ ... }   // Kind → Handler。重複登録は panic
  ```
- `port.JobQueue.Enqueue(ctx, kind string, payload any, opts EnqueueOptions{RunAt, MaxAttempts}) (id string, err error)`。DB 実装は `querierFrom(ctx)` を使い、usecase の Tx に参加する。
- 最初のジョブ `SendWebhook`(payload: `outbox_id`)を定義する(実行は問題 70)。
- `Complete` usecase で、Tx 内で `Enqueue` する例を 1 つ入れる(例: `NotifyCompletion`。実装は「ログを出すだけ」)。

**完了条件**

- [ ] Tx ロールバックで `jobs` に行が残らない
- [ ] `payload` の型が `Job[P]` で固定され、`Enqueue` に別の型を渡すとコンパイルエラーになる(ジェネリクスの型安全)
- [ ] `Registry` の重複登録 panic のテスト
- [ ] `run_at` 未指定は `clock.Now()`

**ヒント**: Redis や SQS を使わない理由(トランザクションに乗せられる、運用が 1 つ減る)を `worker/doc.go` に書く。

---

## 69. 非同期ジョブ②: Worker ループと再試行

**主題**: `FOR UPDATE SKIP LOCKED`、at-least-once、バックオフ、デッドレター、graceful stop **TDD 必須(DB テスト)**

**実装**

- `port.JobQueue.Dequeue(ctx, workerID string, kinds []string) (*Envelope, error)`: `UPDATE jobs SET status='running', locked_at=now, locked_by=$1, attempts=attempts+1 WHERE id = (SELECT id FROM jobs WHERE status='queued' AND run_at <= $2 AND kind = ANY($3) ORDER BY run_at LIMIT 1 FOR UPDATE SKIP LOCKED) RETURNING ...`。無ければ `nil, nil`。
- `Complete(ctx, id)`、`Fail(ctx, id, err, retryAt *time.Time)`(`retryAt` が nil なら `dead`)。
- `worker/executor.go`: ループで `Dequeue` → `Registry` から Handler → `Execute`(ジョブごとのタイムアウト付き context)→ `Complete` / `Fail`。失敗時は `retry.Policy` で `retryAt` を計算し、`attempts >= max_attempts` なら `dead`。panic は失敗扱い。空のときは `PollInterval` 待つ。
- `cmd/worker/main.go`: Wire で組み立て(`InitializeWorker`)、`signal.NotifyContext` で停止。停止時は実行中のジョブを完了させてから終了(`WaitGroup`)。
- スタックしたジョブの回収: `locked_at` が 10 分以上前の `running` を `queued` に戻す `Reaper` を `Executor` に持たせる。

**完了条件**

- [ ] 2 つの Executor を同時に動かしても同じジョブが二重に実行されない DB テスト(`SKIP LOCKED` の効果)
- [ ] 3 回失敗で `dead` になり、`last_error` が入る
- [ ] `run_at` が未来のジョブは取り出されない
- [ ] `Ctrl-C` で実行中のジョブが終わってから終了する(手動確認をメモ)
- [ ] `make run-worker` が動く

**ヒント**: at-least-once なので、ジョブの処理は冪等に書く。「二重実行しても壊れない」が Handler の契約。

---

## 70. 非同期ジョブ③: Outbox リレー

**主題**: Outbox → 外部配信、リレーの冪等性、失敗の記録

**実装**

- `worker/jobs/relay_outbox.go`: `SendWebhook` ジョブの Handler。`OutboxStore.Get(id)` → 送信済みならスキップ → `webhook.Client.Send(event)`(問題 43・44 のリトライ・ブレーカ入り)→ `MarkSent` / `MarkFailed`。
- Outbox に書いた瞬間にジョブを enqueue する(問題 45 の `Publish` を「outbox INSERT + `Enqueue(SendWebhook{outbox_id})`」に拡張)。両方同じ Tx。
- 保険として `RelayScan` ジョブ(定期実行は問題 72)を作り、`sent_at IS NULL AND created_at < now - 5min` を拾って再 enqueue する。
- Webhook の送信先は暫定で `Config.WebhookURL`(テナント別は問題 99)。

**完了条件**

- [ ] `httptest.NewServer` を送信先にした DB テストで、`Complete` → worker 1 周 → サーバーがイベントを受信し `sent_at` が埋まる
- [ ] 同じ `outbox_id` のジョブを 2 回実行しても送信は 1 回
- [ ] 送信先が 500 のとき `MarkFailed` で `attempts` と `last_error` が更新され、ジョブは再試行される
- [ ] 送信先が 400 のときはリトライせず `dead`

**ヒント**: 「Outbox は永続化された TODO リスト」。送れたかどうかは Outbox が真実。

---

## 71. 非同期ジョブ④: CSV インポートの非同期化

**主題**: 重い処理の非同期化、進捗の永続化、ファイルをストレージ経由で渡す

**実装**

- `POST .../tasks/import?async=1`: ファイルをオブジェクトストレージ(`imports/{tenant}/{import_id}.csv`)へ `Put` し、`imports` に `queued` で行を作り、`ImportTasks{import_id}` ジョブを enqueue して 202 と `{"import_id":"...","status_url":"..."}` を返す。
- `worker/jobs/import_tasks.go`: ストレージから読み、問題 66 のロジックを再利用する。`imports.progress`(処理済み行数)を 500 行ごとに更新する(別 Tx。メインの Tx とは独立)。
- `GET .../imports/{id}` で `status`、`total`、`progress`、`errors`(`imports.errors JSONB`)を返す。
- 同期版(問題 66)と非同期版で検証・挿入ロジックが二重にならないよう、`usecase/task/import_tasks.go` に共通の `importer` を置く。

**完了条件**

- [ ] 202 → worker → `done` の DB テスト
- [ ] 検証エラーで `failed` になり、`errors` に行番号付きで残る
- [ ] `progress` が途中で更新される(1200 行で 2 回以上)
- [ ] 同期版と非同期版の usecase テストが共通のテーブルを使っている

**ヒント**: 非同期化しても「冪等・進捗・失敗の記録・再実行」が揃って初めて運用に耐える。

---

## 72. 定期実行と advisory lock

**主題**: cron 相当の処理、多重起動の防止、`pg_try_advisory_lock`

**実装**

- `worker/scheduler.go`: `Schedule{Name, Every time.Duration, Enqueue func(ctx) error}` の一覧を持ち、`Every` ごとに `Enqueue` を呼ぶループ。`clock.Clock` を使う。
- 複数の worker が同時に動いても 1 つだけがスケジュールを動かすように、`SELECT pg_try_advisory_lock($1)` を専用コネクションで取り、取れたプロセスだけがループを回す(取れなければ 30 秒ごとに再挑戦)。
- スケジュール: `RelayScan`(5 分ごと)、`RemindDueTasks`(毎時。期限が 24 時間以内の Task を持つ担当者ごとに `TaskDueSoon` イベントを Outbox に書く。同じ Task に 1 日 1 回だけ。`reminded_at` 列で管理)。
- `cmd/worker` で Executor と Scheduler を並行に動かす。

**完了条件**

- [ ] `Fake` Clock を進めて `Enqueue` が期待回数呼ばれるテスト
- [ ] 2 プロセス相当(2 つの `Pool` 接続)でロックが 1 つしか取れない DB テスト
- [ ] `RemindDueTasks` が同じ Task を 24 時間以内に 2 回通知しない
- [ ] ロックを持つコネクションが落ちたら別プロセスが引き継ぐ(コネクションを閉じてから再取得するテスト)

**ヒント**: advisory lock は「コネクションが生きている間だけ有効」。プールから借りた接続で取ると、返却時に解放される点に注意する。

---

## 73. 設定・シークレット・Dockerfile

**主題**: 環境ごとの設定、fail-fast、マルチステージビルド、compose での一発起動

**実装**

- `Config` を `local | test | production` の `Env` で分岐させる。`production` では `GRPCReflection=false`、`LogFormat=json`、`WebhookSecret` 必須、`DATABASE_URL` の `sslmode=disable` を拒否。
- シークレット型 `type Secret string` を作り、`String()` と `MarshalJSON`、`slog.LogValuer` で `"[REDACTED]"` を返す。`WebhookSecret`、`S3SecretKey`、`DatabaseURL` のパスワード部を `Secret` にする。
- `Dockerfile`(マルチステージ: `golang:1.24` でビルド → `gcr.io/distroless/static` で実行。`server` と `worker` を `--target` で切り替え)。`CGO_ENABLED=0`、`-trimpath`、`-ldflags="-s -w -X main.version=..."`。
- `docker-compose.yml` に `server` と `worker` サービスを追加し(`profiles: ["app"]`)、`make app-up` で `db`、`minio`、`server`、`worker` が上がる。`server` は `depends_on: db: condition: service_healthy`。マイグレーションは起動時に `server` が `goose` ライブラリで自動適用する(`AUTO_MIGRATE=true` のときだけ)。
- `.dockerignore` を書く。

**完了条件**

- [ ] `Secret` を `fmt.Sprintf("%v")`、`json.Marshal`、`slog` に渡しても値が出ないテスト
- [ ] `ENV=production` かつ `sslmode=disable` で起動失敗
- [ ] `make app-up` 後に `curl localhost:8080/readyz` が 200、`taskctl list` が動く
- [ ] イメージサイズが 50MB 以下(`docker images` の結果をメモ)

**ヒント**: 「起動時に設定を全部検証して、駄目なら即落ちる」。稼働中に設定不備が発覚するのが最悪。

---

## 74. golangci-lint と品質ゲート

**主題**: linter の意味を理解して直す、CI 相当のチェック、ルールの機械化

**実装**

- `brew install golangci-lint` を導入し、`make lint` が golangci-lint で動くことを確認する。同梱の `.golangci.yml` の各 linter が何を検出するかを `docs/lint.md` に 1 行ずつ書く。
- 現時点の指摘をすべて解消する。`//nolint` を使う場合は `//nolint:gosec // 理由` の形で理由を必須にする。
- `depguard` のルールが問題 26 の `arch_test` と一致していることを確認し、`arch_test` の役割を「lint で表現できないもの(SQL 定数の文字列検査など)」に絞る。
- `.github/workflows/ci.yml`(GitHub Actions)を書く: `make check`、`make test-db`(`services: postgres`)、`make proto` / `make wire` / `make generate` 後に `git diff --exit-code`(生成物の鮮度)。
- `Makefile` の `check` に `lint` が含まれていることを確認する(含まれている)。

**完了条件**

- [ ] `make lint` の指摘が 0
- [ ] `//nolint` の全箇所に理由がある(`rg "nolint" --glob '*.go'` で確認)
- [ ] CI の YAML が `act` かプッシュで通る(プッシュした場合はリンクをメモ)
- [ ] `docs/lint.md` がある

**ヒント**: linter の指摘は「なぜ駄目か」を理解してから直す。理解できない指摘は調べてから `docs/lint.md` に足す。

---

## 75. チェックポイント: プロジェクトのアーカイブ

**主題**: 中級プラスの内容を縦に 1 本。受け入れ条件だけで実装する。

**仕様**

- `Project` に `archived_at` を追加する。`POST /projects/{pid}/archive`(HTTP)と `rpc ArchiveProject`(gRPC)の両方から、同じ usecase を呼ぶ。`owner` のみ。
- アーカイブ時の処理:
  1. Tx 内で `archived_at` を設定し、`ProjectArchived` イベントを Outbox に書き、`ExportProject{project_id}` ジョブを enqueue する
  2. ジョブは全 Task を CSV(問題 64 の形式)にしてオブジェクトストレージ `exports/{tenant}/{project}/{yyyymmdd}.csv` へ書き、`project_exports(id, project_id, object_key, row_count, created_at)` に記録し、`ProjectExported{download_url(presigned, 24h)}` イベントを Outbox に書く
  3. ジョブは冪等(同じ日に 2 回実行しても 1 ファイル)
- アーカイブ済み Project 配下の Task は書き込み操作がすべて 409(`ErrProjectArchived`)。読み取りは可能。
- `GET /projects?archived=true|false` でフィルタできる。
- 採番(`NextNumber`)はアーカイブ済みでも失敗する(Task 作成が 409 なので自然にそうなるはずだが、テストで固定する)。
- CLI `taskctl project archive <pid>` を追加する。

**完了条件**

- [ ] domain: `ErrProjectArchived` のテーブル駆動テスト(Create、Update、Start、Assign、Comment の 5 操作)
- [ ] usecase: モックで「Outbox → Enqueue の順」「owner 以外は Forbidden」
- [ ] adapter: DB テストで `archived` フィルタ、`project_exports` の一意性
- [ ] worker: MinIO(または memory)へのエクスポートと冪等性
- [ ] gateway: HTTP と gRPC 両方のテスト(同じ usecase を共有していることを Wire の Set で確認)
- [ ] `make check`、`make test-db`、`make lint` が通り、生成物に差分がない
- [ ] `docs/notes/75.md` に振り返り 3 点以上

**自己採点の目安**: 4 時間以内。「アーカイブ済みなら 409」が SQL の `WHERE` や handler に書かれていたら、ルールの置き場所を見直す。
