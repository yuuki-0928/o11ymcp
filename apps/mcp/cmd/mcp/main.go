// cmd/mcp は MCP Server の起動エントリポイント。
// FR-MC-04 の確定方針通り Streamable HTTP を主トランスポートとして提供する。
package main

import (
	"log"
	"net/http"
	"os"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/yuuki-0928/o11y-mcp/mcp/internal/queryclient"
	"github.com/yuuki-0928/o11y-mcp/mcp/internal/tools"
)

func main() {
	// MCP Server自身もQuery APIと同じくAPI Key認証を要求する (v1.0はAPI Keyのみ)。
	apiKey := os.Getenv("MCP_API_KEY")
	if apiKey == "" {
		log.Fatal("MCP_API_KEY environment variable is required")
	}

	queryAPIBaseURL := os.Getenv("QUERY_API_BASE_URL")
	if queryAPIBaseURL == "" {
		log.Fatal("QUERY_API_BASE_URL environment variable is required")
	}
	queryAPIKey := os.Getenv("QUERY_API_KEY")
	if queryAPIKey == "" {
		log.Fatal("QUERY_API_KEY environment variable is required")
	}

	addr := os.Getenv("LISTEN_ADDR")
	if addr == "" {
		addr = ":8081"
	}

	// FR-MC-03: 内部でQuery APIを呼び出す。queryclient.Client が tools.QueryAPI を満たす。
	api := queryclient.New(queryAPIBaseURL, queryAPIKey)

	server := mcp.NewServer(&mcp.Implementation{Name: "o11y-mcp", Version: "v0.1.0"}, nil)

	// FR-MC-02: 4つのツールを登録する。
	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_services",
		Description: "既知のサービス名の一覧を取得する",
	}, tools.ListServicesHandler(api))
	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_trace",
		Description: "Trace IDを指定してトレース全体 (spans) を取得する",
	}, tools.GetTraceHandler(api))
	mcp.AddTool(server, &mcp.Tool{
		Name:        "search_logs",
		Description: "サービス名・重大度・期間などの条件でログを検索する",
	}, tools.SearchLogsHandler(api))
	mcp.AddTool(server, &mcp.Tool{
		Name:        "query_metrics",
		Description: "メトリクスを時系列で取得する (集計関数・バケット幅を指定可能)",
	}, tools.QueryMetricsHandler(api))

	// requireAPIKeyMiddleware は Query API の httpapi.RequireAPIKey と同じ考え方で、
	// MCP Server自体へのアクセスもAPI Keyで保護する (NFR-S-02)。
	handler := requireAPIKeyMiddleware(apiKey, mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
		return server
	}, nil))

	log.Printf("mcp server listening on %s", addr)
	if err := http.ListenAndServe(addr, handler); err != nil {
		log.Fatalf("server error: %v", err)
	}
}

func requireAPIKeyMiddleware(expectedKey string, next http.Handler) http.Handler {
	const prefix = "Bearer "
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := r.Header.Get("Authorization")
		if len(header) <= len(prefix) || header[:len(prefix)] != prefix || header[len(prefix):] != expectedKey {
			http.Error(w, "missing or invalid Authorization header", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}
