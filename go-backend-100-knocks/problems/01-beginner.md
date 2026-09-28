# 初級(1–25): Go文法、テスト、JSON、HTTP API、入力検証

> `docs/...` と書かれたパスはリポジトリルートの `docs/`(このファイルから見て `../../docs/`)を指します。`docs/notes/NN.md` などはそこへ作ってください。

## この範囲のゴール

- Go の構造体・メソッド・interface・エラー・`context` を「テストで裏付けながら」使える
- `net/http` だけで JSON API を作り、入力検証とエラー変換を handler の外に置ける
- 25 問終了時点で、インメモリ実装の Task API がサーバーとして起動し、Webhook を送れる

## 進め方の約束

- 各問題は前問のコードを育てます。新しいディレクトリへ作り直さないでください。
- **TDD 必須** と書かれた問題は、必ず失敗するテストを先に書いてから実装します。
- 完了条件の最後は常に `make check` が通ることです(各問題では省略しています)。
- 外部ライブラリは問題文で指定したものだけを `go get` してください。

## 25 問終了時点のディレクトリ

```
cmd/server/            main.go(HTTP サーバー、graceful shutdown)
app/                   config.go
domain/task/           task.go status.go tags.go priority.go subtask.go events.go
domain/event/          event.go
usecase/port/          task_repository.go event_publisher.go
usecase/task/          create.go get.go list.go update.go delete.go add_subtask.go
adapter/gateway/memory/ task_repository.go event_publisher.go
adapter/gateway/webhook/ client.go
gateway/http/          router.go handler_task.go dto.go errors.go middleware.go
pkg/apperr pkg/clock pkg/id pkg/validate pkg/log pkg/collection
testutil/              builder.go
```

---

## 01. Task 構造体と最初のテスト

**主題**: 構造体、メソッド、パッケージ、`go test`

**実装**

- `domain/task/task.go` に `Task` 構造体を定義する。フィールドは `ID string`、`Title string`、`Description string`、`Status Status`、`CreatedAt time.Time`、`UpdatedAt time.Time`。
- `domain/task/status.go` に `type Status string` と定数 `StatusTodo = "todo"`、`StatusDoing = "doing"`、`StatusDone = "done"`、`StatusCanceled = "canceled"` を定義する。
- `Status` に `String() string` と `IsValid() bool` を実装する。
- `Task` に `IsClosed() bool`(done または canceled で true)を実装する。
- `domain/task/task_test.go` にサブテストで次を書く。
  - `フィールド参照で全ての値が取得できることを確認`
  - `done と canceled のとき IsClosed が true になることを確認`(テーブル駆動)
  - `未定義の Status で IsValid が false になることを確認`

**完了条件**

- [ ] `go test ./domain/...` が通る
- [ ] テーブル駆動テストの各ケースに `t.Run` とシナリオ名がある
- [ ] `Status` の定数以外の文字列(例: `"unknown"`)で `IsValid()` が false になる
- [ ] `go doc ./domain/task` でパッケージコメントと `Task` の doc コメントが表示される

**ヒント**: `go doc` に出すにはパッケージ宣言の直前に `// Package task は ...` と書く。定数は `const ( ... )` のブロックで並べる。

---

## 02. コンストラクタと不変条件

**主題**: エラーを返す関数、センチネルエラー、テーブル駆動テスト **TDD 必須**

**実装**

- `NewTask(title, description string) (*Task, error)` を追加する。
  - `Title` は前後の空白を除去し、1 文字以上 200 文字以下(文字数は `utf8.RuneCountInString` で数える)。
  - `Description` は 2000 文字以下。
  - 初期 `Status` は `StatusTodo`。
- 違反時は `ErrEmptyTitle`、`ErrTitleTooLong`、`ErrDescriptionTooLong` のいずれかを返す。`errors.New` で `var` として定義し、`errors.Is` で判定できるようにする。
- `Task` のフィールドを直接書き換えずにコンストラクタを通す設計にするため、この問題以降、テスト以外では `task.Task{}` リテラルを使わない。

**完了条件**

- [ ] 境界値(0 文字、1 文字、200 文字、201 文字、全角 200 文字)がテーブル駆動テストに含まれる
- [ ] 各失敗ケースを `errors.Is` で検証している
- [ ] 前後の空白だけのタイトルが `ErrEmptyTitle` になる
- [ ] `ID`、`CreatedAt`、`UpdatedAt` はまだゼロ値のままでよい(問題 5・6 で埋める)

**ヒント**: `strings.TrimSpace`、`unicode/utf8`。エラー変数名は `Err` で始める(Go の慣習)。

---

## 03. ステータス遷移のルール

**主題**: メソッドで状態を守る、遷移表、`%w` によるラップ **TDD 必須**

**実装**

- `Task` に `Start()`、`Complete()`、`Cancel()`、`Reopen()` を実装する。許可する遷移は次のとおり。

  | 現在 | Start | Complete | Cancel | Reopen |
  | ---- | ----- | -------- | ------ | ------ |
  | todo | doing | ×        | canceled | ×    |
  | doing | ×    | done     | canceled | ×    |
  | done | ×     | ×        | ×      | todo   |
  | canceled | × | ×        | ×      | todo   |

