package tools

import (
	"context"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/nakagawa-yuki/o11y-mcp/mcp/internal/queryclient"
)

// QueryAPI は各ツールハンドラが必要とする Query API 呼び出しのインターフェース。
// queryclient.Client がこれを満たす (Go は構造的部分型なので明示的な実装宣言は不要)。
// テストでは本物のHTTP呼び出しをせず、このインターフェースに対するフェイクを使う。
type QueryAPI interface {
	ListServices(ctx context.Context) ([]queryclient.Service, error)
	GetTrace(ctx context.Context, traceID string) ([]queryclient.Span, error)
	SearchLogs(ctx context.Context, params queryclient.LogSearchParams) (queryclient.LogSearchResult, error)
	QueryMetrics(ctx context.Context, params queryclient.MetricQueryParams) (queryclient.MetricQueryResult, error)
}

// --- list_services ---

type ListServicesInput struct{}

type ListServicesOutput struct {
	Services []string `json:"services"`
}

// ListServicesHandler は list_services ツールのハンドラを組み立てる。
// 戻り値の型は mcp.AddTool が要求する mcp.ToolHandlerFor[In, Out] に一致させている。
// *mcp.CallToolResult を nil のまま返すと、SDK が Out を JSON化して
// Content/StructuredContent に自動的に詰めてくれる (mcp-go-sdk の CallToolResult のドキュメント参照)。
func ListServicesHandler(api QueryAPI) func(context.Context, *mcp.CallToolRequest, ListServicesInput) (*mcp.CallToolResult, ListServicesOutput, error) {
	return func(ctx context.Context, _ *mcp.CallToolRequest, _ ListServicesInput) (*mcp.CallToolResult, ListServicesOutput, error) {
		services, err := api.ListServices(ctx)
		if err != nil {
			return nil, ListServicesOutput{}, fmt.Errorf("list services: %w", err)
		}
		names := make([]string, len(services))
		for i, s := range services {
			names[i] = s.Name
		}
		return nil, ListServicesOutput{Services: names}, nil
	}
}

// --- get_trace ---

type GetTraceInput struct {
	TraceID string `json:"traceId"`
}

type GetTraceOutput struct {
	Spans []queryclient.Span `json:"spans"`
}

func GetTraceHandler(api QueryAPI) func(context.Context, *mcp.CallToolRequest, GetTraceInput) (*mcp.CallToolResult, GetTraceOutput, error) {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in GetTraceInput) (*mcp.CallToolResult, GetTraceOutput, error) {
		if in.TraceID == "" {
			// LLM がツールの必須引数を渡し忘れた場合、ここで早期に分かりやすいエラーを返す。
			// QueryAPI (ネットワーク越しの呼び出し) まで到達させる必要はない。
			return nil, GetTraceOutput{}, fmt.Errorf("traceId is required")
		}
		spans, err := api.GetTrace(ctx, in.TraceID)
		if err != nil {
			// queryclient.ErrTraceNotFound を含め、そのまま返す (errors.Is で判定可能なようラップしない)。
			return nil, GetTraceOutput{}, err
		}
		return nil, GetTraceOutput{Spans: spans}, nil
	}
}

// --- search_logs ---

type SearchLogsInput struct {
	ServiceName string `json:"serviceName,omitempty"`
	Severity    string `json:"severity,omitempty"`
	Start       string `json:"start,omitempty"` // RFC3339
	End         string `json:"end,omitempty"`   // RFC3339
	Limit       int    `json:"limit,omitempty"`
	Offset      int    `json:"offset,omitempty"`
}

func SearchLogsHandler(api QueryAPI) func(context.Context, *mcp.CallToolRequest, SearchLogsInput) (*mcp.CallToolResult, queryclient.LogSearchResult, error) {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in SearchLogsInput) (*mcp.CallToolResult, queryclient.LogSearchResult, error) {
		params := queryclient.LogSearchParams{
			ServiceName: in.ServiceName,
			Severity:    in.Severity,
			Limit:       in.Limit,
			Offset:      in.Offset,
		}

		if in.Start != "" {
			t, err := time.Parse(time.RFC3339, in.Start)
			if err != nil {
				return nil, queryclient.LogSearchResult{}, fmt.Errorf("start must be an RFC3339 timestamp: %w", err)
			}
			params.Start = t
		}
		if in.End != "" {
			t, err := time.Parse(time.RFC3339, in.End)
			if err != nil {
				return nil, queryclient.LogSearchResult{}, fmt.Errorf("end must be an RFC3339 timestamp: %w", err)
			}
			params.End = t
		}

		result, err := api.SearchLogs(ctx, params)
		if err != nil {
			return nil, queryclient.LogSearchResult{}, fmt.Errorf("search logs: %w", err)
		}
		return nil, result, nil
	}
}

// --- query_metrics ---

type QueryMetricsInput struct {
	MetricName  string `json:"metricName"`
	ServiceName string `json:"serviceName,omitempty"`
	Start       string `json:"start"`
	End         string `json:"end"`
	Aggregation string `json:"aggregation,omitempty"`
	Step        string `json:"step,omitempty"`
}

func QueryMetricsHandler(api QueryAPI) func(context.Context, *mcp.CallToolRequest, QueryMetricsInput) (*mcp.CallToolResult, queryclient.MetricQueryResult, error) {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in QueryMetricsInput) (*mcp.CallToolResult, queryclient.MetricQueryResult, error) {
		if in.MetricName == "" {
			return nil, queryclient.MetricQueryResult{}, fmt.Errorf("metricName is required")
		}

		start, err := time.Parse(time.RFC3339, in.Start)
		if err != nil {
			return nil, queryclient.MetricQueryResult{}, fmt.Errorf("start must be an RFC3339 timestamp: %w", err)
		}
		end, err := time.Parse(time.RFC3339, in.End)
		if err != nil {
			return nil, queryclient.MetricQueryResult{}, fmt.Errorf("end must be an RFC3339 timestamp: %w", err)
		}

		params := queryclient.MetricQueryParams{
			MetricName:  in.MetricName,
			ServiceName: in.ServiceName,
			Start:       start,
			End:         end,
			Aggregation: in.Aggregation,
		}
		if in.Step != "" {
			step, err := time.ParseDuration(in.Step)
			if err != nil {
				return nil, queryclient.MetricQueryResult{}, fmt.Errorf("step must be a duration (e.g. 30s, 5m): %w", err)
			}
			params.Step = step
		}

		result, err := api.QueryMetrics(ctx, params)
		if err != nil {
			return nil, queryclient.MetricQueryResult{}, fmt.Errorf("query metrics: %w", err)
		}
		return nil, result, nil
	}
}
