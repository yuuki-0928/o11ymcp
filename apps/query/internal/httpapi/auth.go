// Package httpapi は Query API の HTTP ハンドラ・ミドルウェア層をまとめる。
// ClickHouse など永続化層への依存はここには置かず、リクエスト/レスポンスの取り扱いに専念させる
// (設計書 docs/design.md の「責務分離」方針に対応)。
package httpapi

import (
	"net/http"
	"strings"
)

// RequireAPIKey は Authorization: Bearer <expectedKey> ヘッダーを検証するミドルウェア。
//
// なぜミドルウェアとして切り出すか:
//   - FR-QR-05 (Query API は認証を要求する) は全エンドポイント共通の要件のため、
//     各ハンドラに認証ロジックを重複させず、http.Handler をラップする形にして一箇所に集約する。
//   - v1.0 は要件定義書で「API Key のみ」と確定している (Entra ID は Out of Scope)。
//     将来 Entra ID 対応を追加する場合も、この関数のシグネチャ (http.Handler を受けて返す) を
//     保ったまま別の認証ミドルウェアを追加・差し替えできるようにしている。
func RequireAPIKey(expectedKey string, next http.Handler) http.Handler {
	// Python の Flask/FastAPI におけるデコレータ/依存性注入に相当する処理を、
	// Go では「http.Handler を受け取り、http.Handler を返す関数」として書くのが定石。
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		const prefix = "Bearer "

		header := r.Header.Get("Authorization")
		if !strings.HasPrefix(header, prefix) {
			// ヘッダーなし・Bearer 以外のスキームはどちらも「キーなし」として扱う。
			http.Error(w, "missing or invalid Authorization header", http.StatusUnauthorized)
			return
		}

		key := strings.TrimPrefix(header, prefix)
		if key != expectedKey {
			http.Error(w, "invalid API key", http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	})
}
