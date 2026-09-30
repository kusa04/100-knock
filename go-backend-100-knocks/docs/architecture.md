# 目標アーキテクチャ

100 問を終えた時点のディレクトリ構成と、各レイヤーの責務です。問題を進める中で迷ったらここへ戻ってください。親リポジトリの `usecase` / `gateway` / `adapter` / `app` / `worker` と対応する構造になっています。

## ディレクトリ構成(最終形)

```
go-backend-100-knocks/
├── cmd/
│   ├── server/          # HTTP + gRPC サーバー(問題 11, 52)
│   ├── worker/          # 非同期ジョブワーカー(問題 69)
│   └── taskctl/         # gRPC クライアント CLI(問題 57)
├── app/                 # 設定、Wire による DI、起動処理(問題 20, 62)
├── domain/
│   ├── task/            # Task 集約、Status、Tags、ドメインイベント(問題 1〜)
│   ├── project/         # Project 集約、採番(問題 40, 60)
│   ├── member/          # Member、Role(問題 42, 50)
│   └── event/           # DomainEvent インターフェース(問題 23)
├── usecase/
│   ├── port/            # Repository / Query / 外部サービスの interface(問題 7, 29)
│   ├── task/            # Interactor(問題 12, 28)
│   ├── query/           # 読み取り専用ユースケース(問題 29)
│   └── mock/            # mockgen 生成物(問題 27)
├── adapter/
│   └── gateway/
│       ├── memory/      # インメモリ実装(問題 7)
│       ├── db/          # PostgreSQL 実装、UnitOfWork(問題 31〜)
│       ├── storage/     # S3 互換オブジェクトストレージ(問題 67)
│       └── webhook/     # 外部 HTTP クライアント(問題 24, 43)
├── gateway/
│   ├── http/            # REST handler、middleware、DTO(問題 11〜)
│   └── rpc/             # gRPC handler、interceptor、convert(問題 52〜)
├── worker/              # ジョブ定義、enqueue / execute(問題 68〜)
├── problems/            # 問題集(01〜04)
├── proto/               # .proto(問題 51)
├── gen/                 # protoc 生成物
├── migrations/          # goose SQL マイグレーション(問題 30〜)
├── pkg/
│   ├── apperr/          # アプリケーションエラー(問題 4)
│   ├── clock/           # Clock 抽象(問題 5)
│   ├── id/              # ID 生成(問題 6)
│   ├── validate/        # 入力検証(問題 13)
│   ├── log/             # slog ヘルパー(問題 19)
│   ├── tenant/          # テナント context(問題 40)
│   ├── auth/            # Principal、API キー、JWT(問題 41, 80)
│   ├── collection/      # ジェネリクスユーティリティ(問題 22)
│   └── ...
├── testutil/            # ビルダー、dbtest、golden(問題 21, 32)
├── docs/                # ADR、性能記録、運用手順(問題 58, 89, 97)
├── docker-compose.yml
├── Makefile
└── go.mod
```

## レイヤーの責務と依存の向き

```
gateway (http / rpc) ──▶ usecase ──▶ domain
        │                   │
        │                   ▼
        │              usecase/port (interface)
        │                   ▲
        ▼                   │ 実装
   adapter/gateway (db / storage / webhook / memory)
        ▲
        │ 組み立て
       app (wire)
```

| レイヤー | 置くもの | 置かないもの |
| -------- | -------- | ------------ |
| domain | エンティティ、値オブジェクト、状態遷移ルール、ドメインイベント、ドメインエラー | DB、HTTP、JSON タグ、ログ、時刻の直接取得(`time.Now()`)|
| usecase | ユースケース単位の手続き、トランザクション境界、認可判定の呼び出し、port の利用 | SQL、HTTP ステータス、proto 型 |
| usecase/port | Repository / Query / 外部サービスの interface | 実装 |
| adapter/gateway | port の実装(SQL、S3、HTTP クライアント)、外部エラー → apperr 変換 | 業務ルール |
| gateway/http, gateway/rpc | リクエスト・レスポンス変換、認証スコープ解決、apperr → ステータス変換、middleware | 業務ルール、SQL |
| app | 設定読み込み、DI、サーバー起動、シャットダウン順序 | 業務ロジック |
| worker | ジョブの型定義、enqueue、execute、再試行ポリシー | HTTP |

依存の向きは問題 26 でテストとして固定し、問題 74 で `depguard` により lint でも検査します。

## テストの約束

- サブテスト名は「〜を確認」「〜こと」のシナリオ形式。
- `t.Parallel()` を使うテストは、共有状態(map、グローバル変数、DB の同じ行)を持たない。
- テーブル駆動テストでは `for _, tt := range tests { t.Run(tt.name, func(t *testing.T) { ... }) }` の形を守り、ケースごとに独立した入力を用意する。
- DB テストは `TEST_DATABASE_URL` が無ければ `t.Skip`。各テストはトランザクション内で実行して最後にロールバックする(問題 32)。
- モックは port(interface)に対してのみ生成する。具象型をモックしない。
- 時刻と ID は `clock.Clock` / `id.Generator` 経由で注入し、テストでは固定値を使う。

## エラーの流れ

```
domain error (ErrInvalidTransition など)
   │  usecase / adapter で apperr.Kind を付与して wrap
   ▼
apperr.Error{Kind, Msg, Err, Fields}
   │  gateway/http: Kind → HTTP status + problem+json
   │  gateway/rpc : Kind → codes.Code + errdetails
   ▼
クライアント(内部メッセージは漏らさない)
```
