// Package cache provides a Redis-backed cache with pub/sub notifications.
// The collector writes state + publishes. The dashboard subscribes + reads on notification.
package cache

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/Nechja/Aegis/apps/dashboard/internal/state"
)

const (
	stateKey   = "aegis:dashboard:state"
	stateTTL   = 5 * time.Minute
	channelKey = "aegis:dashboard:updated"
)

type snapshot struct {
	Services    []state.Service       `json:"services"`
	Nodes       []state.NodeInfo      `json:"nodes"`
	Workloads   []state.Workload      `json:"workloads"`
	Events      []state.ClusterEvent  `json:"events"`
	NetPolicies []state.NetPolicyRule `json:"netPolicies"`
}

type Cache struct {
	rdb redis.UniversalClient
}

func New(addr, sentinelAddr, masterName string) *Cache {
	var rdb redis.UniversalClient

	if sentinelAddr != "" && masterName != "" {
		rdb = redis.NewFailoverClient(&redis.FailoverOptions{
			MasterName:    masterName,
			SentinelAddrs: []string{sentinelAddr},
			DialTimeout:   2 * time.Second,
			ReadTimeout:   2 * time.Second,
		})
		slog.Info("redis cache using sentinel", "sentinel", sentinelAddr, "master", masterName)
	} else {
		rdb = redis.NewClient(&redis.Options{
			Addr:        addr,
			DialTimeout: 2 * time.Second,
			ReadTimeout: 2 * time.Second,
		})
		slog.Info("redis cache direct", "addr", addr)
	}

	return &Cache{rdb: rdb}
}

// Warm loads cached state into the store. Only loads collector-owned fields.
func (c *Cache) Warm(store *state.Store) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	data, err := c.rdb.Get(ctx, stateKey).Bytes()
	if err != nil {
		slog.Debug("redis cache miss", "error", err)
		return false
	}

	var snap snapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		slog.Warn("redis cache unmarshal failed", "error", err)
		return false
	}

	store.SetCollectorData(snap.Services, snap.Nodes, snap.Events)
	if len(snap.NetPolicies) > 0 {
		store.SetNetPolicies(snap.NetPolicies)
	}

	slog.Debug("loaded from redis cache",
		"services", len(snap.Services),
		"nodes", len(snap.Nodes),
		"events", len(snap.Events),
	)
	return true
}

// Save writes the current state to Redis and publishes a notification.
func (c *Cache) Save(store *state.Store) {
	snap := snapshot{
		Services:    store.Services(),
		Nodes:       store.Nodes(),
		Workloads:   store.Workloads(),
		Events:      store.Events(),
		NetPolicies: store.NetPolicies(),
	}

	data, err := json.Marshal(snap)
	if err != nil {
		slog.Warn("redis cache marshal failed", "error", err)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := c.rdb.Set(ctx, stateKey, data, stateTTL).Err(); err != nil {
		slog.Warn("redis cache write failed", "error", err)
		return
	}

	// Notify subscribers that state has been updated
	c.rdb.Publish(ctx, channelKey, "updated")
}

// Subscribe listens for collector updates via pub/sub.
// Calls onUpdate when the collector writes new data.
// Blocks until ctx is cancelled.
func (c *Cache) Subscribe(ctx context.Context, store *state.Store) {
	pubsub := c.rdb.Subscribe(ctx, channelKey)
	defer pubsub.Close()

	slog.Info("subscribed to redis updates")

	ch := pubsub.Channel()
	for {
		select {
		case <-ctx.Done():
			return
		case _, ok := <-ch:
			if !ok {
				slog.Warn("redis subscription closed, reconnecting")
				time.Sleep(2 * time.Second)
				pubsub = c.rdb.Subscribe(ctx, channelKey)
				ch = pubsub.Channel()
				continue
			}
			c.Warm(store)
		}
	}
}

func (c *Cache) Close() {
	c.rdb.Close()
}
