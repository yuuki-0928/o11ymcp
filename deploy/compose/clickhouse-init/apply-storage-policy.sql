-- Ingest (clickhouseexporter) が起動時に自動作成するテーブルを、
-- config.d/storage.xml で定義した hot_cold ストレージポリシーに切り替え、
-- TTLでホット保持7日 → コールド(MinIO)保持30日 → 削除、を設定する (FR-ST-03, FR-ST-04 既定値)。
--
-- ALTER TABLE ... MODIFY SETTING / MODIFY TTL はいずれも冪等 (何度実行しても結果は同じ) なので、
-- このスクリプトは compose 起動のたびに実行してよい。
--
-- otel_traces_trace_id_ts (TraceID逆引き用の内部インデックステーブル) は対象外としている。
-- データ量が小さく、要件定義書のFR-ST-03/04が想定する「テレメトリ本体」ではないため。

ALTER TABLE otel_logs MODIFY SETTING storage_policy = 'hot_cold';
ALTER TABLE otel_logs MODIFY TTL
    toDateTime(Timestamp) + INTERVAL 7 DAY TO VOLUME 'cold',
    toDateTime(Timestamp) + INTERVAL 30 DAY DELETE;

ALTER TABLE otel_traces MODIFY SETTING storage_policy = 'hot_cold';
ALTER TABLE otel_traces MODIFY TTL
    toDateTime(Timestamp) + INTERVAL 7 DAY TO VOLUME 'cold',
    toDateTime(Timestamp) + INTERVAL 30 DAY DELETE;

ALTER TABLE otel_metrics_gauge MODIFY SETTING storage_policy = 'hot_cold';
ALTER TABLE otel_metrics_gauge MODIFY TTL
    toDateTime(TimeUnix) + INTERVAL 7 DAY TO VOLUME 'cold',
    toDateTime(TimeUnix) + INTERVAL 30 DAY DELETE;

ALTER TABLE otel_metrics_sum MODIFY SETTING storage_policy = 'hot_cold';
ALTER TABLE otel_metrics_sum MODIFY TTL
    toDateTime(TimeUnix) + INTERVAL 7 DAY TO VOLUME 'cold',
    toDateTime(TimeUnix) + INTERVAL 30 DAY DELETE;

ALTER TABLE otel_metrics_histogram MODIFY SETTING storage_policy = 'hot_cold';
ALTER TABLE otel_metrics_histogram MODIFY TTL
    toDateTime(TimeUnix) + INTERVAL 7 DAY TO VOLUME 'cold',
    toDateTime(TimeUnix) + INTERVAL 30 DAY DELETE;

ALTER TABLE otel_metrics_summary MODIFY SETTING storage_policy = 'hot_cold';
ALTER TABLE otel_metrics_summary MODIFY TTL
    toDateTime(TimeUnix) + INTERVAL 7 DAY TO VOLUME 'cold',
    toDateTime(TimeUnix) + INTERVAL 30 DAY DELETE;

ALTER TABLE otel_metrics_exponential_histogram MODIFY SETTING storage_policy = 'hot_cold';
ALTER TABLE otel_metrics_exponential_histogram MODIFY TTL
    toDateTime(TimeUnix) + INTERVAL 7 DAY TO VOLUME 'cold',
    toDateTime(TimeUnix) + INTERVAL 30 DAY DELETE;
