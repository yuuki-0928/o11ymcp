package store

import "context"

// MemoryServiceRepository is a temporary in-memory ServiceRepository.
//
// なぜ存在するか: docs/design.md §8 の実装順序では ClickHouse 実装は後回しにしている。
// それまでの間、ハンドラの配線 (cmd/query/main.go) を手元で動かして確認できるようにするための
// 開発用スタブ。ClickHouse 実装 (clickhouse_repository.go、後続コミットで追加予定) が入ったら
// main.go の配線先をそちらに差し替え、本ファイルは削除する。
type MemoryServiceRepository struct {
	Services []Service
}

func (m *MemoryServiceRepository) ListServices(ctx context.Context) ([]Service, error) {
	return m.Services, nil
}

// MemoryTraceRepository is a temporary in-memory TraceRepository (同上の理由で存在)。
// Spans は traceID をキーにグルーピング済みの状態で保持する。
type MemoryTraceRepository struct {
	SpansByTraceID map[string][]Span
}

func (m *MemoryTraceRepository) GetTrace(ctx context.Context, traceID string) ([]Span, error) {
	spans, ok := m.SpansByTraceID[traceID]
	if !ok {
		return nil, ErrTraceNotFound
	}
	return spans, nil
}

// MemoryLogRepository is a temporary in-memory LogRepository (同上の理由で存在)。
// ClickHouse実装と異なり大量データは想定しないため、フィルタは単純な線形走査で行う。
type MemoryLogRepository struct {
	Logs []LogRecord
}

func (m *MemoryLogRepository) SearchLogs(ctx context.Context, query LogQuery) (LogSearchResult, error) {
	matched := make([]LogRecord, 0, len(m.Logs))
	for _, l := range m.Logs {
		if query.ServiceName != "" && l.ServiceName != query.ServiceName {
			continue
		}
		if query.Severity != "" && l.Severity != query.Severity {
			continue
		}
		if !query.Start.IsZero() && l.Timestamp.Before(query.Start) {
			continue
		}
		if !query.End.IsZero() && l.Timestamp.After(query.End) {
			continue
		}
		matched = append(matched, l)
	}

	total := int64(len(matched))

	// offset/limit によるページ切り出し。範囲外アクセスにならないよう境界を丁寧に扱う。
	start := query.Offset
	if start > len(matched) {
		start = len(matched)
	}
	end := start + query.Limit
	if end > len(matched) {
		end = len(matched)
	}

	return LogSearchResult{Logs: matched[start:end], Total: total}, nil
}

// MemoryMetricRepository is a temporary in-memory MetricRepository (同上の理由で存在)。
// 実際の集計 (avg/sum/min/max/count によるバケット化) は ClickHouse 側の SQL に委ねる想定のため、
// ここでは動作確認に必要な最小限として、条件に一致する点をそのまま返すだけに留める。
type MemoryMetricRepository struct {
	Points []MetricPoint
}

func (m *MemoryMetricRepository) QueryMetrics(ctx context.Context, query MetricQuery) ([]MetricPoint, error) {
	matched := make([]MetricPoint, 0, len(m.Points))
	for _, p := range m.Points {
		if p.Timestamp.Before(query.Start) || p.Timestamp.After(query.End) {
			continue
		}
		matched = append(matched, p)
	}
	return matched, nil
}
