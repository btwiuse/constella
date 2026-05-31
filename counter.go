package constella

import (
	"context"
	"encoding/json"
	"log/slog"
	"maps"
	"sync"
	"time"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/peer"
)

const (
	counterTopic = "/constella/counter/0.1.0"
)

type Counter struct {
	mu       sync.RWMutex         `json:"-"`
	Counts   map[string]int64     `json:"counts"`
	SelfID   string               `json:"self"`
	Sum      int64                `json:"sum,omitempty"`
	topic    *pubsub.Topic        `json:"-"`
	subs     *pubsub.Subscription `json:"-"`
	OnUpdate func()               `json:"-"`
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
		Counts: map[string]int64{self.String(): 0},
		SelfID: self.String(),
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

		var msgCounts Counter
		if err := json.Unmarshal(msg.Data, &msgCounts); err != nil {
			slog.Warn("[counter] unmarshal error", "error", err)
			continue
		}

		// skip our own broadcasts
		if msg.ReceivedFrom.String() == c.SelfID {
			continue
		}

		changed := c.merge(msgCounts.Counts)

		if !changed {
			continue
		}

		slog.Info("[counter] merged from pubsub",
			"from", msg.ReceivedFrom,
			"sum", sumCounts(c.Counts),
		)

		c.Broadcast()

		if c.OnUpdate != nil {
			c.OnUpdate()
		}
	}
}

func (c *Counter) Increment() {
	c.mu.Lock()
	c.Counts[c.SelfID]++
	val := c.Counts[c.SelfID]
	c.mu.Unlock()

	slog.Info("[counter] increment", "self", c.SelfID, "newCount", val)

	c.Broadcast()

	if c.OnUpdate != nil {
		c.OnUpdate()
	}
}

func (c *Counter) Snapshot() *Counter {
	c.mu.RLock()
	defer c.mu.RUnlock()

	snap := make(map[string]int64, len(c.Counts))
	maps.Copy(snap, c.Counts)
	return &Counter{
		Counts: snap,
		SelfID: c.SelfID,
		Sum:    sumCounts(c.Counts),
	}
}

func sumCounts(counts map[string]int64) int64 {
	var total int64
	for _, v := range counts {
		total += v
	}
	return total
}

func (c *Counter) merge(counts map[string]int64) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	changed := false
	for pid, count := range counts {
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
	data, err := json.Marshal(c)
	sum := sumCounts(c.Counts)
	c.mu.RUnlock()

	if err != nil {
		slog.Warn("[counter] broadcast marshal error", "error", err)
		return
	}

	c.topic.Publish(context.Background(), data)

	slog.Info("[counter] broadcast", "sum", sum)
}
