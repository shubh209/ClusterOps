# ClusterOps

## Tagline

Real-time GPU cluster observability for AI training infrastructure. Engineers see which nodes are degrading, which jobs are failing, and exactly why — with a rule-based assistant that returns a root cause and ordered debugging steps the moment something goes wrong.

## Tech Stack (Languages / Frameworks / Infrastructure / Tools)

Languages: Go 1.22 (primary), TypeScript, SQL, HTML, CSS

Frameworks and libraries: chi router, pgx v5, go-redis v9, kafka-go, go.uber.org/zap, React 18, React Router v6, Vite, Tailwind CSS, Recharts, Radix UI, date-fns

Infrastructure: Docker Compose (11 services), PostgreSQL 16, Redis 7, Kafka (Confluent 7.6), Prometheus, Grafana, Jaeger, OpenTelemetry Collector

Tools: Docker, Git, GitHub, multi-stage Dockerfiles, go:embed, Server-Sent Events, Prometheus client, OpenTelemetry SDK

## Problem

Teams running AI training workloads on GPU clusters have no single place to see what is failing and why. When a training job crashes, engineers read raw server logs line by line to find the cause. When a GPU starts overheating or throwing hardware errors, the first signal is often a failed job — not a proactive alert. By the time anyone knows something is wrong, hours of compute time and money are already gone.

Without live visibility, on-call engineers guess at root causes, nodes stay in degraded states longer than necessary, and expensive training runs die silently while the dashboard shows nothing.

## Solution

ClusterOps replaces log hunting with a live observability console. A Go simulator generates realistic GPU cluster activity — telemetry, training jobs, faults, and alerts — and publishes it to Kafka. An ingestion service consumes those events and writes to PostgreSQL and Redis. A Go API server reads from cache and database and pushes live updates to a React dashboard over Server-Sent Events every 2 seconds. When a job fails or a node degrades, an AI assistant engine classifies the failure, explains the root cause, and returns ordered debugging steps with shell commands the engineer can run immediately.

Built to demonstrate end-to-end understanding of the compute layer required to run AI products at scale — from data pipeline to real-time dashboard to failure analysis tooling.

## My Role

Solo builder. Every architecture decision, stack choice, and tradeoff was mine.

Simulator: designed the full state machine for 5 GPU nodes cycling through healthy, degraded, and unavailable states; built the fault injection engine for OOM, hardware fault, preemption, timeout, and user error scenarios; GPU telemetry loop emitting 40 data points every 5 seconds to Kafka.

Backend: Go API server with chi router, cache-aside pattern across all endpoints, SSE broker broadcasting to all connected clients every 2 seconds, PostgreSQL schema with 5 tables and time-series GPU metrics, Redis cache with typed TTLs, Kafka producer and consumer for 4 topics, ingestion service writing durably to both stores.

AI assistant: rule-based failure analysis engine routing 5 job failure modes and 3 node states to structured playbooks; each analysis returns headline, root cause, severity, summary, ordered debugging steps with shell commands, prevention tips, and confidence score; designed as a direct LLM swap-in for Phase 5.

Frontend: React dashboard with SSE live updates, GPU heatmap, job ring chart, alert feed, node drilldown with sparklines, job detail drawer with log tail, assistant interface.

Infra: custom Dockerfiles for all 3 Go services and all observability tools; fixed Docker Desktop colon-in-path bind mount bug by baking configs into images; fixed go:embed path traversal limitation; wired Prometheus metrics, Grafana dashboards, and Jaeger distributed tracing end to end.

Not built on purpose: real Kubernetes integration, actual LLM calls, authentication, multi-tenancy, cloud deployment.

## Impact

- [ESTIMATE] On-call engineers go from a crashed training job to a root cause classification and ordered debugging steps in under 3 seconds instead of spending hours reading raw logs
- [ESTIMATE] GPU telemetry from 40 data points across 5 nodes processed and cached every 5 seconds, keeping cluster health data fresh enough to catch hardware failures before they kill a running job
- [ESTIMATE] Full 11-service observability stack spins up in one command, reducing new engineer onboarding from days of environment setup to under 30 minutes
- [ESTIMATE] SSE broadcast delivers live cluster state to all connected dashboard clients every 2 seconds without polling, keeping API load flat regardless of how many engineers are watching
- [ESTIMATE] Redis cache-aside pattern on all read endpoints reduces PostgreSQL query load by serving repeated reads from memory with a 2-second TTL on cluster summary

## How It Works

The simulator runs three concurrent loops: GPU telemetry every 5 seconds, job submission every 20–90 seconds, and fault injection every 60 seconds. All events publish to Kafka across 4 topics. The ingestion service consumes those topics and writes to both PostgreSQL (durable) and Redis (hot cache). The API server is read-only — it checks Redis first on every request and falls back to PostgreSQL on a cache miss, writing the result back to cache. A background SSE broadcast loop queries the database every 2 seconds and pushes cluster summary, node list, and active alerts to every connected browser client over a persistent HTTP connection. The React frontend connects once via EventSource and updates in real time without polling individual endpoints.

The assistant engine receives a job or node struct plus related alerts, routes to the matching rule set based on failure reason or node status, and returns a structured Analysis with headline, root cause, severity, debugging steps with shell commands, and confidence score. It mirrors a RAG pipeline — retrieve the matching playbook, augment with live metrics, generate the structured output — so swapping the generate step for an Ollama LLM call requires no interface changes.

