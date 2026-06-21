package api

import (
	"context"
	"net/http"
	"time"

	"github.com/clusterops/backend/internal/assistant"
	"github.com/clusterops/backend/internal/cache"
	"github.com/clusterops/backend/internal/store"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"
)

// Server is the HTTP API server for ClusterOps.
type Server struct {
	db        *store.DB
	cache     *cache.Client
	assistant *assistant.Engine
	broker    *SSEBroker
	logger    *zap.Logger
	httpSrv   *http.Server
}

// Config holds API server configuration.
type Config struct {
	Addr            string // e.g. ":8080"
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	ShutdownTimeout time.Duration
}

// DefaultConfig returns sensible defaults.
func DefaultConfig() Config {
	return Config{
		Addr:            ":8080",
		ReadTimeout:     10 * time.Second,
		WriteTimeout:    30 * time.Second,
		ShutdownTimeout: 15 * time.Second,
	}
}

// New wires up the server with all dependencies.
func New(db *store.DB, c *cache.Client, asst *assistant.Engine, logger *zap.Logger, cfg Config) *Server {
	s := &Server{
		db:        db,
		cache:     c,
		assistant: asst,
		broker:    newSSEBroker(),
		logger:    logger,
	}

	s.httpSrv = &http.Server{
		Addr:         cfg.Addr,
		Handler:      s.routes(),
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
	}

	return s
}

// routes builds and returns the chi router.
func (s *Server) routes() http.Handler {
	r := chi.NewRouter()

	// ── global middleware ─────────────────────────────────────────────────
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Compress(5))
	r.Use(prometheusMiddleware)
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Content-Type", "X-Request-ID"},
		AllowCredentials: false,
		MaxAge:           300,
	}))

	// ── health & metrics ──────────────────────────────────────────────────
	r.Get("/health", s.handleHealth)
	r.Handle("/metrics", promhttp.Handler())

	// ── SSE stream ────────────────────────────────────────────────────────
	// Clients connect once; they receive push events for cluster_summary,
	// nodes, and alerts without polling individual endpoints.
	r.Get("/api/v1/stream", s.handleSSEStream)

	// ── API v1 ────────────────────────────────────────────────────────────
	r.Route("/api/v1", func(r chi.Router) {
		r.Use(middleware.SetHeader("Content-Type", "application/json"))

		// Cluster
		r.Get("/cluster/summary", s.handleClusterSummary)

		// Nodes
		r.Get("/nodes", s.handleListNodes)
		r.Get("/nodes/{id}", s.handleGetNode)
		r.Get("/nodes/{id}/gpu-series", s.handleNodeGPUTimeSeries)

		// Jobs
		r.Get("/jobs", s.handleListJobs)
		r.Get("/jobs/{id}", s.handleGetJob)
		r.Get("/jobs/{id}/alerts", s.handleJobAlerts)

		// Metrics
		r.Get("/metrics/gpu", s.handleGPUSummary)
		r.Get("/metrics/capacity", s.handleCapacityMetrics)

		// Alerts
		r.Get("/alerts", s.handleListAlerts)

		// AI Assistant
		r.Post("/assistant/analyze", s.handleAssistantAnalyze)
	})

	return r
}

// Start begins serving HTTP and the SSE broadcast loop.
// It blocks until the server is stopped.
func (s *Server) Start(ctx context.Context) error {
	// Start the SSE broadcast loop in the background.
	go s.broadcastLoop(ctx)

	s.logger.Info("api server starting", zap.String("addr", s.httpSrv.Addr))

	errCh := make(chan error, 1)
	go func() {
		if err := s.httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
		return s.shutdown()
	case err := <-errCh:
		return err
	}
}

// shutdown gracefully drains connections.
func (s *Server) shutdown() error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	s.logger.Info("api server shutting down")
	return s.httpSrv.Shutdown(ctx)
}

// contextBackground is a package-level alias to avoid importing context in handlers.go
// (which already has enough imports).
func contextBackground() context.Context {
	return context.Background()
}
