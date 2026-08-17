FROM golang:1.26.6-alpine3.24 AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/ingest .

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/ingest /ingest
# otelcol-config.yaml はイメージに焼き込まず、deploy/compose からボリュームマウントする
# (FR-OP-06: 環境変数/設定ファイルで挙動を切替える方針。設定変更のたびに再ビルド不要にする)。
EXPOSE 4317 4318
ENTRYPOINT ["/ingest"]
CMD ["--config=/etc/o11y-mcp/otelcol-config.yaml"]