- 不正な遷移は `ErrInvalidTransition` を返す。どの遷移で失敗したか分かるよう `fmt.Errorf("%w: %s -> %s", ErrInvalidTransition, from, to)` の形でラップする。
- 遷移表は `map[Status]map[Status]bool` などのデータとして持ち、各メソッドは共通の `transition(to Status) error` を呼ぶ。
- 成功時は `UpdatedAt` を更新する。ただし `time.Now()` はまだ呼ばない(問題 5 で Clock を注入する)。この問題では `UpdatedAt` の更新をスキップしてよい。

**完了条件**

- [ ] 4 状態 × 4 操作 = 16 ケースがテーブル駆動テストで網羅されている
- [ ] 失敗ケースで `errors.Is(err, ErrInvalidTransition)` が true、かつ `err.Error()` に遷移元と遷移先が含まれる
- [ ] 遷移に失敗したとき `Status` が変わっていない

**ヒント**: テーブルの各行は `{name, from, op, wantStatus, wantErr}` の形にすると読みやすい。`op` は `func(*Task) error` で持てる。

---

## 04. アプリケーションエラー(apperr)の設計

**主題**: エラーの種類分け、`errors.As`、レイヤーをまたぐエラーの表現

**実装**

- `pkg/apperr` パッケージを作る。
  ```go
  type Kind int
  const (
      KindUnknown Kind = iota
      KindInvalid       // 入力不正 → 400
      KindNotFound      // 見つからない → 404
      KindConflict      // 競合 → 409
      KindUnauthorized  // 未認証 → 401
      KindForbidden     // 権限なし → 403
      KindUnavailable   // 依存先障害 → 503
  )
  type Error struct {
      Kind Kind
      Msg  string           // クライアントに見せてよい短い説明
      Err  error            // 原因(内部用、クライアントには出さない)
  }
  ```
- `Error` に `Error() string`、`Unwrap() error` を実装する。
- ヘルパー `func New(kind Kind, msg string) *Error`、`func Wrap(kind Kind, msg string, err error) *Error`、`func KindOf(err error) Kind`(`errors.As` で取り出し、該当しなければ `KindUnknown`)、`func Is(err error, kind Kind) bool` を作る。
- `domain/task` のエラーをそのまま外へ出さず、後で usecase が `apperr.Wrap(apperr.KindInvalid, "invalid transition", err)` のように包む前提の設計にする。

**完了条件**

- [ ] `errors.Is(wrapped, task.ErrInvalidTransition)` が `apperr.Wrap` を通しても true になる(`Unwrap` の確認)
- [ ] `KindOf(nil)` と `KindOf(errors.New("x"))` が `KindUnknown`
- [ ] 多段ラップ(`fmt.Errorf("%w", apperr.Wrap(...))`)でも `KindOf` が正しい Kind を返す
- [ ] `Kind` に `String()` があり、テストで `KindNotFound.String() == "not_found"` を確認している

**ヒント**: `errors.As(err, &target)` の `target` は `*apperr.Error` 型の変数のポインタを渡す。

---

## 05. Clock の注入と時刻のテスト

**主題**: interface による外部依存の切り離し、決定的なテスト

**実装**

- `pkg/clock` に次を作る。
  ```go
  type Clock interface { Now() time.Time }
  type Real struct{}                       // time.Now().UTC() を返す
  type Fake struct{ mu sync.Mutex; t time.Time }  // 固定時刻。Set / Advance(d) を持つ
  ```
- `NewTask` のシグネチャを `NewTask(clk clock.Clock, title, description string)` に変え、`CreatedAt`・`UpdatedAt` を `clk.Now()` で埋める。
- 問題 3 の各遷移メソッドにも `clk clock.Clock` を渡し(`Start(clk)` など)、成功時のみ `UpdatedAt` を更新する。
- テストを更新する。固定時刻は `time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)` のように明示する。

**完了条件**

- [ ] `domain/` 配下に `time.Now()` の呼び出しが無い(`rg "time.Now" domain/` が 0 件)
- [ ] `Fake.Advance(1 * time.Hour)` 後に `Complete` すると `UpdatedAt` が 1 時間進むテストがある
- [ ] `Fake` は `t.Parallel()` を付けた複数テストから同時に使っても race detector が反応しない(`go test -race`)
- [ ] `Real.Now()` が UTC を返す(`Location() == time.UTC`)

**ヒント**: すべての時刻を UTC に統一しておくと、後で DB と JSON を扱うときに事故が減る。

---

## 06. ID 生成の抽象化

**主題**: 外部ライブラリの導入、interface による差し替え

**実装**

- `go get github.com/oklog/ulid/v2` を導入する。
- `pkg/id` に `type Generator interface { NewID() string }`、ULID を返す `ULID` 実装、テスト用に固定列を返す `Sequence` 実装(`NewSequence("id-1", "id-2")` の順で返し、尽きたら panic)を作る。
- `NewTask(clk clock.Clock, gen id.Generator, title, description string)` として `ID` を埋める。
- `ULID` 実装は `ulid.Make()` ではなく、`clk.Now()` と `crypto/rand` を使う `ulid.New(ulid.Timestamp(now), entropy)` で組み立てる。乱数源は `ulid.Monotonic` でラップし、並行呼び出しに備えて `sync.Mutex` で守る。

**完了条件**

