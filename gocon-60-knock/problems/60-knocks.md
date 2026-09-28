# Go ファイルアップロード 60 本ノック(ハンズオン版)

対象スライド:「900アプリを支えるプラットフォームへのファイルアップロード導入から学ぶ io, mime, multipart」(坂本 拓也 / Takuya Sakamoto)
https://speakerdeck.com/toyohashi6140/900-o-sasaeru-purattofomu-heno-dounyuu-kara-manabu-io-mime-multipart

---

## 1. ゴール

- 60 問すべてで**何かを形にする**(コード・テスト・スクリプト・実測の表のいずれか)。読むだけ・考えるだけの問題は無い
- 60 問を終えた時点で、**ファイルアップロード API が 1 つ完成している**(ストリーミング受信、多層の上限、content sniffing、環境非依存の拡張子決定、S3 保存、E2E テスト、負荷検証、設計書)
- スライドに登場する `io` / `mime` / `multipart` の標準ライブラリを、**全部自分の手で触った状態**になる
- スライドのテーマ「標準ライブラリをどこまで信じるか」を、自分の実装と実測に基づいて説明できる

## 2. 完成するアプリの仕様

| 項目 | 内容 |
|---|---|
| エンドポイント | `POST /upload`(multipart/form-data、複数ファイル可) |
| リクエスト全体の上限 | `http.MaxBytesReader`(16MB)。超過時は 413 |
| パート単位の上限 | 1 ファイル最大 15MB。累積サイズを読みながらチェック。超過時は 413 |
| パート数・テキストの上限 | パート数 1000、テキストパート 64KB。超過時は 400 / 413 |
| 受信方式 | `r.MultipartReader()` + `NextPart()` によるストリーミング |
| 形式判定 | 先頭バイトによる content sniffing + 許可リスト(jpeg / png / heic / pdf)。許可外は 415 |
| 追加検証 | jpeg / png は `image.DecodeConfig` でヘッダを検証 |
| 拡張子と保存名 | 判定した MIME タイプから固定表で決定。保存名はランダム ID。クライアント申告は使わない |
| 保存先 | ローカルディレクトリ → 最終的に MinIO(S3 互換) |
| 負荷制御 | 同時処理数をセマフォで制限 |
| テスト | `testing/iotest`、偽装ファイルのテーブル駆動テスト、E2E テスト、3 つの OS イメージでの再現実験 |

### 技術スタック

- Go 1.26(`errors.Join` を使い、スライドの `multierr` は使わない)
- `github.com/gabriel-vasile/mimetype`(Q47 以降)
- `github.com/aws/aws-sdk-go-v2`(Q55 以降)
- Docker / docker compose(MinIO、alpine / debian イメージ比較、read-only 実験)
- 負荷計測:`hey` または自作の Go クライアント、`runtime.MemStats` / pprof

### 完成時のディレクトリ構成

```
gocon-60-knock/
├── cmd/
│   ├── server/          最終形の API サーバー
│   ├── client/          io.Pipe でストリーミング送信するクライアント(Q57)
│   ├── loadtest/        負荷試験ツール(Q58。hey を使うなら不要)
│   ├── extprobe/        mime パッケージの環境差を出力するプローブ(Q48)
│   └── genfixtures/     testdata の生成(Q36)
├── internal/
│   ├── iox/             io の契約を確かめる小さな Reader 群とヘルパー(Step 1)
│   ├── testutil/        multipart リクエストの組み立てヘルパー(Q12)
│   ├── handler/         /upload ハンドラ(v1 → v2 → 最終形)
│   ├── limit/           ReadAndValidateSize(Q24 / Q26)
│   ├── sniff/           DetectValidMimeType / ValidateImage(Q38 / Q42 / Q47)
│   ├── mimeext/         GetExtensionByMimeType(Q53)
│   ├── middleware/      同時処理数の制限(Q32)
│   └── storage/         local / s3
├── experiments/         各ノックの実験コード(qNN_xxx/)。本体に残さない実験はここ
├── testdata/            正常ファイル・偽装ファイル(Q36 / Q45)
├── docker/              Dockerfile.readonly / .alpine / .debian-mimetypes / .debian-sharedmime、run-matrix.sh
├── e2e/                 E2E テスト(Q56)
├── docs/                設計書(Q59)、LT 資料(Q60)
├── compose.yaml         MinIO
├── NOTES.md             各 Step の振り返り
└── notes/stepN.md       各ノックの予測・結果・根拠
```

## 3. スライドに登場する標準ライブラリと、手を動かす問題の対応

「読んで知っている」で終わらせないため、各技術に必ず**書いて確かめる問題**を割り当てています。

### `io` / `bytes` / `testing/iotest`

| 技術 | 何を確かめるか | 問題 |
|---|---|---|
| `io.Reader` の契約(`n > 0 && err != nil`) | 契約どおりの Reader を自作し `iotest.TestReader` で検証 | Q1, Q3 |
| `io.Seeker` / `io.ReadSeeker` / `io.ReadCloser` / `http.Request.Body` | 型アサーションで「Seek できる型・できない型」を固定 | Q2 |
| `io.ReadFull` / `io.ReadAtLeast` / `io.ErrUnexpectedEOF` | 短いファイルの先頭読みで正常系として扱う | Q4, Q38 |
| `io.LimitReader` | 「上限に達した」と「EOF」を区別できないことを利用して上限判定 | Q5, Q26 |
| `io.ReadAll` / `io.Copy` / `io.CopyN` / `io.Discard` | 読み捨てでメモリが増えないことを `runtime.MemStats` で確認 | Q6, Q23 |
| `io.MultiReader` | 先頭だけ覗いて元に戻す | Q7, Q33 |
| `io.Pipe` | goroutine と組み合わせたストリーミング送信 | Q8, Q57 |
| `*bytes.Reader` / `Seek(0, io.SeekStart)` | 副作用としての Seek を defer で戻す | Q9, Q31, Q39 |
| `testing/iotest`(`DataErrReader`, `OneByteReader`, `HalfReader`, `TestReader`) | 契約違反を再現するテスト | Q1, Q3, Q10 |

### `net/http` / `net/http/httputil` / `net/http/httptest`

| 技術 | 何を確かめるか | 問題 |
|---|---|---|
| `httputil.DumpRequest` | 生の multipart リクエストを見る | Q13 |
| `Request.ParseMultipartForm` / `FormFile` / `MultipartForm.RemoveAll` | maxMemory の意味、一時ファイル、暗黙の呼び出し | Q14–Q17, Q20 |
| `Request.MultipartReader` | ストリーミング受信 | Q20, Q21, Q23 |
| `http.MaxBytesReader` / `*http.MaxBytesError` / `Server.MaxHeaderBytes` | リクエスト全体の上限と 413 | Q27, Q28 |
| `http.DetectContentType` / `net/http/sniff.go` | 512B・WHATWG mimesniff・HEIC 非対応 | Q37, Q45, Q46 |
| `httptest.NewRequest` / `NewRecorder` / `NewServer` | ハンドラのテスト、E2E | Q12, Q56 |

### `mime/multipart`

| 技術 | 何を確かめるか | 問題 |
|---|---|---|
| boundary / `Content-Disposition` / `multipart.NewReader` | 手書きのボディをパースする | Q11 |
| `multipart.Writer` | テストとクライアントでリクエストを組み立てる | Q12, Q57 |
| `Reader.NextPart` / `NextRawPart` / `Part.FormName` / `Part.FileName` | Content-Transfer-Encoding の扱い、`filepath.Base` の範囲 | Q21, Q43 |
| 読み残したパートの扱い | 途中で `NextPart` を呼ぶと何が起きるか | Q22 |
| `GODEBUG=multipartmaxparts` / `ReadForm` の組み込み上限 | どの経路で効くか | Q29 |

