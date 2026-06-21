// Package store handles all PostgreSQL interactions via pgx.
// Each public function maps to one logical query; no ORM is used.
package store

import (
	"context"
	"embed"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// DB wraps a pgx connection pool and exposes typed query methods.
type DB struct {
	pool   *pgxpool.Pool
	logger *zap.Logger
}

// Config holds everything needed to open a PostgreSQL connection.
type Config struct {
	Host     string
	Port     int
	User     string
	Password string
	DBName   string
	SSLMode  string
}

// DSN returns the connection string for pgx.
func (c Config) DSN() string {
	return fmt.Sprintf(
		"postgres://%s:%s@%s:%d/%s?sslmode=%s",
		c.User, c.Password, c.Host, c.Port, c.DBName, c.SSLMode,
	)
}

// New opens the connection pool and runs any pending migrations.
func New(ctx context.Context, cfg Config, logger *zap.Logger) (*DB, error) {
	pool, err := pgxpool.New(ctx, cfg.DSN())
	if err != nil {
		return nil, fmt.Errorf("pgxpool.New: %w", err)
	}

	// Verify connectivity.
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("postgres ping: %w", err)
	}

	db := &DB{pool: pool, logger: logger}
	if err := db.migrate(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("migration: %w", err)
	}

	logger.Info("postgres connected", zap.String("host", cfg.Host), zap.Int("port", cfg.Port))
	return db, nil
}

// Close tears down the connection pool.
func (db *DB) Close() {
	db.pool.Close()
}

// migrate reads all *.sql files from the embedded migrations directory
// and executes them in filename order.  It is idempotent — every statement
// uses IF NOT EXISTS / CREATE OR REPLACE, so re-running is safe.
func (db *DB) migrate(ctx context.Context) error {
	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return fmt.Errorf("read migrations dir: %w", err)
	}

	// Sort by filename so 001 runs before 002.
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})

	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		data, err := migrationsFS.ReadFile("migrations/" + e.Name())
		if err != nil {
			return fmt.Errorf("read %s: %w", e.Name(), err)
		}
		if _, err := db.pool.Exec(ctx, string(data)); err != nil {
			return fmt.Errorf("exec %s: %w", e.Name(), err)
		}
		db.logger.Info("migration applied", zap.String("file", e.Name()))
	}
	return nil
}

// HealthCheck verifies the database is reachable.
func (db *DB) HealthCheck(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	return db.pool.Ping(ctx)
}
