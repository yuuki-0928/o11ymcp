# o11y-mcp

A minimal-footprint Observability (o11y) OSS tool that ingests, stores, and queries OpenTelemetry (OTel) telemetry, and exposes it to LLMs / AI agents via the Model Context Protocol (MCP). Aimed at individual developers and small teams.

> **Status**: Requirements and design are finalized; implementation has not started yet. See [`docs/requirements.md`](./docs/requirements.md) and [`docs/design.md`](./docs/design.md) (Japanese; translation to follow) for details.

## Features

- OTLP (HTTP/gRPC) ingestion for Traces / Metrics / Logs
- Low-cost, high-compression columnar storage via ClickHouse (Hot/Cold tiered storage)
- REST Query API and an MCP server (Streamable HTTP) for telemetry access
- Docker Compose (primary target) and Azure Container Apps + Bicep (recommended production target)
- Deployable entirely within Japan regions (Japan East by default)
- Maintenance-monitoring pipeline that detects dependency and upstream (OTel/MCP) spec changes

## Architecture

See [`docs/architecture.drawio`](./docs/architecture.drawio) (open with [draw.io](https://app.diagrams.net)).

## Directory layout

```
o11y-mcp/
├── apps/ingest   # OTLP ingestion (OpenTelemetry Collector Builder distribution)
├── apps/query    # Query API (Go, REST)
├── apps/mcp      # MCP Server (Go, Streamable HTTP)
├── infra/azure   # Bicep (modularized)
├── deploy/compose
├── docs/
└── .github/
```

## License

[Apache License 2.0](./LICENSE)

## Contributing

Coming soon.
