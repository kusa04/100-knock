# 中級(26–50): レイヤー分離、CQRS、モック、外部HTTP、トランザクション

> `docs/...` と書かれたパスはリポジトリルートの `docs/`(このファイルから見て `../../docs/`)を指します。`docs/notes/NN.md` などはそこへ作ってください。

## この範囲のゴール

- レイヤーの依存方向を機械的に守り、port に対するモックで usecase を単体テストできる
- PostgreSQL に対して Repository / Query を実装し、トランザクション境界を usecase に置ける
- テナント分離・認証・認可を「handler で解決し usecase / domain で判定する」形で実装できる
- 外部 HTTP との連携を、署名・冪等性・リトライ・Outbox で「壊れにくく」できる

## 事前準備

```bash
open -a Docker
make db-up          # PostgreSQL と MinIO
```

DB を使うテストは `TEST_DATABASE_URL` が無いと自動で skip されます。`make test-db` で実行してください。

## 50 問終了時点の追加ディレクトリ

```
usecase/mock/           mockgen 生成物
usecase/query/          読み取りユースケース
adapter/gateway/db/     conn.go tx.go task_repository.go task_query.go outbox.go ...
domain/project domain/member
pkg/tenant pkg/auth
migrations/             0001_create_tasks.sql ...
testutil/dbtest/        DB テスト基盤
```

---

## 26. レイヤー依存ルールの機械的検査

**主題**: import グラフ、`golang.org/x/tools/go/packages`、アーキテクチャテスト

**実装**

- `arch_test.go`(module ルート、`package main_test` ではなく `package arch_test` としてルート直下に `arch/` を作ってもよい)に、`go/packages` で全パッケージの import を読み、次のルールに違反したら失敗するテストを書く。
  - `domain/**` は `usecase`、`adapter`、`gateway`、`app`、`database/sql`、`net/http`、`encoding/json` を import しない
  - `usecase/**` は `adapter`、`gateway`、`app` を import しない
  - `gateway/**` は `adapter` を import しない(具象実装は `app` が組み立てる)
  - `adapter/**` は `gateway` を import しない
- ルールは `[]rule{from: "domain/", deny: []string{...}}` のようなデータで持ち、失敗メッセージに「どのパッケージが何を import したか」を出す。
- `docs/architecture.md` の表とルールが一致しているか確認し、ずれがあれば表を直す。

**完了条件**

- [ ] 故意に `domain/task` へ `net/http` を import するとテストが失敗し、戻すと通る(手動で確認し、確認したことをコミットメッセージかメモに書く)
- [ ] テストの実行時間が 5 秒以内(`packages.Load` の `Mode` を最小限にする)
- [ ] ルールの追加が 1 行で済む

**ヒント**: `packages.Load(&packages.Config{Mode: packages.NeedName | packages.NeedImports}, "./...")`。

---

## 27. mockgen による Port のモック

**主題**: `go.uber.org/mock`、`//go:generate`、モックで usecase を単体テストする

**実装**

- `go get go.uber.org/mock` を導入する(`mockgen` 本体は `go tool mockgen` で実行可能)。
- `usecase/port/generate.go` に `//go:generate go tool mockgen -source=task_repository.go -destination=../mock/task_repository.go -package=mock` の形で port ごとに書く。`make generate` で生成し、生成物はコミットする。
- `usecase/task` のテストを、メモリ Repository ではなくモックで書き直す。少なくとも次を検証する。
  - `Create`: `Save` が 1 回呼ばれ、渡された Task の `Title` が正規化済み
  - `Get`: `FindByID` が `KindNotFound` を返したらそのまま返す
  - `Complete`: `Save` が失敗したら `Publish` が呼ばれない(順序の検証は `gomock.InOrder`)
- メモリ Repository を使ったテストは削除せず、`_integration_test.go` などに分けて「結合寄りのテスト」として残す。

**完了条件**

- [ ] `make generate` を実行しても差分が出ない(生成物が最新)
- [ ] usecase のテストでモックの期待呼び出し回数(`Times`、`MaxTimes`)が明示されている
- [ ] `gomock.Any()` の乱用がない(引数を検証できるところは `gomock.Cond` かカスタム Matcher)
- [ ] 具象型(`memory.TaskRepository`)に対するモックが存在しない

**ヒント**: モックは「呼び出し側の契約」を検証する道具。戻り値だけでなく「呼ばれないこと」も契約になる。

---

## 28. Interactor パターンへの統一

**主題**: usecase の形を揃える、共通 interface、依存の束ね方

**実装**

- `usecase/usecase.go` に `type Interactor[I, O any] interface { Execute(ctx context.Context, in I) (O, error) }` を定義する。
- `usecase/task` の各 usecase が `Interactor[CreateInput, *CreateOutput]` などを満たすことを `var _` で保証する。
- 各 usecase の依存を `type Deps struct { Repo port.TaskRepository; Publisher port.EventPublisher; Clock clock.Clock; ID id.Generator }` にまとめ、`NewCreate(d Deps)` の形に統一する。ただし各 usecase は必要なフィールドだけを使う。
- handler は各 usecase を interface(`Interactor[...]`)として受け取る。テストでは usecase 自体を差し替えられる。
- `Execute` の共通前処理(入力の `Validate()`)を、`in` が `interface{ Validate() error }` を満たす場合に自動で呼ぶ薄いラッパー `usecase.Validated(next Interactor[I, O]) Interactor[I, O]` として作る。

**完了条件**