- [ ] `ULID` を 1000 回呼んで重複がない(`map[string]struct{}` で確認)
- [ ] 100 goroutine から同時に `NewID()` を呼んでも race detector が反応しない
- [ ] 生成した ID が 26 文字で、`ulid.Parse` に成功する
- [ ] `Sequence` が尽きたときの panic を `t.Run` 内で `defer recover()` により検証している

**ヒント**: ULID は時刻順にソートできるので、問題 15 のカーソルページネーションで役に立つ。

---

## 07. インメモリ Repository と並行安全性

**主題**: port(interface)の定義、map と `sync.RWMutex`、race detector **TDD 必須**

**実装**

- `usecase/port/task_repository.go` に定義する。
  ```go
  type TaskRepository interface {
      Save(ctx context.Context, t *task.Task) error          // 新規・更新の両方
      FindByID(ctx context.Context, id string) (*task.Task, error)
      Delete(ctx context.Context, id string) error
      List(ctx context.Context) ([]*task.Task, error)       // 問題 15 で引数を増やす
  }
  ```
- `adapter/gateway/memory/task_repository.go` に `map[string]*task.Task` と `sync.RWMutex` で実装する。見つからないときは `apperr.New(apperr.KindNotFound, "task not found")` を返す。
- **値のコピー**: `Save` は受け取ったポインタをそのまま保存せず、構造体をコピーして保存する。`FindByID` も内部の値をコピーして返す。呼び出し側の変更が Repository 内部に漏れないようにする。
- `List` は `CreatedAt` 昇順、同値なら `ID` 昇順で返す(`slices.SortFunc`)。

**完了条件**

- [ ] `FindByID` で取得した Task を書き換えても、再取得した値が変わらないテストがある
- [ ] 50 goroutine から `Save` と `FindByID` を同時に呼ぶテストが `-race` で通る
- [ ] 存在しない ID の `FindByID` / `Delete` が `apperr.KindNotFound`
- [ ] `var _ port.TaskRepository = (*TaskRepository)(nil)` でコンパイル時に interface 実装を保証している

**ヒント**: 並行テストは `sync.WaitGroup` と `errgroup` のどちらでもよいが、この段階では標準ライブラリだけで書く。

---

## 08. Value Object(Tags / Priority)

**主題**: 値オブジェクト、正規化、`slices` / `maps` パッケージ、enum の文字列変換

**実装**

- `domain/task/priority.go`: `type Priority int` で `PriorityLow`、`PriorityMedium`(既定)、`PriorityHigh`。`ParsePriority(s string) (Priority, error)` と `String()` を持ち、不正な文字列は `ErrInvalidPriority`。
- `domain/task/tags.go`: `type Tags []string`。`NewTags(raw []string) (Tags, error)` は各要素を `TrimSpace` → 小文字化 → 空文字を除去 → 重複除去 → ソートする。1 タグ 30 文字以内、最大 10 個。違反は `ErrInvalidTags`。
- `Task` に `Priority Priority` と `Tags Tags` を追加し、`NewTask` はオプション引数で受け取れるよう `NewTask(clk, gen, title, description string, opts ...Option)` の Functional Options パターンに変更する。`WithPriority(p)`、`WithTags(tags)` を用意する。
- `Tags` に `Contains(tag string) bool` を実装する。

**完了条件**

- [ ] `NewTags([]string{" Go", "go", "", "API"})` が `["api", "go"]` になるテストがある
- [ ] 11 個のタグ、31 文字のタグがそれぞれ `ErrInvalidTags`
- [ ] `ParsePriority("HIGH")` は大文字小文字を区別せず `PriorityHigh`(仕様として決めてテストに書く)
- [ ] `Tags` はスライスなので `Task` から取り出した後に変更されても内部が壊れないよう、getter でコピーを返すか、doc コメントで「変更禁止」を明記している

**ヒント**: Functional Options は `type Option func(*Task) error` にするとオプション自体が検証エラーを返せる。

---

## 09. context の伝搬とキャンセル

**主題**: `context.Context` の役割、キャンセルとタイムアウト、第一引数の慣習

**実装**

- `memory.TaskRepository` の各メソッド冒頭で `ctx.Err()` を確認し、非 nil なら `apperr.Wrap(apperr.KindUnavailable, "canceled", ctx.Err())` を返す。
- `List` は要素数が多いときを想定し、ループの中でも 100 件ごとに `ctx.Err()` を確認する。
- `pkg/apperr` に `FromContext(err error) *Error` を追加し、`context.Canceled` / `context.DeadlineExceeded` を `KindUnavailable` へ変換する共通関数にする(後の DB 実装でも使う)。

**完了条件**

- [ ] キャンセル済み context で `Save` を呼ぶと `KindUnavailable` かつ `errors.Is(err, context.Canceled)`
- [ ] `context.WithTimeout(ctx, 0)` を渡すと `context.DeadlineExceeded` がラップされている
- [ ] `context.Background()` を直接 Repository に渡しているのはテストと `main` だけ(`rg "context.Background\(\)" --glob '!*_test.go'` で確認)
- [ ] context を構造体のフィールドに保存していない

**ヒント**: context は「呼び出しの寿命」を表す。goroutine を跨ぐときは必ず引数で渡す。

---

## 10. JSON DTO と golden テスト

**主題**: `encoding/json`、構造体タグ、カスタム `MarshalJSON`、golden ファイル

**実装**

