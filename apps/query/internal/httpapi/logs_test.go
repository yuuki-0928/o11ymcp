package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/yuuki-0928/o11y-mcp/query/internal/store"
)

// fakeLogRepository は渡された store.LogQuery を記録しつつ、設定済みの結果/エラーを返す。
// 「ハンドラがクエリパラメータを正しく解釈してリポジトリに渡せているか」を検証するのに使う。
type fakeLogRepository struct {
	gotQuery store.LogQuery
	result   store.LogSearchResult
	err      error
}

func (f *fakeLogRepository) SearchLogs(ctx context.Context, query store.LogQuery) (store.LogSearchResult, error) {
	f.gotQuery = query
	return f.result, f.err
}

func TestSearchLogsHandler_Defaults(t *testing.T) {
	repo := &fakeLogRepository{}
	handler := SearchLogsHandler(repo)

	req := httptest.NewRequest(http.MethodGet, "/v1/logs", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	// パラメータなしの場合、既定値 (limit=50, offset=0) がリポジトリに渡ることを確認する。
	if repo.gotQuery.Limit != 50 {
		t.Errorf("Limit = %d, want 50", repo.gotQuery.Limit)
	}
	if repo.gotQuery.Offset != 0 {
		t.Errorf("Offset = %d, want 0", repo.gotQuery.Offset)
	}
}

func TestSearchLogsHandler_PassesQueryParamsToRepository(t *testing.T) {
	repo := &fakeLogRepository{}
	handler := SearchLogsHandler(repo)

	req := httptest.NewRequest(http.MethodGet,
		"/v1/logs?service=checkout&severity=ERROR&start=2026-07-01T00:00:00Z&end=2026-07-02T00:00:00Z&limit=10&offset=20",
		nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	wantStart := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	wantEnd := time.Date(2026, 7, 2, 0, 0, 0, 0, time.UTC)

	got := repo.gotQuery
	if got.ServiceName != "checkout" {
		t.Errorf("ServiceName = %q, want %q", got.ServiceName, "checkout")
	}
	if got.Severity != "ERROR" {
		t.Errorf("Severity = %q, want %q", got.Severity, "ERROR")
	}
	if !got.Start.Equal(wantStart) {
		t.Errorf("Start = %v, want %v", got.Start, wantStart)
	}
	if !got.End.Equal(wantEnd) {
		t.Errorf("End = %v, want %v", got.End, wantEnd)
	}
	if got.Limit != 10 {
		t.Errorf("Limit = %d, want 10", got.Limit)
	}
	if got.Offset != 20 {
		t.Errorf("Offset = %d, want 20", got.Offset)
	}
}

func TestSearchLogsHandler_ReturnsLogsAndPagingInfo(t *testing.T) {
	ts := time.Date(2026, 7, 16, 1, 0, 0, 0, time.UTC)
	repo := &fakeLogRepository{
		result: store.LogSearchResult{
			Logs: []store.LogRecord{
				{Timestamp: ts, ServiceName: "checkout", Severity: "ERROR", Body: "payment failed", TraceID: "trace-1"},
			},
			Total: 1,
		},
	}
	handler := SearchLogsHandler(repo)

	req := httptest.NewRequest(http.MethodGet, "/v1/logs", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	var got struct {
		Logs []struct {
			ServiceName string `json:"serviceName"`
			Severity    string `json:"severity"`
			Body        string `json:"body"`
			TraceID     string `json:"traceId"`
		} `json:"logs"`
		Limit  int   `json:"limit"`
		Offset int   `json:"offset"`
		Total  int64 `json:"total"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("failed to unmarshal response body %q: %v", rec.Body.String(), err)
	}

	if len(got.Logs) != 1 || got.Logs[0].Body != "payment failed" {
		t.Errorf("unexpected logs: %+v", got.Logs)
	}
	if got.Total != 1 {
		t.Errorf("Total = %d, want 1", got.Total)
	}
}

func TestSearchLogsHandler_RepositoryError(t *testing.T) {
	repo := &fakeLogRepository{err: errors.New("clickhouse unavailable")}
	handler := SearchLogsHandler(repo)

	req := httptest.NewRequest(http.MethodGet, "/v1/logs", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}

// 不正なクエリパラメータはまとめてテーブル駆動でカバーする。
// いずれも「クライアントの入力が悪い」ケースなので 400 を期待する。
func TestSearchLogsHandler_InvalidQueryParams(t *testing.T) {
	tests := []struct {
		name  string
		query string
	}{
		{name: "limitが数値でない", query: "limit=abc"},
		{name: "limitが0以下", query: "limit=0"},
		{name: "limitが上限(500)超え", query: "limit=501"},
		{name: "offsetが負数", query: "offset=-1"},
		{name: "offsetが数値でない", query: "offset=abc"},
		{name: "startがRFC3339でない", query: "start=yesterday"},
		{name: "endがRFC3339でない", query: "end=tomorrow"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &fakeLogRepository{}
			handler := SearchLogsHandler(repo)

			req := httptest.NewRequest(http.MethodGet, "/v1/logs?"+tt.query, nil)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
			}
		})
	}
}
