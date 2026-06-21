-- Migration 002: retention helper
-- Deletes gpu_metrics older than 24 hours to keep the demo DB small.
-- In production this would be a pg_cron job or TimescaleDB retention policy.

CREATE OR REPLACE FUNCTION prune_gpu_metrics() RETURNS void AS $$
BEGIN
    DELETE FROM gpu_metrics WHERE recorded_at < NOW() - INTERVAL '24 hours';
END;
$$ LANGUAGE plpgsql;

-- Manual call: SELECT prune_gpu_metrics();
-- Hooked into the ingestion service every 30 minutes.
