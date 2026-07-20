# コントリビューションガイド

o11y-mcp への貢献に興味を持っていただきありがとうございます。このドキュメントは、
初めてこのリポジトリに触れる方が迷わず開発を始められるようにするためのものです
(FR-OSS-01: コントリビュータ向けオンボーディング整備)。

## プロジェクト構成

このリポジトリは Go の Workspace 単位ではなく、**独立した3つの Go モジュール**で構成されています
(それぞれ単一の Container として個別にデプロイされるため)。

| ディレクトリ | 役割 |
| :--- | :--- |
| `apps/ingest` | OTLP受信 (OpenTelemetry Collector Builderによるカスタムディストリビューション) |
| `apps/query` | Query API (Go, REST) |
| `apps/mcp` | MCP Server (Go, Streamable HTTP) |

設計の背景は [`docs/design.md`](./docs/design.md)、要件は [`docs/requirements.md`](./docs/requirements.md) を参照してください。

## ローカル開発環境のセットアップ

### 必要なもの

- Go 1.26 以降
- Docker Desktop (Docker Compose、および `apps/query`/`apps/ingest` の結合テストで使用)

### ビルド・テスト

各モジュールは個別に `cd` してから操作します。

```bash
cd apps/query   # または apps/mcp, apps/ingest
go build ./...
go vet ./...
go test ./...
```

`apps/query` と `apps/ingest` には、実際にDockerでClickHouseコンテナを起動する結合テストが
`integration` ビルドタグの下にあります (Docker Desktopの起動が必要)。

```bash
go test -tags=integration ./...
```

### アプリを実際に動かして確認する

`deploy/compose/` の Docker Compose で全コンポーネントを一括起動できます。

```bash
cd deploy/compose
cp .env.example .env   # API Key等を必ず変更する
docker compose up -d --build
```

停止・破棄:

```bash
docker compose down -v
```

## コーディング方針

- **テスト駆動開発 (TDD)** を前提とします。新しい振る舞いを追加するときは、まず失敗するテストを書いてから実装してください (Red → Green → Refactor)。
- コメントは「何をしているか」ではなく「なぜそうしたか」(設計判断・トレードオフ・既知の制約の理由)を書いてください。既存コードのコメントの厚さがその基準です。
- 一つのPRは一つの関心事に絞ってください (機能追加とリファクタリングを混ぜない)。

## プルリクエストの出し方

1. `main` からブランチを切る。
2. 変更に対応するテストを追加・更新する。
3. `go vet ./...` と `go test ./...` (該当モジュール) がパスすることを確認する。
4. PRの説明に、変更の**理由** (なぜ必要か) と、動作確認の方法を書く。
5. CI (`.github/workflows/ci.yml`) がグリーンになったらレビューを依頼する。

## Issue の立て方

バグ報告・機能要望は [Issue テンプレート](.github/ISSUE_TEMPLATE/) を使ってください。
再現手順・期待する挙動・実際の挙動をできるだけ具体的に書いていただけると調査が早くなります。

セキュリティに関する報告は [`SECURITY.md`](./SECURITY.md) の手順に従ってください
(通常のIssueには登録しないでください)。

## ライセンス

貢献していただいたコードは [Apache License 2.0](./LICENSE) の下でライセンスされます。
