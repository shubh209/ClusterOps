# CONTEXT.md — ClusterOps

## Project

A **portfolio-grade, fully self-contained observability platform** for AI training cluster operations.
Demonstrates end-to-end product engineering for distributed GPU infrastructure: data pipeline,
real-time API, React console, and rule-based AI debugging assistant.

Target audience: OpenAI Compute Foundations / ML Infrastructure engineering roles.

---

## Domain Glossary

### Cluster
Five simulated GPU nodes collectively forming the training cluster. No Kubernetes dependency —
the cluster state is generated entirely by the Go simulator process.

### Node
A simulated physical GPU server. Each node has 8 NVIDIA H100 80GB GPUs and transitions through
four states: `healthy → degraded → unavailable → degraded → healthy`.

### Job
A synthetic ML training run submitted to the cluster. Each job has a framework (PyTorch / JAX /
TensorFlow), a model name, a GPU count, a priority (1–10), and a lifecycle:
`queued → running → [failed | completed | preempted]`.

### FailureReason
The root cause classification for a failed job. One of:
`oom | hardware_fault | preemption | timeout | user_error`.
This enum is the primary routing key for the AI assistant's rule engine.

### GPUMetric
A point-in-time telemetry snapshot for a single GPU: utilization %, memory used/total (GB),
temperature (°C), power (W). Emitted every 5 seconds by the simulator, stored in PostgreSQL,
cached in Redis, and served as time-series from the API.

### Alert
A threshold or event-driven notification. Severity: `critical | warning | info`.
Types: `node_unavailable | node_degraded | gpu_high_temperature | gpu_memory_full |
job_failed | capacity_waste | job_timeout | cluster_degraded`.

### ClusterSummary
Top-level health rollup: health score (0–100), node counts by status, job counts,
GPU utilization, waste percent, active alert count. Cached in Redis with 2s TTL.
Pushed to all SSE clients every 2 seconds by the broadcast loop.

### HealthScore
A computed metric (0–100) penalising unavailable nodes (−40 max), degraded nodes (−20 max),
GPU waste (−20 max), and high failure rate (−10 max).

### Simulator
A self-contained Go binary that generates all synthetic cluster activity.
Three loops: GPU telemetry every 5s, fault injection every 60s, job submission every 20–90s.
Publishes to Kafka. No minikube or cloud dependency.

### Ingestion Service
Consumes Kafka topics and writes durably to PostgreSQL + Redis.
The only writer to the database — the API server is read-only.

### SSE Broker
The API server's Server-Sent Events multiplexer. Connected clients receive `cluster_summary`,
`nodes`, and `alerts` push events every 2 seconds without polling.

### Rule-Based Assistant
The AI debugging engine at `internal/assistant/`. Input: a Job or Node struct + related alerts.
Output: `Analysis` — Headline, RootCause, Summary, DebuggingSteps (with shell commands),
PreventionTips, Confidence. No LLM dependency. Designed to be swapped with Ollama in Phase 5.

### Harness
The table-driven test suite at `internal/assistant/engine_test.go`. Validates that every
failure scenario produces the correct RootCause, Severity, minimum step count, and confidence.
Serves as a regression baseline when a real LLM is added.

---

## Architecture

```
Simulator (Go)
  └─► Kafka (4 topics) ──► Ingestion Service (Go)
                                 ├─► PostgreSQL  (durable store)
                                 └─► Redis       (hot cache)

API Server (Go, chi)
  ├── reads PostgreSQL + Redis
  ├── GET /api/v1/cluster/summary, /nodes, /jobs, /alerts, /metrics/*
  ├── GET /api/v1/stream         (SSE — pushes every 2s)
  ├── POST /api/v1/assistant/analyze
  └── GET /metrics               (Prometheus)

Frontend (React + Vite, port 3000)
  ├── Dashboard   — SSE-live health score, GPU heatmap, job ring, alerts feed
  ├── Nodes       — node grid + drilldown panel with sparklines
  ├── Jobs        — sortable table + detail drawer with logs
  ├── Alerts      — severity-sorted live feed
  └── Assistant   — job/node selector → analysis card with debugging steps

Observability
  ├── Prometheus  :9090  — scrapes /metrics from API + OTel collector
  ├── Grafana     :3001  — API latency, cache hit rate, error rate, SSE clients
  ├── Jaeger      :16686 — distributed traces from API + ingestion
  └── OTel Collector     — receives OTLP from services, fans out to Jaeger + Prometheus
```

---

## Data Flow

```
simulator → kafka.cluster.nodes    → ingestion → postgres.nodes    → redis (node cache)
simulator → kafka.cluster.jobs     → ingestion → postgres.jobs     → redis (job cache)
simulator → kafka.cluster.gpus     → ingestion → postgres.gpu_metrics
simulator → kafka.cluster.alerts   → ingestion → postgres.alerts   → redis (alert cache)

api server → read postgres / redis → HTTP response
api server → SSE broadcast loop    → frontend EventSource
```

---

## Key Files

| Path | Role |
|------|------|
| `backend/internal/models/` | Shared Go structs (Node, Job, GPUMetric, Alert, events) |
| `backend/internal/store/` | pgx queries — one file per entity |
| `backend/internal/cache/` | Redis helpers with typed TTLs |
| `backend/internal/kafka/` | Typed producer + consumer |
| `backend/internal/simulator/` | State machine + fault injector |
| `backend/internal/ingestion/` | Kafka→DB+Cache bridge |
| `backend/internal/api/` | chi router, handlers, SSE broker, Prometheus metrics |
| `backend/internal/assistant/` | Rule engine + eval harness |
| `backend/migrations/` | SQL schema — idempotent, embedded via go:embed |
| `frontend/src/types/index.ts` | TypeScript mirrors of Go models |
| `frontend/src/lib/api.ts` | Typed fetch client |
| `frontend/src/hooks/useSSE.ts` | EventSource with reconnect backoff |
| `infra/docker-compose.yml` | Full dev stack |
| `infra/otel/config.yaml` | OTel collector pipeline |
| `infra/prometheus/prometheus.yml` | Scrape config |
| `infra/grafana/dashboards/` | Provisioned Grafana dashboard JSON |

---

## AI Meta-Learning Notes

This project demonstrates several AI engineering patterns:

| Pattern | Where |
|---------|-------|
| **LLM as architect** | Used during initial design (CONTEXT.md as grounding document) |
| **LLM as implementer** | Used per-task with structured context in each session |
| **Context engineering** | CONTEXT.md + task list = structured context for every session |
| **Rule-based baseline** | `internal/assistant/` — deterministic output, verifiable |
| **Eval harness** | `engine_test.go` — measures correctness of the rule engine |
| **RAG scaffold** | DebuggingSteps carry structured retrieval (playbooks) — ready for pgvector |
| **MCP stub** | Phase 5: expose PostgreSQL as MCP server for assistant tool calls |

Phase 5 upgrade path:
1. Add Ollama container to docker-compose.yml
2. Install `pgvector` extension in PostgreSQL
3. Embed runbook text as vectors
4. Replace `assistant.Engine.Generate()` with Ollama chat completion + RAG retrieval
5. Expose cluster DB as MCP server so the LLM can query live data directly