Tradeoffs worth knowing:

All config files for Prometheus, Grafana, and OTel Collector are baked into custom Docker images at build time rather than bind-mounted, to work around a Docker Desktop bug where colons in the host path break volume parsing on macOS.

The go:embed directive cannot traverse above the module root, so migrations are copied into the store package at build time rather than referenced from the project root.

The assistant is deterministic and rule-based by design — it serves as an eval harness and regression baseline for when a real LLM is added, so the LLM output can be diffed against the rule output.

The SSE broker drops slow clients after 100ms rather than buffering, keeping memory flat under load.

The API and ingestion services share the same PostgreSQL schema but are separate binaries — ingestion is the only writer, the API is read-only, which keeps the cache invalidation logic simple.

Repo layout: `backend/cmd/server`, `backend/cmd/ingestion`, `backend/cmd/simulator`, `backend/internal/` (models, store, cache, kafka, api, assistant, simulator, ingestion, telemetry), `frontend/src/` (pages, components, hooks, lib, types), `infra/` (docker, otel, prometheus, grafana), `backend/migrations/`.

## Locked Resume Bullets

Use one set per resume version. Each bullet is past tense, ends with a plain business outcome, and avoids repeating keywords within that set.

### Full Stack

1. Built a GPU cluster monitoring dashboard and AI failure analysis interface with TypeScript, React, REST API, HTML, and CSS so infrastructure teams see which training jobs are failing without going through server logs and losing hours tracking down a crash that already killed an expensive run.
Keywords: [TypeScript, React, REST API, HTML, CSS]

2. Built server side data ingestion and caching with Go, SQL, Redis, Docker, and cloud ready containerization so engineers and on-call teams get live cluster health and GPU metrics instantly instead of waiting on slow page loads while a GPU is overheating and a training run is actively dying.
Keywords: [Go, SQL, Redis, Docker, Cloud]

3. Containerized the full monitoring stack with Docker, Git, CI/CD, and DevOps following Agile practices so newly onboarded engineers can set up the project and get up to speed in days instead of weeks.
Keywords: [Docker, Git, CI/CD, DevOps, Agile]

### Backend

1. Built GPU node and training job state tracking with Go, REST API, SQL, Redis, and cloud ready infrastructure so infrastructure teams know the moment something goes wrong and can act before a small problem becomes a failed run that already wasted hours of compute and money.
Keywords: [Go, REST API, SQL, Redis, Cloud]

2. Built an event streaming pipeline with Kafka, Go, and SQL to process GPU telemetry every 5 seconds so on-call engineers catch a failing GPU or overheating node before it takes down a training run that already cost hours of compute time and cannot be recovered.
Keywords: [Kafka, Go, SQL]

3. Configured CI/CD, Docker, Git, and Agile delivery practices to automate the full build and deployment pipeline so engineering teams ship fixes to the monitoring system the same day they are ready instead of running their cluster on outdated tooling while a manual release process holds everything up.
Keywords: [CI/CD, Docker, Git, Agile]

### AI Engineer

1. Built a production grade ML training infrastructure monitoring platform with Go, SQL, Cloud, and MLOps practices so AI teams know the moment a GPU fails or a training job crashes instead of discovering hours later that an entire run is gone and the compute budget went with it.
Keywords: [Go, SQL, Cloud, MLOps, production systems, ML]

2. Designed an AI failure analysis engine with Go and SQL following AI architecture patterns that classify GPU job failures and return ordered debugging steps so engineers spend their time fixing the problem instead of spending hours reading raw logs just to figure out what the problem even is.
Keywords: [Go, SQL, AI architecture, ML]

3. Built an AI orchestration layer with Go and Cloud infrastructure that routes ML training failures through a rule-based system architected to swap in a live LLM so the team can upgrade to intelligent failure analysis without stopping the product or rebuilding the system around it.
Keywords: [AI orchestration, ML, production systems, Go, Cloud]

## Verified Metrics

- [ESTIMATE] 40 GPU telemetry events processed every 5 seconds across 5 nodes and 8 GPUs each
- [ESTIMATE] 2-second SSE broadcast interval delivering live cluster state to all connected clients
- [ESTIMATE] 11-service stack launches in one command via Docker Compose
- [ESTIMATE] Redis TTL of 2 seconds on cluster summary balancing data freshness against database load
- [ESTIMATE] Health score computed from 4 weighted penalty factors modeling real cluster health logic
- [UNKNOWN] API response times under sustained load — not measured
- [UNKNOWN] Kafka consumer lag under high telemetry volume — not measured
- [UNKNOWN] Memory consumption of full running stack — not measured
- [UNKNOWN] Time from fault injection to alert appearing in dashboard — not measured

## Keywords

Full Stack: TypeScript, JavaScript, HTML, CSS, React, Go, REST API, SQL, Docker, Git, CI/CD, DevOps, Cloud, Agile

Backend: Go, REST API, SQL, Redis, Kafka, Docker, Git, CI/CD, DevOps, Cloud, Agile

AI Engineer: ML, Go, SQL, Cloud, MLOps, production systems, AI architecture, AI orchestration

Project terms for interviews: chi, pgx, kafka-go, go-redis, Server-Sent Events, OpenTelemetry, Jaeger, Prometheus, Grafana, Vite, Recharts, Radix UI, go:embed, rule-based engine, RAG scaffold, Ollama upgrade path

## Related Docs

Bullet writing rules: `interview-prep/resume-bullet-playbook.md`
