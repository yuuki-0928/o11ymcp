//go:build integration

// Ingest の結合テスト。実際に ClickHouse コンテナを起動し、
// このディストリビューションの Collector を (サブプロセスではなく) プロセス内で起動して
// OTLP/HTTP でログを送信し、ClickHouse に反映されることを確認する。
// 通常の `go test ./...` では実行されない (integration ビルドタグ)。
// 実行するには: go test -tags=integration .
package main

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	_ "github.com/ClickHouse/clickhouse-go/v2"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/confmap"
	fileprovider "go.opentelemetry.io/collector/confmap/provider/fileprovider"
	"go.opentelemetry.io/collector/otelcol"

	tcclickhouse "github.com/testcontainers/testcontainers-go/modules/clickhouse"
)

// otlpHTTPPort はテスト内でのみ使うポート。本番設定 (otelcol-config.yaml) の
// 4318 とは意図的に変え、他のテスト/手動起動中のIngestと衝突しないようにする。
const otlpHTTPPort = "14318"

func TestIngest_OTLPLogsReachClickHouse(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ctr, err := tcclickhouse.Run(ctx, "clickhouse/clickhouse-server:23.3.8.21-alpine")
	if err != nil {
		t.Fatalf("failed to start clickhouse container: %v", err)
	}
	t.Cleanup(func() {
		if err := ctr.Terminate(context.Background()); err != nil {
			t.Logf("failed to terminate clickhouse container: %v", err)
		}
	})

	chHost, err := ctr.ConnectionHost(ctx)
	if err != nil {
		t.Fatalf("failed to get clickhouse host: %v", err)
	}

	// otelcol-config.yaml と同じ構成 (otlp -> batch -> clickhouse) を、
	// テスト用のポート・接続先で書き出す。create_schema:true なので
	// テーブルはこのテストの実行時に自動作成される。
	configYAML := fmt.Sprintf(`
receivers:
  otlp:
    protocols:
      http:
        endpoint: 0.0.0.0:%s
processors:
  batch:
    timeout: 100ms
exporters:
  clickhouse:
    endpoint: tcp://%s
    username: %s
    password: %s
    database: %s
    create_schema: true
service:
  pipelines:
    logs:
      receivers: [otlp]
      processors: [batch]
      exporters: [clickhouse]
`, otlpHTTPPort, chHost, ctr.User, ctr.Password, ctr.DbName)

	configPath := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(configPath, []byte(configYAML), 0o644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	set := otelcol.CollectorSettings{
		BuildInfo: component.BuildInfo{Command: "o11y-mcp-ingest-test"},
		Factories: components,
		ConfigProviderSettings: otelcol.ConfigProviderSettings{
			ResolverSettings: confmap.ResolverSettings{
				URIs:              []string{"file:" + configPath},
				ProviderFactories: []confmap.ProviderFactory{fileprovider.NewFactory()},
			},
		},
	}

	col, err := otelcol.NewCollector(set)
	if err != nil {
		t.Fatalf("failed to create collector: %v", err)
	}

	runErrCh := make(chan error, 1)
	go func() { runErrCh <- col.Run(ctx) }()
	t.Cleanup(func() {
		col.Shutdown()
		if err := <-runErrCh; err != nil {
			t.Logf("collector run returned: %v", err)
		}
	})

	waitForState(t, col, otelcol.StateRunning)

	sendOTLPLog(t)

	dsn := fmt.Sprintf("clickhouse://%s:%s@%s/%s", ctr.User, ctr.Password, chHost, ctr.DbName)
	db, err := sql.Open("clickhouse", dsn)
	if err != nil {
		t.Fatalf("failed to open clickhouse: %v", err)
	}
	defer db.Close()

	assertLogArrives(t, db)
}

// waitForState は Collector が指定した状態になるまで短い間隔でポーリングする。
// 起動処理 (設定の解決・コンポーネントの起動) は非同期に進むため、
// 固定 sleep ではなく状態を見て待つほうが速く・安定してテストできる。
func waitForState(t *testing.T, col *otelcol.Collector, want otelcol.State) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if col.GetState() == want {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("collector did not reach state %v within timeout (current: %v)", want, col.GetState())
}

func sendOTLPLog(t *testing.T) {
	t.Helper()
	nowNanos := time.Now().UnixNano()
	body := fmt.Sprintf(`{
		"resourceLogs": [{
			"resource": {"attributes": [{"key": "service.name", "value": {"stringValue": "checkout"}}]},
			"scopeLogs": [{
				"scope": {},
				"logRecords": [{
					"timeUnixNano": "%d",
					"severityText": "ERROR",
					"body": {"stringValue": "ingest integration test log"}
				}]
			}]
		}]
	}`, nowNanos)

	url := "http://localhost:" + otlpHTTPPort + "/v1/logs"
	resp, err := http.Post(url, "application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatalf("failed to POST otlp log: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("otlp log POST returned status %d", resp.StatusCode)
	}
}

// assertLogArrives は batchprocessor のタイムアウト分の遅延があるため、
// 固定 sleep ではなく一定回数ポーリングして確認する。
func assertLogArrives(t *testing.T, db *sql.DB) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	var body, serviceName string
	for time.Now().Before(deadline) {
		row := db.QueryRow("SELECT Body, ServiceName FROM otel_logs LIMIT 1")
		if err := row.Scan(&body, &serviceName); err == nil {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	if body != "ingest integration test log" || serviceName != "checkout" {
		t.Fatalf("otel_logs row = (body=%q, serviceName=%q), want (ingest integration test log, checkout)", body, serviceName)
	}
}
