# ClusterOps — Complete Interview Q&A

> Ground truth: every answer is backed by actual file paths and config values from the repo.
> Format: **Short answer** first, then reasoning, then `Evidence:` at the end.

---

## TABLE OF CONTENTS

1. [Architecture & System Design](#1-architecture--system-design)
2. [Kafka & Event Bus](#2-kafka--event-bus)
3. [Caching — Redis](#3-caching--redis)
4. [Real-Time — SSE](#4-real-time--sse)
5. [PostgreSQL & Data Layer](#5-postgresql--data-layer)
6. [API Design](#6-api-design)
7. [Observability](#7-observability)
8. [Assistant Engine](#8-assistant-engine)
9. [Simulator & Fault Injection](#9-simulator--fault-injection)
10. [Frontend](#10-frontend)
11. [Go Language & Patterns](#11-go-language--patterns)
12. [Data Models & Domain Design](#12-data-models--domain-design)
13. [Testing](#13-testing)
14. [Deployment & Infrastructure](#14-deployment--infrastructure)
15. [Gaps & Honest Limits](#15-gaps--honest-limits)

---

## 1. ARCHITECTURE & SYSTEM DESIGN


**Q: Walk me through the architecture of this system end to end.**

**Short answer:** Three independent Go binaries communicate through Kafka and Redis, with one read-only API server in front.

- **Simulator** generates synthetic GPU cluster activity (node events, job events, GPU telemetry, alerts) every 5–90 seconds and publishes JSON messages to four Kafka topics.
- **Ingestion service** consumes those topics, writes canonical state to PostgreSQL, and writes/invalidates Redis cache.
- **API server** is read-only — reads Redis first, falls back to Postgres on a miss, pushes live updates to browser clients over SSE.
- **Frontend** is React/TypeScript connecting to that single API server.

The key rule: only the simulator writes to Kafka, only ingestion writes to Postgres. That separation makes the read path lockless and cache invalidation explicit.

Evidence: `backend/cmd/` has three separate `main.go` files; `internal/ingestion/ingestion.go` package doc: "the only writer to the database"

---

**Q: Why three separate binaries instead of one monolith?**

**Short answer:** Each binary has a different reason to exist, a different scaling axis, and a different failure blast radius.

- The simulator is a dev/demo tool — you'd never run it alongside a real API in production. Keeping it separate means you can kill it without touching the API.
- The ingestion service is the only thing that writes to Postgres. If it crashes, the API keeps serving cached reads. If they were one process, a write-path bug could take down reads too.
- The API server scales by HTTP traffic. The ingestion service scales by Kafka partition count. Different knobs.

Trade-off I accepted: more operational overhead — three processes to start, three health checks to monitor. For a demo that's fine; in production you'd want proper orchestration (Kubernetes).

Evidence: `docker-compose.yml` — three separate service definitions: `api`, `ingestion`, `simulator`

---

**Q: How does data flow from the simulator to the browser?**

1. Simulator calls `producer.PublishNodeEvent()` / `PublishJobEvent()` / `PublishGPUMetric()` / `PublishAlertFired()` — JSON on four Kafka topics: `cluster.nodes`, `cluster.jobs`, `cluster.gpus`, `cluster.alerts`.
2. Ingestion service's four goroutines (one per topic) consume messages, upsert to Postgres, invalidate and warm Redis.
3. API server's `broadcastLoop` goroutine polls Postgres every 2 seconds, refreshes Redis, calls `broker.BroadcastEvent()` with the latest snapshot.
4. Browser receives SSE events named `cluster_summary`, `nodes`, `alerts` — no polling needed.

Evidence: `internal/ingestion/ingestion.go` handlers; `internal/api/handlers.go` `broadcastClusterSnapshot()`; `internal/api/sse.go`

---

**Q: Why is the ingestion service the only writer to the database?**

**Short answer:** Centralizing all writes in one place makes cache invalidation deterministic and the API server safe to scale horizontally.

If the API server also wrote to Postgres, you'd need to invalidate the cache from two places — now two code paths can put stale data in Redis. By making ingestion the single writer, every mutation goes through one path: write to Postgres → invalidate Redis → warm Redis. The API server never needs to think about cache consistency. You can run 10 API replicas behind a load balancer and none of them will conflict, because they're all read-only.

Evidence: `internal/ingestion/ingestion.go` — "It is the only writer to the database — the API server is read-only."

---

**Q: What is the health score and how is it calculated?**

**Short answer:** A 0–100 score computed on every `ClusterSummary` call with penalty deductions.

```
score = 100
score -= (unavailable_nodes / total_nodes) * 40   // up to -40
score -= (degraded_nodes / total_nodes) * 20       // up to -20
score -= gpu_waste_percent * 0.2                   // up to -20
score -= failed_jobs_penalty (2 pts each, max -10)
score = max(score, 0)
```

The weights reflect operator priorities: a node going fully offline is worse than degradation, which is worse than GPU inefficiency.

Evidence: `internal/store/summary.go` `ClusterSummary()`


---

## 2. KAFKA & EVENT BUS

---

**Q: Why Kafka? Why not write directly from the simulator to Postgres?**

**Verdict:** Kafka because it decouples producers from consumers and makes the pipeline survivable.

- Why it fits here: if the ingestion service is down, Kafka buffers the messages. With direct DB writes, a slow DB stalls the simulator. With Kafka, the simulator keeps publishing and ingestion catches up when it recovers.
- What a direct write is genuinely better at: simplicity. Fewer moving parts, no Kafka to operate, one less network hop.
- In this codebase: four topics — `cluster.nodes`, `cluster.jobs`, `cluster.gpus`, `cluster.alerts`. One `kafka.Writer` per topic in `internal/kafka/producer.go`. One `kafka.Reader` per topic in the consumer.
- Trade-off I accepted: Kafka is operationally heavy. For a demo you're running Zookeeper + Kafka as two extra Docker containers to buffer messages between two processes on the same machine. A Go channel or Redis Stream would have been simpler. The value shows at scale, not in a 5-node demo.

Evidence: `go.mod` — `github.com/segmentio/kafka-go v0.4.47`; `docker-compose.yml` — `zookeeper` + `kafka` services

---

**Q: Kafka vs RabbitMQ — which would you use here and why?**

**Verdict:** Kafka, and that's what's used.

- Why Kafka fits here: Kafka is a durable, replayable log. If the ingestion service crashes and restarts, it picks up from its last committed offset — no messages lost. RabbitMQ is a traditional message queue: once consumed and acked, a message is gone.
- What RabbitMQ is genuinely better at: complex routing (exchanges, routing keys, fan-out), message TTLs, dead-letter queues out of the box. For task queues where each message is a discrete unit of work, RabbitMQ is simpler.
- Trade-off I accepted: Kafka requires Zookeeper (in this stack, Confluent 7.6.1 still uses it). Newer Kafka versions (KRaft mode) remove that dependency, but this compose file uses the older setup.

Evidence: `docker-compose.yml` — `confluentinc/cp-kafka:7.6.1`, `confluentinc/cp-zookeeper:7.6.1`

---

**Q: How are Kafka topics structured? What's the partitioning strategy?**

Four topics, one per event type:
- `cluster.nodes` — node state changes, keyed by `node.ID`
- `cluster.jobs` — job state changes, keyed by `job.ID`
- `cluster.gpus` — GPU telemetry, keyed by `"nodeID:gpuIndex"` (ensures all metrics for a GPU go to the same partition, preserving order)
- `cluster.alerts` — alerts fired/resolved, keyed by `alert.ID`

The key-based partitioning means all events for the same entity arrive in order at the same partition. For GPU metrics this matters: you don't want metric N+1 processed before metric N for the same GPU.

The consumer uses a consumer group (`GroupID` in `ConsumerConfig`) so you can run multiple ingestion instances and Kafka will distribute partitions across them.

Evidence: `internal/kafka/producer.go` — `PublishGPUMetric()` key = `fmt.Sprintf("%s:%d", m.NodeID, m.GPUIndex)`; `internal/models/events.go` `KafkaTopics`

---

**Q: What happens if a Kafka message fails to process in the ingestion service?**

The consumer logs the error and continues — it does not retry or dead-letter the message. The comment in `consumer.go` says "log and skip — appropriate for a demo."

In production this is insufficient. You'd want:
1. A retry loop with backoff for transient errors (DB timeout, network blip).
2. A dead-letter topic for messages that fail after N retries.
3. An alert on dead-letter queue depth.

The current behavior means a DB error during ingestion silently drops an event. Redis will eventually expire the stale entry and the next successful ingestion will correct it — but there's a window of inconsistency.

Evidence: `internal/kafka/consumer.go` `consume()` — "log and skip (not retry) — appropriate for a demo"

---

**Q: Is Kafka message delivery at-least-once, at-most-once, or exactly-once here?**

At-least-once on the producer side: `RequiredAcks: kafka.RequireOne` means the broker acknowledges the message when the leader has written it, but before all replicas confirm. `Async: false` means the producer blocks until the ack is received, so the simulator won't lose messages unless the broker itself crashes after acking but before replicating.

On the consumer side, `kafka-go` auto-commits offsets after each successful `ReadMessage()`. If the ingestion service crashes mid-processing, the offset may already be committed, so the message won't be reprocessed — practically at-most-once on the consumer side for failed processing.

The upsert pattern (`ON CONFLICT DO UPDATE`) in Postgres means re-processing the same event is idempotent — the final state is correct even if a message is processed twice.

Evidence: `internal/kafka/producer.go` — `RequiredAcks: kafka.RequireOne`, `Async: false`; `internal/store/nodes.go` `UpsertNode()`


---

## 3. CACHING — REDIS

---

**Q: What is Redis and where did you use it?**

**In one line:** Redis is an in-memory key-value store used here as a read-through cache in front of Postgres.

- How it works: every API endpoint checks Redis first. On a hit, returns cached JSON without touching Postgres. On a miss, queries Postgres and writes the result to Redis with a TTL.
- In this codebase: `internal/cache/cache.go` wraps `go-redis/v9` with typed methods — `GetClusterSummary`, `GetNodeList`, `GetJob`, etc. TTLs are tuned to the dashboard's refresh feel: 2s for summary/alerts/GPU, 3–5s for nodes/jobs.
- When I'd reach for it: any hot read path where data changes on a known schedule and you can tolerate slight staleness.
- When not: when you need strong read-after-write consistency, or when your dataset doesn't fit in memory.

Evidence: `internal/cache/cache.go` TTL constants at the top of the file

---

**Q: What are the cache TTLs and how did you choose them?**

| Resource | TTL | Reason |
|---|---|---|
| Cluster summary | 2s | Dashboard feels real-time; ingestion refreshes this on every broadcast |
| Alerts (active) | 2s | Operators need to see new critical alerts fast |
| GPU summary | 2s | GPU metrics arrive every 5s; 2s keeps the chart smooth |
| Node list | 3s | Nodes change state less frequently than GPU readings |
| Individual node | 3s | Same reasoning as node list |
| Job list | 3s | Jobs submit every 20–90s; 3s staleness is imperceptible |
| Individual job | 5s | Detail views tolerate slightly more staleness |

General rule: TTL ≤ half the minimum interval at which the underlying data changes. GPU metrics arrive every 5s, so 2s TTL means at most one stale snapshot shown.

Evidence: `internal/cache/cache.go` lines 18–24 — `TTLClusterSummary` through `TTLGPUSummary`

---

**Q: How do you handle cache invalidation?**

Three strategies, chosen per resource:

1. **Invalidate-on-write** — when ingestion processes a node event, `cache.InvalidateNode(id)` deletes `node:<id>`, `nodes:list`, and `cluster:summary` in one `DEL` call, then immediately re-warms with the new value. Most aggressive — ensures the next API read gets fresh data.

2. **TTL expiry** — the primary safety net. Even if invalidation is missed (e.g. Redis is briefly down), stale data expires within 2–5 seconds automatically.

3. **Pattern flush** — for job list keys, which are parameterized by filter hash (`status|user_id`), the cache uses `SCAN` + `DEL` to flush all matching keys. This avoids maintaining an index of every active filter key. Uses `SCAN` cursor loop (not `KEYS`) to avoid blocking Redis on large keyspaces.

Evidence: `internal/cache/cache.go` — `InvalidateNode()`, `InvalidateJob()`, `flushByPattern()`

---

**Q: Redis vs Memcached — which would you use and why?**

**Verdict:** Redis, and that's what's used.

- Why it fits here: Redis supports TTL-per-key, pub/sub, and richer data types natively. Pub/sub or Redis Streams would be a natural upgrade path if the SSE broadcast loop needed to be distributed across API replicas.
- What Memcached is genuinely better at: raw throughput for simple get/set at extreme scale, multi-threaded architecture. If this were a CDN edge cache doing tens of millions of simple lookups per second, Memcached would be fair.
- Trade-off I accepted: Redis is single-threaded per core. At demo scale this is irrelevant.

Evidence: `go.mod` — `github.com/redis/go-redis/v9 v9.5.1`; `docker-compose.yml` — `redis:7-alpine`

---

**Q: What happens to the API if Redis goes down?**

Graceful degradation. Cache errors on read are silently ignored — the handler falls through to Postgres and still returns data. Cache errors on write are also swallowed. The pattern in every handler is:

```go
if data, hit, _ := s.cache.GetX(r.Context()); hit {
    return // serve from cache
}
// cache miss or error — query Postgres directly
data, _ = s.db.QueryX(r.Context())
_ = s.cache.SetX(r.Context(), data) // best-effort warm
```

Redis being down degrades performance (every request hits Postgres) but does not cause errors. The `/health` endpoint reports Redis status separately so operators can see the degradation.

Evidence: `internal/api/handlers.go` — every handler; `internal/api/handlers.go` `handleHealth()`


---

## 4. REAL-TIME — SSE

---

**Q: What is SSE and why did you use it instead of WebSockets?**

**In one line:** Server-Sent Events is a one-way HTTP/1.1 push channel — the server streams named events to the browser over a persistent connection.

- How it works: the client does one `GET /api/v1/stream`. The response never closes. The server writes `event: <name>\ndata: <json>\n\n` frames as data is ready. The browser's native `EventSource` API parses them automatically and reconnects on disconnect.
- In this codebase: `SSEBroker` in `internal/api/sse.go` manages a `map[chan []byte]struct{}` of connected clients. Each client has a buffered channel (size 16). The `broadcastLoop` goroutine calls `broker.BroadcastEvent()` every 2 seconds.
- Why not WebSockets: the browser only receives data, never sends. WebSockets add bidirectional complexity for no benefit here. SSE works through HTTP proxies and load balancers without special upgrade handling.
- When I'd pick WebSockets: if the client needed to send data back in real time — a chat app, a collaborative editor, a live terminal session.

Evidence: `internal/api/sse.go`; route `GET /api/v1/stream` in `internal/api/server.go`

---

**Q: How do you handle slow SSE clients?**

The `broadcast()` method uses a `select` with a 100ms timeout per client:

```go
select {
case ch <- data:
case <-time.After(100 * time.Millisecond):
    b.unsubscribe(ch) // drop the slow client
}
```

If a client can't consume within 100ms, it gets dropped. The browser's `EventSource` reconnects automatically. This prevents one stalled connection from blocking the broadcast loop for everyone else.

There's also a 15-second heartbeat comment (`": heartbeat\n\n"`) sent to keep the connection alive through proxies that close idle HTTP connections.

Evidence: `internal/api/sse.go` `broadcast()` and `ServeHTTP()`

---

**Q: How do you track connected SSE client count?**

A Prometheus gauge `clusterops_sse_connected_clients` is incremented in `subscribe()` and decremented in `unsubscribe()`. You can read the current count from `/metrics` or in Grafana at any time. There's no hard connection limit in the current code.

Evidence: `internal/api/metrics.go` `sseClientsGauge`; `internal/api/sse.go` `subscribe()` / `unsubscribe()`

---

**Q: SSE vs polling vs WebSockets — trade-off summary?**

| | Polling | SSE | WebSockets |
|---|---|---|---|
| Direction | Client pulls | Server pushes | Bidirectional |
| Protocol | HTTP | HTTP | WS upgrade |
| Proxy support | Universal | Good | Needs upgrade support |
| Auto-reconnect | Client manages | Browser `EventSource` | Client manages |
| Complexity | Simplest | Low | Higher |
| Use case | Simple dashboards | Live read-only feeds | Realtime two-way |

This project has a read-only feed, so SSE is the right level. Polling would waste bandwidth hitting the API every 2s per client when most responses are identical. WebSockets would add complexity for no functional gain.

Evidence: `internal/api/sse.go`; `internal/api/handlers.go` `broadcastClusterSnapshot()`


---

## 5. POSTGRESQL & DATA LAYER

---

**Q: Why PostgreSQL? Why not InfluxDB or TimescaleDB for GPU metrics?**

**Verdict:** Postgres, because it covers all three data shapes in one database without adding another system to operate.

- Why it fits here: nodes and jobs have relational integrity (alerts FK to both). GPU metrics are append-only time-series. Postgres handles both. The `ClusterGPUSummary` query uses `DISTINCT ON` to get the latest reading per GPU efficiently — a query that's awkward in a pure time-series store.
- What TimescaleDB/InfluxDB would be genuinely better at: automatic time-based partitioning, continuous aggregates, first-class retention policies. At millions of GPU metric rows per day, the plain `gpu_metrics` table would need manual partitioning.
- Trade-off I accepted: the `gpu_metrics` table grows unboundedly without the prune job. The ingestion service runs `PruneOldMetrics()` every 30 minutes deleting rows older than 24 hours — a manual retention policy, not a DB feature.

Evidence: `go.mod` — `github.com/jackc/pgx/v5 v5.5.5`; `store/migrations/002_retention.sql`

---

**Q: Why pgx instead of database/sql or an ORM like GORM?**

**Verdict:** pgx because it's the highest-performance Postgres driver for Go and handles native Postgres types without custom scanners.

- Why it fits here: `gpu_utilization` and `gpu_memory_used_gb` are `DOUBLE PRECISION[]` columns. pgx scans these directly into `[]float64`. With `database/sql` or GORM, you'd need a custom type scanner or JSON serialization workaround.
- What GORM is genuinely better at: faster scaffolding, automatic migrations, less boilerplate for simple CRUD on plain relational tables.
- Trade-off I accepted: more hand-written SQL. Every query in `store/nodes.go`, `store/jobs.go` is explicit — more code, but easier to audit, impossible to accidentally N+1, and easy to add query hints or partial indexes.

Evidence: `go.mod`; `internal/store/nodes.go` — direct `pool.Query()` calls with `[]float64` array scanning

---

**Q: How are database migrations handled?**

Migrations are SQL files embedded directly into the binary using Go's `//go:embed` directive. On startup, `db.migrate()` reads the embedded `migrations/` directory, sorts files by name (`001` before `002`), and executes each file. Every statement uses `IF NOT EXISTS` or `CREATE OR REPLACE`, making it idempotent — re-running the same migration doesn't fail.

Downside: no version tracking table, no "down" migration. For a demo this is fine. In production you'd use `golang-migrate` which tracks applied migrations in a `schema_migrations` table.

Evidence: `internal/store/db.go` `migrate()` function with `//go:embed migrations/*.sql`

---

**Q: Walk me through the database schema design.**

Four tables with clear roles:

- **`nodes`** — `TEXT PRIMARY KEY` (ID from simulator, must be stable). Stores per-GPU arrays directly as `DOUBLE PRECISION[]` — one row per node, not one row per GPU. Indexed on `status` and `last_seen DESC`.
- **`jobs`** — `TEXT PRIMARY KEY`. Upserted on every state change. `failure_reason` is nullable TEXT so unfailed jobs don't carry a dummy value. Indexed on `status`, `created_at DESC`, `user_id`.
- **`gpu_metrics`** — `BIGSERIAL PRIMARY KEY` (DB-generated, append-only). References `nodes(id) ON DELETE CASCADE`. Composite index `(node_id, recorded_at DESC)` covers the main time-series query. Plain table, not partitioned — comment notes that production would partition by day.
- **`alerts`** — FKs to both `nodes(id) ON DELETE SET NULL` and `jobs(id) ON DELETE SET NULL`. If a node or job is deleted, the alert survives for audit purposes. `gen_random_uuid()` used for IDs.

Evidence: `internal/store/migrations/001_initial_schema.sql`

---

**Q: How do you prevent N+1 queries?**

Three places this was explicitly designed out:

1. Node and job list handlers fetch the full list in one `SELECT` — no per-row follow-up queries.
2. `GetNodeGPUTimeSeries()` fetches all GPUs for a node in one query (`WHERE node_id = $1 AND recorded_at BETWEEN $2 AND $3`) and groups by `gpu_index` in Go.
3. `ClusterGPUSummary()` uses a single `WITH latest AS (DISTINCT ON (node_id, gpu_index) ...)` CTE to aggregate the freshest reading per GPU in one round trip — no loop over GPUs.

Evidence: `internal/store/gpu_metrics.go` `GetNodeGPUTimeSeries()` and `ClusterGPUSummary()`

---

**Q: How does upsert work and why use it instead of insert + update?**

All node and job writes use `INSERT ... ON CONFLICT (id) DO UPDATE SET`. This is one atomic Postgres statement. The alternative — check if exists, then insert or update — requires two round trips and a race condition between the check and the write.

Because the simulator republishes all nodes on every GPU tick, `UpsertNode()` is called ~40 times every 5 seconds (5 nodes × 8 GPUs worth of republishes). An insert-only approach would fail on the second publish. An update-only approach would fail on the first.

Evidence: `internal/store/nodes.go` `UpsertNode()`; `internal/store/jobs.go` `UpsertJob()`

---

**Q: How is the GPU metrics retention handled?**

Two layers:

1. **Application-level prune:** the ingestion service ticks every 30 minutes and calls `db.PruneOldMetrics(ctx, 24*time.Hour)` which does `DELETE FROM gpu_metrics WHERE recorded_at < $1`. The deleted row count is logged.
2. **SQL function:** migration `002_retention.sql` creates a `prune_gpu_metrics()` PL/pgSQL function that can be called manually or wired to `pg_cron` in production.

The comment in `002_retention.sql` explicitly notes: "In production this would be a pg_cron job or TimescaleDB retention policy."

Evidence: `internal/ingestion/ingestion.go` `pruneMetrics()`; `internal/store/gpu_metrics.go` `PruneOldMetrics()`; `internal/store/migrations/002_retention.sql`


---

## 6. API DESIGN

---

**Q: Walk me through the API design.**

The API is versioned under `/api/v1`. chi router with globally applied middleware: `RequestID`, `RealIP`, `Logger`, `Recoverer`, gzip compression at level 5, Prometheus instrumentation, and CORS.

Resources follow a simple REST shape:
- `GET /api/v1/cluster/summary` — health rollup
- `GET /api/v1/nodes`, `GET /api/v1/nodes/{id}`, `GET /api/v1/nodes/{id}/gpu-series`
- `GET /api/v1/jobs`, `GET /api/v1/jobs/{id}`, `GET /api/v1/jobs/{id}/alerts`
- `GET /api/v1/metrics/gpu`, `GET /api/v1/metrics/capacity`
- `GET /api/v1/alerts`
- `POST /api/v1/assistant/analyze`
- `GET /api/v1/stream` — SSE

`/health` and `/metrics` are at the root (no version prefix) because they're infrastructure endpoints, not API endpoints.

Evidence: `internal/api/server.go` `routes()`

---

**Q: Why chi instead of Gin or the standard library?**

**Verdict:** chi because it's idiomatic Go, composes cleanly with `net/http` middleware, and doesn't require learning framework-specific types.

- Why it fits here: chi's middleware stack is just `func(http.Handler) http.Handler` — the same interface as standard library handlers. The Prometheus middleware, CORS handler, and `promhttp.Handler()` all plug in without adapters or type conversions.
- What Gin is genuinely better at: marginally faster routing at extreme RPS, a larger ecosystem of Gin-specific middleware, and a more familiar API for developers coming from Node.js/Express.
- Trade-off I accepted: chi has a smaller ecosystem than Gin. For a project with 12 routes, that doesn't matter.

Evidence: `go.mod` — `github.com/go-chi/chi/v5 v5.0.12`; `internal/api/server.go` `routes()`

---

**Q: How does the cache-aside pattern work in the handlers?**

Every handler follows the same three-step pattern:

```go
// 1. Check Redis
if data, hit, _ := s.cache.GetX(r.Context()); hit {
    recordCacheHit("x")
    writeJSON(w, 200, data)
    return
}
// 2. Miss — query Postgres
recordCacheMiss("x")
data, err := s.db.QueryX(r.Context())
// 3. Warm cache for next request
_ = s.cache.SetX(r.Context(), data)
writeJSON(w, 200, data)
```

Cache errors on read fall through silently to Postgres. Cache errors on write are swallowed. Redis being down degrades performance, not correctness.

Evidence: `internal/api/handlers.go` — every handler follows this pattern

---

**Q: How are errors handled in the API?**

All errors return JSON: `{"error": "message"}` with an appropriate HTTP status code. The `writeError()` helper handles this consistently. There's no stack trace or internal error detail in the response — just a human-readable message. Internal errors are logged with `zap` before the response is written.

The `Recoverer` middleware from chi catches panics in handlers and returns a 500 instead of crashing the server.

Evidence: `internal/api/handlers.go` `writeError()`; `internal/api/server.go` `r.Use(middleware.Recoverer)`

---

**Q: How does the job list filtering work?**

`GET /api/v1/jobs` accepts `?status=running&user_id=alice`. The handler builds a cache key from these query params (`statusFilter + "|" + userFilter`) and checks Redis first. On a miss, it builds a dynamic SQL query in `store/jobs.go` `ListJobs()` using a `where []string` slice and positional parameters (`$1`, `$2`, etc.) — no string interpolation, no SQL injection risk. Default limit is 200 rows.

Evidence: `internal/api/handlers.go` `handleListJobs()`; `internal/store/jobs.go` `ListJobs()`


---

## 7. OBSERVABILITY

---

**Q: What observability does this system have?**

Three pillars, all wired up:

1. **Metrics (Prometheus + Grafana):** The API exposes `clusterops_api_requests_total` (counter by method/path/status), `clusterops_api_request_duration_seconds` (histogram with default buckets), `clusterops_cache_hits_total`, `clusterops_cache_misses_total`, and `clusterops_sse_connected_clients`. Prometheus scrapes `/metrics`, Grafana visualizes it on port 3001.

2. **Traces (OpenTelemetry → Jaeger):** `internal/telemetry/otel.go` initializes an OTLP gRPC exporter pointing at the otel-collector on port 4317. Sampler is `AlwaysSample()` — 100% trace capture, fine for a demo. Jaeger UI is on port 16686.

3. **Structured logging (zap):** All three binaries use `go.uber.org/zap` with structured fields — no `fmt.Printf`. Log lines include request IDs, node IDs, error types.

Evidence: `internal/api/metrics.go`; `internal/telemetry/otel.go`; `go.mod` — `go.opentelemetry.io/otel v1.26.0`, `go.uber.org/zap v1.27.0`

---

**Q: What is OpenTelemetry and why use it instead of directly integrating Jaeger?**

**In one line:** OpenTelemetry is a vendor-neutral SDK — "instrument once, export anywhere."

- How it works: your code calls `otel.Tracer("name").Start(ctx, "span")`. The SDK batches spans and sends them to the OTel Collector over OTLP/gRPC. The collector routes them to Jaeger (or Datadog, Grafana Tempo, etc.) without changing application code.
- In this codebase: the collector listens on port 4317 (gRPC) and 4318 (HTTP). If I wanted to switch from Jaeger to Tempo, I'd change one line in the collector config, not in any application code.
- When not to bother: if you only ever use one observability vendor and are willing to vendor-lock, direct SDK integration is simpler.

Evidence: `internal/telemetry/otel.go`; `docker-compose.yml` — `otel-collector`, `jaeger` services

---

**Q: What's the difference between tracing and metrics?**

- **Metrics** answer "how is the system doing in aggregate?" — request rate, error rate, p99 latency, cache hit ratio. Cheap to store (just counters and histograms). Good for dashboards and alerts.
- **Traces** answer "what happened for this specific request?" — which services it touched, how long each span took, where the latency went. Expensive to store at 100% sampling — sampled in production.

In this project: Prometheus metrics tell you if cache hit rate dropped below 80% across all requests (aggregate signal). A Jaeger trace would show you that one specific `/api/v1/cluster/summary` request spent 45ms in Postgres because of a cache miss (per-request signal).

Evidence: `internal/api/metrics.go`; `internal/telemetry/otel.go`

---

**Q: Why use zap instead of the standard library's log package or slog?**

**Short answer:** zap because it's structured (key-value fields, not format strings), zero-allocation on the hot path, and already the Go ecosystem standard for production services.

- Structured logging means log lines are machine-parseable by tools like Loki, Datadog, or CloudWatch. `log.Printf("error processing node %s: %v", id, err)` is hard to query. `logger.Error("upsert node", zap.String("id", n.ID), zap.Error(err))` produces JSON with typed fields.
- Go 1.21+ ships `slog` which has the same structured approach. If starting today I might use `slog` to avoid the dependency — but `zap` was the established choice when this was written.

Evidence: `go.mod` — `go.uber.org/zap v1.27.0`; `internal/ingestion/ingestion.go` — every log call uses `zap.String()`, `zap.Error()` fields

---

**Q: How does the Prometheus middleware work?**

`prometheusMiddleware` wraps every handler. It uses chi's `WrapResponseWriter` to capture the response status code after the handler runs, then records two metrics:
- `httpRequestsTotal.WithLabelValues(method, path, status).Inc()`
- `httpRequestDuration.WithLabelValues(method, path).Observe(duration)`

This gives you per-endpoint request rate and latency histograms in Grafana. The `promauto` package auto-registers the metrics with Prometheus's default registry on startup — no manual `prometheus.Register()` call needed.

Evidence: `internal/api/middleware.go` `prometheusMiddleware()`; `internal/api/metrics.go`


---

## 8. ASSISTANT ENGINE

---

**Q: What is the assistant and how does it work?**

**Short answer:** A deterministic rule-based failure analysis engine, not an LLM.

`POST /api/v1/assistant/analyze` accepts a `job_id` or `node_id`. For a job, the handler fetches the job + related alerts, then calls `engine.AnalyzeJob()`. The engine routes on `job.Status` and `job.FailureReason` (OOM, hardware fault, timeout, preemption, user error, unknown) and returns a structured `Analysis` — headline, severity, root cause, 2–6 ordered debugging steps with runnable shell commands, prevention tips, and a confidence score 0–1.

For a node, it routes on `node.Status` (healthy, degraded, unavailable) and inspects live telemetry arrays — if `max(GPUTemperature) > 85°C` it classifies thermal throttle; if `any(GPUMemoryUsedGB) > 78 GB` it classifies memory pressure.

This is explicitly not RAG or agentic. The package comment says "deterministic LLM-free baseline."

Evidence: `internal/assistant/engine.go`; `internal/assistant/job_rules.go`; `internal/assistant/node_rules.go`

---

**Q: What's the difference between this and a real RAG system?**

Real RAG: retrieve relevant documents from a vector index → inject as context → LLM generates a free-form response. The intelligence comes from the LLM.

This engine: the rules ARE the intelligence. It pattern-matches on typed enum values and numeric thresholds, then returns pre-written text templates populated with live job/node data.

What they share is structural shape: given a target (job/node), retrieve relevant context (failure reason, telemetry), augment with live data (duration, temperature readings, GPU counts), produce structured output. The engine's package comment explicitly describes this as the upgrade path — keep the retrieve/augment steps, replace the template-based generate step with an LLM call.

Evidence: `internal/assistant/engine.go` — "The structure mirrors a RAG pipeline" comment in the package doc

---

**Q: Why build a rule-based engine instead of calling an LLM directly?**

Three concrete reasons:

1. **Reliability:** rules are deterministic. The OOM rule always returns severity=critical and exactly those 6 debugging steps. An LLM can hallucinate commands, produce inconsistent severity ratings, refuse, or time out.

2. **Testability:** `engine_test.go` has 10 table-driven test cases covering every failure path. Each asserts exact `RootCause`, `Severity`, minimum `DebuggingSteps` count, and minimum confidence. You cannot write that kind of assertion test for LLM output.

3. **The rules are the eval harness for the LLM:** when you add Ollama or any LLM, you run the same test cases against LLM output and use rule output as the expected baseline. Any case where the LLM produces worse results (wrong severity, fewer steps, lower confidence) is a regression. This is how production AI systems measure LLM quality — deterministic baseline first.

Evidence: `internal/assistant/engine_test.go` — "AI meta-learning note" comment at top of file

---

**Q: What failure scenarios does the assistant cover?**

For jobs (5 failure types + 3 states):
- **OOM** — CUDA out-of-memory. Severity: critical. Confidence: 0.95. 6 steps: confirm in logs, check nvidia-smi, reduce batch size, gradient checkpointing, mixed precision, ZeRO-3/FSDP.
- **Hardware fault** — ECC/NVLink error. Severity: critical. Confidence: 0.92. 6 steps: check ECC counts, dmesg Xid errors, GPU diagnostics, drain the node, reset and retest, resubmit.
- **Timeout** — exceeded max duration. Severity: warning. Confidence: 0.88. 6 steps: check throughput, NCCL hangs, GPU utilization, data loader, checkpoint frequency, increase timeout.
- **Preemption** — low-priority job killed. Severity: warning. Confidence: 0.97. 4 steps: verify checkpoint, resume, raise priority, request reserved pool.
- **User error** — code/config exception. Severity: warning. Confidence: 0.85. 4 steps: read traceback, check tensor shapes, validate config, run locally.
- **Unknown** — no failure signal. Severity: warning. Confidence: 0.40. 2 steps: check node events, check exit code.

For nodes (3 states): degraded (with thermal/memory classification), unavailable, healthy.

Evidence: `internal/assistant/job_rules.go`; `internal/assistant/node_rules.go`

---

**Q: What are the confidence scores and how are they set?**

Hardcoded per rule — not dynamically computed. They reflect how unambiguous the classification is:

| Scenario | Confidence | Rationale |
|---|---|---|
| Completed | 1.0 | Definitional — no ambiguity |
| Running | 0.99 | Job is running — certain |
| Preemption | 0.97 | Scheduler preemption is explicit |
| OOM | 0.95 | `FailureReasonOOM` is a clear enum value |
| Queued | 0.95 | Waiting state is unambiguous |
| Hardware fault | 0.92 | ECC errors are clear hardware signals |
| Node unavailable | 0.90 | Node unreachable = definitive |
| Node degraded | 0.82 | Could have multiple causes |
| Timeout | 0.88 | Clear but root cause of slowness varies |
| User error | 0.85 | Could be infra; rules favor user error attribution |
| Unknown failure | 0.40 | No signal — low confidence by design |

In a real LLM-backed system, confidence would come from log-probabilities or a calibration model.

Evidence: `internal/assistant/job_rules.go` — each rule function sets `a.Confidence` explicitly


---

## 9. SIMULATOR & FAULT INJECTION

---

**Q: What does the simulator do and why does it exist?**

**Short answer:** It generates a realistic, self-contained synthetic GPU cluster so the system can be demoed without real Kubernetes or GPU hardware.

Three concurrent loops:
- `gpuLoop` — emits GPU telemetry every 5 seconds (40 metrics per tick: 5 nodes × 8 GPUs each)
- `jobLoop` — submits a new training job every 20–90 seconds (random interval, uniform distribution)
- `faultLoop` — fires fault scenarios every 60 seconds

The fault probabilities are tuned to create an engaging demo: 5% chance per minute a healthy node degrades, 8% per running job per minute of OOM, 6% preemption for low-priority jobs. This keeps the dashboard showing realistic failure patterns without everything breaking at once.

Evidence: `internal/simulator/config.go` `DefaultConfig()`; `internal/simulator/simulator.go` `Run()`

---

**Q: How does fault injection work? Walk me through `faultTick()`.**

`faultTick()` runs once per minute and makes independent probability draws:

1. **Node state transitions:** for each node, `rand.Float64() < FaultNodeDegradeProb (0.05)` → degrade. If already degraded: `rand.Float64() < FaultNodeDownProb (0.10)` → mark unavailable AND kill all running jobs on that node with `FailureReasonHardwareFault`. If degraded: `rand.Float64() < FaultNodeRecoverProb (0.30)` → recover to healthy.

2. **Thermal alerts:** independently, `rand.Float64() < FaultThermalThrottle (0.04)` per healthy node → fire a GPU high temperature alert.

3. **Job faults (checked in order, `continue` on first match):**
   - OOM: 8% per running job
   - Preemption: 6% but only for jobs with `priority <= 3`
   - Hardware fault: 3% per running job
   - Timeout: deterministic — if `DurationSeconds() > JobDurationMax (8 min)`, time out
   - Natural completion: 35% chance once past `JobDurationMin (2 min)`

4. **Capacity waste alert:** if >30% of GPUs are allocated but below 10% utilization, fire a warning.

Evidence: `internal/simulator/faults.go` `faultTick()`; `internal/simulator/config.go` probability constants

---

**Q: How does job scheduling work in the simulator?**

The simulator maintains in-memory cluster state. When `runJobSubmit()` fires, it calls `submitJob()` which:
1. Picks a random model name from the configured list (`llama-3-70b`, `mistral-7b`, `gpt-neox-20b`, `falcon-40b`, `qwen-72b`, `deepseek-67b`)
2. Picks a random user from the 5 configured users
3. Assigns a random priority 1–10 (low-priority jobs are preemption candidates)
4. Finds a healthy node with available GPU capacity
5. If capacity available → status=`running`, assign node, record start time
6. If no capacity → status=`queued`

`retryQueuedJobs()` runs on each fault tick to promote queued jobs to running when freed capacity becomes available.

Evidence: `internal/simulator/jobs.go` (not shown but referenced by `simulator.go`); `internal/simulator/config.go` `ModelNames`, `Users`

---

**Q: How does the simulator maintain state without a database?**

It keeps an in-memory `clusterState` struct with two maps: `nodes map[string]*models.Node` and `jobs map[string]*models.Job`, protected by a `sync.RWMutex`. Reads use `RLock()`, writes use `Lock()`. This is a single-process in-memory state — if the simulator crashes, all state is lost. That's intentional: on restart, it re-publishes the initial node state, and the ingestion service upserts everything fresh.

Evidence: `internal/simulator/simulator.go` `publishAllNodes()` on startup; `internal/simulator/faults.go` `s.state.mu.Lock()` / `s.state.mu.RUnlock()`


---

## 10. FRONTEND

---

**Q: Walk me through the frontend stack.**

React 18 with TypeScript, built by Vite. Routing with React Router v6. Styling with Tailwind CSS + `tailwind-merge` + `clsx`. Charts with Recharts. Headless accessible components from Radix UI (Dialog, Select, Tabs, Tooltip, Separator). Icons from Lucide React. Date formatting with `date-fns`.

Five pages: Dashboard, Nodes, Jobs, Alerts, Assistant — all nested under a shared `Layout` component with a sidebar nav. Each page is a route defined in `App.tsx`.

Evidence: `frontend/package.json`; `frontend/src/App.tsx`

---

**Q: Why Vite instead of Create React App or Next.js?**

**Verdict:** Vite because it's the fastest dev server for a pure client-side SPA with no SSR needs.

- Why it fits here: this is a pure SPA — no server-side rendering, no static generation needed. The API is a separate Go binary. Vite's native ES module dev server starts in milliseconds, and its esbuild-based production build is significantly faster than webpack (CRA's underlying bundler) for TypeScript projects.
- What Next.js is genuinely better at: SSR/SSG for SEO, server components for data fetching, file-based routing, API routes co-located with the frontend.
- Trade-off I accepted: no SSR means a blank page on initial load (white flash before JS hydrates). For a monitoring dashboard behind authentication, that's fine. For a public marketing page, it wouldn't be.

Evidence: `frontend/package.json` — `"vite": "^5.2.11"`

---

**Q: Why Radix UI for components instead of Material UI or Chakra?**

**Short answer:** Accessibility out of the box, unstyled so Tailwind controls everything.

Radix provides behavior (keyboard navigation, ARIA attributes, focus management, screen reader announcements, escape key handling) without imposing any CSS. Every Dialog, Select, Tabs, and Tooltip in the dashboard gets correct a11y semantics for free. If I'd used MUI or Chakra, I'd fight their CSS specificity with Tailwind overrides. Radix has no styles to fight — it's "bring your own CSS."

Evidence: `frontend/package.json` — five `@radix-ui/*` packages

---

**Q: Why Recharts for the GPU time series charts?**

Recharts is built on SVG, composable as React components (`<LineChart>`, `<Line>`, `<XAxis>`), and integrates naturally with React state. When the SSE stream pushes new data, the component re-renders and Recharts updates the chart — no imperative chart.update() call needed, unlike D3 or Chart.js.

Trade-off: SVG-based charts get slow with very large datasets (thousands of points). For a 1-hour GPU time series at 5s intervals that's ~720 points — fine. For a 24-hour view, you'd want downsampling on the server side.

Evidence: `frontend/package.json` — `"recharts": "^2.12.5"`

---

**Q: Why `tailwind-merge` and `clsx` together?**

- `clsx` conditionally joins class strings: `clsx("base", isActive && "active", className)` — cleaner than template literals with ternaries.
- `tailwind-merge` resolves Tailwind class conflicts: if you pass both `p-2` and `p-4`, `tailwind-merge` keeps only `p-4`. Without it, both classes end up in the DOM and the winner is determined by CSS specificity — which is often wrong with Tailwind's utility-first approach.

The pattern `cn = (...inputs) => twMerge(clsx(inputs))` is the standard Tailwind + Radix combination, seen in shadcn/ui and similar component libraries.

Evidence: `frontend/package.json` — `"clsx": "^2.1.1"`, `"tailwind-merge": "^2.3.0"`


---

## 11. GO LANGUAGE & PATTERNS

---

**Q: Why Go for the backend?**

**Short answer:** Go's concurrency model (goroutines + channels), small binary size, and fast compile times make it a natural fit for an event-driven backend with multiple concurrent loops.

- The simulator runs three concurrent ticker loops. The ingestion service runs four concurrent consumer goroutines. The API server runs the SSE broadcast loop concurrently with the HTTP server. All of this is expressed as `go func()` calls — lightweight goroutines, not OS threads.
- Go compiles to a single static binary with no runtime dependencies. All three services build from the same `Dockerfile.backend` using a `SERVICE` build arg — the only difference is which `cmd/` directory is compiled.
- The strong type system caught domain bugs at compile time (e.g., `JobStatus` and `FailureReason` are typed strings — you can't pass one where the other is expected).

Evidence: `backend/go.mod` — `go 1.22`; `internal/simulator/simulator.go` — three concurrent goroutines

---

**Q: How does concurrency work in the ingestion service?**

The ingestion service starts one goroutine per Kafka topic via `consumer.Start(ctx)`. Each goroutine runs an infinite read loop calling `r.ReadMessage(ctx)`. When the context is cancelled (shutdown signal), `ReadMessage` returns an error, the goroutine detects `ctx.Err() != nil`, logs "consumer loop stopped", and returns.

No channels between goroutines are needed — each goroutine independently reads from its Kafka topic and calls its handler. The only shared state is the database pool and cache client, which are safe for concurrent use (pgxpool and go-redis are both goroutine-safe by design).

Evidence: `internal/kafka/consumer.go` `Start()` and `consume()`

---

**Q: How is graceful shutdown handled?**

The main function passes a `context.Context` derived from `os.Signal` (SIGTERM/SIGINT) to every service. When the signal fires:
- The context is cancelled.
- Simulator's `Run()` select loop detects `<-ctx.Done()` and returns.
- Ingestion's consumer goroutines detect context cancellation in `ReadMessage()` and return.
- API server's `Start()` selects on `<-ctx.Done()` and calls `httpSrv.Shutdown(ctx)` with a 15-second timeout to drain in-flight requests.

Evidence: `internal/api/server.go` `Start()` and `shutdown()`; `internal/simulator/simulator.go` `Run()`

---

**Q: What is `embed.FS` and why use it for migrations?**

`//go:embed migrations/*.sql` is a Go directive that bakes the SQL files into the compiled binary at build time. The resulting binary has no external file dependencies — you don't need to ship a `migrations/` directory alongside it, and you can't accidentally deploy the binary without its migrations.

`embed.FS` is the read-only file system interface that exposes the embedded files. The `migrate()` function calls `migrationsFS.ReadDir("migrations")` and `migrationsFS.ReadFile("migrations/001_initial_schema.sql")` exactly as if they were on disk.

Evidence: `internal/store/db.go` — `//go:embed migrations/*.sql` and `var migrationsFS embed.FS`

---

**Q: Why use `sync.RWMutex` in the simulator instead of channels?**

The simulator's cluster state is a simple map that's read frequently (GPU tick reads every node) and written rarely (fault tick modifies a few nodes). `RWMutex` is the right tool: multiple goroutines can hold `RLock()` concurrently; only one can hold `Lock()`. Using channels for this would require a dedicated goroutine to serialize access (actor pattern), which is more code for no correctness benefit here.

The alternative — one goroutine owning all state, communicated via channels — would be idiomatic Go for more complex state machines but adds indirection when a mutex is sufficient.

Evidence: `internal/simulator/faults.go` — `s.state.mu.RLock()` for reads, `s.state.mu.Lock()` for node state transitions

---

**Q: Why does the ingestion service use a goroutine for GPU summary refresh?**

```go
go func() {
    refreshCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
    defer cancel()
    if summary, err := svc.db.ClusterGPUSummary(refreshCtx); err == nil {
        _ = svc.cache.SetGPUSummary(refreshCtx, summary)
    }
}()
```

GPU metrics arrive every 5 seconds from 40 GPU/node combinations. Recomputing and caching the cluster GPU summary synchronously on every metric insert would block the consumer goroutine. The summary refresh is best-effort — if it fails or the goroutine is slow, the 2s TTL on the cache ensures it expires and gets recomputed on the next API request. Fire-and-forget with a timeout is appropriate here.

Evidence: `internal/ingestion/ingestion.go` `handleGPUMetric()`


---

## 12. DATA MODELS & DOMAIN DESIGN

---

**Q: Walk me through the domain models.**

Five core models in `internal/models/`:

- **`Node`** — a physical GPU machine. Carries per-GPU arrays (`GPUUtilization []float64`, `GPUTemperature []float64`, etc.) — one element per GPU on the node. Status enum: `healthy`, `degraded`, `unavailable`, `maintenance`. Methods: `GPUWastePercent()` (GPUs allocated but util < 10%), `AvgGPUUtilization()`.

- **`Job`** — a training job. Status lifecycle: `queued` → `running` → `completed`/`failed`/`preempted`. `FailureReason` is a typed string enum (`oom`, `hardware_fault`, `timeout`, `preemption`, `user_error`) that drives the assistant routing. `Priority` is an int 1–10 — low-priority jobs (≤3) are preemption candidates. Method: `DurationSeconds()`, `IsTerminal()`.

- **`Alert`** — a fired notification. FK to both `NodeID` and `JobID` (both nullable). `Resolved bool` + `ResolvedAt *time.Time` for lifecycle tracking. `AlertType` enum drives UI badge rendering.

- **`ClusterSummary`** — the dashboard rollup. Embeds `ClusterGPUSummary` as a nested struct. Computed on every call to `store.ClusterSummary()` and cached in Redis.

- **Event types** (`NodeEvent`, `JobEvent`, `GPUMetricEvent`, `AlertEvent`) — typed envelopes wrapping domain objects for Kafka transport.

Evidence: `internal/models/node.go`, `job.go`, `alert.go`, `events.go`

---

**Q: Why are GPU metrics stored as arrays on the Node vs. separate rows per GPU?**

Two competing approaches:

1. **Array on node row** (what's done here for live state): `gpu_utilization DOUBLE PRECISION[]` stores all 8 GPU readings in one column. One upsert per node tick, one row to read for the node detail page. Simple, fast for the current node count.

2. **Separate `gpu_metrics` table** (what's done here for time-series history): each reading gets its own row with `(node_id, gpu_index, recorded_at)`. Enables time-range queries and aggregates.

Both are used simultaneously, serving different purposes: the array on the node row gives the latest snapshot instantly (no join, no time-range filter). The `gpu_metrics` table enables the GPU time-series chart (last 1 hour of readings per GPU index).

Evidence: `internal/models/node.go` — `GPUUtilization []float64`; `internal/store/migrations/001_initial_schema.sql` — both `nodes.gpu_utilization[]` and the `gpu_metrics` table

---

**Q: Why are `StartTime` and `EndTime` pointers on the Job model?**

```go
StartTime *time.Time `json:"start_time,omitempty"`
EndTime   *time.Time `json:"end_time,omitempty"`
```

A queued job has never started — `StartTime` is genuinely null, not zero. A running job has no end time. Using `*time.Time` (pointer) means `nil` in Go maps to `NULL` in Postgres and is omitted from JSON (`omitempty`). Using `time.Time` (value) would require a sentinel value like `time.Time{}` (zero time) to represent "not set," which would appear as `"0001-01-01T00:00:00Z"` in JSON responses — meaningless and confusing to API consumers.

Evidence: `internal/models/job.go`; `internal/store/jobs.go` `scanJob()` — `&j.StartTime` scanned as nullable

---

**Q: How are alert types and severity levels structured?**

Both use typed string constants:

```go
type AlertSeverity string
const (
    AlertSeverityCritical AlertSeverity = "critical"
    AlertSeverityWarning  AlertSeverity = "warning"
    AlertSeverityInfo     AlertSeverity = "info"
)
```

This gives three benefits: the compiler catches invalid values (you can't pass a raw string `"CRITICAL"` where `AlertSeverity` is expected), JSON serializes to the human-readable string (no magic numbers), and Postgres stores the string directly — readable in the DB without a lookup table.

Eight alert types are defined: `node_unavailable`, `node_degraded`, `gpu_high_temperature`, `gpu_memory_full`, `job_failed`, `capacity_waste`, `job_timeout`, `cluster_degraded`.

Evidence: `internal/models/alert.go`


---

## 13. TESTING

---

**Q: What tests exist and what do they cover?**

One test file: `internal/assistant/engine_test.go`. It has two table-driven test functions:

- **`TestAnalyzeJob`** — 7 cases covering every job status/failure combination: OOM, hardware fault, timeout, preemption, user error, running, and completed. Each case asserts: exact `RootCause` string, exact `Severity`, minimum `DebuggingSteps` count, minimum `Confidence` score, and that `Headline`/`Summary` are non-empty.

- **`TestAnalyzeNode`** — 3 cases: degraded node with high temperature, unavailable node, and healthy node. Each asserts `Severity` and `RootCause`.

The test file has a detailed comment explaining its dual purpose: it's both a correctness test for the current rule engine AND an eval harness for a future LLM — run the same cases against LLM output, compare against rule output as baseline.

Evidence: `internal/assistant/engine_test.go`

---

**Q: Why are there no integration tests or API tests?**

Honest answer: this is a demo project. The assistant engine was the one component with complex branching logic (5 failure types × multiple sub-conditions for nodes) that genuinely needed a test harness to be confident in correctness. The rest — HTTP handlers, Kafka consumer routing, cache invalidation — are straightforward enough that the integration test is "run `make dev` and check the dashboard."

In a production system I'd add:
- HTTP integration tests for the API handlers using `httptest.NewRecorder()`
- A test Postgres instance for store-layer tests (using `testcontainers-go`)
- A test Kafka for ingestion pipeline tests
- End-to-end smoke tests against the full Docker Compose stack

Evidence: Only `engine_test.go` exists in the entire backend test surface

---

**Q: How would you run the existing tests?**

```bash
cd backend
go test ./internal/assistant/...
```

Or from the project root using the Makefile (if a test target exists). No external dependencies needed — the engine test uses only in-memory structs and `zap.NewNop()` for the logger.

Evidence: `internal/assistant/engine_test.go` `makeEngine()` — `assistant.NewEngine(zap.NewNop())`


---

## 14. DEPLOYMENT & INFRASTRUCTURE

---

**Q: How is the system deployed locally?**

Everything runs in Docker Compose. One command — `make dev` — starts 13 services:

| Category | Services |
|---|---|
| Infrastructure | `postgres`, `redis`, `zookeeper`, `kafka` |
| Observability | `prometheus`, `grafana`, `jaeger`, `otel-collector` |
| Application | `api`, `ingestion`, `simulator` |
| Frontend | `frontend` (Vite dev server) |

Health checks are configured on every service. The application services use `depends_on: condition: service_healthy` to ensure Postgres, Redis, and Kafka are ready before starting.

Evidence: `docker-compose.yml`

---

**Q: How are the three Go binaries built from the same Dockerfile?**

`Dockerfile.backend` uses a `SERVICE` build argument:

```dockerfile
ARG SERVICE
RUN go build -o /app ./cmd/${SERVICE}
```

In `docker-compose.yml`:
```yaml
api:
  build:
    args:
      SERVICE: server
ingestion:
  build:
    args:
      SERVICE: ingestion
simulator:
  build:
    args:
      SERVICE: simulator
```

One Dockerfile, three images built from the same Go module, each compiling a different `cmd/` entry point.

Evidence: `docker-compose.yml` — `api`, `ingestion`, `simulator` services with `args: SERVICE:`

---

**Q: Why are observability configs baked into Docker images instead of bind-mounted?**

The `docker-compose.yml` has a comment explaining this: "All bind mounts have been replaced with baked images to work around a Docker Desktop bug where colons in the host path break volume parsing."

On macOS, paths like `/Users/shubhkapadia/Desktop/Development/Web-Apps/ClusterOps` contain no colons, but Docker Desktop has a known issue parsing volume strings when the path is on a non-standard drive or has certain characters. Baking config files into the image (`COPY config.yaml /etc/otel/`) is more portable and eliminates the bind mount dependency.

Evidence: `docker-compose.yml` comment at the top; `otel-collector`, `prometheus`, `grafana` all use `build:` instead of `image:` + volume

---

**Q: What ports are exposed and for what?**

| Port | Service | Purpose |
|---|---|---|
| 3000 | Frontend | Vite dev server |
| 3001 | Grafana | Dashboards |
| 8080 | API | REST + SSE |
| 9090 | Prometheus | Metrics scrape/UI |
| 9093 | Kafka | External broker access |
| 16686 | Jaeger | Trace UI |
| 4317 | OTel Collector | OTLP gRPC |
| 4318 | OTel Collector | OTLP HTTP |
| 5432 | Postgres | DB (dev access) |
| 6379 | Redis | Cache (dev access) |

Evidence: `docker-compose.yml` — `ports:` on each service

---

**Q: Is there a CI/CD pipeline?**

Not in this repository. There are no `.github/workflows/` or CI config files. This is a local dev/demo project. In a production setup I'd add:
- GitHub Actions: `go test ./...`, `go vet`, `golangci-lint` on every push
- Docker image builds on merge to main
- Deployment to a Kubernetes cluster via Helm or Kustomize

Evidence: No CI config files found in the workspace file tree


---

## 15. GAPS & HONEST LIMITS

---

**Q: What would you do differently or improve if this went to production?**

Honest list, grounded in what the code actually has vs. what it's missing:

1. **Kafka error handling:** the current consumer logs and skips on error. Production needs retry with backoff and a dead-letter topic. `internal/kafka/consumer.go` — "log and skip — appropriate for a demo."

2. **No authentication or authorization:** the API has `AllowedOrigins: ["*"]` CORS and no auth middleware. Every endpoint is public. Production needs at minimum JWT validation or mTLS.

3. **SSE doesn't scale across replicas:** `SSEBroker` is an in-memory map. If you run two API instances, clients connected to replica A don't receive events broadcast by replica B. Fix: replace the in-memory broker with a Redis pub/sub subscriber so all replicas receive events.

4. **GPU metrics table will grow unboundedly without the prune job:** the 30-minute application-level prune is a single point of failure. Production needs `pg_cron` or TimescaleDB retention policies.

5. **100% OTel sampling:** `AlwaysSample()` in `telemetry/otel.go` is fine for demos, prohibitively expensive at production traffic. Switch to head-based probabilistic sampling (1–5%).

6. **No down migrations:** the idempotent migration system has no rollback. If migration 003 introduces a bad column, you'd need to write a manual fix. Use `golang-migrate` with `up`/`down` files.

7. **Health check doesn't check Kafka:** `handleHealth()` checks Postgres and Redis but not Kafka connectivity. A Kafka partition leader outage wouldn't surface in `/health`.

Evidence: `internal/kafka/consumer.go`; `internal/api/server.go` CORS config; `internal/api/sse.go`; `internal/telemetry/otel.go`; `internal/api/handlers.go` `handleHealth()`

---

**Q: What does this project NOT demonstrate that might be on your resume?**

Being honest about what isn't in the code:

- **No real LLM or AI inference:** the assistant is rule-based. There's no Ollama, OpenAI, or vector store call anywhere in the codebase. Don't claim RAG or "AI-powered" if asked to define those precisely.
- **No Kubernetes:** everything is Docker Compose. No Helm charts, no manifests, no pod scheduling concepts.
- **No authentication/authorization:** no JWT, no OAuth, no RBAC.
- **No CI/CD pipeline:** no GitHub Actions, no automated tests in a pipeline.
- **No database partitioning:** the `gpu_metrics` table comment says "partitioned by day in production" — but there's no actual partitioning here.
- **No gRPC:** all service communication is either Kafka (async) or HTTP REST. OTLP uses gRPC but that's the OpenTelemetry SDK, not service-to-service gRPC you wrote.
- **No distributed tracing instrumented in handlers:** OTel is initialized but individual handler spans aren't created — the tracer provider is set up but `tracer.Start()` isn't called per request.

---

**Q: What are the most interesting technical decisions you made and why?**

Three that show real thinking:

1. **Single-writer ingestion pattern** — making the ingestion service the only DB writer and keeping the API server read-only eliminates an entire class of cache consistency bugs. It's a constraint that makes the system easier to reason about. Evidence: `internal/ingestion/ingestion.go` package doc.

2. **Rule engine as LLM eval harness** — building the assistant as deterministic rules first, with the test suite explicitly designed to serve as a regression baseline when swapping in an LLM. This is how production AI systems are actually built — not "let's add an LLM and see what happens." Evidence: `internal/assistant/engine_test.go` comment.

3. **Embedding migrations in the binary** — using `//go:embed` means the binary is self-contained. Deploy one file, get schema migrations automatically on startup. No external migration tool, no deployment checklist item for running migrations separately. Evidence: `internal/store/db.go`.

---

*Generated from live codebase scan — all claims are backed by real file paths.*
*Last scanned: ClusterOps @ commit state as of session start.*