- `gateway/http/dto.go` に `TaskResponse` を定義する。domain の `Task` に JSON タグを付けない。
  ```go
  type TaskResponse struct {
      ID          string    `json:"id"`
      Title       string    `json:"title"`
      Description string    `json:"description,omitempty"`
      Status      string    `json:"status"`
      Priority    string    `json:"priority"`
      Tags        []string  `json:"tags"`          // 空でも [] を出す(null にしない)
      CreatedAt   time.Time `json:"created_at"`    // RFC3339、UTC
      UpdatedAt   time.Time `json:"updated_at"`
  }
  func NewTaskResponse(t *task.Task) TaskResponse
  ```
- `CreateTaskRequest`(`title`、`description`、`priority`、`tags`)と、そこから domain へ渡すための変換関数を書く。
- `testutil/golden.go` に `Golden(t, name string, got []byte)` を作る。`-update` フラグ(`flag.Bool`)付きで実行したときは `testdata/<name>.golden` を書き換え、通常はファイルと比較する。
- `TaskResponse` の JSON を golden で検証する。

**完了条件**

- [ ] `Tags` が nil のとき JSON が `"tags": []` になる(`null` ではない)
- [ ] `CreatedAt` が `2026-01-02T03:04:05Z` の形式で出る
- [ ] `go test ./gateway/http/ -update` で golden が更新され、その後の通常実行で通る
- [ ] `Decode` 時に不明なフィールドを拒否する設定(`DisallowUnknownFields`)を使った `CreateTaskRequest` のデコード関数がある

**ヒント**: golden の比較は `bytes.Equal` ではなく差分が見える形(改行区切りで行比較か、`cmp.Diff`)にすると失敗時に読みやすい。`github.com/google/go-cmp` を導入してよい。

---

## 11. 最初の HTTP ハンドラ

**主題**: `net/http`、Go 1.22+ のメソッド付きルーティング、`httptest` **TDD 必須**

**実装**

- `gateway/http/handler_task.go` に `TaskHandler` 構造体(`repo port.TaskRepository`、`clk`、`gen` を持つ)を作る。次の 2 つを実装する。
  - `POST /tasks`: `CreateTaskRequest` をデコードし、`task.NewTask` → `repo.Save` → 201 で `TaskResponse` を返す。
  - `GET /tasks/{id}`: `r.PathValue("id")` で取得し、200 で返す。見つからなければ 404。
- `gateway/http/router.go` に `NewRouter(h *TaskHandler) http.Handler` を作り、`http.NewServeMux` へ `mux.HandleFunc("POST /tasks", ...)` の形で登録する。
- `cmd/server/main.go` を `:8080` で起動するよう書き換える(shutdown は問題 20)。
- テストは `httptest.NewRecorder` と `httptest.NewRequest` で handler を直接呼ぶ。この段階では Repository はメモリ実装を使う。

**完了条件**

- [ ] `POST /tasks` の成功で 201、`Content-Type: application/json; charset=utf-8`、`Location: /tasks/{id}` ヘッダが返る
- [ ] 不正 JSON(`{`)で 400
- [ ] `GET /tasks/{id}` 未存在で 404、レスポンスも JSON
- [ ] handler 内で `defer r.Body.Close()` していない(サーバー側の Body はサーバーが閉じる。理由をコメントに書く)
- [ ] `make run` で起動し、`curl -X POST localhost:8080/tasks -d '{"title":"hello"}'` が通る

**ヒント**: エラーの JSON 化は暫定でよい。問題 14 で整理する。レスポンス書き込みは `json.NewEncoder(w).Encode` を使い、エラーを捨てない。

---

## 12. usecase 層の導入

**主題**: handler から業務手続きを分離する、Input / Output 構造体

**実装**

- `usecase/task/create.go` に次を作る。
  ```go
  type CreateInput struct { Title, Description, Priority string; Tags []string }
  type CreateOutput struct { Task *task.Task }
  type Create struct { repo port.TaskRepository; clk clock.Clock; gen id.Generator }
  func NewCreate(repo, clk, gen) *Create
  func (u *Create) Execute(ctx context.Context, in CreateInput) (*CreateOutput, error)
  ```
- `usecase/task/get.go` に `Get` を同じ形で作る。
- `Execute` は domain のエラーを `apperr` に変換する責務を持つ(例: `ErrEmptyTitle` → `KindInvalid`)。
- `TaskHandler` は Repository を持たず、`Create` と `Get` の usecase を持つ。handler は「デコード → Input へ変換 → Execute → Response へ変換 → エンコード」のみ。
- usecase のテストは、メモリ Repository を使って `Execute` を直接呼ぶ(モックは問題 27 で導入)。

**完了条件**

- [ ] `gateway/http` から `domain/task` のコンストラクタ(`task.NewTask`)を呼んでいない(`rg "task.NewTask" gateway/` が 0 件)
- [ ] `Create.Execute` に `Priority: "urgent"` を渡すと `apperr.KindInvalid`
- [ ] handler のテストは HTTP の関心(ステータス、ヘッダ、JSON 形式)だけを検証し、業務ルールの網羅は usecase のテストに寄せている
- [ ] usecase パッケージが `net/http` を import していない

**ヒント**: usecase は「1 ユースケース = 1 型 = 1 `Execute`」に固定すると後で Wire・モック・gRPC への接続が楽になる。

---

## 13. 入力検証とフィールドエラー