- [ ] すべての usecase が同じ形(`New*(Deps)`、`Execute(ctx, in)`)
- [ ] handler のテストで、usecase を `Interactor` の偽実装(関数型)に差し替えたテストが 1 つ以上ある
- [ ] `Validated` ラッパーのテスト(`Validate` が失敗したら `next` が呼ばれない)
- [ ] `docs/architecture.md` に「usecase の形」を 5 行で追記

**ヒント**: 形を揃えるのは、問題 62 で Wire に登録するときと問題 54 で gRPC から呼ぶときに効く。

---

## 29. CQRS: Query の分離

**主題**: 書き込みモデルと読み取りモデルの分離、読み取り専用 port

**実装**

- `usecase/port/task_query.go` に読み取り専用 port を作る。Repository とは別の interface。
  ```go
  type TaskListItem struct { ID, Title, Status, Priority string; Tags []string; SubtaskTotal, SubtaskDone int; CreatedAt, UpdatedAt time.Time }
  type TaskSummary struct { Total int; ByStatus map[string]int; Overdue int }
  type TaskQuery interface {
      List(ctx, opts ListOptions) (Page[TaskListItem], error)
      Summary(ctx) (TaskSummary, error)
  }
  ```
- `usecase/query/list_tasks.go`、`usecase/query/task_summary.go` を Interactor として作る。`TaskRepository.List` は削除する(一覧は Query の責務)。
- `adapter/gateway/memory/task_query.go` にメモリ実装を作る(Repository と同じ map を共有する構造にする。`memory.Store` を作り、Repository と Query がそれぞれ参照する)。
- `GET /tasks` は Query usecase を使うように変更し、`GET /tasks/summary` を追加する。

**完了条件**

- [ ] `port.TaskRepository` に `List` が無い
- [ ] `TaskListItem` は domain の `Task` を import していない(読み取りモデルは独立した平坦な構造体)
- [ ] `Summary` の `ByStatus` が 4 状態すべてのキーを持つ(0 件でも `0`)
- [ ] `docs/architecture.md` に「Repository は集約単位の読み書き、Query は画面・API 単位の読み取り」を追記

**ヒント**: 読み取りモデルは JOIN 済み・集計済みの形で返す。domain に変換し直さない。

---

## 30. PostgreSQL と migration

**主題**: docker compose、goose、テーブル設計、`make migrate`

**実装**

- `make db-up` で PostgreSQL を起動する。`make psql` で接続できることを確認する。
- `make migrate-create name=create_tasks` で `migrations/YYYYMMDDhhmmss_create_tasks.sql` を作り、次を書く(`-- +goose Up` / `-- +goose Down` の両方)。
  ```sql
  CREATE TABLE tasks (
    id          TEXT PRIMARY KEY,
    title       TEXT NOT NULL CHECK (char_length(title) BETWEEN 1 AND 200),
    description TEXT NOT NULL DEFAULT '',
    status      TEXT NOT NULL CHECK (status IN ('todo','doing','done','canceled')),
    priority    SMALLINT NOT NULL DEFAULT 1,
    version     INTEGER NOT NULL DEFAULT 1,
    created_at  TIMESTAMPTZ NOT NULL,
    updated_at  TIMESTAMPTZ NOT NULL
  );
  CREATE INDEX tasks_created_at_id_idx ON tasks (created_at, id);
  CREATE TABLE subtasks (
    id TEXT PRIMARY KEY, task_id TEXT NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    title TEXT NOT NULL, done BOOLEAN NOT NULL DEFAULT FALSE, position INTEGER NOT NULL
  );
  ```
  `tags` はこの問題では配列型 `TEXT[]` で `tasks` に持つ(問題 38 で正規化する)。
- `make migrate` と `make migrate-test` を実行し、`make migrate-status` で確認する。
- `docs/db.md` に「なぜ `TEXT` の ID か」「なぜ `TIMESTAMPTZ` か」「CHECK 制約を domain の検証と二重に持つ理由」を書く。

**完了条件**

- [ ] `make migrate` → `make migrate-down` → `make migrate` が往復できる
- [ ] `make psql` で `\d tasks` を確認した内容が `docs/db.md` にある
- [ ] `.env.example` に `DATABASE_URL` と `TEST_DATABASE_URL` が揃っている

**ヒント**: マイグレーションは一度適用したファイルを編集しない。直したいときは新しいファイルを足す。

---

## 31. DB 接続・ヘルスチェック

**主題**: `pgx/v5`、接続プール、`/healthz` と `/readyz` の違い

**実装**

- `go get github.com/jackc/pgx/v5` を導入し、`adapter/gateway/db/conn.go` に `Open(ctx, url string, opts Options) (*pgxpool.Pool, error)` を作る。`MaxConns`、`MinConns`、`MaxConnLifetime`、`HealthCheckPeriod` を `Options` から設定する。`Open` の中で 1 回 `Ping` する(3 秒タイムアウト)。
- `app/config.go` に `DatabaseURL`(必須。`String()` ではパスワード部分をマスクする)と接続プール設定を追加する。
- `gateway/http` に `GET /healthz`(プロセスが生きていれば常に 200)と `GET /readyz`(DB へ 1 秒タイムアウトで `Ping`、失敗は 503)を追加する。`readyz` の依存は `interface{ Ping(ctx) error }` として受け取る。
- `main` で `Pool` を作り、shutdown 時に `Close` する(順序: HTTP Shutdown → Pool Close)。

**完了条件**

