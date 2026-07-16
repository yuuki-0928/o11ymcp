//go:build integration

// このファイルは実際に Docker で ClickHouse コンテナを起動する結合テストのため、
// 通常の `go test ./...` では実行されないよう `integration` ビルドタグを付けている。
// 実行するには: go test -tags=integration ./internal/store/...
// (Docker Desktop が起動している必要がある)
package store_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	tcclickhouse "github.com/testcontainers/testcontainers-go/modules/clickhouse"

	"github.com/nakagawa-yuki/o11y-mcp/query/internal/store"
)

// setupClickHouse は結合テスト用に実際の ClickHouse コンテナを起動し、
// クエリに必要な最小限のカラムだけを持つテーブルを用意する。
//
// なぜ upstream の CREATE TABLE をそのまま使わないか:
//
//	upstream (clickhouseexporter) の定義には k8s属性の MATERIALIZED 列や
//	全文検索/bloom filter インデックスなど、パフォーマンス目的の列が多数含まれる。
//	これらはこのリポジトリのSELECT文が参照しないため、結合テストの本質的な検証には不要。
//	実際に使うカラム名・型 (Timestamp, TraceId, ServiceName, Duration 等) は
//	internal/store/clickhouse.go 冒頭コメントの通り upstream ソースで確認済みのものを使う。
func setupClickHouse(t *testing.T) (*sql.DB, *store.ClickHouseRepository) {
	t.Helper()
	ctx := context.Background()

	ctr, err := tcclickhouse.Run(ctx, "clickhouse/clickhouse-server:23.3.8.21-alpine")
	if err != nil {
		t.Fatalf("failed to start clickhouse container: %v", err)
	}
	t.Cleanup(func() {
		if err := ctr.Terminate(context.Background()); err != nil {
			t.Logf("failed to terminate clickhouse container: %v", err)
		}
	})

	dsn, err := ctr.ConnectionString(ctx)
	if err != nil {
		t.Fatalf("failed to get connection string: %v", err)
	}

	db, err := store.OpenClickHouse(dsn)
	if err != nil {
		t.Fatalf("failed to open clickhouse: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	ddls := []string{
		`CREATE TABLE otel_logs (
			Timestamp DateTime64(9),
			TraceId String,
			SpanId String,
			SeverityText LowCardinality(String),
			ServiceName LowCardinality(String),
			Body String
		) ENGINE = MergeTree ORDER BY (Timestamp)`,
		`CREATE TABLE otel_traces (
			Timestamp DateTime64(9),
			TraceId String,
			SpanId String,
			ParentSpanId String,
			SpanName LowCardinality(String),
			ServiceName LowCardinality(String),
			Duration UInt64
		) ENGINE = MergeTree ORDER BY (Timestamp)`,
		`CREATE TABLE otel_metrics_gauge (
			TimeUnix DateTime,
			ServiceName LowCardinality(String),
			MetricName LowCardinality(String),
			Value Float64
		) ENGINE = MergeTree ORDER BY (MetricName, TimeUnix)`,
	}
	for _, ddl := range ddls {
		if _, err := db.ExecContext(ctx, ddl); err != nil {
			t.Fatalf("failed to create table: %v\nddl: %s", err, ddl)
		}
	}

	return db, store.NewClickHouseRepository(db)
}

// insertRows は ClickHouse への挿入を、指定した1文のINSERT文 + 行データで行う。
// clickhouse-go v2 の database/sql インターフェースでは、トランザクション内で
// Prepare したステートメントを複数回 Exec し、Commit 時にまとめてサーバへ送る
// (ネイティブ形式のバッチ挿入と同等の効果を持つ) のが標準的な書き方。
func insertRows(t *testing.T, db *sql.DB, insertSQL string, rows [][]any) {
	t.Helper()
	ctx := context.Background()

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("failed to begin tx: %v", err)
	}
	stmt, err := tx.PrepareContext(ctx, insertSQL)
	if err != nil {
		t.Fatalf("failed to prepare insert: %v", err)
	}
	defer stmt.Close()

	for _, row := range rows {
		if _, err := stmt.ExecContext(ctx, row...); err != nil {
			t.Fatalf("failed to exec insert: %v", err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("failed to commit insert: %v", err)
	}
}

func TestClickHouseRepository_ListServices(t *testing.T) {
	db, repo := setupClickHouse(t)
	now := time.Now().UTC()

	insertRows(t, db,
		"INSERT INTO otel_logs (Timestamp, TraceId, SpanId, SeverityText, ServiceName, Body) VALUES (?, ?, ?, ?, ?, ?)",
		[][]any{
			{now, "t1", "s1", "INFO", "checkout", "order created"}, // logsのみに現れるサービス名としても機能する
		},
	)
	insertRows(t, db,
		"INSERT INTO otel_traces (Timestamp, TraceId, SpanId, ParentSpanId, SpanName, ServiceName, Duration) VALUES (?, ?, ?, ?, ?, ?, ?)",
		[][]any{
			{now, "t1", "s1", "", "POST /checkout", "checkout", uint64(time.Millisecond * 100)},
			{now, "t2", "s2", "", "GET /payment", "payment", uint64(time.Millisecond * 50)}, // tracesのみに現れるサービス名
		},
	)

	services, err := repo.ListServices(context.Background())
	if err != nil {
		t.Fatalf("ListServices() error = %v", err)
	}

	names := make([]string, len(services))
	for i, s := range services {
		names[i] = s.Name
	}
	want := []string{"checkout", "payment"}
	if len(names) != len(want) {
		t.Fatalf("services = %v, want %v", names, want)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Errorf("services[%d] = %q, want %q", i, names[i], want[i])
		}
	}
}

func TestClickHouseRepository_GetTrace(t *testing.T) {
	db, repo := setupClickHouse(t)
	start := time.Date(2026, 7, 16, 0, 0, 0, 0, time.UTC)

	insertRows(t, db,
		"INSERT INTO otel_traces (Timestamp, TraceId, SpanId, ParentSpanId, SpanName, ServiceName, Duration) VALUES (?, ?, ?, ?, ?, ?, ?)",
		[][]any{
			{start, "trace-A", "span-1", "", "POST /checkout", "checkout", uint64(100 * time.Millisecond)},
			{start.Add(10 * time.Millisecond), "trace-A", "span-2", "span-1", "call payment", "payment", uint64(50 * time.Millisecond)},
			{start, "trace-B", "span-9", "", "GET /other", "other", uint64(10 * time.Millisecond)},
		},
	)

	t.Run("存在するTraceId", func(t *testing.T) {
		spans, err := repo.GetTrace(context.Background(), "trace-A")
		if err != nil {
			t.Fatalf("GetTrace() error = %v", err)
		}
		if len(spans) != 2 {
			t.Fatalf("spans = %+v, want 2 spans", spans)
		}
		if spans[0].SpanID != "span-1" || spans[1].SpanID != "span-2" {
			t.Errorf("unexpected span order: %+v", spans)
		}
		wantEnd := start.Add(100 * time.Millisecond)
		if !spans[0].EndTime.Equal(wantEnd) {
			t.Errorf("EndTime = %v, want %v", spans[0].EndTime, wantEnd)
		}
	})

	t.Run("存在しないTraceId", func(t *testing.T) {
		_, err := repo.GetTrace(context.Background(), "does-not-exist")
		if err != store.ErrTraceNotFound {
			t.Errorf("err = %v, want %v", err, store.ErrTraceNotFound)
		}
	})
}

func TestClickHouseRepository_SearchLogs(t *testing.T) {
	db, repo := setupClickHouse(t)
	base := time.Date(2026, 7, 16, 0, 0, 0, 0, time.UTC)

	insertRows(t, db,
		"INSERT INTO otel_logs (Timestamp, TraceId, SpanId, SeverityText, ServiceName, Body) VALUES (?, ?, ?, ?, ?, ?)",
		[][]any{
			{base, "", "", "INFO", "checkout", "order created"},
			{base.Add(time.Minute), "", "", "ERROR", "checkout", "payment failed"},
			{base.Add(2 * time.Minute), "", "", "ERROR", "payment", "gateway timeout"},
		},
	)

	result, err := repo.SearchLogs(context.Background(), store.LogQuery{
		ServiceName: "checkout",
		Severity:    "ERROR",
		Limit:       10,
	})
	if err != nil {
		t.Fatalf("SearchLogs() error = %v", err)
	}
	if result.Total != 1 || len(result.Logs) != 1 {
		t.Fatalf("result = %+v, want 1 matching log", result)
	}
	if result.Logs[0].Body != "payment failed" {
		t.Errorf("Body = %q, want %q", result.Logs[0].Body, "payment failed")
	}
}

func TestClickHouseRepository_QueryMetrics(t *testing.T) {
	db, repo := setupClickHouse(t)
	base := time.Date(2026, 7, 16, 0, 0, 0, 0, time.UTC)

	insertRows(t, db,
		"INSERT INTO otel_metrics_gauge (TimeUnix, ServiceName, MetricName, Value) VALUES (?, ?, ?, ?)",
		[][]any{
			{base, "checkout", "http_request_duration", 100.0},
			{base.Add(10 * time.Second), "checkout", "http_request_duration", 200.0},
			{base.Add(time.Minute), "checkout", "http_request_duration", 300.0},
		},
	)

	points, err := repo.QueryMetrics(context.Background(), store.MetricQuery{
		MetricName:      "http_request_duration",
		ServiceName:     "checkout",
		Start:           base,
		End:             base.Add(2 * time.Minute),
		AggregationFunc: "avg",
		Step:            time.Minute,
	})
	if err != nil {
		t.Fatalf("QueryMetrics() error = %v", err)
	}
	// 1分バケットに base と base+10s が入り avg=150、base+1分 が別バケットで 300 になるはず。
	if len(points) != 2 {
		t.Fatalf("points = %+v, want 2 buckets", points)
	}
	if points[0].Value != 150 {
		t.Errorf("points[0].Value = %v, want 150", points[0].Value)
	}
	if points[1].Value != 300 {
		t.Errorf("points[1].Value = %v, want 300", points[1].Value)
	}
}
