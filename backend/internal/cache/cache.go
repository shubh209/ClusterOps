// Package cache wraps go-redis with typed helpers for ClusterOps hot-path reads.
// Every public method encodes/decodes JSON and applies a sensible default TTL.
// The API server reads from here first; on a miss it falls back to the store.
package cache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/clusterops/backend/internal/models"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// TTLs — tuned so the dashboard feels real-time without hammering Postgres.
const (
	TTLClusterSummary = 2 * time.Second  // dashboard polls every 2s
	TTLNodeList       = 3 * time.Second
	TTLNode           = 3 * time.Second
	TTLJobList        = 3 * time.Second
	TTLJob            = 5 * time.Second
	TTLAlertList      = 2 * time.Second
	TTLGPUSummary     = 2 * time.Second
)

// Key prefixes — keep naming consistent so it's easy to flush by pattern.
const (
	keyClusterSummary = "cluster:summary"
	keyNodeList       = "nodes:list"
	keyNodePrefix     = "node:"       // + id
	keyJobList        = "jobs:list:"  // + encoded filter hash
	keyJobPrefix      = "job:"        // + id
	keyAlertList      = "alerts:list"
	keyActiveAlerts   = "alerts:active"
	keyGPUSummary     = "gpu:summary"
)

// Client wraps a redis.Client with domain-typed methods.
type Client struct {
	rdb    *redis.Client
	logger *zap.Logger
}

// Config holds Redis connection parameters.
type Config struct {
	Addr     string // e.g. "localhost:6379"
	Password string
	DB       int
}