- [ ] `Config.String()` の出力に DB パスワードが含まれないテスト
- [ ] `readyz` のテストで `Ping` を失敗させると 503、成功で 200(`Pinger` を偽実装で差し替え)
- [ ] `DATABASE_URL` が不正なら起動時にエラー終了する(遅延接続にしない)
- [ ] `make run` で起動し `curl localhost:8080/readyz` が 200

**ヒント**: `healthz` は「再起動すべきか」、`readyz` は「トラフィックを流してよいか」。役割を doc コメントに書く。

---

## 32. DB テスト基盤

**主題**: DB を使うテストの独立性、トランザクション巻き戻し、skip の作法

**実装**

- `testutil/dbtest/dbtest.go` に次を作る。
  ```go
  // TEST_DATABASE_URL が無ければ t.Skip。あれば接続し、テスト終了時に閉じる。
  func Pool(t *testing.T) *pgxpool.Pool
  // 各テスト用のトランザクションを開始し、t.Cleanup で必ず Rollback する。
  func Tx(t *testing.T) pgx.Tx
  // マイグレーションが適用済みか確認し、未適用なら分かりやすいメッセージで Fatal。
  ```
- `Pool` はパッケージ内で 1 つを `sync.Once` で共有し、`TestMain` を使わずに済む形にする。
- テストは `Tx` 内で実行し、コミットしない。これにより `t.Parallel()` を付けた複数テストが同じテーブルを使っても衝突しない(ただし行ロックの衝突には注意。ID を `id.Sequence` ではなく ULID にする)。
- `db` パッケージの Repository は `pgx.Tx` と `*pgxpool.Pool` の両方を受け取れるよう、共通の `Querier` interface(`Exec`、`Query`、`QueryRow`)に依存させる。

**完了条件**

- [ ] `make test`(URL 無し)で DB テストが `SKIP` と表示される
- [ ] `make test-db` で実行され、実行後のテーブルに行が残らない(手動で `SELECT count(*) FROM tasks` を確認)
- [ ] `t.Parallel()` を付けた 2 つの DB テストが同時に通る
- [ ] マイグレーション未適用時のメッセージに `make migrate-test` が含まれる

**ヒント**: `Querier` interface を `db` パッケージ内に置くと、後で Tx を context 経由で渡す設計(問題 36)へ移行しやすい。

---

## 33. TaskRepository の Postgres 実装

**主題**: プレースホルダ、`Scan`、`pgx.ErrNoRows` の変換、集約(Task + Subtasks)の保存 **TDD 必須(DB テスト)**

**実装**

- `adapter/gateway/db/task_repository.go` に `port.TaskRepository` を実装する。
  - `Save`: `INSERT ... ON CONFLICT (id) DO UPDATE SET ... WHERE tasks.version = $n` の形で upsert し、`version` を `+1` する。`RowsAffected() == 0` なら `KindConflict`。Subtasks は「全削除 → 全挿入」で保存する(この段階では単純さを優先。理由をコメントに書く)。
  - `FindByID`: `tasks` と `subtasks` を 2 クエリで取得し、domain の `Task` を組み立てる。`pgx.ErrNoRows` → `KindNotFound`。
  - `Delete`: `DELETE ... RETURNING id`、0 行なら `KindNotFound`。
- domain の `Task` を DB から復元するために、`task.Reconstruct(params)` のような「検証を通さず状態を復元する」関数を domain に追加する。doc コメントで「Repository 専用」と明記する。
- SQL 文は `const` で定義する。文字列連結は禁止。
- domain のエラーと `pgconn.PgError`(CHECK 制約違反 `23514`、一意制約 `23505`)を `apperr` へ変換する `mapPgError(err) error` を `db/errors.go` に作る。

**完了条件**

- [ ] `Save` → `FindByID` で全フィールド(Tags、Subtasks、Priority、Version)が往復する DB テスト
- [ ] 古い `Version` で `Save` すると `KindConflict`
- [ ] `rows.Close()` と `rows.Err()` を確認している(`defer rows.Close()` と、ループ後の `rows.Err()`)
- [ ] `title` に 201 文字を無理やり入れる(`Reconstruct` 経由)と CHECK 制約で `KindInvalid` になる
- [ ] `main` で `DATABASE_URL` があれば Postgres 実装、無ければメモリ実装を選ぶ

**ヒント**: `pgx.CollectRows` と `pgx.RowToStructByName` を使うと Scan を減らせるが、まずは手で `Scan` して列と構造体の対応を体で覚える。

---

## 34. SQL でのキーセットページネーション

**主題**: 動的 WHERE の安全な組み立て、`(created_at, id) > ($1, $2)` の行比較、インデックス **TDD 必須(DB テスト)**

**実装**

- `adapter/gateway/db/task_query.go` に `port.TaskQuery.List` を実装する。
  - フィルタ(`status`、`tag`)の有無で WHERE 句が変わる。条件と引数を `[]string` / `[]any` に積み、`$1, $2...` を番号で組み立てる小さなビルダー `db/sqlbuilder.go` を自作する(外部ライブラリ不可)。ユーザー入力は必ず引数側に入れる。
  - カーソルは `WHERE (created_at, id) > ($k, $k+1)` の行比較。
  - `ORDER BY created_at, id LIMIT $n`(`Limit+1` 件取得)。
  - Subtask の件数は `LEFT JOIN LATERAL` か相関サブクエリで `subtask_total`、`subtask_done` として同じクエリで返す。
- タグのフィルタは `$1 = ANY(tags)`。

**完了条件**

