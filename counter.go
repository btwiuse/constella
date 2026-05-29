package constella

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/peer"
)

const (
	counterTopic = "/constella/counter/0.1.0"
)

type Counter struct {
	mu     sync.RWMutex
	Counts map[peer.ID]int64 `json:"counts"`
	self   peer.ID
	topic  *pubsub.Topic
	subs   *pubsub.Subscription
}

func NewCounter(self peer.ID, ps *pubsub.PubSub) (*Counter, error) {
	topic, err := ps.Join(counterTopic)
	if err != nil {
		return nil, err
	}

	subs, err := topic.Subscribe()
	if err != nil {
		return nil, err
	}

	c := &Counter{
		Counts: map[peer.ID]int64{self: 0},
		self:   self,
		topic:  topic,
		subs:   subs,
	}

	slog.Info("[counter] initialized", "self", self, "topic", counterTopic)

	go c.subscribeLoop()

	return c, nil
}

func (c *Counter) subscribeLoop() {
	for {
		msg, err := c.subs.Next(context.Background())
		if err != nil {
			slog.Warn("[counter] subscription error", "error", err)
			time.Sleep(time.Second)
			continue
		}

		var payload struct {
			Counts map[string]int64 `json:"counts"`
		}
		if err := json.Unmarshal(msg.Data, &payload); err != nil {
			slog.Warn("[counter] unmarshal error", "error", err)
			continue
		}

		// skip our own broadcasts
		if msg.ReceivedFrom == c.self {
			continue
		}

		changed := c.merge(payload.Counts)

		if !changed {
			continue
		}

		slog.Info("[counter] merged from pubsub",
			"from", msg.ReceivedFrom,
			"value", c.Value(),
			"peerCount", len(payload.Counts),
		)
	}
}

func (c *Counter) Increment() {
	c.mu.Lock()
	c.Counts[c.self]++
	val := c.Counts[c.self]
	c.mu.Unlock()

	slog.Info("[counter] increment", "self", c.self, "newCount", val)

	c.Broadcast()
}

func (c *Counter) Value() int64 {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var total int64
	for _, v := range c.Counts {
		total += v
	}
	return total
}

func (c *Counter) Snapshot() map[string]int64 {
	c.mu.RLock()
	defer c.mu.RUnlock()

	snap := make(map[string]int64, len(c.Counts))
	for k, v := range c.Counts {
		snap[k.String()] = v
	}
	return snap
}

func (c *Counter) merge(counts map[string]int64) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	changed := false
	for pidStr, count := range counts {
		pid, err := peer.Decode(pidStr)
		if err != nil {
			continue
		}
		existing, ok := c.Counts[pid]
		if !ok || count > existing {
			c.Counts[pid] = count
			changed = true
		}
	}
	return changed
}

func (c *Counter) Broadcast() {
	c.mu.RLock()
	snap := make(map[string]int64, len(c.Counts))
	for k, v := range c.Counts {
		snap[k.String()] = v
	}
	data, err := json.Marshal(struct {
		Counts map[string]int64 `json:"counts"`
	}{Counts: snap})
	c.mu.RUnlock()

	if err != nil {
		slog.Warn("[counter] broadcast marshal error", "error", err)
		return
	}

	c.topic.Publish(context.Background(), data)

	slog.Info("[counter] broadcast", "value", c.Value(), "peers", len(snap))
}
