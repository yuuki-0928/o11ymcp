package httpapi

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/nakagawa-yuki/o11y-mcp/query/internal/store"
)

const (
	defaultMetricsAgg  = "avg"
	defaultMetricsStep = time.Minute
)

// allowedMetricsAgg は FR-QR-03 (集計関数指定可) で受け付ける集計関数の一覧。
// ClickHouse 実装側で対応するSQL集計関数 (avg()/sum()/min()/max()/count()) と1:1になる想定。
var allowedMetricsAgg = map[string]bool{
	"avg":   true,
	"sum":   true,
	"min":   true,
	"max":   true,
	"count": true,
}

type metricPointResponse struct {
	Timestamp time.Time `json:"timestamp"`
	Value     float64   `json:"value"`
}

type queryMetricsResponse struct {
	Metric      string                `json:"metric"`
	Aggregation string                `json:"aggregation"`
	Step        string                `json:"step"`
	Points      []metricPointResponse `json:"points"`
}

// QueryMetricsHandler は GET /v1/metrics (FR-QR-03) を処理するハンドラを組み立てる。
//
// クエリパラメータ:
//
//	metric (必須):        メトリクス名
//	start, end (必須):     RFC3339 の時刻範囲。end は start より後でなければならない
//	service (任意):        サービス名で絞り込み
//	agg (任意, 既定 avg):   avg/sum/min/max/count のいずれか
//	step (任意, 既定 1m):   Go の time.ParseDuration が解釈できる形式 (例: "30s", "5m")
func QueryMetricsHandler(repo store.MetricRepository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()

		metric := q.Get("metric")
		if metric == "" {
			http.Error(w, "metric is required", http.StatusBadRequest)
			return
		}

		startStr := q.Get("start")
		endStr := q.Get("end")
		if startStr == "" || endStr == "" {
			http.Error(w, "start and end are required", http.StatusBadRequest)
			return
		}
		start, err := time.Parse(time.RFC3339, startStr)
		if err != nil {
			http.Error(w, "start must be an RFC3339 timestamp", http.StatusBadRequest)
			return
		}
		end, err := time.Parse(time.RFC3339, endStr)
		if err != nil {
			http.Error(w, "end must be an RFC3339 timestamp", http.StatusBadRequest)
			return
		}
		if !end.After(start) {
			http.Error(w, "end must be after start", http.StatusBadRequest)
			return
		}

		agg := defaultMetricsAgg
		if v := q.Get("agg"); v != "" {
			if !allowedMetricsAgg[v] {
				http.Error(w, "agg must be one of avg, sum, min, max, count", http.StatusBadRequest)
				return
			}
			agg = v
		}

		step := defaultMetricsStep
		if v := q.Get("step"); v != "" {
			d, err := time.ParseDuration(v)
			if err != nil || d <= 0 {
				http.Error(w, "step must be a positive duration (e.g. 30s, 5m)", http.StatusBadRequest)
				return
			}
			step = d
		}

		query := store.MetricQuery{
			MetricName:      metric,
			ServiceName:     q.Get("service"),
			Start:           start,
			End:             end,
			AggregationFunc: agg,
			Step:            step,
		}

		points, err := repo.QueryMetrics(r.Context(), query)
		if err != nil {
			http.Error(w, "failed to query metrics", http.StatusInternalServerError)
			return
		}

		respPoints := make([]metricPointResponse, len(points))
		for i, p := range points {
			respPoints[i] = metricPointResponse{Timestamp: p.Timestamp, Value: p.Value}
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(queryMetricsResponse{
			Metric:      metric,
			Aggregation: agg,
			Step:        step.String(),
			Points:      respPoints,
		})
	}
}
