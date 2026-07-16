// Package queryclient は Query API を HTTP 経由で呼び出す薄いクライアント。
// MCP Server から見ると「Query API」はネットワークの向こうの別サービスであり、
// FR-MC-03 (内部で Query API を呼び出しデータを取得する) を満たすための実装がここ。
package queryclient

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestClient_ListServices(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/services" {
			t.Errorf("path = %q, want /v1/services", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("Authorization = %q, want %q", got, "Bearer test-key")
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"services": []string{"checkout", "payment"}})
	}))
	defer srv.Close()

	c := New(srv.URL, "test-key")
	services, err := c.ListServices(context.Background())
	if err != nil {
		t.Fatalf("ListServices() error = %v", err)
	}
	if len(services) != 2 || services[0].Name != "checkout" || services[1].Name != "payment" {
		t.Errorf("services = %+v, want [checkout, payment]", services)
	}
}

func TestClient_GetTrace(t *testing.T) {
	ts := time.Date(2026, 7, 16, 0, 0, 0, 0, time.UTC)

	t.Run("正常系", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/v1/traces/trace-1" {
				t.Errorf("path = %q, want /v1/traces/trace-1", r.URL.Path)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"spans": []map[string]any{
					{
						"traceId": "trace-1", "spanId": "span-1", "serviceName": "checkout",
						"name": "POST /checkout", "startTime": ts, "endTime": ts.Add(100 * time.Millisecond),
					},
				},
			})
		}))
		defer srv.Close()

		c := New(srv.URL, "test-key")
		spans, err := c.GetTrace(context.Background(), "trace-1")
		if err != nil {
			t.Fatalf("GetTrace() error = %v", err)
		}
		if len(spans) != 1 || spans[0].SpanID != "span-1" {
			t.Errorf("spans = %+v", spans)
		}
	})

	t.Run("404はErrTraceNotFound", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "trace not found", http.StatusNotFound)
		}))
		defer srv.Close()

		c := New(srv.URL, "test-key")
		_, err := c.GetTrace(context.Background(), "unknown")
		if err != ErrTraceNotFound {
			t.Errorf("err = %v, want %v", err, ErrTraceNotFound)
		}
	})
}

func TestClient_SearchLogs(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("service"); got != "checkout" {
			t.Errorf("service query param = %q, want checkout", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"logs":   []map[string]any{{"serviceName": "checkout", "severity": "ERROR", "body": "boom"}},
			"limit":  50,
			"offset": 0,
			"total":  1,
		})
	}))
	defer srv.Close()

	c := New(srv.URL, "test-key")
	result, err := c.SearchLogs(context.Background(), LogSearchParams{ServiceName: "checkout"})
	if err != nil {
		t.Fatalf("SearchLogs() error = %v", err)
	}
	if result.Total != 1 || len(result.Logs) != 1 || result.Logs[0].Body != "boom" {
		t.Errorf("result = %+v", result)
	}
}

func TestClient_QueryMetrics(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("metric"); got != "http_request_duration" {
			t.Errorf("metric query param = %q", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"metric": "http_request_duration", "aggregation": "avg", "step": "1m0s",
			"points": []map[string]any{{"value": 150.0}},
		})
	}))
	defer srv.Close()

	c := New(srv.URL, "test-key")
	result, err := c.QueryMetrics(context.Background(), MetricQueryParams{
		MetricName: "http_request_duration",
		Start:      time.Now().Add(-time.Hour),
		End:        time.Now(),
	})
	if err != nil {
		t.Fatalf("QueryMetrics() error = %v", err)
	}
	if len(result.Points) != 1 || result.Points[0].Value != 150.0 {
		t.Errorf("result = %+v", result)
	}
}

func TestClient_ErrorResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := New(srv.URL, "test-key")
	_, err := c.ListServices(context.Background())
	if err == nil {
		t.Fatal("expected an error for 500 response")
	}
	wantSubstring := fmt.Sprintf("%d", http.StatusInternalServerError)
	if !strings.Contains(err.Error(), wantSubstring) {
		t.Errorf("error = %q, want it to mention status %s", err.Error(), wantSubstring)
	}
}