- [ ] 25 件を `limit=10` でたどるテスト(問題 15 と同じシナリオ)が DB で通る
- [ ] 同一 `created_at` の行を 3 件作り、ページ境界で重複・欠落しない
- [ ] `status` フィルタに `"todo' OR '1'='1"` を渡しても 0 件(SQL として解釈されない)
- [ ] `EXPLAIN` でクエリが `tasks_created_at_id_idx` を使っていることを確認し、実行計画を `docs/db.md` に貼る

**ヒント**: `sqlbuilder` は `Where(cond string, args ...any)` と `Build() (string, []any)` だけあればよい。汎用 ORM を作らない。

---

## 35. SQL での楽観ロック

**主題**: `UPDATE ... WHERE version = $n`、`RowsAffected`、競合の再現テスト

**実装**

- 問題 33 の `Save` を、新規と更新で分けて実装し直す。`Insert` はそのまま、`Update` は `UPDATE tasks SET ..., version = version + 1, updated_at = $x WHERE id = $y AND version = $z`。`Save` は `Task.Version == 0`(または新規フラグ)で分岐するのではなく、`INSERT ... ON CONFLICT DO NOTHING` の結果と `UPDATE` の結果を組み合わせる方式でも構わない。どちらにしたかと理由を書く。
- 競合テスト: 同じ Task を 2 回 `FindByID` し、片方を `Save` した後にもう片方を `Save` すると `KindConflict` になる。
- 同時実行テスト: 10 goroutine が同じ Task に対して `FindByID` → `Rename` → `Save` を行い、成功が 1 回以上・失敗がすべて `KindConflict` であることを確認する(このテストは `dbtest.Tx` ではなく `Pool` を使い、テスト用の行を作って最後に削除する)。

**完了条件**

- [ ] 上記 2 つのテストが `make test-db` で通る
- [ ] `RowsAffected` の判定漏れがない(`UPDATE` の戻り値を捨てていない)
- [ ] usecase レベルで「競合したら 1 回だけ再読込してリトライする」機能は**入れない**(問題文にない)

**ヒント**: 楽観ロックは「後勝ちを防ぐ」ための仕組み。どの操作で競合を許容するかは仕様の問題。

---

## 36. トランザクション抽象(UnitOfWork)

**主題**: トランザクション境界を usecase に置く、context 経由の Tx 伝搬 **TDD 必須**

**実装**

- `usecase/port/transactor.go` に `type Transactor interface { WithinTx(ctx context.Context, fn func(ctx context.Context) error) error }` を定義する。
- `adapter/gateway/db/tx.go` に実装する。`Begin` → `fn(ctxWithTx)` → 成功で `Commit`、エラーまたは panic で `Rollback`。Tx は context の非公開キーに載せ、`db` パッケージ内の `querierFrom(ctx) Querier` で「Tx があれば Tx、無ければ Pool」を返す。Repository / Query はすべて `querierFrom(ctx)` を使う。
- ネストした `WithinTx` は新しい Tx を開かず、既存の Tx に参加する。
- メモリ実装 `memory.Transactor` は `fn(ctx)` を呼ぶだけ(ロールバックはできない旨をコメント)。
- `Deps` に `Tx port.Transactor` を追加し、`Create` などの書き込み usecase を `WithinTx` で包む。

**完了条件**

- [ ] `fn` がエラーを返すと `INSERT` した行が残らない DB テスト
- [ ] `fn` が panic しても Rollback され、panic は再送出される
- [ ] ネストで新しい Tx が開かれない(`pgx` の Tx が 1 つであることを確認するか、`BeginTx` の呼び出し回数をカウント)
- [ ] `dbtest.Tx` と `db.Transactor` の関係を整理する: テストでは `dbtest` が開いた Tx を context に載せ、`WithinTx` はそれに参加する(コミットしない)

**ヒント**: 「Tx を引数で渡す」設計もあるが、port のシグネチャが DB に汚染される。context 経由にする理由を `tx.go` の doc コメントに書く。

---

## 37. 複数テーブルにまたがる更新

**主題**: 原子性、部分失敗の再現、ドメインルールとトランザクション

**実装**

- `subtasks` の保存を「全削除 → 全挿入」から差分更新に変える。`FindByID` で読み込んだ Subtask の ID 集合と保存時の集合を比べ、`DELETE ... WHERE task_id = $1 AND id <> ALL($2)`、`INSERT ... ON CONFLICT (id) DO UPDATE` を組み合わせる。
- `usecase/task/complete.go` を `WithinTx` の中で「親の `Complete`(Subtask が未完了なら失敗)→ `Save` → イベント `Publish`」の順にする。
- 部分失敗テスト: `Publisher` のモックがエラーを返すと、Task の状態更新もロールバックされている(`FindByID` で `doing` のまま)。
- 逆に「Publish は Tx の外で行うべきか」を検討し、この問題では Tx 内で行い、問題 45 で Outbox に置き換えることを `complete.go` のコメントに書く。

**完了条件**

- [ ] 差分更新後も問題 33 の往復テストが通る
- [ ] `Publish` 失敗でロールバックされる DB テスト
- [ ] Subtask を 3 つ持つ Task で 1 つを削除して保存すると、`DELETE` されるのは 1 行だけ(クエリの引数をログで確認するか、行数で検証)

**ヒント**: 集約は「1 トランザクションで整合性を保つ単位」。Task と Subtask が同じトランザクションに乗るのはそのため。

---

## 38. Tags テーブルと N+1 の回避

**主題**: 正規化、`= ANY($1)` によるバッチ取得、クエリ回数のテスト **TDD 必須(DB テスト)**

**実装**

