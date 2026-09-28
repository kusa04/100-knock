# kusa-self-study

## Goバックエンド100本ノック

初学者が、100問を通してこのリポジトリのバックエンド開発に必要な基礎体力を身につけるための実装教材です。題材は汎用的な `Task` API に統一し、親リポジトリ固有の業務知識は一切問わない構成にしています。

## 到達目標

100問を終えた時点で、次を自力で進められる状態を目指します。

- Goのコードを読み、構造体・メソッド・interface・エラー・`context`を適切に使える
- テーブル駆動テスト、モック、DBテスト、race detectorを使って変更を検証できる
- HTTPクライアントとHTTP/gRPC APIを実装し、入力検証とエラー変換ができる
- domain / usecase / port / adapter / handler の責務を分けられる
- SQL、トランザクション、CQRS、テナント分離を安全に実装できる
- Wire、protobuf、非同期ジョブ、CSV、オブジェクトストレージを扱える
- 小さな仕様から縦に1本、設計・実装・テスト・品質確認まで完遂できる

これは実務能力を保証する試験ではありません。実リポジトリの仕様、レビュー、障害対応は別途経験が必要ですが、「どこを読み、何を確認し、どう実装するか」が分かるところまでを狙います。

## 使い方

このリポジトリは学習専用の独立したGo moduleです。未完成の練習コードが実務リポジトリのビルドを壊しません。

```bash
cd go-backend-100-knocks
make check
make run
```

初回の環境構築(Docker、ツール、動作確認)は [docs/setup.md](./docs/setup.md) を参照してください。

各問は前問までのコードを育てる形式です。別解を試したい場合は、問題開始前に自分でコミットしてください。

1. 問題の「実装」を読む
2. 先に失敗するテストを書く(指定がある問題は必須)
3. 最小限のコードでテストを通す
4. 「完了条件」をすべて確認する
5. `make check` を実行し、[PROGRESS.md](./PROGRESS.md) にチェックする

答えが分からないときは、順に「用語を調べる → コンパイルエラーを読む → 小さなテストを書く → 親リポジトリで同じ技術の例を `rg` で探す」と進めてください。

## 問題一覧

| 範囲                                        | レベル     | 主題                                                   |
| ------------------------------------------- | ---------- | ------------------------------------------------------ |
| [1–25](./go-backend-100-knocks/problems/01-beginner.md)           | 初級       | Go文法、テスト、JSON、HTTP API、入力検証               |
| [26–50](./go-backend-100-knocks/problems/02-intermediate.md)      | 中級       | レイヤー分離、CQRS、モック、外部HTTP、トランザクション |
| [51–75](./go-backend-100-knocks/problems/03-intermediate-plus.md) | 中級プラス | protobuf/gRPC、SQL、Wire、CSV、非同期ジョブ            |
| [76–100](./go-backend-100-knocks/problems/04-advanced.md)         | 上級       | 並行処理、セキュリティ、可観測性、性能、総合実装       |

100問で1つのコードベースを育てます。最終的に何が出来上がるか、その先に何を作れるかは [docs/final-app.md](./docs/final-app.md) にまとめています。目標とするディレクトリ構成とレイヤーの責務は [docs/architecture.md](./docs/architecture.md) を参照してください。

## 全問共通ルール

- 問題文にない機能を先回りして作らない。
- エラーを捨てない。意図的に無視する場合は理由をコメントする。
- テストはサブテストを使い、`シナリオ` 形式で命名する(例: `フィールド参照で全ての値が取得できることを確認`)。
- テーブル駆動テストは、並行実行してもデータが衝突しないようにする。
- HTTPレスポンスボディ、DB rows、ファイルなどのリソースを確実に閉じる。
- ユーザー入力をSQL文字列へ連結しない。値はプレースホルダへバインドする。
- ログへ認証情報やリクエスト本文を無条件に出さない。
- handlerには変換、認証スコープ解決、エラー変換だけを置き、ルールはusecaseまたはdomainへ置く。

## 各レベルの卒業判定

各25問の最後はチェックポイントです。直前の問題を見返さず、READMEに書かれた受け入れ条件だけで実装できたら合格です。詰まった箇所は「知らなかった」「知っていたが使えなかった」「確認漏れ」のいずれかをメモしてください。

100問終了後は、親リポジトリで次の順に読み歩くと学習内容を実務へ接続しやすくなります。

1. `usecase/AGENTS.md` と小さなusecase
2. `gateway/rpcs/AGENTS.md` と対応するhandler
3. `adapter/gateway/db/AGENTS.md` とQuery/Repository
4. `app/AGENTS.md` と `app/wire.go`
5. `worker/AGENTS.md` とenqueue/executeの組

業務仕様を覚える前に、まず「各レイヤーが何を担当するか」「入力がどこを通るか」「どこで安全性を担保するか」を追ってください。

## Go ファイルアップロード 60 本ノック

GoCon のスライド「900 アプリを支えるプラットフォームへのファイルアップロード導入から学ぶ io, mime, multipart」を題材に、`io` / `mime` / `multipart` の挙動と設計判断を再現する 60 問です。独立した Go module [gocon-60-knock](./gocon-60-knock/) にまとめています。

- 問題集: [gocon-60-knock/problems/60-knocks.md](./gocon-60-knock/problems/60-knocks.md)
- 回答・実験ログ: [gocon-60-knock/NOTES.md](./gocon-60-knock/NOTES.md)
- 進捗: [gocon-60-knock/PROGRESS.md](./gocon-60-knock/PROGRESS.md)

```bash
cd gocon-60-knock
make check
make run
```
