// Package store は Query API から見た「テレメトリの永続化層」への依存を、
// 具体的な ClickHouse 実装からインターフェースとして切り離すためのパッケージ。
//
// なぜインターフェースで切るか:
//   - docs/design.md の実装順序案 (§8) の通り、Query API のハンドラは
//     まず ClickHouse なしでユニットテストできる状態にしたい。
//   - ClickHouse を使った実装 (clickhouse_repository.go、後続コミットで追加予定) は
//     testcontainers を使った結合テストで別途検証する。
package store

import (
	"context"
	"errors"
	"time"
)

// Service は Query API が扱う「既知のサービス」を表すドメインモデル。
// ClickHouse の生の行データや JSON タグをここに持ち込まず、httpapi 層でレスポンス表現に変換する
// (ドメインモデルとAPIレスポンス表現を分離し、将来レスポンス形式だけ変わってもここに影響させない)。
type Service struct {
	Name string
}

// ServiceRepository は「既知のサービス一覧を取得する」責務のみを持つ最小のインターフェース。
// 必要になったメソッドから増やしていく方針とし、先回りして大きなインターフェースを作らない。
type ServiceRepository interface {
	ListServices(ctx context.Context) ([]Service, error)
}

// Span は OTel の 1 span を表すドメインモデル (docs/design.md §4 の otel_traces テーブルに対応)。
// 属性 (Attributes) 等は FR-QR-02 のスコープでは未使用のため、必要になった時点で追加する
// (先回りしてOTelの全フィールドを持たせない = YAGNI)。
type Span struct {
	TraceID      string
	SpanID       string
	ParentSpanID string
	ServiceName  string
	Name         string
	StartTime    time.Time
	EndTime      time.Time
}

// ErrTraceNotFound は指定された Trace ID に対応する span が1件も無かったことを表す。
// ハンドラ側はこの sentinel error を errors.Is で判定して 404 を返す (それ以外のエラーは500)。
// Go には例外機構が無く、こうした「呼び出し元が種類を判定したいエラー」は
// sentinel error か独自のエラー型として表現するのが一般的な作法。
var ErrTraceNotFound = errors.New("trace not found")

// TraceRepository は「Trace ID から span 一覧を取得する」責務のみを持つインターフェース。
type TraceRepository interface {
	GetTrace(ctx context.Context, traceID string) ([]Span, error)
}
