// Package store は Query API から見た「テレメトリの永続化層」への依存を、
// 具体的な ClickHouse 実装からインターフェースとして切り離すためのパッケージ。
//
// なぜインターフェースで切るか:
//   - docs/design.md の実装順序案 (§8) の通り、Query API のハンドラは
//     まず ClickHouse なしでユニットテストできる状態にしたい。
//   - ClickHouse を使った実装 (clickhouse_repository.go、後続コミットで追加予定) は
//     testcontainers を使った結合テストで別途検証する。
package store

import "context"

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
