# Azure Container Apps 向けのClickHouseイメージ。
# deploy/compose ではホストファイルをボリュームマウントして storage.xml を渡しているが、
# Container Apps にはその手段がないため、設定ファイルをイメージに焼き込む。
# (アカウントキー等の秘密情報はファイルには含めず、from_env でコンテナ起動時に注入する)
FROM clickhouse/clickhouse-server:26.7.3.19-alpine
COPY storage.xml /etc/clickhouse-server/config.d/storage.xml
