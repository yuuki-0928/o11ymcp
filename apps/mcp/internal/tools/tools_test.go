// Package tools は MCP の tool (search_logs, get_trace, list_services, query_metrics) の
// ハンドラ本体。MCP SDK 固有の型 (mcp.CallToolRequest 等) への依存はこの層に閉じ込め、
// Query API 呼び出しは queryclient.Client 経由の QueryAPI インターフェースに委ねる
// (Query API 側の httpapi 層と同じ「インターフェースでハンドラをテスト可能にする」方針)。
package tools

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/yuuki-0928/o11y-mcp/mcp/internal/queryclient"
)

// fakeQueryAPI は QueryAPI のテスト用実装。呼び出された引数を記録しつつ、
// 設定済みの結果/エラーを返す。
type fakeQueryAPI struct {
	services []queryclient.Service
	spans    []queryclient.Span
	logs     queryclient.LogSearchResult
	metrics  queryclient.MetricQueryResult
	err      error

	gotTraceID    string
	gotLogParams  queryclient.LogSearchParams
	gotMetricArgs queryclient.MetricQueryParams
}

func (f *fakeQueryAPI) ListServices(ctx context.Context) ([]queryclient.Service, error) {
	return f.services, f.err
}

func (f *fakeQueryAPI) GetTrace(ctx context.Context, traceID string) ([]queryclient.Span, error) {
	f.gotTraceID = traceID
	return f.spans, f.err
}

func (f *fakeQueryAPI) SearchLogs(ctx context.Context, params queryclient.LogSearchParams) (queryclient.LogSearchResult, error) {
	f.gotLogParams = params
	return f.logs, f.err
}

func (f *fakeQueryAPI) QueryMetrics(ctx context.Context, params queryclient.MetricQueryParams) (queryclient.MetricQueryResult, error) {
	f.gotMetricArgs = params
	return f.metrics, f.err
}

func TestListServicesHandler(t *testing.T) {
	api := &fakeQueryAPI{services: []queryclient.Service{{Name: "checkout"}, {Name: "payment"}}}
	handler := ListServicesHandler(api)

	_, out, err := handler(context.Background(), nil, ListServicesInput{})
	if err != nil {
		t.Fatalf("handler error = %v", err)
	}
	if len(out.Services) != 2 || out.Services[0] != "checkout" {
		t.Errorf("out.Services = %v", out.Services)
	}
}

func TestGetTraceHandler(t *testing.T) {
	t.Run("正常系", func(t *testing.T) {
		api := &fakeQueryAPI{spans: []queryclient.Span{{TraceID: "trace-1", SpanID: "span-1"}}}
		handler := GetTraceHandler(api)

		_, out, err := handler(context.Background(), nil, GetTraceInput{TraceID: "trace-1"})
		if err != nil {
			t.Fatalf("handler error = %v", err)
		}
		if api.gotTraceID != "trace-1" {
			t.Errorf("gotTraceID = %q, want trace-1", api.gotTraceID)
		}
		if len(out.Spans) != 1 || out.Spans[0].SpanID != "span-1" {
			t.Errorf("out.Spans = %+v", out.Spans)
		}
	})

	t.Run("traceIdが空なら呼び出す前にエラー", func(t *testing.T) {
		api := &fakeQueryAPI{}
		handler := GetTraceHandler(api)

		_, _, err := handler(context.Background(), nil, GetTraceInput{})
		if err == nil {
			t.Fatal("expected an error for empty traceId")
		}
		if api.gotTraceID != "" {
			t.Errorf("QueryAPI should not have been called, but gotTraceID = %q", api.gotTraceID)
		}
	})

	t.Run("見つからない場合はエラーを返す", func(t *testing.T) {
		api := &fakeQueryAPI{err: queryclient.ErrTraceNotFound}
		handler := GetTraceHandler(api)

		_, _, err := handler(context.Background(), nil, GetTraceInput{TraceID: "unknown"})
		if !errors.Is(err, queryclient.ErrTraceNotFound) {
			t.Errorf("err = %v, want ErrTraceNotFound", err)
		}
	})
}

func TestSearchLogsHandler(t *testing.T) {
	t.Run("パラメータをQueryAPIに渡す", func(t *testing.T) {
		api := &fakeQueryAPI{logs: queryclient.LogSearchResult{Total: 1}}
		handler := SearchLogsHandler(api)

		_, _, err := handler(context.Background(), nil, SearchLogsInput{
			ServiceName: "checkout",
			Severity:    "ERROR",
			Start:       "2026-07-01T00:00:00Z",
			End:         "2026-07-02T00:00:00Z",
			Limit:       10,
		})
		if err != nil {
			t.Fatalf("handler error = %v", err)
		}
		if api.gotLogParams.ServiceName != "checkout" || api.gotLogParams.Severity != "ERROR" {
			t.Errorf("gotLogParams = %+v", api.gotLogParams)
		}
		wantStart := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
		if !api.gotLogParams.Start.Equal(wantStart) {
			t.Errorf("Start = %v, want %v", api.gotLogParams.Start, wantStart)
		}
	})

	t.Run("startの形式が不正ならエラー", func(t *testing.T) {
		api := &fakeQueryAPI{}
		handler := SearchLogsHandler(api)

		_, _, err := handler(context.Background(), nil, SearchLogsInput{Start: "yesterday"})
		if err == nil {
			t.Fatal("expected an error for invalid start")
		}
	})
}

func TestQueryMetricsHandler(t *testing.T) {
	t.Run("パラメータをQueryAPIに渡す", func(t *testing.T) {
		api := &fakeQueryAPI{metrics: queryclient.MetricQueryResult{Metric: "http_request_duration"}}
		handler := QueryMetricsHandler(api)

		_, out, err := handler(context.Background(), nil, QueryMetricsInput{
			MetricName: "http_request_duration",
			Start:      "2026-07-01T00:00:00Z",
			End:        "2026-07-01T01:00:00Z",
		})
		if err != nil {
			t.Fatalf("handler error = %v", err)
		}
		if api.gotMetricArgs.MetricName != "http_request_duration" {
			t.Errorf("gotMetricArgs.MetricName = %q", api.gotMetricArgs.MetricName)
		}
		if out.Metric != "http_request_duration" {
			t.Errorf("out.Metric = %q", out.Metric)
		}
	})

	t.Run("metricNameが空ならエラー", func(t *testing.T) {
		api := &fakeQueryAPI{}
		handler := QueryMetricsHandler(api)

		_, _, err := handler(context.Background(), nil, QueryMetricsInput{Start: "2026-07-01T00:00:00Z", End: "2026-07-01T01:00:00Z"})
		if err == nil {
			t.Fatal("expected an error for empty metricName")
		}
	})
}
