# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## このリポジトリの性格

Go バックエンド 100 本ノックの**自習用教材**。ユーザー自身が `problems/01〜04` の問題を順に解き、1 つのコードベースを育てていく。git のルートは親ディレクトリ `../`(モノレポ)で、このディレクトリは独立した Go module(`github.com/kusa04/go-backend-100-knocks`、Go 1.26)。

- 教材の説明・共通ルール・到達目標は `../README.md`、環境構築は `../docs/setup.md`、目標アーキテクチャは `../docs/architecture.md`、進捗は `../PROGRESS.md`。問題文中の `docs/...` はこの親側 `../docs/` を指す。
- **解答を勝手に実装しない。** 「実装して」と言われたら「問題文・環境の整備」なのか「解答実装」なのかを 1 行で確認してから着手する。学習目的の伴走を求められたら `mentor` スキルを使う。
- 初期状態はほぼ空(`cmd/server/main.go` のプレースホルダのみ)。`domain/`, `usecase/`, `adapter/`, `gateway/` などは問題を進めるにつれて作られる。
- **問題文の粒度は段階的に下げる。** 初級(1〜25)は実装手順とヒントまで、中級(26〜50)は what のみで細部は学習者に委ねる、中級プラス(51〜75)は「要件 / 決めること / 完了条件」だけ、上級(76〜100)は仕様の要約と完了条件だけ。問題文を追加・修正するときはこの粒度を守り、後半の問題に丁寧な説明やヒントを足さない。

## コマンド

```bash
make help            # ターゲット一覧
make check           # 提出前チェック: fmt-check / vet / lint / test -race(各問題の完了条件)
make fmt             # gofmt
make lint            # golangci-lint。未導入なら go tool staticcheck にフォールバック(問題 74 で本格導入)
make test            # go test -race -count=1 -shuffle=on ./...(DB テストは TEST_DATABASE_URL 未設定なら自動 skip)
make test-db         # DB テスト込み(make db-up && make migrate-test が前提)
make cover           # coverage.html を生成
make bench           # ベンチマーク(問題 89 以降)
make run             # go run ./cmd/server
make run-worker      # go run ./cmd/worker(問題 69 以降)
```

単一パッケージ・単一テストの実行は go コマンドを直接使う。

```bash
go test -race -run 'TestTask' ./domain/task/...
go test -race -run 'TestTask/done_と_canceled' ./domain/task/   # サブテストは / で指定
```

### インフラ・コード生成

```bash
make db-up / db-down / db-reset   # PostgreSQL(5432)+ MinIO(9000/9001)。Docker Desktop 起動が前提
make obs-up                       # Jaeger(16686)+ Prometheus(9091)も起動(問題 85 以降)
make migrate / migrate-test / migrate-down / migrate-status
make migrate-create name=create_tasks
make psql                         # 開発 DB へ接続
make generate                     # go generate(mockgen)
make wire                         # go tool wire gen ./app(問題 62 以降)
make proto                        # protoc → gen/(問題 51 以降、protoc は brew install protobuf)
```

Go 製ツール(wire, goose, mockgen, staticcheck, protoc-gen-go, protoc-gen-go-grpc)は `go.mod` の `tool` ディレクティブに固定済み。`go tool <name>` で呼び、グローバルインストールしない。接続文字列は `.env.example` と Makefile の既定値が同じ(`knock:knock@localhost:5432/knock`、テスト DB は `knock_test`)。

## アーキテクチャ(最終形の依存方向)

詳細は `../docs/architecture.md`。要点のみ。

```
gateway (http / rpc) ──▶ usecase ──▶ domain
                          │
                          ▼
                     usecase/port (interface)  ◀── 実装 ── adapter/gateway (memory / db / storage / webhook)
                                                              ▲ 組み立て: app (Wire)
```

| レイヤー | 置くもの | 置かないもの |
| --- | --- | --- |
| `domain/` | エンティティ、値オブジェクト、状態遷移、ドメインイベント、ドメインエラー | DB、HTTP、JSON タグ、ログ、`time.Now()` |
| `usecase/` | Interactor、トランザクション境界、port の利用 | SQL、HTTP ステータス、proto 型 |
| `usecase/port/` | Repository / Query / 外部サービスの interface | 実装 |
| `adapter/gateway/` | port の実装、外部エラー → `apperr` 変換 | 業務ルール |
| `gateway/http`, `gateway/rpc` | DTO 変換、認証スコープ解決、`apperr` → ステータス変換、middleware | 業務ルール、SQL |
| `app/` | 設定、DI、起動・シャットダウン | 業務ロジック |
| `worker/` | ジョブ定義、enqueue / execute、再試行 | HTTP |

- 依存の向きは `.golangci.yml` の `depguard` で機械的に検査される(domain → usecase/adapter/gateway/database/sql/net/http 禁止、usecase → adapter/gateway 禁止)。
- エラーは domain error → `apperr.Error{Kind, Msg, Err, Fields}` に wrap → gateway 層で Kind から HTTP status / gRPC code へ変換。内部メッセージはクライアントへ漏らさない。
- 時刻と ID は `pkg/clock` / `pkg/id` 経由で注入し、テストでは固定値を使う。

## 教材固有のルール(コードを書くとき)

- 問題文にない機能を先回りして作らない。外部ライブラリは問題文で指定されたものだけ `go get` する。
- 各問題は前問のコードを育てる。新しいディレクトリへ作り直さない。
- **TDD 必須** と書かれた問題は失敗するテストを先に書く。
- サブテスト名は日本語のシナリオ形式(例: `フィールド参照で全ての値が取得できることを確認`)。テーブル駆動テストは `t.Run` を使い、`t.Parallel()` するケースは共有状態を持たない。
- DB テストは `TEST_DATABASE_URL` が無ければ `t.Skip` し、各テストはトランザクション内で実行して最後にロールバックする(問題 32)。
- モックは port(interface)に対してのみ生成する。具象型をモックしない。
- 問題を終えたら `make check` を通し、`../PROGRESS.md` にチェックを付ける。