// New connects to Redis and returns a ready Client.
func New(cfg Config, logger *zap.Logger) (*Client, error) {
	rdb := redis.NewClient(&redis.Options{
		Addr:     cfg.Addr,
		Password: cfg.Password,
		DB:       cfg.DB,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := rdb.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("redis ping: %w", err)
	}
	logger.Info("redis connected", zap.String("addr", cfg.Addr))
	return &Client{rdb: rdb, logger: logger}, nil
}

// Close shuts down the Redis connection.
func (c *Client) Close() error {
	return c.rdb.Close()
}

// ─── helpers ─────────────────────────────────────────────────────────────────

func (c *Client) set(ctx context.Context, key string, val interface{}, ttl time.Duration) error {
	b, err := json.Marshal(val)
	if err != nil {
		return fmt.Errorf("cache marshal %s: %w", key, err)
	}
	return c.rdb.Set(ctx, key, b, ttl).Err()
}

func (c *Client) get(ctx context.Context, key string, dest interface{}) (bool, error) {
	b, err := c.rdb.Get(ctx, key).Bytes()
	if errors.Is(err, redis.Nil) {
		return false, nil // cache miss — not an error
	}
	if err != nil {
		return false, fmt.Errorf("cache get %s: %w", key, err)
	}
	if err := json.Unmarshal(b, dest); err != nil {
		return false, fmt.Errorf("cache unmarshal %s: %w", key, err)
	}
	return true, nil
}

func (c *Client) del(ctx context.Context, keys ...string) error {
	return c.rdb.Del(ctx, keys...).Err()
}

// ─── cluster summary ─────────────────────────────────────────────────────────

func (c *Client) SetClusterSummary(ctx context.Context, s *models.ClusterSummary) error {
	return c.set(ctx, keyClusterSummary, s, TTLClusterSummary)
}

func (c *Client) GetClusterSummary(ctx context.Context) (*models.ClusterSummary, bool, error) {
	var s models.ClusterSummary
	hit, err := c.get(ctx, keyClusterSummary, &s)
	if !hit || err != nil {
		return nil, false, err
	}
	return &s, true, nil
}

// ─── GPU summary ─────────────────────────────────────────────────────────────

func (c *Client) SetGPUSummary(ctx context.Context, s *models.ClusterGPUSummary) error {
	return c.set(ctx, keyGPUSummary, s, TTLGPUSummary)
}

func (c *Client) GetGPUSummary(ctx context.Context) (*models.ClusterGPUSummary, bool, error) {
	var s models.ClusterGPUSummary
	hit, err := c.get(ctx, keyGPUSummary, &s)
	if !hit || err != nil {
		return nil, false, err
	}
	return &s, true, nil
}

// ─── nodes ───────────────────────────────────────────────────────────────────

func (c *Client) SetNodeList(ctx context.Context, nodes []*models.Node) error {
	return c.set(ctx, keyNodeList, nodes, TTLNodeList)
}

func (c *Client) GetNodeList(ctx context.Context) ([]*models.Node, bool, error) {
	var nodes []*models.Node
	hit, err := c.get(ctx, keyNodeList, &nodes)
	if !hit || err != nil {
		return nil, false, err
	}
	return nodes, true, nil
}

func (c *Client) SetNode(ctx context.Context, n *models.Node) error {
	return c.set(ctx, keyNodePrefix+n.ID, n, TTLNode)
}

func (c *Client) GetNode(ctx context.Context, id string) (*models.Node, bool, error) {
	var n models.Node
	hit, err := c.get(ctx, keyNodePrefix+id, &n)
	if !hit || err != nil {
		return nil, false, err
	}
	return &n, true, nil
}

// InvalidateNode removes cached entries for a specific node and the node list.
func (c *Client) InvalidateNode(ctx context.Context, id string) error {
	return c.del(ctx, keyNodePrefix+id, keyNodeList, keyClusterSummary)
}

// ─── jobs ─────────────────────────────────────────────────────────────────────

// SetJobList caches a job list under a key derived from the filter.
func (c *Client) SetJobList(ctx context.Context, filterKey string, jobs []*models.Job) error {
	return c.set(ctx, keyJobList+filterKey, jobs, TTLJobList)
}

func (c *Client) GetJobList(ctx context.Context, filterKey string) ([]*models.Job, bool, error) {
	var jobs []*models.Job
	hit, err := c.get(ctx, keyJobList+filterKey, &jobs)
	if !hit || err != nil {
		return nil, false, err
	}
	return jobs, true, nil
}

func (c *Client) SetJob(ctx context.Context, j *models.Job) error {
	return c.set(ctx, keyJobPrefix+j.ID, j, TTLJob)
}

func (c *Client) GetJob(ctx context.Context, id string) (*models.Job, bool, error) {
	var j models.Job
	hit, err := c.get(ctx, keyJobPrefix+id, &j)
	if !hit || err != nil {
		return nil, false, err
	}
	return &j, true, nil
}

// InvalidateJob removes cached entries for a specific job and all job lists.
func (c *Client) InvalidateJob(ctx context.Context, id string) error {
	// Delete the specific job key.
	if err := c.del(ctx, keyJobPrefix+id); err != nil {
		return err
	}
	// Flush all job list keys by pattern — cheap at demo scale.
	return c.flushByPattern(ctx, keyJobList+"*")
}

// ─── alerts ──────────────────────────────────────────────────────────────────

func (c *Client) SetAlertList(ctx context.Context, alerts []*models.Alert) error {
	return c.set(ctx, keyAlertList, alerts, TTLAlertList)
}

func (c *Client) GetAlertList(ctx context.Context) ([]*models.Alert, bool, error) {
	var alerts []*models.Alert
	hit, err := c.get(ctx, keyAlertList, &alerts)
	if !hit || err != nil {
		return nil, false, err
	}
	return alerts, true, nil
}

func (c *Client) SetActiveAlerts(ctx context.Context, alerts []*models.Alert) error {
	return c.set(ctx, keyActiveAlerts, alerts, TTLAlertList)
}

func (c *Client) GetActiveAlerts(ctx context.Context) ([]*models.Alert, bool, error) {
	var alerts []*models.Alert
	hit, err := c.get(ctx, keyActiveAlerts, &alerts)
	if !hit || err != nil {
		return nil, false, err
	}
	return alerts, true, nil
}

// InvalidateAlerts clears all alert cache keys.
func (c *Client) InvalidateAlerts(ctx context.Context) error {
	return c.del(ctx, keyAlertList, keyActiveAlerts, keyClusterSummary)
}

// ─── utilities ───────────────────────────────────────────────────────────────

// flushByPattern deletes all keys matching a glob pattern.
// Uses SCAN to avoid blocking Redis on large keyspaces.
func (c *Client) flushByPattern(ctx context.Context, pattern string) error {
	var cursor uint64
	for {
		keys, nextCursor, err := c.rdb.Scan(ctx, cursor, pattern, 100).Result()
		if err != nil {
			return fmt.Errorf("flushByPattern scan: %w", err)
		}
		if len(keys) > 0 {
			if err := c.rdb.Del(ctx, keys...).Err(); err != nil {
				return fmt.Errorf("flushByPattern del: %w", err)
			}
		}
		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}
	return nil
}

// FlushAll wipes every ClusterOps key — used in tests and on startup.
func (c *Client) FlushAll(ctx context.Context) error {
	return c.rdb.FlushDB(ctx).Err()
}

// HealthCheck pings Redis.
func (c *Client) HealthCheck(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	return c.rdb.Ping(ctx).Err()
}
