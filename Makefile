# ClusterOps — Makefile
# Usage: make <target>
#
# Targets:
#   dev       — start full stack (infra + services + frontend)
#   down      — stop and remove all containers
#   logs      — tail logs for all services
#   build     — build Go backend images
#   test      — run Go backend tests
#   frontend  — install and start frontend only (for UI dev without Docker)
#   clean     — remove build artefacts and volumes
#   demo      — start stack and print live cluster URL

COMPOSE       = docker compose -f docker-compose.yml
BACKEND_DIR   = ./backend
FRONTEND_DIR  = ./frontend

.PHONY: dev down logs build test frontend clean demo tidy

# ─── Stack lifecycle ─────────────────────────────────────────────────────────

dev:
	@echo "Starting ClusterOps full stack..."
	$(COMPOSE) up -d --build
	@echo ""
	@echo "  Console    →  http://localhost:3000"
	@echo "  API        →  http://localhost:8080"
	@echo "  Grafana    →  http://localhost:3001"
	@echo "  Prometheus →  http://localhost:9090"
	@echo "  Jaeger     →  http://localhost:16686"
	@echo ""
	@echo "Waiting for simulator to start publishing events..."
	@sleep 5
	@echo "Done. Run 'make logs' to watch the data flow."

down:
	$(COMPOSE) down --remove-orphans

logs:
	$(COMPOSE) logs -f --tail=50

logs-%:
	$(COMPOSE) logs -f --tail=100 $*

# ─── Build ───────────────────────────────────────────────────────────────────

build:
	$(COMPOSE) build api ingestion simulator

tidy:
	cd $(BACKEND_DIR) && go mod tidy

# ─── Test ────────────────────────────────────────────────────────────────────

test:
	cd $(BACKEND_DIR) && go test ./... -v -count=1

test-assistant:
	cd $(BACKEND_DIR) && go test ./internal/assistant/... -v -run TestAnalyze

# ─── Frontend (local, no Docker) ─────────────────────────────────────────────

frontend:
	cd $(FRONTEND_DIR) && npm install && npm run dev

frontend-build:
	cd $(FRONTEND_DIR) && npm install && npm run build

# ─── Infra only (for backend development without rebuilding images) ───────────

infra:
	$(COMPOSE) up -d postgres redis zookeeper kafka prometheus grafana jaeger otel-collector
	@echo "Infrastructure ready. Start API/ingestion/simulator locally with:"
	@echo "  cd backend && go run ./cmd/server"
	@echo "  cd backend && go run ./cmd/ingestion"
	@echo "  cd backend && go run ./cmd/simulator"

# ─── Demo ────────────────────────────────────────────────────────────────────

demo: dev
	@echo ""
	@echo "─────────────────────────────────────────────────"
	@echo "  ClusterOps Demo"
	@echo "─────────────────────────────────────────────────"
	@echo "  1. Open http://localhost:3000 — Dashboard"
	@echo "  2. Watch the GPU heatmap update every 5s"
	@echo "  3. A job will fail within ~60s"
	@echo "  4. Go to Jobs → click the failed job"
	@echo "  5. Go to Assistant → select the job → Analyze"
	@echo "  6. See the rule-based debugging playbook"
	@echo "  7. Grafana at http://localhost:3001 for metrics"
	@echo "  8. Jaeger at http://localhost:16686 for traces"
	@echo "─────────────────────────────────────────────────"

# ─── Clean ───────────────────────────────────────────────────────────────────

clean:
	$(COMPOSE) down -v --remove-orphans
	cd $(BACKEND_DIR) && go clean -cache
	rm -rf $(FRONTEND_DIR)/dist $(FRONTEND_DIR)/node_modules
