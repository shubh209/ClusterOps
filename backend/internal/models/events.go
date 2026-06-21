package models

import "time"

// EventType identifies what kind of cluster event is flowing through Kafka.
type EventType string

const (
	EventTypeNodeUpdate   EventType = "node.update"
	EventTypeJobUpdate    EventType = "job.update"
	EventTypeGPUMetric    EventType = "gpu.metric"
	EventTypeAlertFired   EventType = "alert.fired"
	EventTypeAlertResolved EventType = "alert.resolved"
)

// KafkaTopics defines the topic names used across producer and consumer.
var KafkaTopics = struct {
	Nodes  string
	Jobs   string
	GPUs   string
	Alerts string
}{
	Nodes:  "cluster.nodes",
	Jobs:   "cluster.jobs",
	GPUs:   "cluster.gpus",
	Alerts: "cluster.alerts",
}

// ClusterEvent is the envelope wrapping every Kafka message.
type ClusterEvent struct {
	Type      EventType   `json:"type"`
	Timestamp time.Time   `json:"timestamp"`
	Payload   interface{} `json:"payload"`
}

// NodeEvent carries a node state change.
type NodeEvent struct {
	Type      EventType `json:"type"`
	Timestamp time.Time `json:"timestamp"`
	Node      Node      `json:"payload"`
}

// JobEvent carries a job state change.
type JobEvent struct {
	Type      EventType `json:"type"`
	Timestamp time.Time `json:"timestamp"`
	Job       Job       `json:"payload"`
}

// GPUMetricEvent carries a GPU telemetry snapshot.
type GPUMetricEvent struct {
	Type      EventType `json:"type"`
	Timestamp time.Time `json:"timestamp"`
	Metric    GPUMetric `json:"payload"`
}

// AlertEvent carries a fired or resolved alert.
type AlertEvent struct {
	Type      EventType `json:"type"`
	Timestamp time.Time `json:"timestamp"`
	Alert     Alert     `json:"payload"`
}
