package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/clusterops/backend/internal/store"
	"github.com/go-chi/chi/v5"
)

// ─── helpers ─────────────────────────────────────────────────────────────────

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// ─── cluster summary ─────────────────────────────────────────────────────────

func (s *Server) handleClusterSummary(w http.ResponseWriter, r *http.Request) {
	// Try Redis first.
	if summary, hit, _ := s.cache.GetClusterSummary(r.Context()); hit {
		recordCacheHit("cluster_summary")
		writeJSON(w, http.StatusOK, summary)
		return
	}
	recordCacheMiss("cluster_summary")

	summary, err := s.db.ClusterSummary(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to compute cluster summary")
		return
	}

	_ = s.cache.SetClusterSummary(r.Context(), summary)
	writeJSON(w, http.StatusOK, summary)
}

// ─── nodes ───────────────────────────────────────────────────────────────────

func (s *Server) handleListNodes(w http.ResponseWriter, r *http.Request) {
	if nodes, hit, _ := s.cache.GetNodeList(r.Context()); hit {
		recordCacheHit("node_list")
		writeJSON(w, http.StatusOK, nodes)
		return
	}
	recordCacheMiss("node_list")

	nodes, err := s.db.ListNodes(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list nodes")
		return
	}

	_ = s.cache.SetNodeList(r.Context(), nodes)
	writeJSON(w, http.StatusOK, nodes)
}

func (s *Server) handleGetNode(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	if node, hit, _ := s.cache.GetNode(r.Context(), id); hit {
		recordCacheHit("node")
		writeJSON(w, http.StatusOK, node)
		return
	}
	recordCacheMiss("node")

	node, err := s.db.GetNode(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "node not found")
		return
	}

	_ = s.cache.SetNode(r.Context(), node)
	writeJSON(w, http.StatusOK, node)
}

func (s *Server) handleNodeGPUTimeSeries(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	from, to := parseTimeRange(r)

	series, err := s.db.GetNodeGPUTimeSeries(r.Context(), id, from, to)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get GPU time series")
		return
	}
	writeJSON(w, http.StatusOK, series)
}

// ─── jobs ─────────────────────────────────────────────────────────────────────

func (s *Server) handleListJobs(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	statusFilter := q.Get("status")
	userFilter := q.Get("user_id")
	cacheKey := statusFilter + "|" + userFilter

	if jobs, hit, _ := s.cache.GetJobList(r.Context(), cacheKey); hit {
		recordCacheHit("job_list")
		writeJSON(w, http.StatusOK, jobs)
		return
	}
	recordCacheMiss("job_list")

	jobs, err := s.db.ListJobs(r.Context(), store.JobFilter{
		Status: statusFilter,
		UserID: userFilter,
		Limit:  200,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list jobs")
		return
	}

	_ = s.cache.SetJobList(r.Context(), cacheKey, jobs)
	writeJSON(w, http.StatusOK, jobs)
}

func (s *Server) handleGetJob(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	if job, hit, _ := s.cache.GetJob(r.Context(), id); hit {
		recordCacheHit("job")
		writeJSON(w, http.StatusOK, job)
		return
	}
	recordCacheMiss("job")

	job, err := s.db.GetJob(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "job not found")
		return
	}

	_ = s.cache.SetJob(r.Context(), job)
	writeJSON(w, http.StatusOK, job)
}

// handleJobAlerts returns all alerts linked to a specific job — used by the
// job detail panel to surface related incidents inline.
func (s *Server) handleJobAlerts(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	alerts, err := s.db.GetAlertsByJob(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get job alerts")
		return
	}
	writeJSON(w, http.StatusOK, alerts)
}

// ─── metrics ──────────────────────────────────────────────────────────────────

func (s *Server) handleGPUSummary(w http.ResponseWriter, r *http.Request) {
	if summary, hit, _ := s.cache.GetGPUSummary(r.Context()); hit {
		recordCacheHit("gpu_summary")
		writeJSON(w, http.StatusOK, summary)
		return
	}
	recordCacheMiss("gpu_summary")

	summary, err := s.db.ClusterGPUSummary(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to compute GPU summary")
		return
	}

	_ = s.cache.SetGPUSummary(r.Context(), summary)
	writeJSON(w, http.StatusOK, summary)
}

func (s *Server) handleCapacityMetrics(w http.ResponseWriter, r *http.Request) {
	nodes, err := s.db.ListNodes(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list nodes")
		return
	}

	type capacityRow struct {
		NodeID        string  `json:"node_id"`
		Hostname      string  `json:"hostname"`
		Status        string  `json:"status"`
		TotalGPUs     int     `json:"total_gpus"`
		AllocatedGPUs int     `json:"allocated_gpus"`
		IdleGPUs      int     `json:"idle_gpus"`
		WastePercent  float64 `json:"waste_percent"`
		AvgUtil       float64 `json:"avg_utilization_pct"`
	}

	rows := make([]capacityRow, 0, len(nodes))
	for _, n := range nodes {
		rows = append(rows, capacityRow{
			NodeID:        n.ID,
			Hostname:      n.Hostname,
			Status:        string(n.Status),
			TotalGPUs:     n.GPUCount,
			AllocatedGPUs: n.AllocatedGPUs,
			IdleGPUs:      n.GPUCount - n.AllocatedGPUs,
			WastePercent:  n.GPUWastePercent(),
			AvgUtil:       n.AvgGPUUtilization(),
		})
	}
	writeJSON(w, http.StatusOK, rows)
}

