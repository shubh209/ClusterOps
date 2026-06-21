# ClusterOps — AI Training Cluster Operations Console

A full-stack observability platform for AI training infrastructure. Operators see why training jobs fail, where GPU capacity is being wasted, and get AI-assisted debugging playbooks — all in real time.

Built as a portfolio project for OpenAI / ML infrastructure engineering roles.

---

## Demo Story

1. `make demo` starts the full stack
2. The simulator generates 5 GPU nodes and submits training jobs (LLaMA, Mistral, Falcon, etc.)
3. Within 60 seconds, a node degrades and a job fails with an OOM error
4. The **Dashboard** shows the cluster health drop and a new alert appear live
5. **Jobs** page shows the failed job with its log tail and failure badge
6. **Assistant** page analyzes the job: root cause (GPU OOM), 6 debugging steps, shell commands, prevention tips
7. **Grafana** shows API latency, cache hit rate, and SSE client count
8. **Jaeger** shows distributed traces across the API and ingestion services

---

## Architecture

```
Simulator → Kafka → Ingestion → PostgreSQL + Redis
                                      ↓
                             Go API Server (chi)
                                      ↓
                          React Frontend (Vite + Tailwind)
                                      ↓
                    Prometheus / Grafana / Jaeger / OTel
```

---

## Tech Stack

| Layer | Technology |
|-------|-----------|
| API Server | Go 1.22 + chi router |
| Data Pipeline | Kafka (Confluent) + Go consumers |
| Database | PostgreSQL 16 (pgx v5) |
| Cache | Redis 7 (go-redis) |
| AI Assistant | Rule-based engine (Go) — Ollama-ready |
| Frontend | React 18 + TypeScript + Vite |
| Charts | Recharts |
| Styling | Tailwind CSS (dark terminal theme) |
| Real-time | Server-Sent Events (SSE) |
| Metrics | Prometheus + Grafana |
| Tracing | OpenTelemetry → Jaeger |
| Container | Docker Compose |

All services are free and open-source. No cloud account or API keys required.

---

## Quick Start

**Prerequisites:** Docker Desktop, Node.js 20+, Go 1.22+

```bash
# Clone and start everything
git clone <repo>
cd ClusterOps
make demo
```

Opens:
- **Console** → http://localhost:3000
- **Grafana** → http://localhost:3001
- **Prometheus** → http://localhost:9090
- **Jaeger** → http://localhost:16686

---

## Local Development (without Docker)

```bash
# Start infrastructure only (Postgres, Redis, Kafka, Prometheus, Grafana, Jaeger)
make infra

# In separate terminals:
cd backend && go run ./cmd/simulator    # generates synthetic cluster data
cd backend && go run ./cmd/ingestion   # Kafka → PostgreSQL + Redis
cd backend && go run ./cmd/server      # REST API + SSE on :8080

cd frontend && npm install && npm run dev  # React app on :3000
```

---

## Project Structure

```
ClusterOps/
├── backend/
│   ├── cmd/
│   │   ├── server/       # API server entrypoint
│   │   ├── ingestion/    # Kafka consumer entrypoint
│   │   └── simulator/    # Synthetic cluster generator
│   ├── internal/
│   │   ├── models/       # Shared Go types
│   │   ├── store/        # PostgreSQL queries (pgx)
│   │   ├── cache/        # Redis helpers
│   │   ├── kafka/        # Producer + consumer
│   │   ├── simulator/    # State machine + fault injector
│   │   ├── ingestion/    # Kafka→DB+Cache pipeline
│   │   ├── api/          # HTTP handlers + SSE + Prometheus
│   │   ├── assistant/    # Rule-based failure analysis engine
│   │   └── telemetry/    # OpenTelemetry setup
│   └── migrations/       # SQL schema (embedded)
├── frontend/
│   └── src/
│       ├── pages/        # Dashboard, Nodes, Jobs, Alerts, Assistant
│       ├── components/   # Shared UI components
│       ├── hooks/        # useSSE, useQuery
│       ├── lib/          # API client, utilities
│       └── types/        # TypeScript models (mirrors Go)
├── infra/
│   ├── docker/           # Dockerfile.backend (multi-stage, any service)
│   ├── otel/             # OTel collector config
│   ├── prometheus/       # Scrape config
│   └── grafana/          # Dashboard JSON + provisioning
├── docker-compose.yml
├── Makefile
└── CONTEXT.md            # Full domain glossary + architecture
```

---

## AI Engineering Concepts Demonstrated

This project was built using AI-assisted engineering. The patterns used:

| Concept | Implementation |
|---------|---------------|
| **LLM as implementer** | All code generated with structured context per session |
| **Context engineering** | `CONTEXT.md` as grounding doc; task list for session continuity |
| **Rule-based baseline** | `internal/assistant/` — deterministic failure analysis |
| **Eval harness** | `engine_test.go` — table-driven tests validate every failure scenario |
| **RAG scaffold** | DebuggingSteps = structured retrieval from a playbook knowledge base |
| **MCP-ready** | Phase 5: expose PostgreSQL as an MCP server for direct LLM tool calls |

### Phase 5 — Ollama + RAG upgrade path

```bash
# Add to docker-compose.yml
ollama:
  image: ollama/ollama
  ports: ["11434:11434"]

# Pull a model
docker exec ollama ollama pull llama3
```

Then:
1. Add `pgvector` to PostgreSQL
2. Embed runbook text as vectors
3. Replace `assistant.Engine` with Ollama chat + retrieval
4. Expose cluster DB as an MCP server for tool-call access

---

## Running Tests

```bash
# All backend tests
make test

# Assistant eval harness only
make test-assistant
```

The harness validates: correct `RootCause`, `Severity`, minimum `DebuggingSteps` count,
and `Confidence` threshold for 7 job failure scenarios + 3 node failure scenarios.
