# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## このリポジトリの性格

Go ファイルアップロード 60 本ノックの**自習用教材**。GoCon スライド「900 アプリを支えるプラットフォームへのファイルアップロード導入から学ぶ io, mime, multipart」を題材に、ユーザー自身が `problems/60-knocks.md` の Q1〜Q60 を 6 Step で解き、`POST /upload` API を 1 つ育てる。git のルートは親ディレクトリ `../`(モノレポ)で、ここは独立した Go module(`github.com/kusa04/gocon-60-knock`、Go 1.26)。

- このディレクトリに入っているのは**問題文と実行環境だけ**。コード・テスト・testdata・Dockerfile・notes の中身はユーザーが自分で作る。初期状態は `cmd/server/main.go` のプレースホルダのみ。
- **解答を勝手に実装しない。** 「実装して」と言われたら「問題文・環境の整備」なのか「解答実装」なのかを 1 行で確認してから着手する。学習目的の伴走を求められたら `mentor` スキルを使う。
- 姉妹教材 `../go-backend-100-knocks` と Makefile / lint / editorconfig を揃えている。こちらは DB を使わないので PostgreSQL / goose / wire / protobuf は無く、代わりに MinIO と Docker イメージ比較がある。

## コマンド

```bash
make help            # ターゲット一覧
make check           # 提出前チェック: fmt-check / vet / lint / test -race(各問題の合格基準)
make fmt             # gofmt
make lint            # golangci-lint。未導入なら go tool staticcheck にフォールバック
make test            # go test -race -count=1 -shuffle=on ./...(MinIO が必要なテストは自動 skip にすること)
make test-s3         # MinIO を使うテストも実行(S3_* 環境変数を注入。make minio-up が前提、Q55 以降)
make cover           # coverage.html を生成
make run             # go run ./cmd/server(Q14 で /upload を実装してから。STORAGE=local|s3 で保存先切替)
```

単一パッケージ・単一テストは go コマンドを直接使う。

```bash
go test -race -run 'TestLimitReader' ./internal/iox/...
go test -race -run 'TestUpload/上限超過' ./internal/handler/   # サブテストは / で指定
```

### 実験・インフラ

```bash
make bigfile SIZE=200    # tmp/big-200mb.bin を生成(Q18 / Q23 / Q57)
make fixtures            # go run ./cmd/genfixtures -out testdata(cmd/genfixtures は Q36 で作る)
make minio-up            # MinIO(9000 / console 9001、knock / knockknock)を起動しバケット uploads を作成
make minio-down / minio-reset
make mime-matrix         # alpine / debian+mime.types / debian+shared-mime-info の 3 環境で同じテストを実行(docker/run-matrix.sh は Q50 で作る)
make clean               # bin / coverage / uploads / tmp を削除
```

- 外部ライブラリは `go.mod` に**取得済み**(`mimetype`、`aws-sdk-go-v2` の config / credentials / service/s3 / feature/s3/manager)。使い始めるまで `// indirect` のままでよい。問題文で指定されたもの以外は追加しない。
- `multierr` は使わず `errors.Join` を使う。
- staticcheck は `go tool staticcheck`(`go.mod` の `tool` ディレクティブ)。グローバルインストールしない。
- `.env.example` の上限値: リクエスト全体 16MB(`MAX_REQUEST_BYTES`)、1 ファイル 15MB(`MAX_FILE_BYTES`)、パート数 1000(`MAX_PARTS`)。

## アーキテクチャ(最終形)

問題集 2 章に完成時の構成と仕様がある。要点のみ。

```
cmd/server        最終形の API サーバー
cmd/client        io.Pipe でストリーミング送信するクライアント(Q57)
cmd/genfixtures   testdata 生成(Q36)  cmd/extprobe  mime の環境差プローブ(Q48)  cmd/loadtest(Q58)
internal/iox        io の契約を確かめる Reader 群とヘルパー(Step 1)
internal/testutil   multipart リクエスト組み立てヘルパー(Q12)
internal/handler    /upload ハンドラ(v1: ParseMultipartForm → v2: MultipartReader → 最終形)
internal/limit      ReadAndValidateSize(Q24 / Q26)
internal/sniff      DetectValidMimeType / ValidateImage(Q38 / Q42 / Q47)
internal/mimeext    GetExtensionByMimeType(Q53)
internal/middleware 同時処理数の制限(Q32)
internal/storage    local / s3
experiments/qNN_xxx 本体に残さない実験コード
testdata/ e2e/ docker/ docs/
```

リクエスト処理の流れ(最終形): `http.MaxBytesReader`(全体 16MB、413)→ `r.MultipartReader()` + `NextPart()` でストリーミング → パートごとに累積サイズ検査(15MB、413)、パート数・テキストパート上限(400 / 413)→ 先頭バイトの content sniffing + 許可リスト jpeg / png / heic / pdf(許可外 415)、jpeg / png は `image.DecodeConfig` で追加検証 → 拡張子は判定した MIME タイプから固定表で決定、保存名はランダム ID(クライアント申告は使わない)→ `internal/storage` へ保存(local → MinIO)。同時処理数はセマフォで制限。

## 教材固有のルール

- ★1: 仕様をコードで確かめる。★2: **必ず予測を先に `notes/stepN.md` に書いてから実行**し、外れた予測も残す。★3: 完了ごとに git commit し、Step 単位でタグを打つ(`step1`〜`step5`、`v1.0`)。★4: 比較表 + 決めた案の実装または計測。
- `notes/stepN.md` は 1 問あたり「予測(★2 のみ)/ 結果・作ったもの / 根拠 / 関連コード」の見出し。書式は `notes/README.md`。実験ログは要約せずそのまま貼り、長ければ `notes/logs/` に置く。Go のソースを根拠にするときはファイル名・関数名(できれば行番号と Go バージョン)を書く。
- 各 Step の最後に「なぜこう書いたか」を 3 行で `NOTES.md` にまとめる。合格基準を満たせなかった問題は見出しに `[要復習]` を付け、次の Step の前に解き直す。
- 各問題の合格基準を満たし `make check` が通ったら `PROGRESS.md` にチェックを付ける。
- `docker run --read-only`(Q19)や 3 環境比較用の Dockerfile(Q50)は問題文に従って自分で用意する。負荷計測(Q58)の `hey` / `vegeta` は入っていない。