### `mime`

| 技術 | 何を確かめるか | 問題 |
|---|---|---|
| `mime.ExtensionsByType` / `TypeByExtension` | 実行環境(OS の MIME DB)で結果が変わる | Q48, Q50, Q51, Q54 |
| `mime/type.go` の組み込み表、`type_unix.go` の読み込み順 | globs2 → mime.types | Q49 |
| `mime.AddExtensionType` | 追記のみ、import 経路依存 | Q52 |
| `mime.ParseMediaType` | `; charset=` 付きの型を正規化 | Q47, Q53 |

### `errors` / `os` / `image` / `runtime` / その他

| 技術 | 何を確かめるか | 問題 |
|---|---|---|
| `errors.Join` / 名前付き戻り値 + defer | Seek の失敗を握りつぶさない | Q9, Q40 |
| `errors.Is` / `errors.As` | エラーを HTTP ステータスに変換 | Q27, Q30 |
| `os.CreateTemp` / `os.TempDir` / `TMPDIR` | 一時ファイルの場所とライフサイクル | Q15, Q16, Q19 |
| `image.DecodeConfig` / `image/png` / `image/jpeg` | 先頭だけ正しい壊れた画像を弾く | Q36, Q42 |
| `archive/zip`、`crypto/rand`、`encoding/hex` | docx 偽装ファイルの生成、保存名の生成 | Q36, Q43 |
| `runtime.MemStats` / `net/http/pprof` | メモリピークの計測 | Q6, Q18, Q57, Q58 |
| `golang.org/x/sync/semaphore` またはバッファ付き chan | 同時処理数の制限 | Q32, Q58 |

## 4. 難易度と構成

| レベル | 内容 | 成果物の例 |
|---|---|---|
| ★1 | 仕様を**コードで**確かめる | 小さな関数とテスト |
| ★2 | 挙動を**予測してから**実験する | 予測を書いた notes + 再現テスト / スクリプト + 実測の表 |
| ★3 | アプリに機能を足す | 動く実装 + テスト + git commit |
| ★4 | トレードオフを比較して決める | 比較表 + 決めた案の実装または計測 |

| Step | 範囲 | 本数 | アプリの到達点 | git タグ |
|---|---|---|---|---|
| 1 | io の契約を手で確かめる | Q1–Q10(10) | `internal/iox` と `iotest` によるテスト基盤 | `step1` |
| 2 | multipart を素朴に受ける | Q11–Q20(10) | `/upload` v1(ParseMultipartForm)と弱点の観測 | `step2` |
| 3 | ストリーミングと多層防御 | Q21–Q34(14) | `/upload` v2(MultipartReader + 3 層の上限) | `step3` |
| 4 | content sniffing | Q35–Q44(10) | 偽装ファイルを弾き、保存名を自前で決める | `step4` |
| 5 | HEIC と OS 依存 | Q45–Q54(10) | HEIC 対応、3 環境で同じ拡張子 | `step5` |
| 6 | 総合 | Q55–Q60(6) | S3 保存、E2E、io.Pipe クライアント、負荷検証、設計書、LT | `v1.0` |

### 進め方のルール

1. 各問題は前問のコードを育てる。作り直さない
2. ★2 の問題は、**必ず予測を先に `notes/stepN.md` に書いてから**実行する。外れた予測こそ notes に残す
3. ★3 の問題は完了ごとに git commit する。Step の最後にタグを打つ
4. 各問題の「合格基準」をすべて満たしてから次へ進む。満たせなかった問題には印をつけ、次の Step の前に解き直す
5. 各 Step の最後に、`NOTES.md` の振り返りを書く
6. 外部ライブラリは問題文で指定したものだけを使う
7. 常に `make check` が通る状態を保つ

---

## 5. ノック一覧

### Step 1　io の契約を手で確かめる(Q1–Q10)

> 成果物:`internal/iox` パッケージ。ここで作った Reader とヘルパーは Step 3 以降で本体から使う。

**Q1 ★1｜契約どおりの Reader を自作する**
- 使う技術:`io.Reader`、`testing/iotest.TestReader`
- 作るもの:`internal/iox/chunk.go` に `ChunkReader`(元データを `chunk` バイトずつ返し、**最後の chunk で `n > 0` と `io.EOF` を同時に返す**Reader)。`len(p) == 0` なら `(0, nil)` を返す
- 合格基準:
  - [ ] `iotest.TestReader(NewChunkReader(data, 3), data)` が通る
  - [ ] 「最後の Read が `(n>0, io.EOF)` を返す」ことを直接検証するテストがある
  - [ ] `Read` の doc コメントに、`io.Reader` の契約 3 点(n>0 と err の同時返却、err より先に n を処理、`(0, nil)` の意味)を自分の言葉で書いている

**Q2 ★1｜Seek できる型・できない型を型で固定する**
- 使う技術:`io.Seeker`、`io.ReadSeeker`、`io.ReadCloser`、`*bytes.Reader`、`*os.File`、`http.Request.Body`
- 作るもの:`internal/iox/interfaces_test.go`。コンパイル時アサーション(`var _ io.ReadSeeker = (*bytes.Reader)(nil)` など)と、`httptest.NewRequest` で作った `Request.Body` を `.(io.Seeker)` で実行時アサーションするテスト
- 合格基準:
  - [ ] `*bytes.Reader`、`*os.File` が `io.ReadSeeker` を満たし、`Request.Body` は満たさないことがテストで示されている
  - [ ] `Request.Body` が Seek できない理由(実体はどの型で、どこから読んでいるか)を `$GOROOT/src/net/http/transfer.go` の該当箇所を引いてテストのコメントに書いている
  - [ ] Reader / Seeker / Closer / ReadSeeker / ReadCloser の関係を `notes/step1.md` に図(Mermaid か ASCII)で描いている

**Q3 ★2｜最後のバイトが消えるバグを再現して直す**
- 使う技術:`iotest.DataErrReader`、`iotest.OneByteReader`
- 作るもの:`internal/iox/readall.go` に `ReadAllBuggy`(`if err == io.EOF { break }` を `append` より先に書いた版)と `ReadAllFixed`。テーブル駆動テストで `strings.Reader` / `DataErrReader` / `OneByteReader` / Q1 の `ChunkReader` を通す
- 予測:どの Reader でバグ版が何バイト失うかを先に書く
- 合格基準:
  - [ ] バグ版が `DataErrReader` と `ChunkReader` で末尾を失い、`strings.Reader` では失わないことがテストで示されている
  - [ ] 修正版は全 Reader で一致する
  - [ ] 予測と結果の差分が notes にある

**Q4 ★2｜io.ReadFull と io.ErrUnexpectedEOF を正常系にする**
- 使う技術:`io.ReadFull`、`io.ReadAtLeast`、`io.ErrUnexpectedEOF`
- 作るもの:`internal/iox/head.go` に `ReadHead(r io.Reader, n int) ([]byte, error)`(最大 n バイトを読む。n 未満で EOF になっても**エラーにしない**。1 バイトも読めなければ空スライスと nil)
- 予測:100 バイトの Reader に 512 を要求したとき、`io.ReadFull` の戻り値 `(n, err)` を先に書く。`Read` を 1 回だけ呼ぶ実装との違いも
- 合格基準:
  - [ ] 0 / 100 / 512 / 1000 バイトの入力と、`OneByteReader`(1 回の Read で 512 読めない Reader)でテストしている
  - [ ] `io.ReadFull` が `io.ErrUnexpectedEOF` を返す条件を、`$GOROOT/src/io/io.go` の `ReadAtLeast` を引いてテストのコメントに書いている

