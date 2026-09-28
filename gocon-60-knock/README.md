# gocon-60-knock

Go ファイルアップロード 60 本ノック。GoCon のスライド「900 アプリを支えるプラットフォームへのファイルアップロード導入から学ぶ io, mime, multipart」(坂本 拓也 氏)を題材に、`io` / `mime` / `multipart` の挙動と設計判断を自分の手で再現するための練習用 module です。

- 問題集(60 問): [problems/60-knocks.md](./problems/60-knocks.md)
- 回答・予測・実験ログの置き場: [NOTES.md](./NOTES.md) と `notes/stepN.md`(書式は [notes/README.md](./notes/README.md))
- 進捗: [PROGRESS.md](./PROGRESS.md)

このディレクトリには **問題文と実行環境だけ** が入っています。コード・テスト・testdata・Dockerfile・notes の中身はすべて自分で作ります。

## 60 問終了時に完成するアプリ

| 項目 | 内容 |
| --- | --- |
| エンドポイント | `POST /upload`(multipart/form-data、複数ファイル可) |
| リクエスト全体の上限 | `http.MaxBytesReader`(例: 16MB)。超過時は 413 |
| パート単位の上限 | 1 ファイル最大 15MB。累積サイズを読みながらチェック |
| 受信方式 | `r.MultipartReader()` + `NextPart()` によるストリーミング |
| 形式判定 | 先頭バイトによる content sniffing + 許可リスト(jpeg / png / heic / pdf) |
| 拡張子の決定 | 判定した MIME タイプから決定(クライアント申告は使わない)。HEIC はライブラリに委譲 |
| 保存先 | ローカルディレクトリ → 最終的に MinIO(S3 互換) |
| テスト | 偽装ファイルを含むテーブル駆動テスト、E2E テスト、複数 OS イメージでの再現実験 |

全 60 問がハンズオン形式です。各問題に「使う技術」「作るもの」「合格基準」があり、6 つの Step で 1 つのアプリを育てます。ディレクトリ構成の目安と、スライドに登場する標準ライブラリと問題の対応表は問題集の 2 章・3 章を参照してください。

| Step | 範囲 | 到達点 | git タグ |
| --- | --- | --- | --- |
| 1 | Q1–Q10 | `internal/iox` と `testing/iotest` によるテスト基盤 | `step1` |
| 2 | Q11–Q20 | `/upload` v1(ParseMultipartForm)と弱点の観測 | `step2` |
| 3 | Q21–Q34 | `/upload` v2(MultipartReader + 3 層の上限 + セマフォ) | `step3` |
| 4 | Q35–Q44 | 偽装ファイルを弾き、保存名を自前で決める | `step4` |
| 5 | Q45–Q54 | HEIC 対応、3 環境で同じ拡張子 | `step5` |
| 6 | Q55–Q60 | S3 保存、E2E、io.Pipe クライアント、負荷検証、設計書、LT | `v1.0` |

## 使い方

```bash
cp .env.example .env   # 必要に応じて編集
make help              # ターゲット一覧
make check             # fmt / vet / lint / test -race
make run               # サーバー起動(Q14 で /upload を実装してから)
```

Docker を使うノック:

```bash
open -a Docker         # Docker Desktop を起動
make minio-up          # Q55: MinIO を起動(コンソール http://localhost:9001, knock / knockknock)
make test-s3           # Q55: MinIO を使うテストも実行
make mime-matrix       # Q50: 3 つのイメージで比較(docker/run-matrix.sh は自分で作る)
make fixtures          # Q36: testdata を生成(cmd/genfixtures は自分で作る)
make bigfile SIZE=200  # Q18 / Q23 / Q57: 実験用の 200MB ファイルを tmp/ に生成
```

`docker run --read-only`(Q19)や 3 環境比較用の Dockerfile(Q50)は問題文に従って自分で用意します。

## 環境について

- Go 1.26(`go.mod` の go ディレクティブに従います)
- 外部ライブラリは `go.mod` に **取得済み**(オフラインでも進められるように):
  - `github.com/gabriel-vasile/mimetype`(Q47 以降)
  - `github.com/aws/aws-sdk-go-v2`(config / credentials / service/s3 / feature/s3/manager。Q55 以降)
  - `honnef.co/go/tools/cmd/staticcheck`(`go tool staticcheck`。`make lint` が使う)
  - 使い始めるまでは `// indirect` のままで構いません。問題文で指定されたもの以外は追加しないでください
- 負荷計測(Q58)の `hey` / `vegeta` は入っていません。`brew install hey` するか、Go で同等のツールを自作してください
- testdata(Q36 / Q45)は自分で生成します。HEIC は iPhone で撮影した実物があればそれも置いてください

## 進め方のルール(問題集より)

1. ★2 の問題は、必ず予測を先に `notes/stepN.md` に書いてから実行する
2. ★3 の問題は、完了ごとに git commit する(Step 単位でタグを打つ: `step1` 〜 `step5`、`v1.0`)
3. 各 Step の最後に「なぜこう書いたか」を 3 行で notes にまとめる
4. 合格基準を満たせなかった問題には印をつけ、次の Step の前に解き直す

## 環境構築との対応

Makefile、compose、lint、editorconfig は [../go-backend-100-knocks](../go-backend-100-knocks) に揃えています。DB を使わないので PostgreSQL / goose / wire / protobuf の項目を外し、代わりに MinIO(`make minio-up`)と Docker イメージ比較(`make mime-matrix`)を置いています。
