#!/bin/sh
# ClickHouseのS3ディスク (cold volume) が使うバケットを用意する。
# ClickHouseはバケットを自動作成しないため、ClickHouse起動前にこのバケットが存在している必要がある。
set -eu

echo "waiting for minio to become reachable..."
until mc alias set local http://minio:9000 "$MINIO_ROOT_USER" "$MINIO_ROOT_PASSWORD" 2>/dev/null; do
  sleep 2
done

mc mb --ignore-existing local/o11y-mcp-cold
echo "bucket ready."