**Q5 ★2｜io.LimitReader で「上限を超えたか」を判定する**
- 使う技術:`io.LimitReader`、`*io.LimitedReader`
- 作るもの:`internal/iox/atmost.go` に `ReadAtMost(r io.Reader, max int64) (data []byte, exceeded bool, err error)`。`io.LimitReader(r, max+1)` で読み、`max` を超えて読めたら `exceeded = true`
- 予測:`LimitReader(r, n)` が n バイト読み終えた後の Read の戻り値と、「元の r が本当に EOF なのか」を区別できるかを先に書く
- 合格基準:
  - [ ] 0 / max−1 / max / max+1 / max×2 バイトの境界値テストがある
  - [ ] `exceeded = true` のとき、`data` の長さが max+1 で止まっている(それ以上読んでいない)ことを、読んだバイト数を数える Reader で確認している

**Q6 ★2｜io.Copy と io.Discard で読み捨ててもメモリが増えないことを測る**
- 使う技術:`io.Copy`、`io.CopyN`、`io.Discard`、`runtime.MemStats`、`crypto/rand.Reader`
- 作るもの:`internal/iox/discard_test.go`。`io.LimitReader(rand.Reader, 200<<20)` を `io.Discard` に `io.Copy` し、前後の `HeapAlloc` の差を表示するテスト(`-short` で skip)。比較用に `io.ReadAll` した場合も測る
- 予測:`io.Copy` 版と `io.ReadAll` 版で HeapAlloc がどれだけ増えるかを先に書く
- 合格基準:
  - [ ] `io.Copy` 版の増分が数 MB 以下、`io.ReadAll` 版が 200MB 以上であることを実測の数値で notes に書いている
  - [ ] `io.Copy` が内部で使うバッファサイズを `$GOROOT/src/io/io.go` の `copyBuffer` から読み取って notes に書いている

**Q7 ★2｜io.MultiReader で先頭だけ覗いて元に戻す**
- 使う技術:`io.MultiReader`、`bytes.NewReader`
- 作るもの:`internal/iox/peek.go` に `Peek(r io.Reader, n int) (head []byte, rest io.Reader, err error)`。`ReadHead`(Q4)で先頭を読み、`io.MultiReader(bytes.NewReader(head), r)` で「読んでいないのと同じ」Reader を返す
- 予測:`rest` を `io.ReadAll` した結果が元データと一致するか、`head` が n 未満のときはどうなるかを先に書く
- 合格基準:
  - [ ] 元データが n 未満 / ちょうど / 超のケースで `rest` の全読みが元データと一致する
  - [ ] `rest` は `io.Seeker` を満たさない(型アサーションで確認)。Step 3 の Q33 で「Seek できない代わりに何が得られるか」を比較する

**Q8 ★2｜io.Pipe で書き手と読み手を goroutine で分ける**
- 使う技術:`io.Pipe`、`*io.PipeWriter.CloseWithError`
- 作るもの:`internal/iox/pipe_test.go`。書き手 goroutine が 1MB を書き、読み手が `io.Copy(io.Discard, pr)` で読むテスト。(a) 書き手を同じ goroutine で先に実行するとデッドロックする版(`t.Skip` で無効化し、コメントで理由を説明)、(b) 書き手が `CloseWithError` した場合に読み手がそのエラーを受け取る版
- 予測:(a) がどこで止まるか、(b) で `io.Copy` が返すエラーは何かを先に書く
- 合格基準:
  - [ ] (b) で読み手が書き手のエラーを `errors.Is` で判定できる
  - [ ] `io.Pipe` がバッファを持たない(書き込みは読み込みが追いつくまでブロックする)ことを `$GOROOT/src/io/pipe.go` を引いて notes に書いている

**Q9 ★3｜Seek という副作用を defer と errors.Join で管理する**
- 使う技術:`io.ReadSeeker`、`Seek(0, io.SeekStart)`、名前付き戻り値、`defer`、`errors.Join`
- 作るもの:`internal/iox/rewind.go` に `WithRewind(rs io.ReadSeeker, fn func(io.Reader) error) (err error)`。`fn` に rs を渡して読ませ、**必ず** `Seek(0, io.SeekStart)` で先頭に戻す。`fn` のエラーと Seek のエラーを `errors.Join` で合成して返す
- 合格基準:
  - [ ] `fn` が成功しても失敗しても、呼び出し後に `rs` の位置が 0 に戻っている
  - [ ] Seek が失敗する `ReadSeeker` のフェイクを使い、`fn` のエラーと Seek のエラーの**両方**が `errors.Is` で取り出せる
  - [ ] 名前付き戻り値を使わない版(`func(...) error`)では Seek のエラーが消えることを示すテストがある

**Q10 ★3｜iotest を使ったテスト基盤を整えて Step 1 を締める**
- 使う技術:`iotest.HalfReader`、`iotest.TimeoutReader`、`iotest.ErrReader`、テーブル駆動テスト
- 作るもの:`internal/iox/iox_test.go` に「Step 1 で作った全ヘルパー × iotest の各 Reader」のテーブル駆動テスト。`ErrReader` で「EOF 以外のエラーは素通しする」ことも確認
- 合格基準:
  - [ ] `make check` が通る
  - [ ] `NOTES.md` の Step 1 振り返りに「`io.Reader` の契約で一番間違えやすい点」を 3 行で書いている
  - [ ] `git tag step1`

---

### Step 2　multipart を素朴に受ける(Q11–Q20)

> 成果物:`/upload` v1(`ParseMultipartForm`)と、その弱点を観測する道具。

**Q11 ★1｜multipart/form-data を手書きして標準ライブラリにパースさせる**
- 使う技術:boundary、`Content-Disposition`(name / filename)、パートごとの `Content-Type`、終端 `--boundary--`、`multipart.NewReader`
- 作るもの:`experiments/q11_handwritten/handwritten_test.go`。テキスト 1 つとファイル 1 つを含むボディを**文字列リテラルで手書き**し、`multipart.NewReader(strings.NewReader(body), boundary)` で 2 パートが取り出せることを確認
- 合格基準:
  - [ ] `FormName()`、`FileName()`、パートの `Content-Type`、パート本文がすべて期待どおり
  - [ ] CRLF を LF にした版、終端の `--` を落とした版がどう失敗するかもテストしている

**Q12 ★1｜multipart.Writer でリクエストを組み立てるヘルパーを作る**
- 使う技術:`multipart.Writer`(`CreateFormFile`、`CreateFormField`、`CreatePart`、`FormDataContentType`)、`httptest.NewRequest`
- 作るもの:`internal/testutil/multipart.go` に `Part` 構造体(name / filename / contentType / body)と `NewMultipartRequest(t, url, parts ...Part) *http.Request`。以降のすべてのテストで使う
- 合格基準:
  - [ ] Q11 の手書きと同じ内容を `multipart.Writer` で作り、両方を `multipart.NewReader` で読んで同じ結果になる
  - [ ] `CreatePart` を使って**任意の Content-Type と Content-Transfer-Encoding** を付けられる(Q21 で使う)

**Q13 ★2｜生のリクエストを DumpRequest で見る**
- 使う技術:`httputil.DumpRequest(r, true)`、`curl -F`
- 作るもの:`experiments/q13_dump/main.go`(受けたリクエストを標準出力にダンプするサーバ)と `run.sh`(`curl -F` で 2 ファイル + テキスト 1 つを送る)
- 予測:Q11 の手書きと何が違うか(boundary の形、パートの並び順、`Content-Type` の推定、ヘッダの順序)を先に書く
- 合格基準:
  - [ ] ダンプの実出力を `notes/logs/` に保存し、予測との差分を notes に書いている
  - [ ] curl が boundary をどう決め、パートの順をどう決めるかを説明できる

