// cmd/query は Query API の起動エントリポイント。
// ルーティングと依存の配線 (どのリポジトリ実装を使うか) だけをここに書き、
// ロジック自体は internal/httpapi, internal/store に置く (main を薄く保つ)。
package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/nakagawa-yuki/o11y-mcp/query/internal/httpapi"
	"github.com/nakagawa-yuki/o11y-mcp/query/internal/store"
)

// devSeedTraceID は手元動作確認用のダミー trace データのID。
// ClickHouse実装が入ったら本ファイルのシード処理ごと削除する。
const devSeedTraceID = "trace-1"

func main() {
	// FR-QR-05: 認証必須。キー未設定のまま起動して「認証なしで動いてしまう」事故を防ぐため、
	// 環境変数が空なら起動自体を失敗させる。
	apiKey := os.Getenv("QUERY_API_KEY")
	if apiKey == "" {
		log.Fatal("QUERY_API_KEY environment variable is required")
	}

	addr := os.Getenv("LISTEN_ADDR")
	if addr == "" {
		addr = ":8080"
	}

	// TODO(clickhouse-repository): ClickHouse実装ができ次第、この一時的なインメモリ実装から
	// store.NewClickHouseServiceRepository(dsn) / NewClickHouseTraceRepository(dsn) 等に差し替える。
	serviceRepo := &store.MemoryServiceRepository{
		Services: []store.Service{{Name: "checkout"}, {Name: "payment"}},
	}
	now := time.Now().UTC()
	traceRepo := &store.MemoryTraceRepository{
		SpansByTraceID: map[string][]store.Span{
			devSeedTraceID: {
				{
					TraceID:     devSeedTraceID,
					SpanID:      "span-1",
					ServiceName: "checkout",
					Name:        "POST /checkout",
					StartTime:   now,
					EndTime:     now.Add(120 * time.Millisecond),
				},
			},
		},
	}
	logRepo := &store.MemoryLogRepository{
		Logs: []store.LogRecord{
			{Timestamp: now, ServiceName: "checkout", Severity: "ERROR", Body: "payment gateway timeout", TraceID: devSeedTraceID},
			{Timestamp: now, ServiceName: "checkout", Severity: "INFO", Body: "order created"},
		},
	}
	metricRepo := &store.MemoryMetricRepository{
		Points: []store.MetricPoint{
			{Timestamp: now.Add(-2 * time.Minute), Value: 120},
			{Timestamp: now.Add(-1 * time.Minute), Value: 150},
			{Timestamp: now, Value: 90},
		},
	}

	mux := http.NewServeMux()
	mux.Handle("/v1/services", httpapi.RequireAPIKey(apiKey, httpapi.ListServicesHandler(serviceRepo)))
	mux.Handle("GET /v1/traces/{traceId}", httpapi.RequireAPIKey(apiKey, httpapi.GetTraceHandler(traceRepo)))
	mux.Handle("/v1/logs", httpapi.RequireAPIKey(apiKey, httpapi.SearchLogsHandler(logRepo)))
	mux.Handle("/v1/metrics", httpapi.RequireAPIKey(apiKey, httpapi.QueryMetricsHandler(metricRepo)))
	// /healthz は Container Apps / Docker のヘルスチェックから叩かれる想定のため認証をかけない。
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	srv := &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	// コンテナ環境 (Container Apps / Docker) からの停止シグナルを受けて、
	// 処理中のリクエストを打ち切らずに終了できるようにする (Graceful Shutdown)。
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		log.Println("shutting down...")
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil {
			log.Printf("graceful shutdown failed: %v", err)
		}
	}()

	log.Printf("query api listening on %s", addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server error: %v", err)
	}
}
