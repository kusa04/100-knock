# migrations

goose 形式の SQL マイグレーションを置きます(問題 30 から)。

```bash
make migrate-create name=create_tasks   # 新規ファイルの作成
make migrate                            # 開発 DB へ適用
make migrate-test                       # テスト DB へ適用
make migrate-status
```