**Q14 ★3｜/upload v1:ParseMultipartForm で保存する**
- 使う技術:`r.ParseMultipartForm(32 << 20)`、`r.FormFile`、`multipart.File`、`io.Copy`、`os.OpenFile(O_EXCL)`
- 作るもの:`internal/handler/handler.go`(v1)と `internal/storage/local.go`(`Local.Save(ctx, key, r io.Reader) (path, error)`)。`cmd/server` から起動。ハンドラ入口・Parse 完了・保存完了で**時刻付きログ**を出す
- 合格基準:
  - [ ] `curl -F file=@... http://localhost:8080/upload` で保存でき、`shasum -a 256` が元ファイルと一致する
  - [ ] Q12 のヘルパーと `httptest.NewRecorder` で、200 と保存内容を検証するテストがある
  - [ ] git commit

**Q15 ★2｜maxMemory の正体を一時ファイルで確かめる**
- 使う技術:`os.TempDir`、`TMPDIR`、`t.Setenv`、`t.TempDir`
- 作るもの:`internal/handler/tempfile_test.go`。`TMPDIR` を `t.TempDir()` に向け、maxMemory = 1MB で 512KB と 4MB のファイルを送り、`ParseMultipartForm` 直後に `multipart-*` ファイルがあるか・サイズはいくつかを観測
- 予測:maxMemory を超えたとき、一時ファイルに書かれるのは「超過分」か「ファイル全体」かを先に書く
- 合格基準:
  - [ ] 実測(ファイルの有無とサイズ)が notes にあり、`$GOROOT/src/mime/multipart/formdata.go` の `readForm` の該当行で説明できる
  - [ ] 「maxMemory は受信サイズの上限ではない」ことをテストが示している(maxMemory の 4 倍を送っても 200 が返る)

**Q16 ★2｜一時ファイルのライフサイクルと RemoveAll**
- 使う技術:`os.CreateTemp`、`MultipartForm.RemoveAll`、`net/http` の自動クリーンアップ
- 作るもの:Q15 のテストを拡張。(a) ハンドラ内で `RemoveAll` を呼ばない、(b) 呼ぶ、(c) `http.Server` 経由(`httptest.NewServer`)で処理が終わった後、の 3 通りで一時ファイルが残るかを観測
- 予測:3 通りそれぞれで残るかどうかを先に書く
- 合格基準:
  - [ ] `net/http` がリクエスト終了時に何を呼んでいるか(`$GOROOT/src/net/http/server.go` の `finishRequest` 付近)をソースで確認して notes に書いている
  - [ ] v1 ハンドラに `defer r.MultipartForm.RemoveAll()` を入れるべきかの判断を、テスト結果を根拠に書いている

**Q17 ★2｜FormFile の暗黙の ParseMultipartForm**
- 使う技術:`r.FormFile` の内部、`defaultMaxMemory`
- 作るもの:`experiments/q17_formfile/formfile_test.go`。`ParseMultipartForm` を呼ばずに `FormFile` を呼び、`r.MultipartForm` が設定されること、暗黙の maxMemory が何 MB かを Q15 と同じ観測方法で確かめる
- 予測:暗黙の maxMemory の値を `$GOROOT/src/net/http/request.go` で読んでから書く
- 合格基準:
  - [ ] 暗黙の値の直前・直後のサイズで一時ファイルの有無が変わることをテストで示している

**Q18 ★3｜200MB を送って v1 の弱点を観測する**
- 使う技術:`runtime.MemStats`、`ps -o rss`、`os.TempDir`、`make bigfile`
- 作るもの:`cmd/server` に `MEMSTATS=1` で `HeapAlloc` / `Sys` を 500ms ごとにログする機能。`experiments/q18_observe/run.sh` で 200MB を送りながら (1) ハンドラログの時刻、(2) メモリ、(3) `multipart-*` の出現とサイズ、を 0.5 秒ごとに記録
- 予測:「ハンドラ入口のログはいつ出るか」「HeapAlloc は maxMemory の何倍まで行くか」「一時ファイルはいつ現れ、いつ消えるか」を先に書く
- 合格基準:
  - [ ] 「受信が終わるまでハンドラの処理が進まない」「maxMemory を超えた分がディスクに書かれる」をログと数値で示せる
  - [ ] 実測の表(時刻・HeapAlloc・一時ファイルサイズ)が notes にある

**Q19 ★2｜読み取り専用ファイルシステムで v1 を動かす**
- 使う技術:`docker run --read-only --tmpfs /tmp:size=8m`、マルチステージ Dockerfile
- 作るもの:`docker/Dockerfile.readonly`(golang:1.26 でビルド → 小さな実行イメージ)と `experiments/q19_readonly/run.sh`(`--read-only` と 8MB の tmpfs で起動し、5MB と 50MB を送る)
- 予測:5MB / 50MB それぞれで何が起きるか(ステータス、エラーメッセージ)を先に書く
- 合格基準:
  - [ ] 実出力(`read-only file system` / `no space left on device` など)を notes に貼り、予測との差分を書いている
  - [ ] 「一時ファイルのディスクが実行環境に依存する」という障害シナリオを、この結果を根拠に 3 行で書いている

**Q20 ★2｜ParseMultipartForm と MultipartReader を併用するとどうなるか**
- 使う技術:`r.MultipartReader()`、`http.ErrNotMultipart`、Body は一度しか読めない
- 作るもの:`experiments/q20_bothapis/bothapis_test.go`。(a) `ParseMultipartForm` → `MultipartReader`、(b) `MultipartReader` → `ParseMultipartForm`、(c) `MultipartReader` を 2 回、のエラーを確認
- 予測:3 通りのエラーの有無とメッセージを先に書く
- 合格基準:
  - [ ] 各エラーの発生箇所を `$GOROOT/src/net/http/request.go` の `multipartReader` で示している
  - [ ] `NOTES.md` の Step 2 振り返りに「なぜ ParseMultipartForm は手軽で、何を隠しているか」を 3 行で書き、`git tag step2`

---

### Step 3　ストリーミングと多層防御(Q21–Q34)

> 成果物:`/upload` v2。受信しながら検証し、3 層の上限を持つ。

**Q21 ★1｜NextPart と NextRawPart の違いをテストで固定する**
- 使う技術:`Reader.NextPart`、`Reader.NextRawPart`、`Content-Transfer-Encoding: quoted-printable`、`Part.FormName`、`Part.FileName`
- 作るもの:`experiments/q21_nextpart/nextpart_test.go`。Q12 の `CreatePart` で `Content-Transfer-Encoding: quoted-printable` のパートを作り、`NextPart` と `NextRawPart` で本文がどう変わるかを比較
- 合格基準:
  - [ ] `NextPart` はデコードし、`NextRawPart` はしないことをテストで示している(`$GOROOT/src/mime/multipart/multipart.go` の `nextPart` を引く)
  - [ ] `FormName()` が `Content-Disposition: form-data` 以外で空になることも確認している

**Q22 ★2｜パートを読み切らずに次へ進む**
- 使う技術:`NextPart` の内部での読み飛ばし、`partReader`
- 作るもの:`experiments/q22_partialread/partialread_test.go`。3 パートのボディで、2 パート目を 10 バイトだけ読んで `NextPart` を呼び、3 パート目の内容が正しく取れるかを確認
- 予測:読み残しはどう処理されるか(捨てられる / エラー / 次のパートに混ざる)を先に書く
- 合格基準:
  - [ ] 読み残しの処理箇所を `multipart.go` の `NextPart` / `Part.Close` で示している
  - [ ] 「上限超過で残りを読まない」を実現するには何が必要か(ハンドラを抜けて接続を閉じる)を notes に書いている

