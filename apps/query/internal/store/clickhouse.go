// ClickHouse実装は、docs/design.md §4 の通り Ingest (OTel Collector の
// `clickhouseexporter`) が作成するテーブルにそのまま乗る前提で書かれている。
// テーブル定義 (otel_logs / otel_traces / otel_metrics_gauge) の実際のカラムは
// upstream (opentelemetry-collector-contrib) の
// exporter/clickhouseexporter/internal/sqltemplates/*.sql を一次情報として確認済み。
// Ingest実装時にバージョン差分でカラムが変わっていた場合は、このファイルのSQLを追随させる。
package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	// clickhouse-go/v2 をブランクインポートすることで database/sql に "clickhouse" ドライバが登録される。
	// Go の database/sql は「登録された名前の文字列」でドライバを引くだけの薄い抽象化なので、
	// import自体には識別子を使わない (副作用目的の import であることを示すのが `_`)。
	_ "github.com/ClickHouse/clickhouse-go/v2"
)

// OpenClickHouse は ClickHouse への接続を開き、疎通確認 (Ping) まで行う。
// dsn の例: "clickhouse://user:password@localhost:9000/default"
func OpenClickHouse(dsn string) (*sql.DB, error) {
	db, err := sql.Open("clickhouse", dsn)
	if err != nil {
		return nil, fmt.Errorf("open clickhouse: %w", err)
	}
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("ping clickhouse: %w", err)
	}
	return db, nil
}

// ClickHouseRepository は ServiceRepository / TraceRepository / LogRepository / MetricRepository
// をすべて1つの *sql.DB に対する実装として提供する。
//
// なぜ1つの struct にまとめるか:
//   - 4つのインターフェースはいずれも「同じ ClickHouse への読み取り専用クエリ」という同じ関心事であり、
//     個別の struct に分けても接続の使い回し以外にメリットがない。
//   - main.go 側は1つの ClickHouseRepository を作り、必要なインターフェース型として
//     各ハンドラに渡す (Go はメソッドを持つ型が自動的にインターフェースを満たすため、
//     明示的な "implements" 宣言は不要)。
type ClickHouseRepository struct {
	db *sql.DB
}

func NewClickHouseRepository(db *sql.DB) *ClickHouseRepository {
	return &ClickHouseRepository{db: db}
}

// ListServices は otel_logs / otel_traces の両方に現れるサービス名を重複排除して返す。
// ログしか出さないサービス・トレースしか出さないサービスのどちらも取りこぼさないための UNION。
func (r *ClickHouseRepository) ListServices(ctx context.Context) ([]Service, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT ServiceName FROM (
			SELECT DISTINCT ServiceName FROM otel_logs
			UNION DISTINCT
			SELECT DISTINCT ServiceName FROM otel_traces
		)
		ORDER BY ServiceName
	`)
	if err != nil {
		return nil, fmt.Errorf("query services: %w", err)
	}
	defer rows.Close()

	var services []Service
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("scan service: %w", err)
		}
		services = append(services, Service{Name: name})
	}
	return services, rows.Err()
}

// GetTrace は otel_traces から TraceId が一致する行を取得する。
// otel_traces には EndTime が無く、開始時刻 (Timestamp) と所要時間 (Duration, ナノ秒) が
// 保存されているため、EndTime = Timestamp + Duration として組み立てる。
func (r *ClickHouseRepository) GetTrace(ctx context.Context, traceID string) ([]Span, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT Timestamp, TraceId, SpanId, ParentSpanId, ServiceName, SpanName, Duration
		FROM otel_traces
		WHERE TraceId = ?
		ORDER BY Timestamp
	`, traceID)
	if err != nil {
		return nil, fmt.Errorf("query trace: %w", err)
	}
	defer rows.Close()

	var spans []Span
	for rows.Next() {
		var (
			start         time.Time
			durationNanos uint64
			s             Span
		)
		if err := rows.Scan(&start, &s.TraceID, &s.SpanID, &s.ParentSpanID, &s.ServiceName, &s.Name, &durationNanos); err != nil {
			return nil, fmt.Errorf("scan span: %w", err)
		}
		s.StartTime = start
		s.EndTime = start.Add(time.Duration(durationNanos))
		spans = append(spans, s)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(spans) == 0 {
		return nil, ErrTraceNotFound
	}
	return spans, nil
}

// buildLogsWhereClause は LogQuery のうち指定されているフィールドだけを
// WHERE句の条件として組み立てる。未指定 (ゼロ値) のフィールドは条件に含めない。
//
// すべて `?` プレースホルダ + args のペアで返し、値をSQL文字列に直接埋め込まない
// (SQLインジェクション対策。NFR-S-04)。
func buildLogsWhereClause(query LogQuery) (string, []any) {
	var conds []string
	var args []any

	if query.ServiceName != "" {
		conds = append(conds, "ServiceName = ?")
		args = append(args, query.ServiceName)
	}
	if query.Severity != "" {
		conds = append(conds, "SeverityText = ?")
		args = append(args, query.Severity)
	}
	if !query.Start.IsZero() {
		conds = append(conds, "Timestamp >= ?")
		args = append(args, query.Start)
	}
	if !query.End.IsZero() {
		conds = append(conds, "Timestamp <= ?")
		args = append(args, query.End)
	}

	if len(conds) == 0 {
		return "", args
	}
	return " WHERE " + strings.Join(conds, " AND "), args
}

