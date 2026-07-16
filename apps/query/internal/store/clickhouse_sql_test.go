package store

import (
	"testing"
	"time"
)

// buildLogsWhereClause は ClickHouse への SQL 文字列を組み立てる純粋関数のため、
// DBなしでロジックだけを高速に検証できる。ここを最初に固めてから、
// 実際にClickHouseへ接続する結合テスト (clickhouse_repository_test.go) に進む。
func TestBuildLogsWhereClause(t *testing.T) {
	t.Run("条件なしなら空文字列", func(t *testing.T) {
		where, args := buildLogsWhereClause(LogQuery{})
		if where != "" {
			t.Errorf("where = %q, want empty", where)
		}
		if len(args) != 0 {
			t.Errorf("args = %v, want empty", args)
		}
	})

	t.Run("ServiceNameのみ", func(t *testing.T) {
		where, args := buildLogsWhereClause(LogQuery{ServiceName: "checkout"})
		wantWhere := " WHERE ServiceName = ?"
		if where != wantWhere {
			t.Errorf("where = %q, want %q", where, wantWhere)
		}
		if len(args) != 1 || args[0] != "checkout" {
			t.Errorf("args = %v, want [checkout]", args)
		}
	})

	t.Run("全条件指定", func(t *testing.T) {
		start := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
		end := time.Date(2026, 7, 2, 0, 0, 0, 0, time.UTC)
		where, args := buildLogsWhereClause(LogQuery{
			ServiceName: "checkout",
			Severity:    "ERROR",
			Start:       start,
			End:         end,
		})
		wantWhere := " WHERE ServiceName = ? AND SeverityText = ? AND Timestamp >= ? AND Timestamp <= ?"
		if where != wantWhere {
			t.Errorf("where = %q, want %q", where, wantWhere)
		}
		wantArgs := []any{"checkout", "ERROR", start, end}
		if len(args) != len(wantArgs) {
			t.Fatalf("args = %v, want %v", args, wantArgs)
		}
		for i := range wantArgs {
			if args[i] != wantArgs[i] {
				t.Errorf("args[%d] = %v, want %v", i, args[i], wantArgs[i])
			}
		}
	})
}

func TestServiceFilterClause(t *testing.T) {
	if got := serviceFilterClause(""); got != "" {
		t.Errorf("serviceFilterClause(\"\") = %q, want empty", got)
	}
	if got := serviceFilterClause("checkout"); got != " AND ServiceName = ?" {
		t.Errorf("serviceFilterClause(checkout) = %q, want %q", got, " AND ServiceName = ?")
	}
}