**主題**: フィールド単位の検証結果、エラーの集約、検証の置き場所 **TDD 必須**

**実装**

- `pkg/validate` に次を作る。
  ```go
  type FieldError struct { Field, Message string }
  type Errors []FieldError
  func (e Errors) Error() string
  type Validator struct { errs Errors }
  func (v *Validator) Required(field, value string)
  func (v *Validator) MaxRunes(field, value string, max int)
  func (v *Validator) OneOf(field, value string, allowed ...string)
  func (v *Validator) Check(field string, ok bool, msg string)
  func (v *Validator) Err() error   // errs が空なら nil、あれば apperr.KindInvalid でラップした Errors
  ```
- `apperr.Error` に `Fields validate.Errors` を持たせるか、`Err` に `validate.Errors` を入れて `errors.As` で取り出せるようにする(どちらかを選び、理由を doc コメントに書く)。
- `CreateInput` に `Validate() error` を実装し、`Execute` の冒頭で呼ぶ。検証項目: `title` 必須・200 文字以内、`description` 2000 文字以内、`priority` は `low|medium|high` または空、`tags` は 10 個以内。
- domain の `NewTask` の検証は残す(二重に見えるが、domain は最後の砦、usecase は利用者向けの親切なメッセージ、と役割が違う)。

**完了条件**

- [ ] 複数フィールドが同時に不正なとき、1 回の呼び出しで全フィールドのエラーが返る(最初の 1 件で止めない)
- [ ] `FieldError.Field` はリクエストの JSON キー名(`title`)であり Go のフィールド名(`Title`)ではない
- [ ] `Validator` はゼロ値で使える(`var v validate.Validator`)
- [ ] 検証ロジックが `gateway/http` に無い

**ヒント**: 「境界での検証(usecase)」と「不変条件の保護(domain)」を分けて考える。

---

## 14. エラーから HTTP ステータスへの変換

**主題**: `apperr.Kind` → HTTP status、問題詳細形式(RFC 9457 風)、内部情報の隠蔽

**実装**

- `gateway/http/errors.go` に `writeError(w http.ResponseWriter, r *http.Request, err error)` を作る。
  - `Kind` → ステータスの対応表を `map[apperr.Kind]int` で持つ。`KindUnknown` は 500。
  - レスポンスは `Content-Type: application/problem+json`、本文は `{"type":"about:blank","title":"Not Found","status":404,"detail":"task not found","errors":[{"field":"title","message":"..."}]}`。`errors` はフィールドエラーがあるときだけ。
  - 500 のときは `detail` を固定文言 `"internal error"` にし、`err.Error()` を本文に含めない。
- handler の各分岐で `writeError` を呼ぶ。JSON デコード失敗も `apperr.KindInvalid` に変換する。

**完了条件**

- [ ] `Kind` ごとのステータスがテーブル駆動テストで検証されている(7 種すべて)
- [ ] `errors.New("db exploded: password=xxx")` を渡したとき、レスポンス本文に `password` が含まれない
- [ ] 検証エラーで `errors` 配列が出て、それ以外では `errors` キー自体が無い(`omitempty`)
- [ ] `writeError` の書き込みエラー(`Encode` の戻り値)を捨てていない

**ヒント**: 500 のときだけ原因をログへ出したいが、ログはまだ無い。`TODO(問題19)` コメントを残しておく。

---

## 15. 一覧・フィルタ・カーソルページネーション

**主題**: クエリパラメータ、キーセットページネーション、不透明カーソル **TDD 必須**

**実装**

- `port.TaskRepository.List` を次に変える。
  ```go
  type ListFilter struct { Status *task.Status; Tag *string }
  type ListOptions struct { Filter ListFilter; Limit int; After *Cursor }
  type Cursor struct { CreatedAt time.Time; ID string }
  type ListResult struct { Items []*task.Task; Next *Cursor }
  List(ctx, opts ListOptions) (ListResult, error)
  ```
- 並び順は `CreatedAt ASC, ID ASC` に固定し、`After` より後の要素から `Limit` 件を返す。`Limit+1` 件取得して次ページの有無を判定し、あれば `Next` を埋める。
- `usecase/task/list.go` に `List` usecase を作る。`Limit` は既定 20、上限 100。
- `gateway/http` で `GET /tasks?status=todo&tag=go&limit=20&cursor=<opaque>` を実装する。カーソルは `Cursor` を JSON → base64url にした不透明文字列にし、デコード失敗は 400。レスポンスは `{"items":[...],"next_cursor":"..."}`(`next_cursor` は最終ページで `null`)。
- カーソルのエンコード・デコードは `gateway/http/cursor.go` に置く(usecase は不透明文字列を知らない)。

**完了条件**

- [ ] 25 件を `limit=10` で 3 回たどると 10 / 10 / 5 件で、`next_cursor` が最後だけ `null`
- [ ] 同一 `CreatedAt` の Task が複数あっても、ページをまたいで重複・欠落しない(テストで同時刻を意図的に作る)
- [ ] `limit=1000` は 100 に丸められ、`limit=0` や `limit=-1` は 400
- [ ] `cursor=@@@` で 400、改竄されて JSON 構造が違うものも 400

**ヒント**: 「オフセットではなくキーセットにする理由」を `cursor.go` の doc コメントに 3 行で書く(問題 34 で SQL 化するときに読み返す)。

---

