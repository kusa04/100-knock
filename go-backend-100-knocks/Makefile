# go-backend-100-knocks
# `make help` でターゲット一覧を表示します。

SHELL := /bin/bash
GO    ?= go
PKGS  := ./...

DATABASE_URL      ?= postgres://knock:knock@localhost:5432/knock?sslmode=disable
TEST_DATABASE_URL ?= postgres://knock:knock@localhost:5432/knock_test?sslmode=disable
MIGRATIONS_DIR    := migrations
PROTO_DIR         := proto
GEN_DIR           := gen
BIN_DIR           := $(CURDIR)/bin

GO_FILES := $(shell find . -name '*.go' -not -path './gen/*' -not -path './bin/*' -not -name '*.pb.go' -not -name 'wire_gen.go')

.DEFAULT_GOAL := help

.PHONY: help
help: ## ターゲット一覧
	@grep -E '^[a-zA-Z0-9_-]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN{FS=":.*?## "}{printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2}'

# ---------------------------------------------------------------------------
# 品質チェック
# ---------------------------------------------------------------------------
.PHONY: check
check: fmt-check vet lint test ## 提出前チェック(fmt / vet / lint / test -race)

.PHONY: fmt
fmt: ## gofmt で整形
	@gofmt -w $(GO_FILES)

.PHONY: fmt-check
fmt-check: ## gofmt 未適用ファイルがあれば失敗
	@out=$$(gofmt -l $(GO_FILES)); \
	if [ -n "$$out" ]; then echo "gofmt が必要なファイル:"; echo "$$out"; exit 1; fi

.PHONY: vet
vet: ## go vet
	$(GO) vet $(PKGS)

.PHONY: lint
lint: ## golangci-lint(未導入なら staticcheck)
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run $(PKGS); \
	else \
		echo "golangci-lint が見つからないため staticcheck を実行します(問題 74 で導入)"; \
		$(GO) tool staticcheck $(PKGS); \
	fi

.PHONY: test
test: ## ユニットテスト(race detector 有効、DB テストは自動 skip)
	$(GO) test -race -count=1 -shuffle=on $(PKGS)

.PHONY: test-db
test-db: ## DB を使うテストも実行(make db-up && make migrate-test が前提)
	TEST_DATABASE_URL="$(TEST_DATABASE_URL)" $(GO) test -race -count=1 -shuffle=on $(PKGS)

.PHONY: cover
cover: ## カバレッジを HTML で出力
	$(GO) test -race -count=1 -coverprofile=coverage.out $(PKGS)
	$(GO) tool cover -html=coverage.out -o coverage.html
	@echo "coverage.html を開いてください"

.PHONY: bench
bench: ## ベンチマーク(問題 89 以降)
	$(GO) test -run '^$$' -bench . -benchmem $(PKGS)

# ---------------------------------------------------------------------------
# 実行
# ---------------------------------------------------------------------------
.PHONY: run
run: ## API サーバーを起動
	$(GO) run ./cmd/server

.PHONY: run-worker
run-worker: ## ジョブワーカーを起動(問題 69 以降)
	$(GO) run ./cmd/worker

# ---------------------------------------------------------------------------
# コード生成
# ---------------------------------------------------------------------------
.PHONY: generate
generate: ## go generate(mockgen など)
	$(GO) generate $(PKGS)

.PHONY: wire
wire: ## Wire のコード生成(問題 62 以降)
	$(GO) tool wire gen ./app

.PHONY: proto
proto: ## protobuf / gRPC のコード生成(問題 51 以降)
	@command -v protoc >/dev/null 2>&1 || { echo "protoc が必要です: brew install protobuf"; exit 1; }
	@mkdir -p $(BIN_DIR) $(GEN_DIR)
	$(GO) build -o $(BIN_DIR)/protoc-gen-go google.golang.org/protobuf/cmd/protoc-gen-go
	$(GO) build -o $(BIN_DIR)/protoc-gen-go-grpc google.golang.org/grpc/cmd/protoc-gen-go-grpc
	PATH="$(BIN_DIR):$$PATH" protoc -I $(PROTO_DIR) \
		--go_out=$(GEN_DIR) --go_opt=paths=source_relative \
		--go-grpc_out=$(GEN_DIR) --go-grpc_opt=paths=source_relative \
		$$(find $(PROTO_DIR) -name '*.proto')

# ---------------------------------------------------------------------------
# ローカルインフラ(Docker)
# ---------------------------------------------------------------------------
.PHONY: db-up
db-up: ## PostgreSQL と MinIO を起動
	docker compose up -d --wait db minio

.PHONY: db-down
db-down: ## コンテナを停止(データは保持)
	docker compose down

.PHONY: db-reset
db-reset: ## コンテナとボリュームを削除して作り直す
	docker compose down -v
	docker compose up -d --wait db minio

.PHONY: obs-up
obs-up: ## Jaeger / Prometheus も含めて起動(問題 85 以降)
	docker compose --profile observability up -d --wait

.PHONY: psql
psql: ## 開発 DB へ接続
	docker compose exec db psql -U knock -d knock

# ---------------------------------------------------------------------------
# マイグレーション(goose)
# ---------------------------------------------------------------------------
.PHONY: migrate
migrate: ## 開発 DB へマイグレーションを適用
	$(GO) tool goose -dir $(MIGRATIONS_DIR) postgres "$(DATABASE_URL)" up

.PHONY: migrate-test
migrate-test: ## テスト DB へマイグレーションを適用
	$(GO) tool goose -dir $(MIGRATIONS_DIR) postgres "$(TEST_DATABASE_URL)" up

.PHONY: migrate-down
migrate-down: ## 開発 DB のマイグレーションを 1 つ戻す
	$(GO) tool goose -dir $(MIGRATIONS_DIR) postgres "$(DATABASE_URL)" down

.PHONY: migrate-status
migrate-status: ## 適用状況を表示
	$(GO) tool goose -dir $(MIGRATIONS_DIR) postgres "$(DATABASE_URL)" status

.PHONY: migrate-create
migrate-create: ## 新規マイグレーション作成: make migrate-create name=create_tasks
	@test -n "$(name)" || { echo "name=<snake_case> を指定してください"; exit 1; }
	$(GO) tool goose -dir $(MIGRATIONS_DIR) create $(name) sql

# ---------------------------------------------------------------------------
# ツール・依存
# ---------------------------------------------------------------------------
.PHONY: tools
tools: ## 開発ツールを go.mod の tool ディレクティブへ固定(初回のみ)
	$(GO) get -tool honnef.co/go/tools/cmd/staticcheck@latest
	$(GO) get -tool github.com/google/wire/cmd/wire@latest
	$(GO) get -tool go.uber.org/mock/mockgen@latest
	$(GO) get -tool github.com/pressly/goose/v3/cmd/goose@latest
	$(GO) get -tool google.golang.org/protobuf/cmd/protoc-gen-go@latest
	$(GO) get -tool google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
	$(GO) mod tidy

.PHONY: tidy
tidy: ## go mod tidy
	$(GO) mod tidy

.PHONY: clean
clean: ## 生成物を削除
	rm -rf $(BIN_DIR) coverage.out coverage.html
