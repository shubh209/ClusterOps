package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// SSEBroker manages a set of connected SSE clients and broadcasts events to all of them.
// Events are fire-and-forget: slow clients are dropped after a 100ms send timeout.
type SSEBroker struct {
	mu      sync.RWMutex
	clients map[chan []byte]struct{}
}

// newSSEBroker creates a ready broker.
func newSSEBroker() *SSEBroker {
	return &SSEBroker{
		clients: make(map[chan []byte]struct{}),
	}
}

// subscribe registers a new client channel and returns it.
func (b *SSEBroker) subscribe() chan []byte {
	ch := make(chan []byte, 16)
	b.mu.Lock()
	b.clients[ch] = struct{}{}
	b.mu.Unlock()
	sseClientsGauge.Inc()
	return ch
}

// unsubscribe removes and closes a client channel.
func (b *SSEBroker) unsubscribe(ch chan []byte) {
	b.mu.Lock()
	delete(b.clients, ch)
	b.mu.Unlock()
	close(ch)
	sseClientsGauge.Dec()
}

// broadcast sends data to all connected clients.
// Clients that can't receive within 100ms are dropped.
func (b *SSEBroker) broadcast(data []byte) {
	b.mu.RLock()
	snapshot := make([]chan []byte, 0, len(b.clients))
	for ch := range b.clients {
		snapshot = append(snapshot, ch)
	}
	b.mu.RUnlock()

	for _, ch := range snapshot {
		select {
		case ch <- data:
		case <-time.After(100 * time.Millisecond):
			// Slow client — drop it.
			b.unsubscribe(ch)
		}
	}
}

// BroadcastEvent serialises val as an SSE event with the given name and broadcasts it.
func (b *SSEBroker) BroadcastEvent(event string, val interface{}) {
	payload, err := json.Marshal(val)
	if err != nil {
		return
	}
	// SSE wire format: "event: <name>\ndata: <json>\n\n"
	msg := fmt.Sprintf("event: %s\ndata: %s\n\n", event, payload)
	b.broadcast([]byte(msg))
}

// ServeHTTP upgrades the connection to an SSE stream.
// Clients receive a heartbeat every 15s to prevent proxy timeouts.
func (b *SSEBroker) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// SSE requires these headers.
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // disable nginx buffering

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "SSE not supported", http.StatusInternalServerError)
		return
	}

	ch := b.subscribe()
	defer b.unsubscribe(ch)

	// Send an initial connected event.
	fmt.Fprintf(w, "event: connected\ndata: {}\n\n")
	flusher.Flush()

	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case msg, ok := <-ch:
			if !ok {
				return
			}
			fmt.Fprint(w, string(msg))
			flusher.Flush()
		case <-heartbeat.C:
			fmt.Fprintf(w, ": heartbeat\n\n")
			flusher.Flush()
		}
	}
}
