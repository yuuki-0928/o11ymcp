package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRequireAPIKeyMiddleware(t *testing.T) {
	okHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	const validKey = "test-secret-key"
	middleware := requireAPIKeyMiddleware(validKey, okHandler)

	tests := []struct {
		name       string
		authHeader string
		wantStatus int
	}{
		{name: "ヘッダーなしは401", authHeader: "", wantStatus: http.StatusUnauthorized},
		{name: "キーが違うと401", authHeader: "Bearer wrong-key", wantStatus: http.StatusUnauthorized},
		{name: "正しいBearerキーなら200", authHeader: "Bearer " + validKey, wantStatus: http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/", nil)
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
