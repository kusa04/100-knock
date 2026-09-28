# NOTES.md:各 Step の振り返り

各ノックの予測・結果・根拠は `notes/stepN.md` に書きます(書式は [notes/README.md](./notes/README.md))。ここには各 Step の振り返りだけを書きます。

| Step | 範囲 | ファイル | git タグ |
| --- | --- | --- | --- |
| 1 | Q1–Q10 io の契約を手で確かめる | [notes/step1.md](./notes/step1.md) | `step1` |
| 2 | Q11–Q20 multipart を素朴に受ける | [notes/step2.md](./notes/step2.md) | `step2` |
| 3 | Q21–Q34 ストリーミングと多層防御 | [notes/step3.md](./notes/step3.md) | `step3` |
| 4 | Q35–Q44 content sniffing | [notes/step4.md](./notes/step4.md) | `step4` |
| 5 | Q45–Q54 HEIC と OS 依存 | [notes/step5.md](./notes/step5.md) | `step5` |
| 6 | Q55–Q60 総合 | [notes/step6.md](./notes/step6.md) | `v1.0` |

## Step 1 の振り返り

`io.Reader` の契約で一番間違えやすい点を 3 行で。

-
-
-

## Step 2 の振り返り

なぜ ParseMultipartForm は「手軽」なのか、そして何を隠しているのかを 3 行で。

-
-
-

## Step 3 の振り返り

「疑ったこと / 確かめたこと / 選んだ設計」を自分の実装に置き換えて。

- 疑ったこと:
- 確かめたこと:
- 選んだ設計:

## Step 4 の振り返り

「Seek という副作用」を管理するために書いたコードを、1 行ずつ理由つきで。

## Step 5 の振り返り

スライドの結論「OS の知識は OS に訊く」を、自分のサービスではなぜ採用しなかったかを 3 行で。

-
-
-

## Step 6

設計書(Q59)と LT 資料(Q60)の置き場所:`docs/design.md`、`docs/lt.md`
