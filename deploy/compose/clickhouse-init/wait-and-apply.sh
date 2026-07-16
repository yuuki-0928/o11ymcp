#!/bin/sh
# Ingest (clickhouseexporter) はコレクタ起動時にテーブルを自動作成するが、
# その完了タイミングをdocker composeのhealthcheckだけでは表現しづらい
# (Ingestに現状ヘルスチェック用のHTTPエンドポイントを組み込んでいないため)。
# そのため、対象テーブルが実際に現れるまでポーリングしてから移行SQLを適用する。
set -eu

CH_HOST="${CLICKHOUSE_HOST:-clickhouse}"
CH_USER="${CLICKHOUSE_USER:-default}"
CH_PASSWORD="${CLICKHOUSE_PASSWORD:-}"

echo "waiting for otel_logs table to be created by ingest..."
until clickhouse-client --host="$CH_HOST" --user="$CH_USER" --password="$CH_PASSWORD" \
    --query "EXISTS TABLE otel_logs" 2>/dev/null | grep -q '^1$'; do
  sleep 2
done

echo "applying storage policy / TTL migration..."
clickhouse-client --host="$CH_HOST" --user="$CH_USER" --password="$CH_PASSWORD" \
    --queries-file=/migrations/apply-storage-policy.sql

echo "done."