// ─── alerts ──────────────────────────────────────────────────────────────────

func (s *Server) handleListAlerts(w http.ResponseWriter, r *http.Request) {
	activeOnly := r.URL.Query().Get("active") == "true"

	if !activeOnly {
		if alerts, hit, _ := s.cache.GetAlertList(r.Context()); hit {
			recordCacheHit("alert_list")
			writeJSON(w, http.StatusOK, alerts)
			return
		}
		recordCacheMiss("alert_list")
	} else {
		if alerts, hit, _ := s.cache.GetActiveAlerts(r.Context()); hit {
			recordCacheHit("alert_active")
			writeJSON(w, http.StatusOK, alerts)
			return
		}
		recordCacheMiss("alert_active")
	}

	alerts, err := s.db.ListAlerts(r.Context(), activeOnly, 200)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list alerts")
		return
	}

	if activeOnly {
		_ = s.cache.SetActiveAlerts(r.Context(), alerts)
	} else {
		_ = s.cache.SetAlertList(r.Context(), alerts)
	}
	writeJSON(w, http.StatusOK, alerts)
}

// ─── health ──────────────────────────────────────────────────────────────────

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	dbErr := s.db.HealthCheck(r.Context())
	cacheErr := s.cache.HealthCheck(r.Context())

	status := "ok"
	code := http.StatusOK
	if dbErr != nil || cacheErr != nil {
		status = "degraded"
		code = http.StatusServiceUnavailable
	}

	writeJSON(w, code, map[string]interface{}{
		"status":    status,
		"postgres":  errStr(dbErr),
		"redis":     errStr(cacheErr),
		"timestamp": time.Now(),
	})
}

// ─── SSE stream ──────────────────────────────────────────────────────────────

func (s *Server) handleSSEStream(w http.ResponseWriter, r *http.Request) {
	s.broker.ServeHTTP(w, r)
}

// ─── utility ─────────────────────────────────────────────────────────────────

func parseTimeRange(r *http.Request) (from, to time.Time) {
	q := r.URL.Query()
	to = time.Now()
	from = to.Add(-1 * time.Hour) // default: last hour

	if v := q.Get("from"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			from = t
		}
	}
	if v := q.Get("to"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			to = t
		}
	}
	return from, to
}

func errStr(err error) string {
	if err == nil {
		return "ok"
	}
	return err.Error()
}

// handleAssistantAnalyze is a placeholder that routes to the assistant package.
// Wired properly in server.go after the assistant package is built (task 8).
func (s *Server) handleAssistantAnalyze(w http.ResponseWriter, r *http.Request) {
	var req struct {
		JobID  string `json:"job_id"`
		NodeID string `json:"node_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Resolve the target — job takes priority over node.
	if req.JobID != "" {
		job, err := s.db.GetJob(r.Context(), req.JobID)
		if err != nil {
			writeError(w, http.StatusNotFound, "job not found")
			return
		}
		alerts, _ := s.db.GetAlertsByJob(r.Context(), req.JobID)
		analysis := s.assistant.AnalyzeJob(job, alerts)
		writeJSON(w, http.StatusOK, analysis)
		return
	}

	if req.NodeID != "" {
		node, err := s.db.GetNode(r.Context(), req.NodeID)
		if err != nil {
			writeError(w, http.StatusNotFound, "node not found")
			return
		}
		analysis := s.assistant.AnalyzeNode(node)
		writeJSON(w, http.StatusOK, analysis)
		return
	}

	writeError(w, http.StatusBadRequest, "job_id or node_id required")
}

// handleSSEBroadcaster pushes live updates to connected SSE clients
// by polling the DB every 2 seconds and broadcasting diffs.
// Started as a goroutine from server.go.
func (s *Server) broadcastLoop(ctx interface{ Done() <-chan struct{} }) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.broadcastClusterSnapshot()
		}
	}
}

func (s *Server) broadcastClusterSnapshot() {
	ctx := contextBackground()

	if summary, err := s.db.ClusterSummary(ctx); err == nil {
		_ = s.cache.SetClusterSummary(ctx, summary)
		s.broker.BroadcastEvent("cluster_summary", summary)
	}

	if nodes, err := s.db.ListNodes(ctx); err == nil {
		_ = s.cache.SetNodeList(ctx, nodes)
		s.broker.BroadcastEvent("nodes", nodes)
	}

	if alerts, err := s.db.ListAlerts(ctx, true, 50); err == nil {
		_ = s.cache.SetActiveAlerts(ctx, alerts)
		s.broker.BroadcastEvent("alerts", alerts)
	}
}
