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

type fakeTraceRepository struct {
	spans []store.Span
	err   error
}

func (f *fakeTraceRepository) GetTrace(ctx context.Context, traceID string) ([]store.Span, error) {
	return f.spans, f.err
}

// newTraceRequest は {traceId} パスパラメータを付与したテスト用リクエストを作る。
// r.PathValue は本来 http.ServeMux が "GET /v1/traces/{traceId}" のようなパターンで
// マッチした際に自動でセットするが、ハンドラを直接 ServeHTTP するユニットテストでは
// マッチが起きないため、Go 1.22 で追加された SetPathValue で手動セットする。
func newTraceRequest(traceID string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/v1/traces/"+traceID, nil)
	req.SetPathValue("traceId", traceID)
	return req
}

func TestGetTraceHandler(t *testing.T) {
	t.Run("正常系: spanの一覧をJSONで返す", func(t *testing.T) {
		start := time.Date(2026, 7, 16, 0, 0, 0, 0, time.UTC)
		repo := &fakeTraceRepository{
			spans: []store.Span{
				{
					TraceID:     "trace-1",
					SpanID:      "span-1",
					ServiceName: "checkout",
					Name:        "POST /checkout",
					StartTime:   start,
					EndTime:     start.Add(100 * time.Millisecond),
				},
			},
		}
		handler := GetTraceHandler(repo)

		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, newTraceRequest("trace-1"))

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
		}

		var got struct {
			Spans []struct {
				TraceID     string `json:"traceId"`
				SpanID      string `json:"spanId"`
				ServiceName string `json:"serviceName"`
			} `json:"spans"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatalf("failed to unmarshal response body %q: %v", rec.Body.String(), err)
		}
		if len(got.Spans) != 1 {
			t.Fatalf("spans length = %d, want 1", len(got.Spans))
		}
		if got.Spans[0].SpanID != "span-1" || got.Spans[0].ServiceName != "checkout" {
			t.Errorf("unexpected span: %+v", got.Spans[0])
		}
	})

	t.Run("見つからない場合は404", func(t *testing.T) {
		repo := &fakeTraceRepository{err: store.ErrTraceNotFound}
		handler := GetTraceHandler(repo)

		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, newTraceRequest("unknown-trace"))

		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
		}
	})

	t.Run("その他のエラーは500", func(t *testing.T) {
		repo := &fakeTraceRepository{err: errors.New("clickhouse unavailable")}
		handler := GetTraceHandler(repo)

		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, newTraceRequest("trace-1"))

		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
		}
	})
}
