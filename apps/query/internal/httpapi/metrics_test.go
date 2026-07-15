package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/nakagawa-yuki/o11y-mcp/query/internal/store"
)

// fakeMetricRepository は渡された store.MetricQuery を記録しつつ設定済みの結果/エラーを返す。
type fakeMetricRepository struct {
	gotQuery store.MetricQuery
	points   []store.MetricPoint
	err      error
}

func (f *fakeMetricRepository) QueryMetrics(ctx context.Context, query store.MetricQuery) ([]store.MetricPoint, error) {
	f.gotQuery = query
	return f.points, f.err
}

// 必須パラメータ (metric, start, end) を満たした基本のクエリ文字列。
// 各テストはここに個別のパラメータを追加/上書きする形で使う。
const baseMetricsQuery = "metric=http_request_duration&start=2026-07-01T00:00:00Z&end=2026-07-01T01:00:00Z"

func TestQueryMetricsHandler_Defaults(t *testing.T) {
	repo := &fakeMetricRepository{}
	handler := QueryMetricsHandler(repo)

	req := httptest.NewRequest(http.MethodGet, "/v1/metrics?"+baseMetricsQuery, nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	// agg / step を省略した場合の既定値 (avg / 1分) がリポジトリに渡ることを確認する。
	if repo.gotQuery.AggregationFunc != "avg" {
		t.Errorf("AggregationFunc = %q, want %q", repo.gotQuery.AggregationFunc, "avg")
	}
	if repo.gotQuery.Step != time.Minute {
		t.Errorf("Step = %v, want %v", repo.gotQuery.Step, time.Minute)
	}
}

func TestQueryMetricsHandler_PassesQueryParamsToRepository(t *testing.T) {
	repo := &fakeMetricRepository{}
	handler := QueryMetricsHandler(repo)

	req := httptest.NewRequest(http.MethodGet,
		"/v1/metrics?"+baseMetricsQuery+"&service=checkout&agg=max&step=5m", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	got := repo.gotQuery
	if got.MetricName != "http_request_duration" {
		t.Errorf("MetricName = %q, want %q", got.MetricName, "http_request_duration")
	}
	if got.ServiceName != "checkout" {
		t.Errorf("ServiceName = %q, want %q", got.ServiceName, "checkout")
	}
	if got.AggregationFunc != "max" {
		t.Errorf("AggregationFunc = %q, want %q", got.AggregationFunc, "max")
	}
	if got.Step != 5*time.Minute {
		t.Errorf("Step = %v, want %v", got.Step, 5*time.Minute)
	}
	wantStart := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	wantEnd := time.Date(2026, 7, 1, 1, 0, 0, 0, time.UTC)
	if !got.Start.Equal(wantStart) || !got.End.Equal(wantEnd) {
		t.Errorf("Start/End = %v/%v, want %v/%v", got.Start, got.End, wantStart, wantEnd)
	}
}

func TestQueryMetricsHandler_ReturnsPoints(t *testing.T) {
	ts := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	repo := &fakeMetricRepository{
		points: []store.MetricPoint{
			{Timestamp: ts, Value: 12.5},
			{Timestamp: ts.Add(time.Minute), Value: 15.0},
		},
	}
	handler := QueryMetricsHandler(repo)

	req := httptest.NewRequest(http.MethodGet, "/v1/metrics?"+baseMetricsQuery, nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	var got struct {
		Metric      string `json:"metric"`
		Aggregation string `json:"aggregation"`
		Points      []struct {
			Value float64 `json:"value"`
		} `json:"points"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("failed to unmarshal response body %q: %v", rec.Body.String(), err)
	}
	if got.Metric != "http_request_duration" {
		t.Errorf("metric = %q, want %q", got.Metric, "http_request_duration")
	}
	if len(got.Points) != 2 || got.Points[1].Value != 15.0 {
		t.Errorf("unexpected points: %+v", got.Points)
	}
}

func TestQueryMetricsHandler_RepositoryError(t *testing.T) {
	repo := &fakeMetricRepository{err: errors.New("clickhouse unavailable")}
	handler := QueryMetricsHandler(repo)

	req := httptest.NewRequest(http.MethodGet, "/v1/metrics?"+baseMetricsQuery, nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}

func TestQueryMetricsHandler_InvalidQueryParams(t *testing.T) {
	tests := []struct {
		name  string
		query string
	}{
		{name: "metricが未指定", query: "start=2026-07-01T00:00:00Z&end=2026-07-01T01:00:00Z"},
		{name: "startが未指定", query: "metric=m&end=2026-07-01T01:00:00Z"},
		{name: "endが未指定", query: "metric=m&start=2026-07-01T00:00:00Z"},
		{name: "startがRFC3339でない", query: "metric=m&start=yesterday&end=2026-07-01T01:00:00Z"},
		{name: "endがRFC3339でない", query: "metric=m&start=2026-07-01T00:00:00Z&end=tomorrow"},
		{name: "endがstart以前", query: "metric=m&start=2026-07-01T01:00:00Z&end=2026-07-01T00:00:00Z"},
		{name: "aggが未知の値", query: baseMetricsQuery + "&agg=median"},
		{name: "stepがパースできない", query: baseMetricsQuery + "&step=abc"},
		{name: "stepが0以下", query: baseMetricsQuery + "&step=0s"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &fakeMetricRepository{}
			handler := QueryMetricsHandler(repo)

			req := httptest.NewRequest(http.MethodGet, "/v1/metrics?"+tt.query, nil)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want %d, body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
			}
		})
	}
}
