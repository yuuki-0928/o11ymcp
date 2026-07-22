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

	"github.com/yuuki-0928/o11y-mcp/query/internal/httpapi"
	"github.com/yuuki-0928/o11y-mcp/query/internal/store"
)

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

	// FR-OP-06: 挙動を環境変数で切替える方針の一環。
	// 例: clickhouse://user:password@localhost:9000/default
	dsn := os.Getenv("CLICKHOUSE_DSN")
	if dsn == "" {
		log.Fatal("CLICKHOUSE_DSN environment variable is required")
	}
	db, err := store.OpenClickHouse(dsn)
	if err != nil {
		log.Fatalf("failed to connect to clickhouse: %v", err)
	}
	defer db.Close()

	// ServiceRepository/TraceRepository/LogRepository/MetricRepository の4つを
	// 1つの ClickHouseRepository で満たす (internal/store/clickhouse.go のコメント参照)。
	repo := store.NewClickHouseRepository(db)

	mux := http.NewServeMux()
	mux.Handle("/v1/services", httpapi.RequireAPIKey(apiKey, httpapi.ListServicesHandler(repo)))
	mux.Handle("GET /v1/traces/{traceId}", httpapi.RequireAPIKey(apiKey, httpapi.GetTraceHandler(repo)))
	mux.Handle("/v1/logs", httpapi.RequireAPIKey(apiKey, httpapi.SearchLogsHandler(repo)))
	mux.Handle("/v1/metrics", httpapi.RequireAPIKey(apiKey, httpapi.QueryMetricsHandler(repo)))
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
