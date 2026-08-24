FROM golang:1.27.0-alpine3.24 AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/mcp ./cmd/mcp

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/mcp /mcp
EXPOSE 8081
ENTRYPOINT ["/mcp"]