// SearchLogs は otel_logs に対して条件検索 + ページングを行う。
// Total (全件数) は LIMIT/OFFSET を付けない COUNT(*) を別途発行して求める
// (1回のクエリで「このページの行」と「全件数」を同時に取るSQLも書けるが、
// ClickHouseでは素直にCOUNTクエリを分けたほうが可読性が高くコストも大差ない)。
func (r *ClickHouseRepository) SearchLogs(ctx context.Context, query LogQuery) (LogSearchResult, error) {
	where, whereArgs := buildLogsWhereClause(query)

	var total int64
	countSQL := "SELECT count() FROM otel_logs" + where
	if err := r.db.QueryRowContext(ctx, countSQL, whereArgs...).Scan(&total); err != nil {
		return LogSearchResult{}, fmt.Errorf("count logs: %w", err)
	}

	selectSQL := "SELECT Timestamp, ServiceName, SeverityText, Body, TraceId, SpanId FROM otel_logs" +
		where + " ORDER BY Timestamp DESC LIMIT ? OFFSET ?"
	selectArgs := append(append([]any{}, whereArgs...), query.Limit, query.Offset)

	rows, err := r.db.QueryContext(ctx, selectSQL, selectArgs...)
	if err != nil {
		return LogSearchResult{}, fmt.Errorf("search logs: %w", err)
	}
	defer rows.Close()

	var logs []LogRecord
	for rows.Next() {
		var l LogRecord
		if err := rows.Scan(&l.Timestamp, &l.ServiceName, &l.Severity, &l.Body, &l.TraceID, &l.SpanID); err != nil {
			return LogSearchResult{}, fmt.Errorf("scan log: %w", err)
		}
		logs = append(logs, l)
	}
	if err := rows.Err(); err != nil {
		return LogSearchResult{}, err
	}

	return LogSearchResult{Logs: logs, Total: total}, nil
}

// allowedClickHouseAgg は QueryMetrics が SQL に直接埋め込んでよい集計関数名のホワイトリスト。
// httpapi層 (QueryMetricsHandler) でも同じ集合を検証しているが、
// 「関数名はプレースホルダで渡せない (識別子はバインドパラメータの対象外)」という
// SQLの制約上、文字列を直接埋め込まざるを得ない箇所なので、
// 呼び出し元の検証を信頼せずこの層でも独立に検証する (多層防御、NFR-S-04)。
var allowedClickHouseAgg = map[string]bool{
	"avg": true, "sum": true, "min": true, "max": true, "count": true,
}

// serviceFilterClause は ServiceName が指定されている場合のみ AND 条件を追加する。
func serviceFilterClause(serviceName string) string {
	if serviceName == "" {
		return ""
	}
	return " AND ServiceName = ?"
}

// QueryMetrics は otel_metrics_gauge を Step 秒単位のバケットに丸めて集計する。
func (r *ClickHouseRepository) QueryMetrics(ctx context.Context, query MetricQuery) ([]MetricPoint, error) {
	if !allowedClickHouseAgg[query.AggregationFunc] {
		return nil, fmt.Errorf("unsupported aggregation function: %q", query.AggregationFunc)
	}

	stepSeconds := int(query.Step.Seconds())
	if stepSeconds <= 0 {
		stepSeconds = 60
	}

	// %d (数値) と %s (ホワイトリスト済みの集計関数名/固定WHERE句文字列) のみを
	// fmt.Sprintf で埋め込む。値そのもの (MetricName, Start, End, ServiceName) は
	// すべて後続の QueryContext の可変長引数 (プレースホルダ) 経由で渡す。
	sqlText := fmt.Sprintf(`
		SELECT toStartOfInterval(TimeUnix, INTERVAL %d SECOND) AS bucket, %s(Value) AS value
		FROM otel_metrics_gauge
		WHERE MetricName = ? AND TimeUnix >= ? AND TimeUnix <= ?%s
		GROUP BY bucket
		ORDER BY bucket
	`, stepSeconds, query.AggregationFunc, serviceFilterClause(query.ServiceName))

	args := []any{query.MetricName, query.Start, query.End}
	if query.ServiceName != "" {
		args = append(args, query.ServiceName)
	}

	rows, err := r.db.QueryContext(ctx, sqlText, args...)
	if err != nil {
		return nil, fmt.Errorf("query metrics: %w", err)
	}
	defer rows.Close()

	var points []MetricPoint
	for rows.Next() {
		var p MetricPoint
		if err := rows.Scan(&p.Timestamp, &p.Value); err != nil {
			return nil, fmt.Errorf("scan metric point: %w", err)
		}
		points = append(points, p)
	}
	return points, rows.Err()
}
