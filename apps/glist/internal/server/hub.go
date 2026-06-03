package server

import "sync"

// Hub is a tiny per-list pub/sub. Subscribers receive an empty signal every
// time Publish(listID) is called; they're expected to refetch the list state
// themselves. Capacity 4 buffer per subscriber so a burst of mutations
// collapses into a single notify.
type Hub struct {
	mu   sync.Mutex
	subs map[int64]map[chan struct{}]struct{}
}

func NewHub() *Hub {
	return &Hub{subs: map[int64]map[chan struct{}]struct{}{}}
}

// Subscribe returns a receive-only channel plus a cancel func the caller must
// invoke on disconnect.
func (h *Hub) Subscribe(listID int64) (<-chan struct{}, func()) {
	ch := make(chan struct{}, 4)
	h.mu.Lock()
	m, ok := h.subs[listID]
	if !ok {
		m = map[chan struct{}]struct{}{}
		h.subs[listID] = m
	}
	m[ch] = struct{}{}
	h.mu.Unlock()
	return ch, func() {
		h.mu.Lock()
		if m, ok := h.subs[listID]; ok {
			delete(m, ch)
			if len(m) == 0 {
				delete(h.subs, listID)
			}
		}
		h.mu.Unlock()
		// channel intentionally not closed: a slow publisher racing a close
		// would panic. GC reaps once no one holds a ref.
	}
}

// Publish wakes every subscriber for listID. Non-blocking: if a subscriber
// is already pending (buffer full), the signal is dropped — the subscriber
// will still receive the next mutation.
func (h *Hub) Publish(listID int64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.subs[listID] {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}
