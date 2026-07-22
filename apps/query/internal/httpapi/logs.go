package httpapi

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/yuuki-0928/o11y-mcp/query/internal/store"
)

const (
	// defaultLogsLimit / maxLogsLimit の値は NFR-P-02 (P95 2秒以内) を踏まえ、
	// 1リクエストで巨大な結果を返して応答が遅くなるのを防ぐための上限。
	defaultLogsLimit = 50
	maxLogsLimit     = 500
)

type logRecordResponse struct {
	Timestamp   time.Time `json:"timestamp"`
	ServiceName string    `json:"serviceName"`
	Severity    string    `json:"severity"`
	Body        string    `json:"body"`
	TraceID     string    `json:"traceId,omitempty"`
	SpanID      string    `json:"spanId,omitempty"`
}

type searchLogsResponse struct {
	Logs   []logRecordResponse `json:"logs"`
	Limit  int                 `json:"limit"`
	Offset int                 `json:"offset"`
	Total  int64               `json:"total"`
}

// SearchLogsHandler は GET /v1/logs (FR-QR-01, FR-QR-04) を処理するハンドラを組み立てる。
//
// クエリパラメータの解釈:
//
//	service, severity: 完全一致条件 (未指定なら絞り込みなし)
//	start, end:         RFC3339 形式の時刻 (例: 2026-07-01T00:00:00Z)
//	limit, offset:      ページング。limit省略時は defaultLogsLimit、上限は maxLogsLimit
func SearchLogsHandler(repo store.LogRepository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()

		query := store.LogQuery{
			ServiceName: q.Get("service"),
			Severity:    q.Get("severity"),
			Limit:       defaultLogsLimit,
		}

		if v := q.Get("start"); v != "" {
			t, err := time.Parse(time.RFC3339, v)
			if err != nil {
				http.Error(w, "start must be an RFC3339 timestamp", http.StatusBadRequest)
				return
			}
			query.Start = t
		}
		if v := q.Get("end"); v != "" {
			t, err := time.Parse(time.RFC3339, v)
			if err != nil {
				http.Error(w, "end must be an RFC3339 timestamp", http.StatusBadRequest)
				return
			}
			query.End = t
		}
		if v := q.Get("limit"); v != "" {
			n, err := strconv.Atoi(v)
			if err != nil || n <= 0 || n > maxLogsLimit {
				http.Error(w, "limit must be a positive integer up to "+strconv.Itoa(maxLogsLimit), http.StatusBadRequest)
				return
			}
			query.Limit = n
		}
		if v := q.Get("offset"); v != "" {
			n, err := strconv.Atoi(v)
			if err != nil || n < 0 {
				http.Error(w, "offset must be a non-negative integer", http.StatusBadRequest)
				return
			}
			query.Offset = n
		}

		result, err := repo.SearchLogs(r.Context(), query)
		if err != nil {
			http.Error(w, "failed to search logs", http.StatusInternalServerError)
			return
		}

		logs := make([]logRecordResponse, len(result.Logs))
		for i, l := range result.Logs {
			logs[i] = logRecordResponse{
				Timestamp:   l.Timestamp,
				ServiceName: l.ServiceName,
				Severity:    l.Severity,
				Body:        l.Body,
				TraceID:     l.TraceID,
				SpanID:      l.SpanID,
			}
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(searchLogsResponse{
			Logs:   logs,
			Limit:  query.Limit,
			Offset: query.Offset,
			Total:  result.Total,
		})
	}
}