- マイグレーション: `tags(id, name UNIQUE)` と `task_tags(task_id, tag_id, PRIMARY KEY(task_id, tag_id))` を作り、`tasks.tags` 列から移行する(`INSERT ... SELECT unnest(tags)`)。移行後に列を削除する(`Down` で戻す)。
- `Save` はタグを `INSERT INTO tags ... ON CONFLICT (name) DO NOTHING` → `task_tags` を差分更新。
- `List` は Task 一覧を取ったあと、`SELECT task_id, name FROM task_tags JOIN tags ... WHERE task_id = ANY($1)` の 1 クエリで全件分のタグを取り、Go 側で `map[taskID][]string` に分配する。ループ内でクエリを発行しない。
- `adapter/gateway/db/tracer.go` に `pgx` の `QueryTracer` を実装し、テストで実行クエリ数を数えられるようにする(`dbtest.CountQueries(t, pool)` のような形)。

**完了条件**

- [ ] 30 件の Task を一覧するときのクエリ数が固定(例: 2 回)であることをテストで検証している
- [ ] タグのフィルタ(`tag=go`)が `EXISTS (SELECT 1 FROM task_tags ...)` で書けており、インデックスを使う
- [ ] マイグレーションの `Down` でデータが失われない(往復テストを手動で行い `docs/db.md` に記録)

**ヒント**: N+1 は「1 件ごとにクエリを投げる」こと。まず数え、次に減らす。

---

## 39. Query の Postgres 実装(読み取りモデル)

**主題**: 集計クエリ、`GROUP BY`、読み取りモデルへの直接マッピング

**実装**

- `port.TaskQuery.Summary` を実装する。`SELECT status, count(*) FROM tasks GROUP BY status` と、`due_at < now() AND status NOT IN ('done','canceled')` の件数を返す。`due_at`(`TIMESTAMPTZ NULL`)列をマイグレーションで追加し、domain の `Task` にも `DueAt *time.Time` を追加する(`WithDueAt` オプション、過去日付は `ErrInvalidDueAt`)。
- `Summary` の「現在時刻」は SQL の `now()` ではなく、引数で渡す(`clock.Clock` から取得して `$1` にバインドする)。理由: テストの決定性と、後でテナント別の締め時刻を扱うため。
- `TaskListItem` に `DueAt *time.Time` と `Overdue bool` を追加する。
- 書き込み側(`Repository`)と読み取り側(`Query`)の整合性テスト: `Save` した内容が `List` の `TaskListItem` に正しく現れる。

**完了条件**

- [ ] `Summary` のテストで `now` を固定して `Overdue` の境界(同時刻は overdue でない)を検証している
- [ ] `ByStatus` に 4 状態すべてのキーがある(SQL の結果に無い状態も Go 側で 0 で埋める)
- [ ] `TaskListItem` を domain の `Task` 経由で組み立てていない(`Scan` から直接)

**ヒント**: 読み取りモデルは「API が返したい形」を先に決め、その形に合う SQL を書く。

---

## 40. Project とテナント分離

**主題**: マルチテナント、context によるテナント伝搬、すべてのクエリに `tenant_id` **TDD 必須**

**実装**

- `domain/project` に `Project{ID, TenantID, Name, Key(例: "PROJ"), CreatedAt}` を作る。`Key` は 2〜10 文字の英大文字。
- `domain/task.Task` に `TenantID` と `ProjectID` を追加する。マイグレーションで `tenants`、`projects` テーブルと、`tasks.tenant_id`、`tasks.project_id`(NOT NULL、FK)を追加する。既存行はダミーテナント `t_default` に寄せる。
- `pkg/tenant` に `WithTenant(ctx, id) context.Context` と `FromContext(ctx) (string, bool)` を作る。
- **すべての** Repository / Query メソッドは `tenant.FromContext` でテナントを取得し、無ければ `apperr.KindUnauthorized` を返す。すべての SQL に `AND tenant_id = $n` を付ける。`FindByID` も `WHERE id = $1 AND tenant_id = $2`。
- `usecase/project` に `Create`、`Get`、`List` を作る。
- HTTP は暫定で `X-Tenant-ID` ヘッダから context に載せるミドルウェアを作る(問題 41 で認証に置き換える)。エンドポイントは `/projects` と `/projects/{projectID}/tasks` に変更する。
- `memory` 実装もテナントで分離する。

**完了条件**

- [ ] テナント A で作成した Task をテナント B の context で `FindByID` すると `KindNotFound`(`KindForbidden` ではない。存在自体を知らせない)
- [ ] テナント無しの context で Repository を呼ぶと `KindUnauthorized`
- [ ] `rg "FROM tasks" adapter/gateway/db` の全ヒットに `tenant_id` 条件が付いている(目視 + `arch_test` に「`tasks` を参照する SQL 定数は `tenant_id` を含む」という文字列検査を追加)
- [ ] `docs/architecture.md` に「テナントは handler で解決し、context で運び、adapter で必ず条件に入れる」を追記

**ヒント**: テナント条件の付け忘れは最も危険なバグ。機械的に検査できる仕組みを必ず入れる。

---

## 41. API キー認証と Principal

**主題**: 認証ミドルウェア、ハッシュ保存、`Principal` の型設計

**実装**

