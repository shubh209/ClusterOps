// Package kafka provides a typed producer and consumer for ClusterOps events.
// All messages are JSON-encoded. Each topic maps to one event type.
package kafka

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/clusterops/backend/internal/models"
	"github.com/segmentio/kafka-go"
	"go.uber.org/zap"
)

// ProducerConfig holds connection settings for the Kafka producer.
type ProducerConfig struct {
	Brokers []string // e.g. ["localhost:9092"]
}

// Producer wraps per-topic kafka.Writer instances.
type Producer struct {
	writers map[string]*kafka.Writer
	logger  *zap.Logger
}

// NewProducer creates one kafka.Writer per topic.
func NewProducer(cfg ProducerConfig, logger *zap.Logger) *Producer {
	topics := []string{
		models.KafkaTopics.Nodes,
		models.KafkaTopics.Jobs,
		models.KafkaTopics.GPUs,
		models.KafkaTopics.Alerts,
	}

	writers := make(map[string]*kafka.Writer, len(topics))
	for _, topic := range topics {
		writers[topic] = &kafka.Writer{
			Addr:         kafka.TCP(cfg.Brokers...),
			Topic:        topic,
			Balancer:     &kafka.LeastBytes{},
			RequiredAcks: kafka.RequireOne,
			Async:        false, // synchronous for reliability in demo
			// Automatically create topics on first write.
			AllowAutoTopicCreation: true,
		}
	}

	logger.Info("kafka producer ready", zap.Strings("brokers", cfg.Brokers))
	return &Producer{writers: writers, logger: logger}
}

// Close shuts down all writers.
func (p *Producer) Close() {
	for topic, w := range p.writers {
		if err := w.Close(); err != nil {
			p.logger.Warn("kafka writer close", zap.String("topic", topic), zap.Error(err))
		}
	}
}

// publish serialises val as JSON and writes it to the given topic.
// key is used for partition routing (empty = round-robin).
func (p *Producer) publish(ctx context.Context, topic, key string, val interface{}) error {
	b, err := json.Marshal(val)
	if err != nil {
		return fmt.Errorf("marshal %s: %w", topic, err)
	}

	msg := kafka.Message{
		Key:   []byte(key),
		Value: b,
		Time:  time.Now(),
	}

	w, ok := p.writers[topic]
	if !ok {
		return fmt.Errorf("no writer for topic %q", topic)
	}

	if err := w.WriteMessages(ctx, msg); err != nil {
		return fmt.Errorf("write %s: %w", topic, err)
	}
	return nil
}

// ─── typed publish helpers ────────────────────────────────────────────────────

// PublishNodeEvent sends a node state change to cluster.nodes.
func (p *Producer) PublishNodeEvent(ctx context.Context, n *models.Node) error {
	evt := models.NodeEvent{
		Type:      models.EventTypeNodeUpdate,
		Timestamp: time.Now(),
		Node:      *n,
	}
	return p.publish(ctx, models.KafkaTopics.Nodes, n.ID, evt)
}

// PublishJobEvent sends a job state change to cluster.jobs.
func (p *Producer) PublishJobEvent(ctx context.Context, j *models.Job) error {
	evt := models.JobEvent{
		Type:      models.EventTypeJobUpdate,
		Timestamp: time.Now(),
		Job:       *j,
	}
	return p.publish(ctx, models.KafkaTopics.Jobs, j.ID, evt)
}

// PublishGPUMetric sends a GPU telemetry snapshot to cluster.gpus.
// Keyed by "nodeID:gpuIndex" so all metrics for a GPU go to the same partition.
func (p *Producer) PublishGPUMetric(ctx context.Context, m *models.GPUMetric) error {
	evt := models.GPUMetricEvent{
		Type:      models.EventTypeGPUMetric,
		Timestamp: time.Now(),
		Metric:    *m,
	}
	key := fmt.Sprintf("%s:%d", m.NodeID, m.GPUIndex)
	return p.publish(ctx, models.KafkaTopics.GPUs, key, evt)
}

// PublishAlertFired sends a newly fired alert to cluster.alerts.
func (p *Producer) PublishAlertFired(ctx context.Context, a *models.Alert) error {
	evt := models.AlertEvent{
		Type:      models.EventTypeAlertFired,
		Timestamp: time.Now(),
		Alert:     *a,
	}
	return p.publish(ctx, models.KafkaTopics.Alerts, a.ID, evt)
}

// PublishAlertResolved sends an alert resolution event to cluster.alerts.
func (p *Producer) PublishAlertResolved(ctx context.Context, a *models.Alert) error {
	evt := models.AlertEvent{
		Type:      models.EventTypeAlertResolved,
		Timestamp: time.Now(),
		Alert:     *a,
	}
	return p.publish(ctx, models.KafkaTopics.Alerts, a.ID, evt)
}