**Q23 ★3｜/upload v2a:MultipartReader でストリーミング受信する**
- 使う技術:`r.MultipartReader()`、`NextPart`、`io.Copy(io.Discard, part)`
- 作るもの:`internal/handler/handler.go` を v2 に書き換える。ファイルパートは一旦読み捨ててサイズだけログ、テキストパートは値をログ。v1 は `experiments/q14_v1/` に退避して残す
- 合格基準:
  - [ ] Q18 と同じ 200MB 送信で、ハンドラのログが**送信開始直後**に出始める(時刻差を notes に)
  - [ ] Q14 のテストが v2 でも通る(保存は一旦しないので、保存の検証だけ外す)
  - [ ] git commit

**Q24 ★3｜ReadAndValidateSize(自前ループ版)**
- 使う技術:`io.Reader` の契約(Q1 / Q3)、`bytes.Buffer`、`*bytes.Reader`
- 作るもの:`internal/limit/limit.go` に `ReadAndValidateSize(r io.Reader, bufSize int, maxSize int64) (io.ReadSeeker, error)`。バッファは 1 回だけ確保し、累積が maxSize を超えた**瞬間**に `ErrSizeExceeded` を返し、残りを読まない
- 合格基準:
  - [ ] Q3 のバグを踏んでいない(`DataErrReader` と `ChunkReader` でテスト)
  - [ ] 0 / max−1 / max / max+1 の境界値テストがある
  - [ ] 上限超過時に読んだバイト数が max+bufSize 以下であることを、読んだ量を数える Reader で確認している

**Q25 ★2｜スライドの擬似コードを、テストで反証する**
- 使う技術:`testing.AllocsPerRun`、`iotest`
- 作るもの:`internal/limit/review_test.go`。スライドの擬似コード(ループ内で `make`、EOF 判定が `append` の前、など)をそのまま `slideVersion` として実装し、(a) `AllocsPerRun` で確保回数の差、(b) `DataErrReader` で末尾欠落、(c) EOF 以外のエラーの扱い、を Q24 版と比較するテスト
- 予測:改善点を 3 つ以上先に書く
- 合格基準:
  - [ ] 3 つ以上の改善点が、それぞれテストの失敗または数値の差として示されている

**Q26 ★3｜LimitReader 版へのリファクタ**
- 使う技術:`io.LimitReader(r, maxSize+1)`、`io.ReadAll`(Q5)
- 作るもの:`internal/limit/limit.go` に `ReadAndValidateSizeLimit(r io.Reader, maxSize int64) (io.ReadSeeker, error)`。Q24 版と**同じ結果**になることをテーブル駆動テストで示す
- 合格基準:
  - [ ] 境界値(0 / max−1 / max / max+1)× `iotest` の各 Reader で、両実装の戻り値(データ・エラー)が一致する
  - [ ] ハンドラ v2 でこちらを採用し、`ErrSizeExceeded` を 413 に変換する。git commit

**Q27 ★3｜http.MaxBytesReader でリクエスト全体に上限を付ける**
- 使う技術:`http.MaxBytesReader`、`*http.MaxBytesError`、`errors.As`、`Server.MaxHeaderBytes`
- 作るもの:ハンドラ先頭に `r.Body = http.MaxBytesReader(w, r.Body, 16<<20)`。`errors.As` で判定し 413。`experiments/q27_maxbytes/` に「`MaxHeaderBytes` はヘッダだけ、Body には既定の上限が無い」ことを示すテスト(1KB のヘッダ制限で 100MB の Body が通る)
- 合格基準:
  - [ ] 17MB のリクエストで 413、`MaxBytesError` が `NextPart` 経由でも `part.Read` 経由でも拾える
  - [ ] 超過時にサーバが接続をどう扱うか(`$GOROOT/src/net/http/request.go` の `maxBytesReader.Read`)を notes に書いている
  - [ ] git commit

**Q28 ★2｜どちらの上限が先に効くか**
- 使う技術:Q12 のヘルパー、Q26 / Q27 の上限
- 作るもの:`internal/handler/limits_test.go`。全体 16MB・パート 15MB で (a) 10MB × 2、(b) 15.5MB × 1、(c) 14MB × 1 + 3MB のテキストパート、(d) 15MB ちょうど × 1、を送る
- 予測:各ケースのステータスと、どの上限が効くかを先に書く
- 合格基準:
  - [ ] 予測と結果の表が notes にあり、(d) の結果を「boundary とヘッダも全体上限に含まれる」ことで説明できる(境界のバイト数を実測する)

**Q29 ★3｜パート数の上限:組み込みと自前**
- 使う技術:`GODEBUG=multipartmaxparts`、`ReadForm` の `maxParts`、`multipart.ErrMessageTooLarge`
- 作るもの:`experiments/q29_maxparts/maxparts_test.go`。1001 パートのリクエストを生成し、(a) `ParseMultipartForm` 経路、(b) `MultipartReader` 経路、(c) `GODEBUG=multipartmaxparts=100` を付けた子プロセス、で上限が効くかを確認。効かない経路のために、ハンドラ v2 に `MaxParts`(既定 1000)を実装し、超過で 400
- 予測:(a)(b)(c) それぞれの結果と、`t.Setenv("GODEBUG", ...)` がテスト内で効くかを先に書く
- 合格基準:
  - [ ] 結果の表と、上限が適用されている関数(`formdata.go`)の行を notes に書いている
  - [ ] `MaxParts=3` で 4 パートを送ると 400 になるテストがある。git commit

**Q30 ★3｜エラーを HTTP ステータスに変換する層を作る**
- 使う技術:`errors.Is`、`errors.As`、`fmt.Errorf("%w")`、`http.ErrNotMultipart`、`http.ErrMissingBoundary`
- 作るもの:`internal/handler/errors.go` に `classify(err) (status int, msg string)`。413(全体 / パート / テキスト 64KB)、415(Step 4 で使う `sniff.ErrNotAllowed` の枠)、400(multipart 構文・パート数・非 multipart)、500(それ以外)。テキストパートにも 64KB の上限を付ける
- 合格基準:
  - [ ] `classify` のテーブル駆動テストで、ラップされたエラー(`fmt.Errorf("part %q: %w", ...)`)でも正しく分類される
  - [ ] クライアントに返すメッセージに内部パスやスタックが含まれない

**Q31 ★3｜io.ReadSeeker に変換する理由を、S3 SDK のシグネチャで確認する**
- 使う技術:`*bytes.Reader`、Q9 の `WithRewind`、aws-sdk-go-v2 の `PutObjectInput.Body`
- 作るもの:`internal/limit` の戻り値が `*bytes.Reader` であることを型で固定するテスト。`go doc github.com/aws/aws-sdk-go-v2/service/s3 PutObjectInput` と `go doc .../feature/s3/manager Uploader.Upload` で Body の型を確認し、notes に「MIME 判定で巻き戻すため」「SDK が何を要求するか」の 2 点を書く
- 合格基準:
  - [ ] SDK の Body の型と、Seek できない Reader を渡した場合に SDK がどうするか(`go mod download` 済みのソースを `grep` して該当箇所を引く)が notes にある
  - [ ] ハンドラ v2 が「上限検証 → `*bytes.Reader`」の形になっている

**Q32 ★4｜メモリの最悪見積もりと、セマフォによる同時処理数の制限**
- 使う技術:バッファ付き chan または `golang.org/x/sync/semaphore`、`http.Handler` ミドルウェア、`context`
- 作るもの:`internal/middleware/concurrency.go` に `Limit(n int, next http.Handler) http.Handler`(満杯なら 503、または `ctx` が切れるまで待つ)。`cmd/server` で `MAX_CONCURRENT_UPLOADS` から適用
- 合格基準:
  - [ ] 「同時 100 リクエスト × 各 5 ファイル × 15MB」の最悪メモリの見積もり式と、対策 3 案(セマフォ / 一時ファイル / 先頭だけバッファ + `io.MultiReader`)の比較表が notes にある
  - [ ] ミドルウェアのテスト(n=2 で 3 本同時に投げ、3 本目が 503 または待機する)がある。git commit