- マイグレーション: `api_keys(id, tenant_id, name, key_hash BYTEA UNIQUE, created_at, expires_at NULL, revoked_at NULL)`。
- `pkg/auth` に `Principal{TenantID, SubjectID, Kind(api_key|user), Role}` と context ヘルパー。
- `port.APIKeyStore` に `FindByHash(ctx, hash []byte) (*APIKey, error)` と、DB 実装。キーの平文は保存せず SHA-256 のハッシュを保存する。
- ミドルウェア `Authenticate(store)`: `Authorization: Bearer <key>` を取り出し、ハッシュで検索。見つからない・失効・期限切れは 401(`WWW-Authenticate: Bearer` を付ける)。成功時は `Principal` と `tenant` を context に載せる。`X-Tenant-ID` ミドルウェアは削除する。
- `cmd/server` とは別に `cmd/apikey` を作り、`go run ./cmd/apikey create --tenant t_1 --name dev` で平文キーを 1 回だけ表示し、ハッシュを DB に保存する。
- `/healthz`、`/readyz` は認証不要。

**完了条件**

- [ ] 平文キーが DB・ログ・エラーメッセージに現れない
- [ ] `expires_at` を過去にしたキーで 401
- [ ] `FindByHash` はテナント条件無しで検索してよい唯一の場所であることをコメントで明記し、`arch_test` の例外リストに登録
- [ ] `curl -H "Authorization: Bearer <key>" localhost:8080/projects` が通る手順を `docs/setup.md` に追記

**ヒント**: 認証(誰か)と認可(何ができるか)を混ぜない。この問題は認証だけ。

---

## 42. ロールベース認可

**主題**: 認可ルールの置き場所、`Forbidden` と `NotFound` の使い分け **TDD 必須**

**実装**

- `domain/member` に `Role`(`owner`、`member`、`viewer`)と `Member{ID, TenantID, UserID, Role}` を作る。`api_keys` にも `role` 列を追加する。
- `domain/authz`(または `domain/member/policy.go`)に `type Action string` と `func Can(role Role, action Action) bool` を定義する。表:

  | Action | owner | member | viewer |
  | ------ | ----- | ------ | ------ |
  | task.read | ○ | ○ | ○ |
  | task.write | ○ | ○ | × |
  | task.delete | ○ | × | × |
  | project.write | ○ | × | × |

- 各 usecase の `Execute` 冒頭で `auth.PrincipalFrom(ctx)` を取り、`authz.Can` を確認して `KindForbidden`。handler には書かない。
- `viewer` が `DELETE /tasks/{id}` すると 403(存在する場合)。存在しない場合は 404 より 403 を優先するか決めてテストに書く(推奨: 認可チェックを先に行い 403)。

**完了条件**

- [ ] 表の 12 セルがテーブル駆動テストで網羅されている
- [ ] usecase テストで `viewer` の `Principal` を context に載せて `KindForbidden` を確認している
- [ ] `gateway/http` に `Role` の判定が無い
- [ ] `Principal` が無い context で usecase を呼ぶと `KindUnauthorized`

**ヒント**: 認可判定を usecase に置く理由は、gRPC(問題 52)からも同じルールが適用されるようにするため。

---

## 43. Webhook 送信の本格化(署名・冪等キー)

**主題**: HMAC 署名、`Idempotency-Key`、エラーの分類(再試行可 / 不可)**TDD 必須**

**実装**

- 問題 24 の `webhook.Client` を拡張する。
  - 本文の HMAC-SHA256 を `X-Signature: sha256=<hex>` に付ける。鍵は `Config.WebhookSecret`(必須ではないが、URL があるなら必須)。
  - `Idempotency-Key: <event.ID>` ヘッダを付ける(イベントに `ID` を持たせる。`event.Base` に追加)。
  - `X-Timestamp` を付け、署名対象を `timestamp + "." + body` にする(リプレイ対策)。
- エラー分類: `adapter/gateway/webhook/errors.go` に `type DeliveryError struct { StatusCode int; Retryable bool; Err error }`。5xx・タイムアウト・接続エラーは `Retryable: true`、4xx は `false`(429 だけ `true`)。
- テスト用に「受信側」の検証関数 `webhook.Verify(secret, timestamp, body, signature) bool` を作り、`httptest.NewServer` 側で署名を検証する。`hmac.Equal` を使う。

**完了条件**

- [ ] 署名検証のテスト(正しい鍵で true、1 バイト違いで false、`hmac.Equal` 使用)
- [ ] ステータス別 `Retryable` のテーブル駆動テスト(200、400、404、429、500、503、タイムアウト)
- [ ] 同じイベントを 2 回送っても `Idempotency-Key` が同じ
- [ ] `Config.String()` に `WebhookSecret` が出ない

**ヒント**: 「誰が送ったか(署名)」「同じものを何度受けても一度だけ処理(冪等キー)」「古いものを再送されない(タイムスタンプ)」の 3 点セット。

---

## 44. 外部 HTTP のレジリエンス(リトライ・サーキットブレーカ)

**主題**: 指数バックオフとジッタ、リトライ上限、サーキットブレーカ、`FakeClock` による時間のテスト **TDD 必須**

**実装**

- `pkg/retry` に `Do(ctx, policy Policy, fn func(ctx) error) error` を作る。`Policy{MaxAttempts, BaseDelay, MaxDelay, Jitter}`、遅延は `min(MaxDelay, BaseDelay * 2^n) ± Jitter`。`fn` のエラーが `interface{ Retryable() bool }` を満たし `false` なら即座に諦める。待ち時間は `clock.Clock` 由来の `Sleep(ctx, d)` を通す(`Fake` では即時進行)。
- `pkg/breaker` に簡易サーキットブレーカを作る。状態 `closed → open(連続 N 回失敗)→ half-open(OpenDuration 経過後 1 回だけ試す)→ closed`。`open` 中の呼び出しは `ErrOpen` を即返す。`sync.Mutex` で守る。外部ライブラリ不可。
- `webhook.Client` に `retry` と `breaker` を組み込む。`Publish` は「breaker.Do(retry.Do(送信))」の順。
- `Retryable: false` のエラー(4xx)はリトライせず、breaker の失敗にも数えない(仕様として決める)。

