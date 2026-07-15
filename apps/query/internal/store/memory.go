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