**Q33 ★4｜どこまでが「ストリーミング」か:MultiReader 版と比較する**
- 使う技術:Q7 の `Peek`、`io.MultiReader`、`io.Copy`
- 作るもの:`experiments/q33_streaming/` に「先頭 4KB だけバッファし、残りを `io.MultiReader` で保存先に流す」プロトタイプ。同じ 15MB を (a) 現行(フルバッファ)、(b) プロトタイプ、で処理したときの `HeapAlloc` ピークを測る
- 合格基準:
  - [ ] 「受信と検証はストリーミング / 検証後の保持はフルバッファ」を段階ごとの表で区別し、(b) を採用しなかった理由(保存中に上限超過が判明する、Seek できない、リトライ不可)を実測と合わせて notes に書いている

**Q34 ★3｜Step 3 の締め:上限まわりの E2E テストとタグ**
- 使う技術:`httptest.NewServer`、Q12 のヘルパー
- 作るもの:`internal/handler/handler_test.go` を整理し、正常 / 全体超過 / パート超過 / テキスト超過 / パート数超過 / 非 multipart の 6 ケースを 1 つのテーブルにまとめる
- 合格基準:
  - [ ] `make check` が通る
  - [ ] `NOTES.md` の Step 3 振り返りを「疑ったこと / 確かめたこと / 選んだ設計」で書き、`git tag step3`

---

### Step 4　content sniffing(Q35–Q44)

> 成果物:偽装ファイルを弾き、保存名を自前で決める `/upload` v3。

**Q35 ★1｜自己申告は信用できないことを、失敗するテストで示す**
- 使う技術:`curl -F "file=@x;type=image/png;filename=cute.png"`、Q12 の `Part.contentType`
- 作るもの:`internal/handler/sniff_test.go` に「MZ ヘッダの中身を `filename=cute.png; type=image/png` で送ると v2 が 200 を返してしまう」テスト(この時点では**わざと失敗させる**。Q41 で通す)
- 合格基準:
  - [ ] クライアントが拡張子と Content-Type の両方を自由に設定できることを curl の例と一緒に notes に書いている
  - [ ] テストは Q41 まで `t.Skip("Q41 で通す")` にしてコミットする

**Q36 ★2｜testdata を生成し、マジックナンバーを xxd で確認する**
- 使う技術:`image/png`、`image/jpeg`(`image.NewRGBA` + `Encode`)、`archive/zip`(docx の最小構成)、`encoding/binary`、`xxd`
- 作るもの:`cmd/genfixtures/main.go`。正常な png / jpg / pdf(最小のテキスト)、MZ ヘッダを `.png` にリネームした偽装、pdf を `.jpg` にリネームした偽装、docx、空ファイル、10 バイト、PNG 署名 8 バイトだけ、を `testdata/` に生成(`make fixtures`)。HEIC は Q45 で追加
- 予測:各ファイルの先頭 16 バイトを先に書く
- 合格基準:
  - [ ] `xxd testdata/* | head` の実出力を `notes/logs/` に保存し、PNG / JPEG / PDF / MZ の先頭バイトを表にしている
  - [ ] `docx` が zip として `[Content_Types].xml` と `word/document.xml` を持っている

**Q37 ★2｜http.DetectContentType の出力を表にする**
- 使う技術:`http.DetectContentType`、`net/http/sniff.go` の `sniffLen`
- 作るもの:`experiments/q37_detect/detect_test.go`。空スライス / プレーンテキスト / MZ / PNG 先頭 8B / 1KB の PNG / 先頭 512B を超える位置にだけ署名があるデータ、の戻り値をテーブル駆動テストで固定
- 予測:6 ケースの戻り値を先に書く
- 合格基準:
  - [ ] 何バイトまで見るかを `sniff.go` の該当行で確認し、テストの 6 番目のケースで「513 バイト目以降は見ない」ことを示している

**Q38 ★3｜DetectValidMimeType(標準ライブラリ版)**
- 使う技術:Q4 の `ReadHead`、Q9 の `WithRewind`、`http.DetectContentType`、`slices.Contains`
- 作るもの:`internal/sniff/sniff.go` に `DetectValidMimeType(rs io.ReadSeeker, allowed []string) (mt string, err error)`。先頭 512B を読み、判定し、許可リストと照合し、**呼び出し後は必ず先頭に戻す**。許可外は `ErrNotAllowed`
- 合格基準:
  - [ ] 100 バイトのファイル(`io.ErrUnexpectedEOF`)と空ファイルを正常系として扱う
  - [ ] 呼び出し後の `Seek(0, io.SeekCurrent)` が 0 であるテストと、Seek が失敗するフェイクでエラーが `errors.Is` で取り出せるテストがある
  - [ ] git commit

**Q39 ★2｜巻き戻しを忘れると保存ファイルが壊れる**
- 使う技術:`crypto/sha256`、`image.Decode`
- 作るもの:`experiments/q39_noseek/noseek_test.go`。Q38 から defer の巻き戻しを消した版で PNG を「判定 → 保存」し、保存結果のハッシュと `image.Decode` の結果を確認
- 予測:保存されたファイルの長さ、ハッシュの一致/不一致、`image.Decode` のエラー内容を先に書く
- 合格基準:
  - [ ] 先頭 512B が欠けたファイルになることを、長さ・ハッシュ不一致・デコードエラーの 3 点で示している

**Q40 ★2｜名前付き戻り値と defer:握りつぶされるエラー**
- 使う技術:名前付き戻り値、`defer`、shadowing(`:=`)
- 作るもの:`experiments/q40_namedreturn/namedreturn_test.go`。(a) 名前付き戻り値 + defer で err を上書きできる、(b) 名前なしだと Seek のエラーが消える、(c) defer 内で `_, err := rs.Seek(...)` と書くと shadowing で消える、の 3 つを示す
- 予測:(c) がコンパイルエラーになるか、通るか、通るなら何が起きるかを先に書く
- 合格基準:
  - [ ] 3 ケースがテストで示され、`go vet` が (c) を検出するかどうかも確認して notes に書いている

**Q41 ★3｜偽装ファイルのテーブル駆動テストと、ハンドラへの組み込み**
- 使う技術:Q36 の testdata、`sniff.ErrNotAllowed` → 415
- 作るもの:`internal/sniff/sniff_test.go`(受理:png / jpg / pdf、拒否:MZ→.png / docx / 空 / 10B / テキスト)と、ハンドラ v3(上限検証の後に `DetectValidMimeType`)。Q35 の Skip を外す
- 合格基準:
  - [ ] Q35 のテストが通る(偽装 PNG が 415)
  - [ ] 「pdf を `.jpg` で送る」ケースが**受理される**こと(中身が許可リスト内)と、レスポンスの `mime_type` が `application/pdf` であることをテストしている
  - [ ] git commit

**Q42 ★4｜許可リストの限界と image.DecodeConfig**
- 使う技術:`image.DecodeConfig`、`image/png`、`image/jpeg`、`_ "image/..."` の登録
- 作るもの:`internal/sniff/validate.go` に `ValidateImage(rs io.ReadSeeker, mt string) error`(jpeg / png はヘッダをデコードして検証、他は no-op、巻き戻しは Q9 のヘルパー)。ハンドラに組み込み、「PNG 署名 8 バイトだけ」のファイルを 415 にする
- 合格基準:
  - [ ] 許可リスト vs 拒否リストの比較表と、sniffing だけでは防げない攻撃(先頭だけ正しい / polyglot / decompression bomb)と対策(DecodeConfig / フルデコード / 再エンコード / 外部ツール)のコスト比較表が notes にある
  - [ ] 「先頭だけ正しい PNG」が `DetectValidMimeType` を通り `ValidateImage` で止まるテストがある。git commit

