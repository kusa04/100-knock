# gocon-60-knock
# `make help` でターゲット一覧を表示します。

SHELL := /bin/bash
GO    ?= go
PKGS  := ./...
BIN_DIR := $(CURDIR)/bin

GO_FILES := $(shell find . -name '*.go' -not -path './bin/*')

.DEFAULT_GOAL := help

.PHONY: help
help: ## ターゲット一覧
	@grep -E '^[a-zA-Z0-9_-]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN{FS=":.*?## "}{printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}'

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
		echo "golangci-lint が見つからないため staticcheck を実行します"; \
		$(GO) tool staticcheck $(PKGS); \
	fi

.PHONY: test
test: ## ユニット・E2E テスト(race detector 有効。MinIO が必要なテストは自動 skip にすること)
	$(GO) test -race -count=1 -shuffle=on $(PKGS)

.PHONY: test-s3
test-s3: ## MinIO を使うテストも実行(make minio-up が前提。Q55 以降)
	S3_ENDPOINT=http://localhost:9000 S3_ACCESS_KEY=knock S3_SECRET_KEY=knockknock S3_BUCKET=uploads \
		$(GO) test -race -count=1 $(PKGS)

.PHONY: cover
cover: ## カバレッジを HTML で出力
	$(GO) test -race -count=1 -coverprofile=coverage.out $(PKGS)
	$(GO) tool cover -html=coverage.out -o coverage.html
	@echo "coverage.html を開いてください"

# ---------------------------------------------------------------------------
# 実行
# ---------------------------------------------------------------------------
.PHONY: run
run: ## /upload サーバーを起動(保存先は STORAGE 環境変数で local / s3 を切替。Q14 以降)
	$(GO) run ./cmd/server

.PHONY: bigfile
bigfile: ## 実験用の大きなファイルを生成: make bigfile SIZE=200 → tmp/big-200mb.bin(Q18 / Q23 / Q57)
	@mkdir -p tmp
	@head -c $$(( $(or $(SIZE),200) * 1024 * 1024 )) /dev/urandom > tmp/big-$(or $(SIZE),200)mb.bin
	@ls -lh tmp/big-$(or $(SIZE),200)mb.bin

# ---------------------------------------------------------------------------
# testdata
# ---------------------------------------------------------------------------
.PHONY: fixtures
fixtures: ## Q36: testdata/ の正常ファイル・偽装ファイルを生成(cmd/genfixtures は Q36 で作成)
	$(GO) run ./cmd/genfixtures -out testdata

# ---------------------------------------------------------------------------
# ローカルインフラ(Docker)
# ---------------------------------------------------------------------------
.PHONY: minio-up
minio-up: ## MinIO を起動しバケット uploads を作成(Q55 以降)
	docker compose up -d --wait minio
	docker compose run --rm minio-init

.PHONY: minio-down
minio-down: ## MinIO を停止(データは保持)
	docker compose down

.PHONY: minio-reset
minio-reset: ## MinIO のボリュームごと削除して作り直す
	docker compose down -v
	$(MAKE) minio-up

.PHONY: mime-matrix
mime-matrix: ## Q50: alpine / debian+mime.types / debian+shared-mime-info の 3 環境で同じテストを実行(docker/run-matrix.sh は Q50 で作成)
	@test -x docker/run-matrix.sh || { echo "docker/run-matrix.sh を Q50 で作成してください"; exit 1; }
	./docker/run-matrix.sh

# ---------------------------------------------------------------------------
# ツール・依存
# ---------------------------------------------------------------------------
.PHONY: tools
tools: ## 開発ツールを go.mod の tool ディレクティブへ固定(初回のみ)
	$(GO) get -tool honnef.co/go/tools/cmd/staticcheck@latest
	$(GO) mod tidy

.PHONY: tidy
tidy: ## go mod tidy
	$(GO) mod tidy

.PHONY: clean
clean: ## 生成物を削除
	rm -rf $(BIN_DIR) coverage.out coverage.html uploads tmp
