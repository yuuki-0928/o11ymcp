# 設計書：o11y-mcp

| 項目 | 内容 |
| :--- | :--- |
| 対象 | [`docs/requirements.md`](./requirements.md) (v1.0 確定版) |
| 位置付け | 全体像が分かる粒度の概略設計。**詳細な実装判断・理由はコード内コメントで補う**方針とする |
| 構成図 | [`docs/architecture.drawio`](./architecture.drawio) (draw.io / https://app.diagrams.net で開く) |

---

## 1. 設計方針

- 本書は「大まかな構成と利用サービスが分かること」を目的とし、詳細な実装レベルの判断はコードコメントに委ねる。
- 極力、実績のある OSS コンポーネントを**組み合わせて**実現し、自前実装の範囲を最小化する (= FR-OSS-01/02, FR-MON-01/02 が求める「運用の楽さ」に直結する)。
- 3つの自作サービス (Ingest / Query API / MCP Server) は **Go** に統一する。理由:
  - OpenTelemetry Collector 本体・公式 SDK・ClickHouse ドライバ・MCP 公式 Go SDK が揃っており、自作コードを増やさずに済む。
  - 単一バイナリで軽量コンテナ化でき、コスト目標 (NFR-C-03, 月額5,000円以下) に有利。
  - 言語・Lint・CI テンプレートを1本化でき、コントリビュータのオンボーディングコストが下がる (FR-OSS-01)。

---

## 2. システム構成

### 2.1 コンポーネント一覧と役割

| コンポーネント | 役割 | 実装方針 |
| :--- | :--- | :--- |
| **Ingest** (`apps/ingest`) | OTLP (HTTP/gRPC) を受信し ClickHouse に永続化する | 自作コードではなく、**OpenTelemetry Collector Builder (OCB)** でカスタムビルドした Collector distribution。`receiver/otlpreceiver` (公式) + `exporter/clickhouseexporter` (collector-contrib 公式) を組み合わせる |
| **Query API** (`apps/query`) | ClickHouse に対する検索・集計を REST で公開する | Go 製カスタム実装 (net/http)。ClickHouse スキーマは Ingest (clickhouseexporter) が作るテーブルにそのまま乗る |
| **MCP Server** (`apps/mcp`) | LLM/エージェント向けに `search_logs` 等のツールを MCP (Streamable HTTP) で公開する | Go 製カスタム実装。MCP 公式 Go SDK を使用し、内部で Query API を HTTP 呼び出しする (FR-MC-03) |
| **ClickHouse** | テレメトリの列指向ストレージ。Hot/Cold Tiered Storage | OSS ClickHouse をコンテナで実行。Azure では Blob を Cold Tier に接続 |
| **Grafana** (任意・開発用) | ClickHouse データソースでの可視化 | 自作 UI を作らず Grafana の ClickHouse datasource プラグインで代替 (要件 4章 UI 方針) |

### 2.2 データの流れ

1. 計装対象アプリ (OTel SDK) → (任意で OTel Collector 経由) → **Ingest** に OTLP/HTTP または OTLP/gRPC で送信。
2. Ingest (`clickhouseexporter`) が OTel データモデルのまま ClickHouse に書き込む。
3. **Query API** が ClickHouse に SQL を発行し、REST (JSON) で検索結果を返す。
4. **MCP Server** が LLM からのツール呼び出しを受け、Query API を HTTP 経由で呼び出し、整形して LLM に返す。
5. **Grafana** は ClickHouse に直接接続し、人間向けの可視化を提供する (Query API を経由しない)。

### 2.3 認証

- Ingest / Query API / MCP Server はいずれも `Authorization: Bearer <API_KEY>` を要求する (NFR-S-02, v1.0 は API Key のみ)。
- API Key はコンテナの環境変数 (`INGEST_API_KEY` 等) または Azure Key Vault (Managed Identity 経由) から供給する。

---

## 3. 利用サービス・技術スタック

| 区分 | 選定 | 選定理由 |
| :--- | :--- | :--- |
| 言語 (Ingest/Query/MCP) | Go 1.23+ | OTel/ClickHouse/MCP のエコシステムが揃い、軽量・単一バイナリ |
| Ingest 実装基盤 | OpenTelemetry Collector Builder (OCB) | 自前で OTLP 受信を実装せず、公式実装を再利用 (保守負荷の最小化) |
| Ingest エクスポータ | `clickhouseexporter` (collector-contrib) | OTel データモデルに沿った ClickHouse スキーマが既に用意されている |
| データストア | ClickHouse (Apache-2.0) | 列指向 + 高圧縮、Tiered Storage で Hot/Cold 分離可能 (FR-ST-02/03) |
| Cold Tier (Azure) | Azure Blob Storage (Cool/Cold, LRS, Japan East) | GB単価最小、実質無制限 (FR-ST-05/07) |
| Cold Tier (Compose) | MinIO (S3互換) | ローカル/オンプレでも同じ `s3` disk 設定を使い回せる |
| MCP SDK | MCP 公式 Go SDK | Streamable HTTP に対応 (FR-MC-04) |
| シークレット管理 (Azure) | Key Vault + Managed Identity | コードにシークレットを埋め込まない (NFR-S-03) |
| コンピュート (Azure) | Azure Container Apps (Consumption) | Scale-to-Zero でアイドルコストを抑制 (NFR-C-01) |
| コンテナオーケストレーション (IaaS) | Docker Compose | 一次ターゲット。`docker compose up` 一発 (FR-OP-01) |
| IaC (Azure) | Bicep (モジュール化) + azd | `azd up` 一発デプロイ (FR-OP-02/03) |
| 自己 o11y (Azure) | Application Insights (Basic) | 無償枠優先 (NFR-C-01) |
| 自己 o11y (Compose) | Prometheus + Grafana | 追加コストなしで同梱可能 |
| 可視化 | Grafana (ClickHouse datasource プラグイン) | 自作 UI を持たない方針 |
| CI/CD | GitHub Actions | OSS 前提、無償枠あり |
| 依存関係監視 | Dependabot (GitHub 標準機能) | 追加サービス不要でリポジトリに組み込める。Renovate より運用がシンプル (FR-MON-01) |
| 上流仕様監視 | GitHub Actions (週次 schedule) + カスタムスクリプト | OTel/MCP のリリースを確認し Issue を自動起票 (FR-MON-02) |
| リリース自動化 | GitHub Actions (タグ push トリガー) + `goreleaser` | SemVer/CHANGELOG/GHCR push を一本化 (FR-OSS-02) |
| ライセンス | Apache-2.0 | 企業利用がしやすく OTel エコシステムと整合 |

---

## 4. データモデル概要

- Ingest (`clickhouseexporter`) が作成する標準テーブル群をそのまま利用する (概略):
  - `otel_traces` — Trace ID / Span ID / 親 Span ID / 時刻 / 属性 (Map) / resource 属性
  - `otel_logs` — 時刻 / 重大度 / body / トレース相関 (Trace ID / Span ID) / resource 属性
  - `otel_metrics_gauge` / `otel_metrics_sum` / `otel_metrics_histogram` — メトリクス種別ごとのテーブル
- TTL (FR-ST-04) は ClickHouse の `TTL` 句 + Tiered Storage 移送ルールをマイグレーション SQL (`apps/ingest/schema/`) で管理し、既定値 (Hot 7日 / Cold 30日 / 削除) をコード内コメントで根拠付きで説明する。
- Query API はこれらのテーブルに対する読み取り専用の SQL を発行する。書き込みは Ingest のみが行う (責務分離)。

---

## 5. API / MCP インターフェース概要

### 5.1 Query API (REST, JSON)
| メソッド | パス | 概要 | 対応要件 |
| :--- | :--- | :--- | :--- |
| GET | `/v1/logs` | 期間・サービス名・重大度・属性で検索 (ページング付き) | FR-QR-01, FR-QR-04 |
| GET | `/v1/traces/{traceId}` | Trace ID から span 一覧を取得 | FR-QR-02 |
| GET | `/v1/metrics` | メトリクスの時系列取得 (集計関数指定) | FR-QR-03 |
| GET | `/v1/services` | 既知のサービス一覧 | UC-03 補助 |

### 5.2 MCP tools
| tool | 概要 | 内部呼び出し |
| :--- | :--- | :--- |
| `search_logs` | 条件を指定してログを検索 | `GET /v1/logs` |
| `get_trace` | Trace ID からトレース全体を取得 | `GET /v1/traces/{traceId}` |
| `list_services` | サービス一覧を取得 | `GET /v1/services` |
| `query_metrics` | メトリクスを時系列取得 | `GET /v1/metrics` |

各 tool の入出力スキーマ・ページング/トリム仕様 (FR-MC-05) は実装時にコード内コメント (JSON Schema 定義箇所) で詳細化する。

---

## 6. リポジトリ構成

```
o11y-mcp/
├── apps/
│   ├── ingest/       # OCB manifest + 生成物 + clickhouseexporter設定 + 結合テスト + Dockerfile
│   ├── query/        # Go製 REST API (Query API) + Dockerfile
│   └── mcp/          # Go製 MCP Server (Streamable HTTP) + Dockerfile
├── infra/
│   └── azure/
│       ├── main.bicep          # azdエントリポイント (サブスクリプションスコープ)
│       ├── main.parameters.json
│       ├── clickhouse/         # Azure向けClickHouseカスタムイメージ (Dockerfile + storage.xml)
│       └── modules/            # network / monitoring / security / storage / containerapps
├── deploy/
│   └── compose/
│       ├── docker-compose.yml
│       ├── clickhouse/config.d/    # Hot/Cold storage policy (MinIO向け)
│       ├── clickhouse-init/        # TTL/storage_policy migration (起動時に自動適用)
│       ├── minio-init/             # コールド用バケットの初期化
│       └── grafana/provisioning/   # ClickHouseデータソース自動登録
├── azure.yaml         # azd プロジェクト定義
├── docs/              # 要件定義書・設計書・構成図
├── .github/
│   ├── workflows/     # CI, リリース自動化, 依存/上流監視 (未整備)
│   └── ISSUE_TEMPLATE/ (未整備)
├── CONTRIBUTING.md (未整備)
├── CODE_OF_CONDUCT.md (未整備)
├── SECURITY.md (未整備)
├── LICENSE (Apache-2.0, 未整備)
└── README.md (JP) / README.en.md (EN)
```

---

## 7. CI/CD・監視パイプライン概要

| ワークフロー | トリガー | 内容 | 対応要件 |
| :--- | :--- | :--- | :--- |
| `ci.yml` | push / PR | lint / test / build / (main branch は) イメージ push to GHCR | FR-OP-07 |
| `release.yml` | タグ push (`v*`) | SemVer 検証 → CHANGELOG 生成 → GHCR へバージョンタグ付きイメージ push → GitHub Release 作成 | FR-OSS-02 |
| `dependabot.yml` (設定ファイル) | 週次 | 依存パッケージの脆弱性/更新を検知し PR 自動作成 | FR-MON-01 |
| `upstream-watch.yml` | 週次 schedule | OTel / MCP 仕様の最新リリースを取得し、現行採用バージョンと差分があれば Issue を自動起票 | FR-MON-02 |

---

## 8. 実装順序 (TDD)

依存関係が少なく、テストしやすい順に実装する。

1. **Query API** — 純粋な Go ロジック (ハンドラ + ClickHouse クエリ層) から着手。ClickHouse への依存はリポジトリ層でインターフェース化し、ユニットテストではモック、結合テストでは実 ClickHouse (testcontainers) を使う。 ✅ 実装済み
2. **MCP Server** — Query API の HTTP クライアントをインターフェース化し、tool ハンドラをユニットテスト。 ✅ 実装済み
3. **Ingest** — OCB manifest によるビルド確認 + 結合テスト (OTLP 送信 → ClickHouse 反映を testcontainers で検証)。 ✅ 実装済み
4. **IaC (Compose → Bicep)** — アプリが動く状態を確認してからデプロイ資材を整備。 ✅ Compose実装済み・Bicep実装済み (詳細は §9)
5. **OSS 運用資材・監視パイプライン** — 最後に CONTRIBUTING / テンプレート / Dependabot / upstream-watch を整備。 ✅ 実装済み (ただしCI/リリース/upstream-watchの各ワークフローはリモートpush後の実行確認が未実施、§9.3同様の既知の制約)

---

## 9. Azure (Bicep + azd) 実装メモ

`infra/azure/` の実装にあたり、設計時点では想定していなかった制約がいくつか見つかったため、実際の対応を記録する。

### 9.1 モジュール構成
`main.bicep` (サブスクリプションスコープ、リソースグループ作成) から以下をこの順で呼び出す:

| モジュール | 責務 |
| :--- | :--- |
| `modules/monitoring.bicep` | Log Analytics Workspace + Application Insights |
| `modules/network.bicep` | VNet + Container Apps用サブネット (delegation付き) |
| `modules/security.bicep` | Key Vault (RBAC認可) + User Assigned Managed Identity + シークレット3種 + ロール割り当て |
| `modules/storage.bicep` | Storage Account (Blob=コールド層 / Files=ClickHouseホット永続ボリューム) |
| `modules/containerapps.bicep` | Container Apps環境 + clickhouse/ingest/query/mcpの4アプリ |

### 9.2 ClickHouseのAzure向けカスタムイメージ
Container Apps にはDocker Composeのようなホストファイルのボリュームマウント手段がないため、
`deploy/compose/clickhouse/config.d/storage.xml` (MinIO/S3向け) と同じ役割の設定を
`infra/azure/clickhouse/storage.xml` (Azure Blob Storage向け、`type: azure_blob_storage`) として用意し、
`infra/azure/clickhouse/Dockerfile` で公式ClickHouseイメージに焼き込む。
アカウントキー等の秘密情報はファイルに含めず、ClickHouse公式の `from_env` 属性で
Container Appの環境変数から実行時に注入する (§4のTTL方針と同様、ボリューム名を
組み込みdefaultポリシーと同じ `default` にする必要がある点も compose 版と共通)。

### 9.3 既知の制約・follow-up
- **OTLP/gRPC (FR-IN-02, Should) はAzure版では未対応。** Container AppsのConsumptionプラン環境では
  1アプリにつき1つのingress (targetPort) しか持てず、複数ポート公開 (`additionalPortMappings`) は
  ワークロードプロファイル環境限定のため。OTLP/HTTP (FR-IN-01, Must) のみ公開している。
- **TTL/ストレージポリシーのmigration (`deploy/compose/clickhouse-init/` 相当) はAzure版で自動化していない。**
  Ingestがテーブルを自動作成した後、`deploy/compose/clickhouse-init/apply-storage-policy.sql` と同内容を
  手動で (`az containerapp exec` 等から) 一度適用する必要がある。自動化するには Container Apps Jobs
  ([https://learn.microsoft.com/azure/container-apps/jobs]) の追加が候補だが、スコープ超過のため見送った。
- **自己o11y (Application Insights) は接続文字列の配線のみ。** Ingest/Query/MCPのアプリコード自体はまだ
  OTel SDKで自身を計装していないため、`APPLICATIONINSIGHTS_CONNECTION_STRING` は現状未使用。
- **検証状況**: `az bicep build` によるコンパイル確認 (警告2件のみ、エラーなし) までは実施。
  実サブスクリプションへの `az deployment sub what-if` 以降 (実リソース作成を伴わない検証) は
  Azure CLI再認証待ちの状態で本書更新時点では未実施。実際の `azd up` によるデプロイ・動作確認は
  さらに別途、費用が発生することを確認の上で実施する。