## 16. 部分更新と楽観ロック

**主題**: PATCH、ポインタで「未指定」を表す、バージョンによる競合検出 **TDD 必須**

**実装**

- `Task` に `Version int` を追加する。`NewTask` で 1、`Save` されるたびに Repository 側で `+1` する(メモリ実装では `Save` 時に保存済みの `Version` と一致しなければ `apperr.KindConflict`)。
- `usecase/task/update.go`:
  ```go
  type UpdateInput struct {
      ID          string
      Version     int       // 必須。クライアントが最後に見た値
      Title       *string   // nil は「変更しない」
      Description *string
      Priority    *string
      Tags        *[]string
  }
  ```
  domain に `Task.Rename(clk, title)`、`ChangeDescription`、`ChangePriority`、`ReplaceTags` を追加し、usecase から呼ぶ。
- `PATCH /tasks/{id}` を追加する。リクエストボディに `version` を含める。競合時は 409。
- 状態遷移は別エンドポイント `POST /tasks/{id}/start|complete|cancel|reopen` として追加する(PATCH で `status` を受け付けない。理由をコメントに書く)。

**完了条件**

- [ ] `{"version":1,"title":"a"}` を 2 回送ると 2 回目が 409
- [ ] `description: ""` を送ると空文字に更新され、`description` キーを省略すると変更されない(ポインタ nil と空文字を区別する)
- [ ] 不正な遷移(`done` に `start`)で 409 ではなく 400 か 409 か、どちらにするかを決めて `errors.go` の対応表とテストに反映している(推奨: `KindConflict` → 409)
- [ ] レスポンスの `version` が更新後の値

**ヒント**: JSON で「キーなし」と「null」を区別したい場合は `*string` で十分(両方 nil)。区別が必要になったら `json.RawMessage` を検討する。

---

## 17. 削除と冪等性

**主題**: DELETE の意味論、冪等な API、テストの独立性

**実装**

- `usecase/task/delete.go` と `DELETE /tasks/{id}` を実装する。成功は 204 No Content。
- 削除済み・未存在の ID に対する DELETE をどう扱うか決める。推奨は「存在しなければ 404」。ただし「冪等性」の観点で 204 を返す設計もある。どちらを選んだかと理由を `handler_task.go` の doc コメントに書く。
- 削除された Task は `GET` で 404、`List` に出てこない。

**完了条件**

- [ ] 削除後に `GET` が 404
- [ ] handler テストの各サブテストが独自の Repository インスタンスを持ち、`t.Parallel()` を付けても衝突しない
- [ ] 204 のレスポンスで本文が空(`Content-Length: 0`、`Content-Type` を付けない)

**ヒント**: `httptest.NewRecorder()` の `Body.Len()` で本文が空か確認できる。

---

## 18. ミドルウェアチェーン(RequestID / Recover)

**主題**: `func(http.Handler) http.Handler`、`context.WithValue` の正しい使い方、panic からの回復 **TDD 必須**

**実装**

- `gateway/http/middleware.go` に次を作る。
  - `Chain(h http.Handler, mws ...Middleware) http.Handler`(先頭のミドルウェアが最外側になる)
  - `RequestID()`: `X-Request-ID` ヘッダがあればそれを、無ければ `id.Generator` で生成した値を context に入れ、レスポンスヘッダにも付ける。context キーは非公開型 `type ctxKey int` を使う。取り出し関数 `RequestIDFrom(ctx) string` を公開する。
  - `Recover()`: handler の panic を捕捉し 500 を返す。`http.ErrAbortHandler` は再 panic する。
- `NewRouter` でチェーンを適用する。

**完了条件**

- [ ] `X-Request-ID: abc` を送るとレスポンスにも `abc` が返る
- [ ] panic するダミー handler を包んで 500 が返り、テストプロセスが落ちない
- [ ] `Chain(h, A, B)` で A → B → h → B → A の順で実行されることをテストで確認している(各ミドルウェアが順序を記録する)
- [ ] `context.WithValue` のキーに文字列型を使っていない

**ヒント**: Recover の中で `debug.Stack()` を取っておく。ログは次の問題で付ける。

---

## 19. 構造化ログ(slog)

**主題**: `log/slog`、ロガーの注入、出してはいけない情報

**実装**

- `pkg/log` に `New(w io.Writer, level slog.Level, format string) *slog.Logger`(`json` / `text`)と、context にロガーを載せる `WithLogger(ctx, l)` / `FromContext(ctx) *slog.Logger`(無ければ `slog.Default()`)を作る。
- ミドルウェア `Logging(base *slog.Logger)` を追加する。リクエストごとに `request_id` を付けた子ロガーを context に載せ、完了時に `method`、`path`、`status`、`duration_ms`、`bytes` を Info で 1 行出す。ステータス取得のため `http.ResponseWriter` をラップする。
- `Recover` はスタックトレース付きで Error ログを出す。`writeError` は 500 のときだけ原因 `err` を Error ログに出す(問題 14 の TODO を解消)。
- `Authorization` ヘッダ、リクエスト本文、クエリ文字列は出さない。

**完了条件**

- [ ] `bytes.Buffer` に JSON ログを書き出し、`request_id` と `status` が含まれることを確認するテストがある
- [ ] `Authorization: Bearer secret` を付けたリクエストのログに `secret` が含まれない
- [ ] ラップした `ResponseWriter` が `http.Flusher` を透過する(`Unwrap()` を実装するか、型アサーションのテストを書く)
- [ ] `slog.Default()` へ直接書いているのは `main` だけ

