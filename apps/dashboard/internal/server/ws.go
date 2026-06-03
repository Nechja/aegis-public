package server

import (
	"context"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"nhooyr.io/websocket"
)

type wsClient struct {
	conn *websocket.Conn
	send chan []byte
}

type wsHub struct {
	mu      sync.Mutex
	clients map[*wsClient]struct{}
	getFn   func() []byte
}

func newWSHub(getFn func() []byte) *wsHub {
	return &wsHub{
		clients: make(map[*wsClient]struct{}),
		getFn:   getFn,
	}
}

func (h *wsHub) add(c *wsClient) {
	h.mu.Lock()
	h.clients[c] = struct{}{}
	h.mu.Unlock()
}

func (h *wsHub) remove(c *wsClient) {
	h.mu.Lock()
	delete(h.clients, c)
	h.mu.Unlock()
}

// signal broadcasts current state to all clients.
// Called from watcher's onChange — runs in the watcher's goroutine,
// so we serialize once and fan out via each client's send channel.
func (h *wsHub) signal() {
	data := h.getFn()

	h.mu.Lock()
	for c := range h.clients {
		// non-blocking send — drop if client is backed up
		select {
		case c.send <- data:
		default:
		}
	}
	h.mu.Unlock()
}

func (h *wsHub) handleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		slog.Warn("ws accept failed", "error", err)
		return
	}

	client := &wsClient{
		conn: conn,
		send: make(chan []byte, 2),
	}
	h.add(client)
	slog.Info("ws client connected", "remote", r.RemoteAddr)

	// send current state immediately
	client.send <- h.getFn()

	// writer goroutine — drains send channel to the websocket
	done := make(chan struct{})
	go func() {
		defer close(done)
		for data := range client.send {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			err := conn.Write(ctx, websocket.MessageText, data)
			cancel()
			if err != nil {
				return
			}
		}
	}()

	// read loop to detect disconnection
	for {
		_, _, err := conn.Read(context.Background())
		if err != nil {
			break
		}
	}

	h.remove(client)
	close(client.send)
	<-done
	conn.Close(websocket.StatusNormalClosure, "")
	slog.Info("ws client disconnected", "remote", r.RemoteAddr)
}

// signalDebounced returns a function that coalesces rapid calls,
// invoking signal() at most once per interval.
func (h *wsHub) signalDebounced(interval time.Duration) func() {
	var mu sync.Mutex
	var timer *time.Timer

	return func() {
		mu.Lock()
		defer mu.Unlock()
		if timer != nil {
			timer.Stop()
		}
		timer = time.AfterFunc(interval, h.signal)
	}
}
