package httpapi

import (
	"encoding/json"
	"net/http"

	"github.com/nakagawa-yuki/o11y-mcp/query/internal/store"
)

// listServicesResponse は GET /v1/services のレスポンス body (docs/design.md §5.1)。
// store.Service をそのまま返さず、API として安定させたい形 (サービス名の配列) に詰め替える。
type listServicesResponse struct {
	Services []string `json:"services"`
}

// ListServicesHandler は GET /v1/services を処理するハンドラを組み立てて返す。
//
// 引数を store.ServiceRepository インターフェースにしている理由:
//   - ハンドラのテストで本物の ClickHouse を起動しなくて済む (fakeServiceRepository で代替可能)。
//   - main.go 側で「本番は ClickHouse 実装、テストはフェイク」を差し替えられる
//     (Go にはDIコンテナのような仕組みは無く、関数引数やstructフィールドで依存を渡すのが定石)。
func ListServicesHandler(repo store.ServiceRepository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		services, err := repo.ListServices(r.Context())
		if err != nil {
			// リポジトリのエラー内容 (ClickHouse の詳細なエラーメッセージ等) をそのまま
			// クライアントに返すと内部実装の漏洩になりうるため、固定メッセージに丸める。
			// 詳細はサーバー側ログ (今後 slog を導入予定) に出す想定。
			http.Error(w, "failed to list services", http.StatusInternalServerError)
			return
		}

		names := make([]string, len(services))
		for i, s := range services {
			names[i] = s.Name
		}

		w.Header().Set("Content-Type", "application/json")
		// エンコードエラーはこの時点で書き込み済みのレスポンスに対しては対処しようがないため、
		// (ヘッダーが既に送出済み) ここでは意図的に無視している。
		_ = json.NewEncoder(w).Encode(listServicesResponse{Services: names})
	}
}
