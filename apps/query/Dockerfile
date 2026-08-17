# マルチステージビルド: ビルド時の依存 (Goツールチェイン) を最終イメージに残さないことで、
# イメージサイズとアタックサーフェスを最小化する (NFR-C-01のコスト最小化、NFR-S-04にも寄与)。
FROM golang:1.26.6-alpine3.24 AS build
WORKDIR /src

# go.mod/go.sum だけ先にコピーして `go mod download` することで、
# ソースコードだけ変更した再ビルド時に依存解決のレイヤーキャッシュを効かせる。
COPY go.mod go.sum ./
RUN go mod download

COPY . .
# CGO_ENABLED=0: distroless (libcなし) でも動く静的バイナリにする。
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/query ./cmd/query

# distroless: シェルやパッケージマネージャを含まない最小の実行イメージ。
# nonroot: 非rootユーザー (UID 65532) で実行する (最小権限の原則)。
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/query /query
EXPOSE 8080
ENTRYPOINT ["/query"]