**完了条件**

- [ ] リトライのテスト: 2 回失敗して 3 回目成功、`MaxAttempts=3` で 3 回失敗すると最後のエラーが返る、4xx で 1 回しか呼ばれない
- [ ] バックオフ間隔のテスト: `Fake` Clock で `Sleep` に渡された `d` の列を記録し、上限とジッタ範囲を検証
- [ ] breaker のテスト: 連続失敗で open、`OpenDuration` 経過で half-open、成功で closed に戻る。50 goroutine から叩いて race が無い
- [ ] キャンセル済み context でリトライ待ちが即座に終わる

**ヒント**: リトライは「相手が回復する前提」、ブレーカは「相手が回復するまで叩かない前提」。両方あって初めて連鎖障害を防げる。

---

## 45. Outbox パターン

**主題**: イベント発行の原子性、「DB に書く」と「外へ送る」の分離 **TDD 必須(DB テスト)**

**実装**

- マイグレーション: `outbox(id TEXT PK, tenant_id, event_name, aggregate_id, payload JSONB, occurred_at, created_at, sent_at NULL, attempts INT DEFAULT 0, last_error TEXT NULL)`。`sent_at IS NULL` に部分インデックス。
- `adapter/gateway/db/outbox.go` に `port.EventPublisher` を実装する。`Publish` は Tx 内で `outbox` に INSERT するだけ(外部通信をしない)。イベントの JSON 化は `event` パッケージに `Marshal(e Event) ([]byte, error)` として置く。
- usecase は `Publisher` として Outbox 実装を使う(`main` / 後の Wire で差し替え)。webhook.Client は usecase から直接呼ばれなくなる(問題 70 でリレーが呼ぶ)。
- `port.OutboxStore` に `FetchUnsent(ctx, limit) ([]OutboxRow, error)`、`MarkSent(ctx, id)`、`MarkFailed(ctx, id, err)` を作る(リレーは問題 70)。

**完了条件**

- [ ] `Create` usecase の Tx がロールバックされると `outbox` にも行が残らない DB テスト
- [ ] `outbox.payload` から元のイベントを復元できる(`Unmarshal` のラウンドトリップテスト)
- [ ] `FetchUnsent` は `tenant_id` 条件を**付けない**(システム横断の処理)。`arch_test` の例外に登録し、理由をコメント
- [ ] `docs/architecture.md` に Outbox の図(書き込み Tx → outbox → リレー → 外部)を追記

**ヒント**: 「DB へのコミットと外部送信を同時に成功させる」ことは不可能。だから「DB に書くことだけを約束し、送信は後で必ず行う」。

---

## 46. 集約ルートとイベントの永続化

**主題**: 集約の境界、イベントの一貫した扱い、usecase の共通化

**実装**

- すべての書き込み usecase(Create、Update、Start/Complete/Cancel/Reopen、Delete、Subtask 系、Project 系)を「`WithinTx` 内で `Save` → `PullEvents` → `Publish`」に統一する。重複を `usecase/task/save.go` の `saveAndPublish(ctx, t *task.Task) error` にまとめる。
- `Project` にもイベント(`ProjectCreated`)を持たせ、`event.Recorder` 構造体(`events []Event`、`Record(e)`、`PullEvents()`)を作って `Task` と `Project` に埋め込む。
- イベントに `TenantID` を含める(Outbox の `tenant_id` 列に入れるため)。

**完了条件**

- [ ] `rg "PullEvents" usecase/` のヒットが `saveAndPublish` 系の 1〜2 箇所だけ
- [ ] `event.Recorder` のテスト(`Record` → `PullEvents` → 空)
- [ ] すべての書き込み usecase のテストで「`Publish` が Tx 内で呼ばれる」ことをモックの `InOrder` で検証している
- [ ] domain の各集約にイベント一覧の表が doc コメントにある

**ヒント**: 「集約ルート以外からイベントを発行しない」。Subtask 完了のイベントも `Task` が記録する。

---

## 47. コメントとソフトデリート

**主題**: 子エンティティの別テーブル、`deleted_at`、削除済みの除外を漏らさない **TDD 必須(DB テスト)**

**実装**

- マイグレーション: `task_comments(id, tenant_id, task_id, author_id, body TEXT CHECK(char_length(body) <= 4000), created_at, updated_at)`。`tasks.deleted_at TIMESTAMPTZ NULL` を追加し、部分インデックス `WHERE deleted_at IS NULL`。
- コメントは Task 集約の外(別の Repository: `port.CommentRepository`)にする。理由: コメントは数が多く、Task の `Version` を上げたくない。doc コメントに書く。
- `Delete` usecase を物理削除からソフトデリートに変更する。`FindByID`、`List`、`Summary`、コメント一覧など**すべての読み取り**で `deleted_at IS NULL` を条件に入れる。
- `POST /projects/{pid}/tasks/{id}/comments`、`GET .../comments?cursor=`(キーセット)、`DELETE .../comments/{cid}`(作者本人か owner のみ)。
- 削除済み Task へのコメント投稿は 404。

**完了条件**

