# 環境構築

## 必要なもの

| ツール | 用途 | 導入 |
| ------ | ---- | ---- |
| Go 1.24 以上 | ビルド・テスト。`go.mod` の go ディレクティブに従い、必要なら Go が自動でツールチェインを取得します | https://go.dev/dl/ |
| Docker Desktop | PostgreSQL / MinIO / Jaeger / Prometheus | https://www.docker.com/ |
| make | タスクランナー | macOS 標準 |
| protoc | protobuf コンパイラ(問題 51 以降) | `brew install protobuf` |
| golangci-lint | 総合 linter(問題 74 で導入。それまでは staticcheck で代用) | `brew install golangci-lint` |

Go 製のツール(wire, goose, mockgen, staticcheck, protoc-gen-go, protoc-gen-go-grpc)は `go.mod` の `tool` ディレクティブに固定済みで、`go tool <name>` で実行できます。グローバルインストールは不要です。

## 初回セットアップ

```bash
cd go-backend-100-knocks
cp .env.example .env        # 必要に応じて編集
make check                  # fmt / vet / lint / test -race
make run                    # 起動確認
```

Docker を使う問題(30 以降)では次を実行します。

```bash
open -a Docker              # Docker Desktop を起動
make db-up                  # PostgreSQL(5432)と MinIO(9000/9001)を起動
make migrate                # 開発 DB へマイグレーション適用
make migrate-test           # テスト DB へマイグレーション適用
make test-db                # DB を使うテストも含めて実行
```

可観測性の問題(85 以降)では `make obs-up` で Jaeger(http://localhost:16686)と Prometheus(http://localhost:9091)も起動します。

## 接続情報

| サービス | URL / 認証 |
| -------- | ---------- |
| PostgreSQL(開発) | `postgres://knock:knock@localhost:5432/knock?sslmode=disable` |
| PostgreSQL(テスト) | `postgres://knock:knock@localhost:5432/knock_test?sslmode=disable` |
| MinIO API | http://localhost:9000(`knock` / `knockknock`) |
| MinIO Console | http://localhost:9001 |
| Jaeger UI | http://localhost:16686 |
| Prometheus | http://localhost:9091 |

## よく使うコマンド

```bash
make help                   # ターゲット一覧
make test                   # ユニットテストのみ(DB テストは自動 skip)
make test-db                # DB テスト込み
make cover                  # カバレッジ HTML
make generate               # go generate(mockgen)
make proto                  # protobuf / gRPC 生成
make wire                   # Wire 生成
make migrate-create name=create_tasks
make psql                   # 開発 DB へ psql で接続
make db-reset               # DB を作り直す
```

## トラブルシュート

- `make check` で `gofmt が必要なファイル` と出る: `make fmt` を実行してください。
- `make db-up` が失敗する: Docker Desktop が起動しているか、5432 / 9000 番ポートが他プロセスに使われていないか確認してください。
- DB テストが常に skip される: `TEST_DATABASE_URL` が未設定です。`make test-db` を使うか、`.env` を読み込んでください。
- `go tool xxx` が遅い: 初回はビルドキャッシュを作るため時間がかかります。2 回目以降は高速です。