**ヒント**: `slog.Handler` を自作せず、`Logger.With(...)` で属性を積む。ログレベルは問題 20 で設定から入れる。

---

## 20. 設定読み込みと graceful shutdown

**主題**: 環境変数からの設定、起動と停止の作法、`signal.NotifyContext`

**実装**

- `app/config.go` に `Config` 構造体(`Port int`、`LogLevel slog.Level`、`LogFormat string`、`ShutdownTimeout time.Duration`)と `Load() (Config, error)` を作る。環境変数は `PORT`、`LOG_LEVEL`、`LOG_FORMAT`、`SHUTDOWN_TIMEOUT`。未設定は既定値、不正値はエラー(起動しない)。外部ライブラリは使わず `os.LookupEnv` と `strconv` で書く。
- `cmd/server/main.go`:
  1. `Config` を読む
  2. ロガー、Clock、ID 生成、メモリ Repository、usecase、handler、router を組み立てる
  3. `http.Server{ReadHeaderTimeout: 5s, ...}` を goroutine で `ListenAndServe`
  4. `signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)` で待ち、`ShutdownTimeout` 付きで `srv.Shutdown`
  5. `main` は `run(ctx, cfg) error` を呼ぶだけにし、エラーは `os.Exit(1)`
- `Config` に `String()` を実装し、将来のシークレットが混ざっても出ないよう、明示的にフィールドを列挙して出力する。

**完了条件**

- [ ] `PORT=abc` で `Load` がエラー、`PORT` 未設定で 8080
- [ ] `Load` のテストは `t.Setenv` を使い、`t.Parallel()` を付けていない(`t.Setenv` は並列不可)
- [ ] `make run` 後に `Ctrl-C` で「shutting down」のログが出て、処理中のリクエストが完了してから終了する(`sleep` を挟むダミーエンドポイントで手動確認し、手順を PR 用メモとして `docs/notes/20.md` に残す)
- [ ] `ReadHeaderTimeout` が設定されている(`gosec` G112 対策)

**ヒント**: `run` 関数に切り出しておくと、問題 94 で shutdown のテストを書くときに助かる。

---

## 21. テストヘルパーと testify

**主題**: `t.Helper()`、ビルダー、`t.Cleanup`、`testify`

**実装**

- `go get github.com/stretchr/testify` を導入し、既存テストの `if got != want { t.Errorf(...) }` を `assert` / `require` に置き換える(前提条件は `require`、検証は `assert`)。
- `testutil/builder.go` に `TaskBuilder` を作る。
  ```go
  b := testutil.NewTask(t).Title("x").Status(task.StatusDoing).CreatedAt(fixed).Build()
  ```
  `NewTask(t)` は `t.Helper()` を呼び、既定値は決定的(固定 Clock と `id.Sequence`)。
- `testutil/http.go` に `DoJSON(t, handler, method, path string, body any) *httptest.ResponseRecorder` と `DecodeJSON[T any](t, rec) T` を作る。
- すべてのテストに `t.Parallel()` を付ける(`t.Setenv` を使うものを除く)。

**完了条件**

- [ ] `testutil` 内の全ヘルパーが `t.Helper()` を呼んでいる(失敗行が呼び出し側になる)
- [ ] ビルダーで `Status(StatusDone)` を指定しても domain の遷移メソッドを迂回して直接フィールドをセットしている理由(テスト用の特権)が doc コメントにある
- [ ] `go test -race -count=3 ./...` が通る(`-count=3` でフレークがないことを確認)
- [ ] `assert.Equal` の引数順が `(t, expected, actual)` になっている

**ヒント**: 期待値と実測値の順を揃えるだけで、失敗メッセージの読みやすさが変わる。

---

## 22. ジェネリクスで共通処理を切り出す

**主題**: 型パラメータ、制約、汎用ページ型

**実装**

- `pkg/collection` に `Map[T, U any]`、`Filter[T any]`、`Uniq[T comparable]`、`GroupBy[T any, K comparable]`、`Chunk[T any](xs []T, size int) [][]T` を実装する(nil 入力で nil ではなく空スライスを返すかを決め、テストで固定する)。
- `usecase/port` の `ListResult` を `Page[T any] struct { Items []T; Next *Cursor }` に置き換える。
- `gateway/http` のレスポンス変換で `collection.Map(page.Items, NewTaskResponse)` を使う。
- `Chunk` は問題 61 のバルクインサートで使う。

**完了条件**

- [ ] 各関数にテーブル駆動テスト(空、1 件、複数、重複あり)がある
- [ ] `Chunk(xs, 0)` の挙動(panic か空か)を決めてテストしている
- [ ] 既存の手書きループが 3 箇所以上ジェネリクスに置き換わっている
- [ ] 型推論だけで呼べる(呼び出し側で `Map[task.Task, TaskResponse](...)` と型を書いていない)

**ヒント**: 「汎用にしすぎない」。使う場所が 1 つしかないものはジェネリクスにしない。

---

## 23. ドメインイベントと EventPublisher

**主題**: interface と埋め込み、集約がイベントを記録する、publisher の抽象化

**実装**