- [ ] ソフトデリート後に `GET` 404、`List` 非表示、`Summary` の件数に含まれない(3 つとも DB テスト)
- [ ] `arch_test` に「`FROM tasks` を含む SQL 定数は `deleted_at IS NULL` を含む」を追加(例外は `Delete` 自身と問題 95 の移行クエリ)
- [ ] 作者でも owner でもない `member` がコメントを削除すると 403
- [ ] コメント本文がログに出ない

**ヒント**: ソフトデリートの最大のリスクは「除外条件の付け忘れ」。テナント条件と同じく、機械的に検査する。

---

## 48. 検索(pg_trgm)

**主題**: 部分一致検索、LIKE のエスケープ、拡張とインデックス

**実装**

- マイグレーション: `CREATE EXTENSION IF NOT EXISTS pg_trgm;` と `CREATE INDEX tasks_title_trgm_idx ON tasks USING gin (title gin_trgm_ops);`。
- `ListFilter` に `Query *string` を追加し、`title ILIKE '%' || $n || '%'` で検索する。`%`、`_`、`\` を入力側でエスケープする関数 `db.EscapeLike(s string) string` を作る(`ESCAPE '\'` を明示)。
- `q` は 100 文字以内、前後空白除去、空なら無視(usecase の検証)。
- `GET .../tasks?q=` に対応する。

**完了条件**

- [ ] `q=100%` で「100%」を含むタイトルだけがヒットし、全件ヒットしない
- [ ] `q=a_b` で `a_b` を含むものだけ(`axb` がヒットしない)
- [ ] `EXPLAIN` で GIN インデックスが使われる(件数が少ないと seq scan になるので、`SET enable_seqscan = off` で確認した旨を `docs/db.md` に記録)
- [ ] `q` に 101 文字で 400

**ヒント**: 全文検索(`tsvector`)は形態素の問題があるので、日本語タイトルには trigram が扱いやすい。

---

## 49. HTTP→DB の統合テスト

**主題**: 実 DB を使ったエンドツーエンドのシナリオテスト、テストデータの独立、golden

**実装**

- `gateway/http/integration_test.go`(`dbtest` を使い、URL 無しなら skip)に、Postgres 実装で組み立てた `Router` に対するシナリオテストを書く。
  1. API キーを作成(テスト用ヘルパーで直接 INSERT)
  2. `POST /projects` → `POST /projects/{pid}/tasks` → `PATCH` → `POST .../start` → `GET /tasks?status=doing` → `POST .../complete` → `GET /summary`
  3. 別テナントのキーで同じ Task を `GET` すると 404
- テストごとに専用テナントを ULID で作り、`t.Parallel()` で並列に走らせても衝突しない。`dbtest.Tx` は使わず、`t.Cleanup` でテナント配下の行を削除する(削除順序を FK に合わせる)。
- レスポンスは `id`、`created_at` などの可変値をマスクしてから golden と比較する(`testutil.MaskJSON(b, "id", "created_at", ...)`)。

**完了条件**

- [ ] シナリオテストが `make test-db` で通り、`-count=3` でフレークがない
- [ ] `Cleanup` 後にテナントの行が残らない
- [ ] golden ファイルに ULID や時刻が含まれていない
- [ ] このテストが「usecase・adapter のテストを置き換えない」理由を doc コメントに書く(遅い・原因の切り分けが難しい)

**ヒント**: 統合テストは少数精鋭。1 ユースケースに 1 本の「幸せな経路」+ 権限境界だけで十分。

---

## 50. チェックポイント: タスク割り当て

**主題**: 中級の内容を縦に 1 本。受け入れ条件だけで実装する。

**仕様**

- `members(id, tenant_id, user_id, display_name, role, created_at)` を追加し、`cmd/apikey` を `cmd/admin` に改名して `member add` サブコマンドも持たせる。
- Task に `AssigneeID *string` を追加。`POST .../tasks/{id}/assign {"member_id": "...", "version": n}`、`POST .../tasks/{id}/unassign`。
- **ルール**
  - 同じテナントの Member にのみ割り当てられる(別テナントなら 404、存在確認は `port.MemberRepository`)
  - `viewer` ロールの Member には割り当てられない(`ErrCannotAssignViewer` → 409)
  - 割り当て操作は `task.write` 権限が必要
  - closed な Task には割り当てられない
- イベント `TaskAssigned{AssigneeID}`、`TaskUnassigned` を Outbox に書く。
- 読み取り: `GET /me/tasks?status=`(自分に割り当てられた Task のキーセットページネーション)。`TaskListItem` に `assignee` を含める(JOIN で `display_name` を出す)。
- `Summary` に `by_assignee: [{member_id, display_name, count}]` を追加する。

**完了条件**

- [ ] domain: ルール 4 つのテーブル駆動テスト
- [ ] usecase: モックで `MemberRepository` と `TaskRepository` の呼び出し順序と `Publish` を検証
- [ ] adapter: DB テストで別テナント 404、`GET /me/tasks` のページング、`Summary.by_assignee`
- [ ] handler: 403 / 404 / 409 / 200 のテスト
- [ ] 統合テスト(問題 49)にシナリオを 1 本追加
- [ ] `arch_test` が通る(新しい SQL にも `tenant_id`、`deleted_at IS NULL`)
- [ ] `make check` と `make test-db` が通る
- [ ] `docs/notes/50.md` に振り返り 3 点以上

**自己採点の目安**: 3 時間以内。`assign` のルールが handler や SQL に書かれていたら問題 42 を読み直す。`by_assignee` を Go 側で N+1 にしていたら問題 38 を読み直す。