**Q43 ★4｜保存名は自分で決める:Part.FileName とパストラバーサル**
- 使う技術:`Part.FileName()` の `filepath.Base`、`crypto/rand`、`encoding/hex`、`os.O_EXCL`
- 作るもの:`experiments/q43_filename/filename_test.go`(`../../etc/passwd`、`..\..\x.png`、`evil.exe.png`、`写真 1.jpg`、`dir/` を `FileName()` に通した結果の表)。ハンドラ v3 の保存名を `rand 16B hex + 拡張子`(拡張子は Step 5 まで固定表)にし、`storage.Local.Save` で key に `/` `\` `..` が含まれていたら拒否
- 合格基準:
  - [ ] `FileName()` が何をサニタイズし何をしないかの表が notes にあり、`multipart.go` の該当行を引いている
  - [ ] クライアント申告名がレスポンスの `original_name` に**だけ**現れ、保存パスには使われないテストがある。git commit

**Q44 ★3｜Step 4 の締め:Seek の副作用を 1 行ずつ説明する**
- 作るもの:`internal/sniff` の `DetectValidMimeType` に、巻き戻しまわりの各行の「なぜ」をコメントで書く。`NOTES.md` の Step 4 振り返りに、そのコメントを 1 行ずつ抜き出して理由を書く
- 合格基準:
  - [ ] `make check` が通る
  - [ ] `git tag step4`

---

### Step 5　HEIC と OS 依存(Q45–Q54)

> 成果物:HEIC を受理し、どの OS イメージでも同じ拡張子で保存する `/upload` v4。

**Q45 ★2｜HEIC を標準で判定すると何になるか**
- 使う技術:ISOBMFF の `ftyp` ボックス、`encoding/binary`、`http.DetectContentType`
- 作るもの:`cmd/genfixtures` に HEIC(offset 4 に `ftyp`、major brand `heic`)を追加。iPhone の実物があればそれも `testdata/` に置く。`experiments/q45_heic/heic_test.go` で `DetectContentType` の結果を確認。同じ構造で brand を `mp42` に変えた場合も
- 予測:HEIC と mp42 の結果を先に書く
- 合格基準:
  - [ ] HEIC が `application/octet-stream` になり、それが fallback であることを `sniff.go` の該当行で示している
  - [ ] HEIC の判定材料が先頭ではなく offset 4 にあることを `xxd` の出力で示している

**Q46 ★2｜sniff.go の対応表を読んで、対応・非対応をテストで表にする**
- 使う技術:`net/http/sniff.go` の `sniffSignatures`、WHATWG mimesniff、golang/go #52144
- 作るもの:`experiments/q46_sniffsig/sniffsig_test.go`。`sniffSignatures` に載っている形式(png / jpeg / gif / webp / mp4 / pdf / zip など)と載っていない形式(heic / avif など)の先頭バイトを並べ、判定結果をテーブルで固定。issue #52144 を `gh api` か WebFetch で読み、Go チームが HEIC を入れない理由を notes に
- 予測:表の各行の結果を先に書く
- 合格基準:
  - [ ] 対応表がハードコードで OS に依存しないこと(ファイルを読む処理が無いこと)をソースで示している
  - [ ] 「mimesniff はブラウザ間で判定をそろえる仕様で、ブラウザで扱わない形式は対象外」を issue の引用つきで書いている

**Q47 ★3｜mimetype ライブラリへの置き換え**
- 使う技術:`github.com/gabriel-vasile/mimetype`(`Detect`、`MIME.Is`、`SetLimit`)、`mime.ParseMediaType`
- 作るもの:`internal/sniff` の判定を `mimetype.Detect` に置き換え(標準版は `detectStd` として残す)。ライブラリが既定で何バイト見るかをソースで確認し、512B / 既定値それぞれを渡したときに判定が変わる形式が無いか `testdata` 全部で実験
- 合格基準:
  - [ ] Q41 のテストに HEIC(受理、`image/heic`)を足して全部通る
  - [ ] 渡すバイト数を決めた根拠(実験の表)が notes にある。git commit

**Q48 ★2｜mime.ExtensionsByType は環境で変わる:プローブを作る**
- 使う技術:`mime.ExtensionsByType`、`mime.TypeByExtension`、`runtime.GOOS`、`mime/type_unix.go` が参照するファイル群
- 作るもの:`cmd/extprobe/main.go`。GOOS、`type_unix.go` が読むファイルの存在有無、`ExtensionsByType`(heic / heif / jpeg / png / pdf / plain)、`TypeByExtension`(.heic / .heif / .jpg / .jpeg / .png / .pdf)を表(Markdown)で出力
- 予測:macOS・素の alpine・debian でそれぞれ `ExtensionsByType("image/heic")` が何を返すかを先に書く
- 合格基準:
  - [ ] macOS での実出力が notes にあり、alpine / debian の予測とその根拠(どのファイルを読むか)が書いてある

**Q49 ★2｜mime パッケージの初期化を図にし、組み込み表をテストで固定する**
- 使う技術:`mime/type.go` の `builtinTypesLower`、`type_unix.go` の `initMimeUnix`、`sync.Once`
- 作るもの:`experiments/q49_builtin/builtin_test.go`。組み込み表に**ある**拡張子(.png / .jpg / .pdf)と**無い**拡張子(.heic)を `TypeByExtension` で確認(この環境では OS の表が混ざるので、Q50 のコンテナ内でも同じテストを走らせる)。読み込み順(globs2 系 → 見つかったら終了 → 無ければ mime.types 系)の図を notes に
- 合格基準:
  - [ ] 図に「globs2 系が見つかったらそこで止まり、無いときだけ mime.types 系を読む」が表現され、`type_unix.go` の該当行を引いている

**Q50 ★3｜3 つのイメージで同じテストを走らせる**
- 使う技術:マルチステージ Dockerfile、`go test -c`、`CGO_ENABLED=0`、alpine / debian の `media-types`(または `mime-support`)/ `shared-mime-info`
- 作るもの:`docker/Dockerfile.alpine`、`docker/Dockerfile.debian-mimetypes`、`docker/Dockerfile.debian-sharedmime`、`docker/run-matrix.sh`(3 イメージをビルドし、Q48 のプローブと Q49 のテストバイナリを実行して結果を 1 つの表に)。`make mime-matrix` で再現
- 合格基準:
  - [ ] 3 環境の表が notes にあり、Q48 の予測と照合している
  - [ ] `make mime-matrix` 1 コマンドで再現できる

**Q51 ★2｜heif と heic のずれを、コンテナの中で grep する**
- 使う技術:`/usr/share/mime/globs2`、`/etc/mime.types`、`grep`
- 作るもの:`run-matrix.sh` に、各イメージ内で `heic` / `heif` を含む行を両ファイルから grep して出力する処理を追加
- 予測:globs2 と mime.types で `.heic` の対応先(`image/heic` か `image/heif` か)を先に書く
- 合格基準:
  - [ ] Q50 の `ExtensionsByType("image/heic")` が nil になる環境を、このずれで説明できている
  - [ ] golang/go #22318 と #52065 を読み、「必要最小限だけ内蔵し、残りは OS に委ねる」方針と、mailcap / mime.types / shared-mime-info の年表(誰と合意するための表か)を notes に書いている

**Q52 ★3｜案2:mime.AddExtensionType を init で登録して、効かない経路を再現する**
- 使う技術:`mime.AddExtensionType`、パッケージ初期化順、import 経路、テストバイナリと本番バイナリの違い
- 作るもの:`experiments/q52_addextension/`。`heicreg` パッケージの `init` で `.heic → image/heic` を登録し、(a) main から import した場合、(b) していない場合、(c) テストバイナリから見た場合、(d) `ExtensionsByType` の結果に登録がどう追記されるか、をプログラムとテストで再現
- 合格基準:
  - [ ] 初期化順や配置の問題を 1 つ以上再現し、`type.go` の `AddExtensionType` / `setExtensionType` / `once.Do(initMime)` を引いて理由を書いている

**Q53 ★3｜GetExtensionByMimeType:固定表で決定的にする**
- 使う技術:`mime.ParseMediaType`、固定の `map`、決定的なフォールバック(最短 → 辞書順)
- 作るもの:`internal/mimeext/mimeext.go` に `GetExtensionByMimeType(mt string) (string, error)`。許可リストの 4 型は固定表、それ以外は `ExtensionsByType` から決定的に 1 つ選ぶか `ErrUnknownMimeType`。スライドのコード(`[]string` を `(string, error)` として返していて型が合わない)をどう直したかを notes に
- 合格基準:
  - [ ] Q50 の 3 環境すべてで heic / jpeg / png / pdf の拡張子が同じ(テストバイナリを 3 環境で実行)
  - [ ] `; charset=utf-8` 付きの型でも動く。ハンドラ v4 で採用し git commit

**Q54 ★4｜3 案の比較と、ExtensionsByType("image/jpeg") の環境差**
- 作るもの:`run-matrix.sh` に `ExtensionsByType("image/jpeg")` の**個数と順序**を出す処理を追加。案1(イメージに MIME DB を焼く)/ 案2(AddExtensionType)/ 案3(OS 非依存の表)を、環境依存・依存ライブラリ・保守性・テストしやすさの軸で比較表にする
- 合格基準:
  - [ ] 個数と順序の実測が 3 環境分あり、「保存時の拡張子が常に同じになる設計」を実装(Q53)と結びつけて書いている
  - [ ] 比較表の根拠として Q50 と Q52 の実験結果を引いている
  - [ ] `NOTES.md` の Step 5 振り返りに「OS の知識は OS に訊く」を採用しなかった理由を 3 行で書き、`git tag step5`

---

### Step 6　総合(Q55–Q60)

> 成果物:S3 保存、E2E、ストリーミングクライアント、負荷検証、設計書、LT。

**Q55 ★3｜MinIO に保存する**
- 使う技術:`compose.yaml`(用意済み、`make minio-up`)、aws-sdk-go-v2(`config.LoadDefaultConfig`、`s3.NewFromConfig` + `BaseEndpoint` + path-style、`PutObject`、`GetObject`)
- 作るもの:`internal/storage/s3.go`。`STORAGE=s3` で切替。`s3_test.go` は `S3_ENDPOINT` が無ければ skip、あれば PutObject → GetObject でハッシュ一致を確認(`make test-s3`)
- 合格基準:
  - [ ] `STORAGE=s3 make run` → curl で jpg と heic を上げ、MinIO コンソール(または `mc ls`)で**判定した拡張子**のオブジェクトが見える
  - [ ] Q31 で確認した「SDK は Reader / ReadSeeker のどちらを求めるか」を、実装時に `ContentLength` を渡す理由と合わせて notes に書いている。git commit

**Q56 ★3｜E2E テスト**
- 使う技術:`httptest.NewServer`、Q12 のヘルパー、`testdata`
- 作るもの:`e2e/upload_test.go`。正常(jpg + png + pdf + heic + テキスト → 200、拡張子・MIME・ハッシュ)/ 偽装(MZ→.png、docx → 415)/ 壊れた画像(PNG 署名だけ → 415)/ パート上限超過(413)/ 全体上限超過(413)/ パート数過多(400)/ 非 multipart(400)
- 合格基準:
  - [ ] `go test ./...` 1 回ですべて通り、各ケースのステータスとレスポンス JSON を検証している

**Q57 ★3｜クライアント側もストリーミングする**
- 使う技術:`io.Pipe`(Q8)、`multipart.Writer`、goroutine、`runtime.MemStats`
- 作るもの:`cmd/client/main.go`。`io.Pipe` の書き手 goroutine で `multipart.Writer` に `io.Copy(part, file)` し、読み手を `http.Post` の Body に渡す。`-memstats` で送信側のピークを表示。サーバ側は Q18 の `MEMSTATS=1`
- 合格基準:
  - [ ] 200MB を送ったときの送信側・受信側それぞれの `HeapAlloc` ピークを数値で notes に書いている(受信側は全体上限を一時的に上げるか、上限超過で 413 が返るまでのピークを測る)
  - [ ] `io.ReadAll` でファイルを読んで送る版との差も測っている

**Q58 ★4｜負荷検証:Q32 の対策の効果を測る**
- 使う技術:`hey`(`brew install hey`)または `cmd/loadtest/main.go`(Go 製、`-c` 同時数 / `-n` 総数、ステータス分布と p50 / p95 / p99)、`MAX_CONCURRENT_UPLOADS`、`/debug/pprof`
- 作るもの:同時 100 リクエスト(各 5 ファイル × 3MB)を、セマフォなし / あり(10)で実行し、`HeapAlloc` / `Sys` のピークとレイテンシを表に
- 合格基準:
  - [ ] Q32 の見積もりと実測の比較、対策前後の表(またはグラフ)が notes にある
  - [ ] 選んだ同時数の根拠が書いてある

**Q59 ★4｜設計書を 1 ページで書く**
- 作るもの:`docs/design.md`。構成は「疑ったこと / 確かめたこと / 選んだ設計」。各項目に根拠となる問題番号と実測を添える
- 合格基準:
  - [ ] スライドと異なる判断をした箇所(例:LimitReader 版、4096B 読み取り、`DecodeConfig`、固定表、テキストパート上限)に、その理由が明記されている
  - [ ] 再設計が必要になる閾値(ファイル上限、許可リストの拡張、ベースイメージ変更)が書いてある

**Q60 ★4｜卒業試験:標準ライブラリをどこまで信じるか**
- 作るもの:`docs/lt.md`(5 分の LT 資料)。「信じた箇所」と「信じなかった箇所」を、自分の実装の関数名と実測の数値で列挙する
- 合格基準:
  - [ ] スライドを見ずに、io / mime / multipart の設計判断を自分の実装に基づいて誰かに説明できる
  - [ ] `make check` が通り、`git tag v1.0`

---

## 6. マイルストーン

| マイルストーン | 完了条件 | git タグ |
|---|---|---|
| M1 | Step 1 完了:`internal/iox` と iotest によるテスト基盤 | `step1` |
| M2 | Step 2 完了:v1 の弱点を 200MB と read-only で観測済み | `step2` |
| M3 | Step 3 完了:ストリーミングと 3 層の上限、セマフォ | `step3` |
| M4 | Step 4 完了:偽装ファイルを弾き、保存名を自前で決める | `step4` |
| M5 | Step 5 完了:HEIC 対応、3 環境で同じ拡張子 | `step5` |
| M6 | Step 6 完了:S3 保存・E2E・負荷検証・設計書・LT | `v1.0` |

## 7. 参考資料

- スライド本体(上記 URL)
- Go 公式ドキュメント:`io`、`testing/iotest`、`net/http`(ParseMultipartForm、MultipartReader、DetectContentType、MaxBytesReader)、`mime`(ExtensionsByType、AddExtensionType)、`mime/multipart`、`image`
- Go のソース:`io/io.go`、`io/pipe.go`、`net/http/request.go`、`net/http/sniff.go`、`net/http/transfer.go`、`mime/type.go`、`mime/type_unix.go`、`mime/multipart/multipart.go`、`mime/multipart/formdata.go`
- golang/go issues:#22318、#52065、#52144、#46578
- WHATWG MIME Sniffing Standard
- freedesktop.org shared-mime-info specification
- Debian media-types(mime.types)
- github.com/gabriel-vasile/mimetype
- aws-sdk-go-v2 `service/s3`、`feature/s3/manager`
