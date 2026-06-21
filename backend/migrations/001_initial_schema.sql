-- Migration 001: initial schema for ClusterOps
-- Run order matters: nodes → jobs (FK) → gpu_metrics (FK) → alerts

CREATE EXTENSION IF NOT EXISTS "pgcrypto"; -- for gen_random_uuid()

-- ─── nodes ───────────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS nodes (
    id                   TEXT PRIMARY KEY,
    hostname             TEXT NOT NULL,
    status               TEXT NOT NULL DEFAULT 'healthy',
    gpu_count            INT  NOT NULL DEFAULT 0,
    gpu_model            TEXT NOT NULL DEFAULT '',
    cpu_cores            INT  NOT NULL DEFAULT 0,
    memory_gb            DOUBLE PRECISION NOT NULL DEFAULT 0,
    allocated_gpus       INT  NOT NULL DEFAULT 0,
    gpu_utilization      DOUBLE PRECISION[] NOT NULL DEFAULT '{}',
    gpu_memory_used_gb   DOUBLE PRECISION[] NOT NULL DEFAULT '{}',
    gpu_memory_total_gb  DOUBLE PRECISION[] NOT NULL DEFAULT '{}',
    gpu_temperature_c    DOUBLE PRECISION[] NOT NULL DEFAULT '{}',
    gpu_power_watts      DOUBLE PRECISION[] NOT NULL DEFAULT '{}',
    labels               JSONB NOT NULL DEFAULT '{}',
    last_seen            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at           TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_nodes_status    ON nodes (status);
CREATE INDEX IF NOT EXISTS idx_nodes_last_seen ON nodes (last_seen DESC);

-- ─── jobs ─────────────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS jobs (
    id               TEXT PRIMARY KEY,
    name             TEXT NOT NULL,
    status           TEXT NOT NULL DEFAULT 'queued',
    framework        TEXT NOT NULL DEFAULT 'pytorch',
    model_name       TEXT NOT NULL DEFAULT '',
    requested_gpus   INT  NOT NULL DEFAULT 0,
    assigned_nodes   TEXT[] NOT NULL DEFAULT '{}',
    start_time       TIMESTAMPTZ,
    end_time         TIMESTAMPTZ,
    failure_reason   TEXT,
    failure_message  TEXT NOT NULL DEFAULT '',
    log_tail         TEXT[] NOT NULL DEFAULT '{}',
    priority         INT  NOT NULL DEFAULT 5,
    user_id          TEXT NOT NULL DEFAULT 'system',
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_jobs_status     ON jobs (status);
CREATE INDEX IF NOT EXISTS idx_jobs_created_at ON jobs (created_at DESC);
CREATE INDEX IF NOT EXISTS idx_jobs_user_id    ON jobs (user_id);

-- ─── gpu_metrics ──────────────────────────────────────────────────────────────
-- TimeSeries table; will grow large — partitioned by day in production.
-- For demo, plain table is fine.
CREATE TABLE IF NOT EXISTS gpu_metrics (
    id               BIGSERIAL PRIMARY KEY,
    node_id          TEXT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
    gpu_index        INT  NOT NULL,
    recorded_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    utilization_pct  DOUBLE PRECISION NOT NULL DEFAULT 0,
    memory_used_gb   DOUBLE PRECISION NOT NULL DEFAULT 0,
    memory_total_gb  DOUBLE PRECISION NOT NULL DEFAULT 0,
    temperature_c    DOUBLE PRECISION NOT NULL DEFAULT 0,
    power_watts      DOUBLE PRECISION NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_gpu_metrics_node_time
    ON gpu_metrics (node_id, recorded_at DESC);
CREATE INDEX IF NOT EXISTS idx_gpu_metrics_time
    ON gpu_metrics (recorded_at DESC);

-- ─── alerts ───────────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS alerts (
    id           TEXT PRIMARY KEY DEFAULT gen_random_uuid()::TEXT,
    severity     TEXT NOT NULL DEFAULT 'info',
    type         TEXT NOT NULL,
    title        TEXT NOT NULL,
    message      TEXT NOT NULL DEFAULT '',
    node_id      TEXT REFERENCES nodes(id) ON DELETE SET NULL,
    job_id       TEXT REFERENCES jobs(id)  ON DELETE SET NULL,
    triggered_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    resolved_at  TIMESTAMPTZ,
    resolved     BOOLEAN NOT NULL DEFAULT FALSE
);

CREATE INDEX IF NOT EXISTS idx_alerts_resolved    ON alerts (resolved);
CREATE INDEX IF NOT EXISTS idx_alerts_triggered   ON alerts (triggered_at DESC);
CREATE INDEX IF NOT EXISTS idx_alerts_severity    ON alerts (severity);
CREATE INDEX IF NOT EXISTS idx_alerts_node_id     ON alerts (node_id) WHERE node_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_alerts_job_id      ON alerts (job_id)  WHERE job_id  IS NOT NULL;
