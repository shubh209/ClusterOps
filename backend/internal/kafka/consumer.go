package kafka

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/clusterops/backend/internal/models"
	"github.com/segmentio/kafka-go"
	"go.uber.org/zap"
)

// ConsumerConfig holds settings for a Kafka consumer group.
type ConsumerConfig struct {
	Brokers []string
	GroupID string // consumer group — allows multiple ingestion instances
}

// Handlers is the set of callbacks invoked when a message is consumed.
// Each handler receives a decoded event; returning an error causes the
// consumer to log and skip (not retry) — appropriate for a demo.
type Handlers struct {
	OnNodeEvent    func(ctx context.Context, evt models.NodeEvent) error
	OnJobEvent     func(ctx context.Context, evt models.JobEvent) error
	OnGPUMetric    func(ctx context.Context, evt models.GPUMetricEvent) error
	OnAlertEvent   func(ctx context.Context, evt models.AlertEvent) error
}

// Consumer manages one kafka.Reader per topic.
type Consumer struct {
	readers  map[string]*kafka.Reader
	handlers Handlers
	logger   *zap.Logger
}

// NewConsumer creates one kafka.Reader per topic and attaches handlers.
func NewConsumer(cfg ConsumerConfig, h Handlers, logger *zap.Logger) *Consumer {
	topics := []string{
		models.KafkaTopics.Nodes,
		models.KafkaTopics.Jobs,
		models.KafkaTopics.GPUs,
		models.KafkaTopics.Alerts,
	}

	readers := make(map[string]*kafka.Reader, len(topics))
	for _, topic := range topics {
		readers[topic] = kafka.NewReader(kafka.ReaderConfig{
			Brokers:  cfg.Brokers,
			GroupID:  cfg.GroupID,
			Topic:    topic,
			MinBytes: 1,
			MaxBytes: 1 << 20, // 1 MB
		})
	}

	logger.Info("kafka consumer ready",
		zap.Strings("brokers", cfg.Brokers),
		zap.String("group", cfg.GroupID),
	)
	return &Consumer{readers: readers, handlers: h, logger: logger}
}

// Close shuts down all readers.
func (c *Consumer) Close() {
	for topic, r := range c.readers {
		if err := r.Close(); err != nil {
			c.logger.Warn("kafka reader close", zap.String("topic", topic), zap.Error(err))
		}
	}
}

// Start begins consuming all topics concurrently.
// Each topic runs in its own goroutine; all stop when ctx is cancelled.
func (c *Consumer) Start(ctx context.Context) {
	for topic, reader := range c.readers {
		go c.consume(ctx, topic, reader)
	}
}

// consume is the per-topic read loop.
func (c *Consumer) consume(ctx context.Context, topic string, r *kafka.Reader) {
	c.logger.Info("consumer loop started", zap.String("topic", topic))
	for {
		msg, err := r.ReadMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				c.logger.Info("consumer loop stopped", zap.String("topic", topic))
				return
			}
			c.logger.Error("kafka read error", zap.String("topic", topic), zap.Error(err))
			continue
		}

		if err := c.dispatch(ctx, topic, msg.Value); err != nil {
			c.logger.Error("dispatch error",
				zap.String("topic", topic),
				zap.String("key", string(msg.Key)),
				zap.Error(err),
			)
		}
	}
}

// dispatch routes a raw JSON message to the appropriate handler.
func (c *Consumer) dispatch(ctx context.Context, topic string, value []byte) error {
	switch topic {
	case models.KafkaTopics.Nodes:
		if c.handlers.OnNodeEvent == nil {
			return nil
		}
		var evt models.NodeEvent
		if err := json.Unmarshal(value, &evt); err != nil {
			return fmt.Errorf("unmarshal NodeEvent: %w", err)
		}
		return c.handlers.OnNodeEvent(ctx, evt)

	case models.KafkaTopics.Jobs:
		if c.handlers.OnJobEvent == nil {
			return nil
		}
		var evt models.JobEvent
		if err := json.Unmarshal(value, &evt); err != nil {
			return fmt.Errorf("unmarshal JobEvent: %w", err)
		}
		return c.handlers.OnJobEvent(ctx, evt)

	case models.KafkaTopics.GPUs:
		if c.handlers.OnGPUMetric == nil {
			return nil
		}
		var evt models.GPUMetricEvent
		if err := json.Unmarshal(value, &evt); err != nil {
			return fmt.Errorf("unmarshal GPUMetricEvent: %w", err)
		}
		return c.handlers.OnGPUMetric(ctx, evt)

	case models.KafkaTopics.Alerts:
		if c.handlers.OnAlertEvent == nil {
			return nil
		}
		var evt models.AlertEvent
		if err := json.Unmarshal(value, &evt); err != nil {
			return fmt.Errorf("unmarshal AlertEvent: %w", err)
		}
		return c.handlers.OnAlertEvent(ctx, evt)

	default:
		return fmt.Errorf("unknown topic %q", topic)
	}
}