- `domain/event/event.go` に定義する。
  ```go
  type Event interface { Name() string; OccurredAt() time.Time; AggregateID() string }
  type Base struct { At time.Time; ID string }   // 埋め込み用
  ```
- `domain/task/events.go` に `TaskCreated`、`TaskStatusChanged{From, To}`、`TaskDeleted` を `event.Base` を埋め込んで定義する。
- `Task` に非公開フィールド `events []event.Event` と `PullEvents() []event.Event`(返して空にする)を追加し、`NewTask` と各遷移メソッドでイベントを積む。
- `usecase/port/event_publisher.go` に `type EventPublisher interface { Publish(ctx, events ...event.Event) error }`。
- `adapter/gateway/memory/event_publisher.go` に記録するだけの実装(テスト用に `Published() []event.Event`)。
- `Create` と各遷移 usecase は `Save` 後に `PullEvents` → `Publish` を呼ぶ。

**完了条件**

- [ ] `Complete` 後の `PullEvents()` が `TaskStatusChanged{From: doing, To: done}` を 1 件返し、2 回目の `PullEvents()` は空
- [ ] usecase テストで `Publish` に渡ったイベント名を検証している
- [ ] `event.Event` を実装する各型に `var _ event.Event = TaskCreated{}` がある
- [ ] Repository の `Save` はイベントを知らない(Publisher と分離されている)

**ヒント**: 今はインメモリで「発行したつもり」だが、問題 45 で Outbox として DB に書き、問題 70 で実際に配信する。

---

## 24. HTTP クライアント(Webhook 送信)

**主題**: `http.Client` の正しい使い方、タイムアウト、`httptest.NewServer`、Body のクローズ **TDD 必須**

**実装**

- `adapter/gateway/webhook/client.go` に `Client` を作る。`New(baseURL string, httpClient *http.Client)`、`Publish(ctx, events ...event.Event) error` で `port.EventPublisher` を実装する。
- 各イベントを `{"name":"task.created","aggregate_id":"...","occurred_at":"...","request_id":"..."}` として `POST {baseURL}/events` に送る。`Content-Type: application/json`、`User-Agent: go-backend-100-knocks/0.1`。
- `http.Client` は `Timeout` 付きで外から注入し、`http.DefaultClient` を使わない。リクエストは `http.NewRequestWithContext` で作る。
- 2xx 以外はエラー。5xx は `apperr.KindUnavailable`、4xx は `apperr.KindUnknown` でラップし、ステータスコードをメッセージに含める。本文は最大 1KB だけ読んでエラーメッセージに含める(`io.LimitReader`)。
- `main` で `WEBHOOK_URL` が設定されていればこの Client、無ければメモリ Publisher を使う(`Config` に追加)。

**完了条件**

- [ ] `httptest.NewServer` で受け取った JSON を検証し、`Content-Type` と `User-Agent` を確認している
- [ ] サーバーが 500 を返すと `KindUnavailable`、404 なら `KindUnknown`
- [ ] サーバーが 3 秒 sleep する場合に `Timeout: 100ms` の Client でエラーになり、テストが 3 秒待たない
- [ ] `resp.Body.Close()` を必ず呼んでいる(`defer` と、閉じる前に `io.Copy(io.Discard, ...)` で読み切る理由をコメント)
- [ ] キャンセル済み context で呼ぶと即座にエラー

**ヒント**: リトライはまだ入れない(問題 44)。「送れなかったらどうする?」を `client.go` の TODO に書く。

---

## 25. チェックポイント: サブタスク機能

**主題**: ここまでの内容を縦に 1 本。以下の受け入れ条件だけで実装する(前問を見返さない)。

**仕様**

- Task は複数の Subtask を持てる。Subtask は `ID`、`Title`(1〜100 文字)、`Done bool`。
- `POST /tasks/{id}/subtasks` で追加(201)、`POST /tasks/{id}/subtasks/{subID}/done` で完了、`DELETE /tasks/{id}/subtasks/{subID}` で削除(204)。
- Subtask は Task の一部(集約内エンティティ)であり、独立した Repository を持たない。`Task.Save` で丸ごと保存する。
- **ルール**: 未完了の Subtask が 1 つでもあると親 Task を `Complete` できない(`ErrOpenSubtasks` → 409)。
- Subtask の追加・完了・削除でも親の `Version` が上がり、`UpdatedAt` が更新される。
- `TaskResponse` に `subtasks: [{id, title, done}]` を含める。
- イベント `SubtaskAdded`、`SubtaskCompleted` を発行する。

**完了条件**

- [ ] domain のテスト: 上限(1 Task あたり 50 個)と `ErrOpenSubtasks` を含む
- [ ] usecase のテスト: 各操作の成功と `KindNotFound`(親がない、子がない)、`KindConflict`(バージョン不一致)
- [ ] handler のテスト: ステータスコードと JSON 形状
- [ ] 楽観ロックが Subtask 操作でも効くテスト
- [ ] golden ファイルの更新
- [ ] `make check`(`-race`)が通り、`go test -count=3` でフレークがない
- [ ] `docs/notes/25.md` に「知らなかった / 知っていたが使えなかった / 確認漏れ」を 3 点以上

**自己採点の目安**: 2 時間以内に完了条件をすべて満たせたら中級へ。ルールの置き場所(domain の `Complete` 内)が handler や usecase に漏れていたら、問題 12・13 を読み直す。
