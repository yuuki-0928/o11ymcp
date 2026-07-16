package queryclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// ErrTraceNotFound は Query API が 404 (trace not found) を返したことを表す。
// httpapi.GetTraceHandler と同様、呼び出し元が「見つからない」を区別できるよう
// sentinel error として公開する。
var ErrTraceNotFound = errors.New("trace not found")

// Client は Query API を HTTP 経由で呼び出す薄いラッパー。
type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

// New は Query API クライアントを作る。baseURL は末尾のスラッシュの有無を問わない。
func New(baseURL, apiKey string) *Client {
	return &Client{
		baseURL:    strings.TrimSuffix(baseURL, "/"),
		apiKey:     apiKey,
		httpClient: http.DefaultClient,
	}
}

// doGet は認証ヘッダー付きでGETし、200番台でなければエラーとして返す共通処理。
// 各メソッドが個別にリクエスト組み立て/ステータスチェックを重複させないためのヘルパー。
func (c *Client) doGet(ctx context.Context, path string, query url.Values) (*http.Response, error) {
	u := c.baseURL + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("call query api: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		defer resp.Body.Close()
		return resp, fmt.Errorf("query api returned status %d", resp.StatusCode)
	}
	return resp, nil
}

// Service は list_services ツールが返すサービス。
type Service struct {
	Name string `json:"name"`
}

func (c *Client) ListServices(ctx context.Context) ([]Service, error) {
	resp, err := c.doGet(ctx, "/v1/services", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var body struct {
		Services []string `json:"services"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("decode services response: %w", err)
	}

	services := make([]Service, len(body.Services))
	for i, name := range body.Services {
		services[i] = Service{Name: name}
	}
	return services, nil
}

// Span は get_trace ツールが返す1 span。
type Span struct {
	TraceID      string    `json:"traceId"`
	SpanID       string    `json:"spanId"`
	ParentSpanID string    `json:"parentSpanId"`
	ServiceName  string    `json:"serviceName"`
	Name         string    `json:"name"`
	StartTime    time.Time `json:"startTime"`
	EndTime      time.Time `json:"endTime"`
}

func (c *Client) GetTrace(ctx context.Context, traceID string) ([]Span, error) {
	resp, err := c.doGet(ctx, "/v1/traces/"+url.PathEscape(traceID), nil)
	if err != nil {
		if resp != nil && resp.StatusCode == http.StatusNotFound {
			return nil, ErrTraceNotFound
		}
		return nil, err
	}
	defer resp.Body.Close()

	var body struct {
		Spans []Span `json:"spans"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("decode trace response: %w", err)
	}
	return body.Spans, nil
}

// LogRecord は search_logs ツールが返す1ログレコード。
type LogRecord struct {
	Timestamp   time.Time `json:"timestamp"`
	ServiceName string    `json:"serviceName"`
	Severity    string    `json:"severity"`
	Body        string    `json:"body"`
	TraceID     string    `json:"traceId"`
	SpanID      string    `json:"spanId"`
}

// LogSearchParams は Query API の GET /v1/logs に渡す検索条件。ゼロ値は「指定なし」を表す。
type LogSearchParams struct {
	ServiceName string
	Severity    string
	Start       time.Time
	End         time.Time
	Limit       int
	Offset      int
}

type LogSearchResult struct {
	Logs   []LogRecord `json:"logs"`
	Limit  int         `json:"limit"`
	Offset int         `json:"offset"`
	Total  int64       `json:"total"`
}

func (c *Client) SearchLogs(ctx context.Context, params LogSearchParams) (LogSearchResult, error) {
	q := url.Values{}
	if params.ServiceName != "" {
		q.Set("service", params.ServiceName)
	}
	if params.Severity != "" {
		q.Set("severity", params.Severity)
	}
	if !params.Start.IsZero() {
		q.Set("start", params.Start.UTC().Format(time.RFC3339))
	}
	if !params.End.IsZero() {
		q.Set("end", params.End.UTC().Format(time.RFC3339))
	}
	if params.Limit > 0 {
		q.Set("limit", strconv.Itoa(params.Limit))
	}
	if params.Offset > 0 {
		q.Set("offset", strconv.Itoa(params.Offset))
	}

	resp, err := c.doGet(ctx, "/v1/logs", q)
	if err != nil {
		return LogSearchResult{}, err
	}
	defer resp.Body.Close()

	var result LogSearchResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return LogSearchResult{}, fmt.Errorf("decode logs response: %w", err)
	}
	return result, nil
}

// MetricPoint は query_metrics ツールが返す1時系列ポイント。
type MetricPoint struct {
	Timestamp time.Time `json:"timestamp"`
	Value     float64   `json:"value"`
}

// MetricQueryParams は Query API の GET /v1/metrics に渡す条件。
type MetricQueryParams struct {
	MetricName  string
	ServiceName string
	Start       time.Time
	End         time.Time
	Aggregation string
	Step        time.Duration
}

type MetricQueryResult struct {
	Metric      string        `json:"metric"`
	Aggregation string        `json:"aggregation"`
	Step        string        `json:"step"`
	Points      []MetricPoint `json:"points"`
}

func (c *Client) QueryMetrics(ctx context.Context, params MetricQueryParams) (MetricQueryResult, error) {
	q := url.Values{}
	q.Set("metric", params.MetricName)
	q.Set("start", params.Start.UTC().Format(time.RFC3339))
	q.Set("end", params.End.UTC().Format(time.RFC3339))
	if params.ServiceName != "" {
		q.Set("service", params.ServiceName)
	}
	if params.Aggregation != "" {
		q.Set("agg", params.Aggregation)
	}
	if params.Step > 0 {
		q.Set("step", params.Step.String())
	}

	resp, err := c.doGet(ctx, "/v1/metrics", q)
	if err != nil {
		return MetricQueryResult{}, err
	}
	defer resp.Body.Close()

	var result MetricQueryResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return MetricQueryResult{}, fmt.Errorf("decode metrics response: %w", err)
	}
	return result, nil
}
