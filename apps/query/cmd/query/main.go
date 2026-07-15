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
	// store.NewClickHouseServiceRepository(dsn) 等に差し替える。
	repo := &store.MemoryServiceRepository{
		Services: []store.Service{{Name: "checkout"}, {Name: "payment"}},
	}

	mux := http.NewServeMux()
	mux.Handle("/v1/services", httpapi.RequireAPIKey(apiKey, httpapi.ListServicesHandler(repo)))
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
