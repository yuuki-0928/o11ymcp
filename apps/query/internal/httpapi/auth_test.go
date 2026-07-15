package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TDD Red: このテストは RequireAPIKey がまだ存在しないので最初はコンパイルエラーで失敗する。
// 次のステップで auth.go に最小実装を追加してテストを通す(Green)。
func TestRequireAPIKey(t *testing.T) {
	// テスト対象のハンドラにたどり着いたら 200 を返す、成功判定用のダミーハンドラ。
	okHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	const validKey = "test-secret-key"
	middleware := RequireAPIKey(validKey, okHandler)

	tests := []struct {
		name       string
		authHeader string // 空文字はヘッダーなしを表す
		wantStatus int
	}{
		{
			name:       "ヘッダーなしは401",
			authHeader: "",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "Bearerスキームでないと401",
			authHeader: "Basic " + validKey,
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "キーが違うと401",
			authHeader: "Bearer wrong-key",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "正しいBearerキーなら200",
			authHeader: "Bearer " + validKey,
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		// Go にはPythonの pytest.mark.parametrize に近い仕組みがない代わりに、
		// テーブル駆動テスト + t.Run でサブテスト化するのが標準的な書き方。
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/v1/services", nil)
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}
			rec := httptest.NewRecorder()

			middleware.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
		})
	}
}
