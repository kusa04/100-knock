# 進捗

各ノックの合格基準をすべて満たし、`make check` が通ったらチェックを付けてください。予測・結果・根拠は `notes/stepN.md` に、Step の振り返りは `NOTES.md` に残します。


## Step 1(Q1–Q10): io の契約を手で確かめる

- [ ] Q01 ★1｜契約どおりの Reader を自作する
- [ ] Q02 ★1｜Seek できる型・できない型を型で固定する
- [ ] Q03 ★2｜最後のバイトが消えるバグを再現して直す
- [ ] Q04 ★2｜io.ReadFull と io.ErrUnexpectedEOF を正常系にする
- [ ] Q05 ★2｜io.LimitReader で「上限を超えたか」を判定する
- [ ] Q06 ★2｜io.Copy と io.Discard で読み捨ててもメモリが増えないことを測る
- [ ] Q07 ★2｜io.MultiReader で先頭だけ覗いて元に戻す
- [ ] Q08 ★2｜io.Pipe で書き手と読み手を goroutine で分ける
- [ ] Q09 ★3｜Seek という副作用を defer と errors.Join で管理する
- [ ] Q10 ★3｜iotest を使ったテスト基盤を整えて Step 1 を締める
- [ ] Step 1 の振り返り(NOTES.md)と `git tag step1`

## Step 2(Q11–Q20): multipart を素朴に受ける

- [ ] Q11 ★1｜multipart/form-data を手書きして標準ライブラリにパースさせる
- [ ] Q12 ★1｜multipart.Writer でリクエストを組み立てるヘルパーを作る
- [ ] Q13 ★2｜生のリクエストを DumpRequest で見る
- [ ] Q14 ★3｜/upload v1:ParseMultipartForm で保存する
- [ ] Q15 ★2｜maxMemory の正体を一時ファイルで確かめる
- [ ] Q16 ★2｜一時ファイルのライフサイクルと RemoveAll
- [ ] Q17 ★2｜FormFile の暗黙の ParseMultipartForm
- [ ] Q18 ★3｜200MB を送って v1 の弱点を観測する
- [ ] Q19 ★2｜読み取り専用ファイルシステムで v1 を動かす
- [ ] Q20 ★2｜ParseMultipartForm と MultipartReader を併用するとどうなるか
- [ ] Step 2 の振り返り(NOTES.md)と `git tag step2`

## Step 3(Q21–Q34): ストリーミングと多層防御

- [ ] Q21 ★1｜NextPart と NextRawPart の違いをテストで固定する
- [ ] Q22 ★2｜パートを読み切らずに次へ進む
- [ ] Q23 ★3｜/upload v2a:MultipartReader でストリーミング受信する
- [ ] Q24 ★3｜ReadAndValidateSize(自前ループ版)
- [ ] Q25 ★2｜スライドの擬似コードを、テストで反証する
- [ ] Q26 ★3｜LimitReader 版へのリファクタ
- [ ] Q27 ★3｜http.MaxBytesReader でリクエスト全体に上限を付ける
- [ ] Q28 ★2｜どちらの上限が先に効くか
- [ ] Q29 ★3｜パート数の上限:組み込みと自前
- [ ] Q30 ★3｜エラーを HTTP ステータスに変換する層を作る
- [ ] Q31 ★3｜io.ReadSeeker に変換する理由を、S3 SDK のシグネチャで確認する
- [ ] Q32 ★4｜メモリの最悪見積もりと、セマフォによる同時処理数の制限
- [ ] Q33 ★4｜どこまでが「ストリーミング」か:MultiReader 版と比較する
- [ ] Q34 ★3｜Step 3 の締め:上限まわりの E2E テストとタグ
- [ ] Step 3 の振り返り(NOTES.md)と `git tag step3`

## Step 4(Q35–Q44): content sniffing

- [ ] Q35 ★1｜自己申告は信用できないことを、失敗するテストで示す
- [ ] Q36 ★2｜testdata を生成し、マジックナンバーを xxd で確認する
- [ ] Q37 ★2｜http.DetectContentType の出力を表にする
- [ ] Q38 ★3｜DetectValidMimeType(標準ライブラリ版)
- [ ] Q39 ★2｜巻き戻しを忘れると保存ファイルが壊れる
- [ ] Q40 ★2｜名前付き戻り値と defer:握りつぶされるエラー
- [ ] Q41 ★3｜偽装ファイルのテーブル駆動テストと、ハンドラへの組み込み
- [ ] Q42 ★4｜許可リストの限界と image.DecodeConfig
- [ ] Q43 ★4｜保存名は自分で決める:Part.FileName とパストラバーサル
- [ ] Q44 ★3｜Step 4 の締め:Seek の副作用を 1 行ずつ説明する
- [ ] Step 4 の振り返り(NOTES.md)と `git tag step4`

## Step 5(Q45–Q54): HEIC と OS 依存

- [ ] Q45 ★2｜HEIC を標準で判定すると何になるか
- [ ] Q46 ★2｜sniff.go の対応表を読んで、対応・非対応をテストで表にする
- [ ] Q47 ★3｜mimetype ライブラリへの置き換え
- [ ] Q48 ★2｜mime.ExtensionsByType は環境で変わる:プローブを作る
- [ ] Q49 ★2｜mime パッケージの初期化を図にし、組み込み表をテストで固定する
- [ ] Q50 ★3｜3 つのイメージで同じテストを走らせる
- [ ] Q51 ★2｜heif と heic のずれを、コンテナの中で grep する
- [ ] Q52 ★3｜案2:mime.AddExtensionType を init で登録して、効かない経路を再現する
- [ ] Q53 ★3｜GetExtensionByMimeType:固定表で決定的にする
- [ ] Q54 ★4｜3 案の比較と、ExtensionsByType("image/jpeg") の環境差
- [ ] Step 5 の振り返り(NOTES.md)と `git tag step5`

## Step 6(Q55–Q60): 総合

- [ ] Q55 ★3｜MinIO に保存する
- [ ] Q56 ★3｜E2E テスト
- [ ] Q57 ★3｜クライアント側もストリーミングする
- [ ] Q58 ★4｜負荷検証:Q32 の対策の効果を測る
- [ ] Q59 ★4｜設計書を 1 ページで書く
- [ ] Q60 ★4｜卒業試験:標準ライブラリをどこまで信じるか
- [ ] Step 6 の振り返り(NOTES.md)と `git tag v1.0`

## マイルストーン

| M | 完了条件 | git タグ | 状態 |
| --- | --- | --- | --- |
| M1 | Step 1 完了:`internal/iox` と iotest によるテスト基盤 | `step1` | |
| M2 | Step 2 完了:v1 の弱点を 200MB と read-only で観測済み | `step2` | |
| M3 | Step 3 完了:ストリーミングと 3 層の上限、セマフォ | `step3` | |
| M4 | Step 4 完了:偽装ファイルを弾き、保存名を自前で決める | `step4` | |
| M5 | Step 5 完了:HEIC 対応、3 環境で同じ拡張子 | `step5` | |
| M6 | Step 6 完了:S3 保存・E2E・負荷検証・設計書・LT | `v1.0` | |
