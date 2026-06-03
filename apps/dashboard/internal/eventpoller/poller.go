// Package eventpoller periodically polls k8s for warning events.
// Uses a simple list call with field selector, not an informer.
package eventpoller

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/Nechja/Aegis/apps/dashboard/internal/state"
)

type Poller struct {
	client kubernetes.Interface
	store  *state.Store
	stop   chan struct{}
}

func Start(client kubernetes.Interface, store *state.Store, interval time.Duration) *Poller {
	p := &Poller{
		client: client,
		store:  store,
		stop:   make(chan struct{}),
	}
	go p.run(interval)
	slog.Info("event poller started", "interval", interval)
	return p
}

func (p *Poller) Stop() { close(p.stop) }

func (p *Poller) run(interval time.Duration) {
	p.poll()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			p.poll()
		case <-p.stop:
			return
		}
	}
}

func (p *Poller) poll() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	list, err := p.client.CoreV1().Events("").List(ctx, metav1.ListOptions{
		FieldSelector: "type=Warning",
		Limit:         50,
	})
	if err != nil {
		slog.Warn("event poll failed", "error", err)
		return
	}

	var events []state.ClusterEvent
	for _, ev := range list.Items {
		ts := ev.LastTimestamp.Time
		if ts.IsZero() {
			ts = ev.EventTime.Time
		}
		if ts.IsZero() {
			ts = ev.CreationTimestamp.Time
		}

		events = append(events, state.ClusterEvent{
			Timestamp: ts,
			Type:      ev.Type,
			Reason:    ev.Reason,
			Object:    fmt.Sprintf("%s/%s", ev.InvolvedObject.Kind, ev.InvolvedObject.Name),
			Namespace: ev.Namespace,
			Message:   ev.Message,
		})
	}

	sort.Slice(events, func(i, j int) bool {
		return events[i].Timestamp.After(events[j].Timestamp)
	})

	p.store.SetEvents(events)
	slog.Debug("events refreshed", "count", len(events))
}
