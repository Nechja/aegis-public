package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Nechja/Aegis/apps/dashboard/internal/cache"
	"github.com/Nechja/Aegis/apps/dashboard/internal/prompoller"
	"github.com/Nechja/Aegis/apps/dashboard/internal/server"
	"github.com/Nechja/Aegis/apps/dashboard/internal/state"
)

func main() {
	slog.Info("starting aegis dashboard")

	addr := envOr("LISTEN_ADDR", ":8080")
	promURL := envOr("PROMETHEUS_URL", "http://prometheus-k8s.monitoring.svc.cluster.local:9090")
	redisSentinel := envOr("REDIS_SENTINEL_ADDR", "rfs-aegis.redis.svc.cluster.local:26379")
	redisMaster := envOr("REDIS_MASTER_NAME", "mymaster")

	store := state.NewStore()

	// Redis — warm start from collector's cached data
	rc := cache.New("", redisSentinel, redisMaster)
	defer rc.Close()
	rc.Warm(store)

	// Subscribe to collector updates via Redis pub/sub — no polling
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go rc.Subscribe(ctx, store)

	// Prometheus poller (workloads + node metrics)
	pp := prompoller.Start(promURL, store, 30*time.Second)
	defer pp.Stop()

	// HTTP + WebSocket server
	srv := &http.Server{
		Addr:    addr,
		Handler: server.New(store),
	}

	go func() {
		slog.Info("listening", "addr", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("shutting down")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	srv.Shutdown(shutdownCtx)
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
