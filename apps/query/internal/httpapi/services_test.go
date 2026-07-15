package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/nakagawa-yuki/o11y-mcp/query/internal/store"
)

// fakeServiceRepository は store.ServiceRepository のテスト用実装。
// ClickHouse に依存せずハンドラのロジックだけを検証するため、
// 「本物のリポジトリの代わりに差し込める」ようインターフェースに対して実装する。
// (Pythonでの unittest.mock.Mock 相当の役割を、Goでは明示的な struct + インターフェース実装で行う)
type fakeServiceRepository struct {
	services []store.Service
	err      error
}

func (f *fakeServiceRepository) ListServices(ctx context.Context) ([]store.Service, error) {
	return f.services, f.err
}

func TestListServicesHandler(t *testing.T) {
	t.Run("正常系: サービス一覧をJSONで返す", func(t *testing.T) {
		repo := &fakeServiceRepository{
			services: []store.Service{{Name: "checkout"}, {Name: "payment"}},
		}
		handler := ListServicesHandler(repo)

		req := httptest.NewRequest(http.MethodGet, "/v1/services", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
		}

		var got struct {
			Services []string `json:"services"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatalf("failed to unmarshal response body %q: %v", rec.Body.String(), err)
		}

		want := []string{"checkout", "payment"}
		if len(got.Services) != len(want) {
			t.Fatalf("services = %v, want %v", got.Services, want)
		}
		for i, name := range want {
			if got.Services[i] != name {
				t.Errorf("services[%d] = %q, want %q", i, got.Services[i], name)
			}
		}
	})

	t.Run("異常系: リポジトリがエラーを返したら500", func(t *testing.T) {
		repo := &fakeServiceRepository{err: errors.New("clickhouse unavailable")}
		handler := ListServicesHandler(repo)

		req := httptest.NewRequest(http.MethodGet, "/v1/services", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
		}
	})
}
