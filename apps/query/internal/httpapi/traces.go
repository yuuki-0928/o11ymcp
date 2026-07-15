package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/nakagawa-yuki/o11y-mcp/query/internal/store"
)

// spanResponse は GET /v1/traces/{traceId} のレスポンスに含まれる1 spanの表現。
type spanResponse struct {
	TraceID      string    `json:"traceId"`
	SpanID       string    `json:"spanId"`
	ParentSpanID string    `json:"parentSpanId,omitempty"`
	ServiceName  string    `json:"serviceName"`
	Name         string    `json:"name"`
	StartTime    time.Time `json:"startTime"`
	EndTime      time.Time `json:"endTime"`
}

type getTraceResponse struct {
	Spans []spanResponse `json:"spans"`
}

// GetTraceHandler は GET /v1/traces/{traceId} (FR-QR-02) を処理するハンドラを組み立てる。
//
// パスパラメータの取り方について:
//
//	Go 1.22 で http.ServeMux が "GET /v1/traces/{traceId}" のようなパターンマッチに対応した。
//	これ以前は gorilla/mux 等の外部ルーターが必須だったが、本プロジェクトの要求 (Must/Should
//	要件の範囲) では標準ライブラリのみで足りるため、依存を増やさずに済むこちらを採用する。
func GetTraceHandler(repo store.TraceRepository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		traceID := r.PathValue("traceId")
		if traceID == "" {
			http.Error(w, "traceId is required", http.StatusBadRequest)
			return
		}

		spans, err := repo.GetTrace(r.Context(), traceID)
		switch {
		case errors.Is(err, store.ErrTraceNotFound):
			// 「存在しないTrace ID」はクライアントの入力に起因するので 404。
			// リポジトリ層のエラーではなく sentinel error で判定することで、
			// ClickHouse実装が変わってもこのハンドラのロジックは変えずに済む。
			http.Error(w, "trace not found", http.StatusNotFound)
			return
		case err != nil:
			http.Error(w, "failed to get trace", http.StatusInternalServerError)
			return
		}

		resp := make([]spanResponse, len(spans))
		for i, s := range spans {
			resp[i] = spanResponse{
				TraceID:      s.TraceID,
				SpanID:       s.SpanID,
				ParentSpanID: s.ParentSpanID,
				ServiceName:  s.ServiceName,
				Name:         s.Name,
				StartTime:    s.StartTime,
				EndTime:      s.EndTime,
			}
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(getTraceResponse{Spans: resp})
	}
}
