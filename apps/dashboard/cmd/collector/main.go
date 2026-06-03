package main

import (
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Nechja/Aegis/apps/dashboard/internal/cache"
	"github.com/Nechja/Aegis/apps/dashboard/internal/eventpoller"
	"github.com/Nechja/Aegis/apps/dashboard/internal/informers"
	"github.com/Nechja/Aegis/apps/dashboard/internal/state"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

func main() {
	slog.Info("starting aegis collector")

	domain := envOr("TAILSCALE_DOMAIN", "example.ts.net")
	healthAddr := envOr("HEALTH_ADDR", ":8081")
	redisSentinel := envOr("REDIS_SENTINEL_ADDR", "rfs-aegis.redis.svc.cluster.local:26379")
	redisMaster := envOr("REDIS_MASTER_NAME", "mymaster")

	client, err := buildClient()
	if err != nil {
		slog.Error("failed to create k8s client", "error", err)
		os.Exit(1)
	}

	store := state.NewStore()

	rc := cache.New("", redisSentinel, redisMaster)
	defer rc.Close()
	store.OnChange(func() { rc.Save(store) })

	inf := informers.Start(client, store, domain)
	defer inf.Stop()

	ep := eventpoller.Start(client, store, 60*time.Second)
	defer ep.Stop()

	// Health endpoint — reports unhealthy if no data update in 5 minutes
	go func() {
		mux := http.NewServeMux()
		mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
			if store.IsHealthy(5 * time.Minute) {
				w.WriteHeader(http.StatusOK)
				fmt.Fprintf(w, "ok (last update %s ago)", time.Since(store.LastUpdate()).Round(time.Second))
			} else {
				w.WriteHeader(http.StatusServiceUnavailable)
				if store.LastUpdate().IsZero() {
					fmt.Fprint(w, "no data yet")
				} else {
					fmt.Fprintf(w, "stale (last update %s ago)", time.Since(store.LastUpdate()).Round(time.Second))
				}
			}
		})
		slog.Info("health endpoint listening", "addr", healthAddr)
		http.ListenAndServe(healthAddr, mux)
	}()

	slog.Info("collector running")

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	slog.Info("collector shutting down")
}

func buildClient() (kubernetes.Interface, error) {
	cfg, err := rest.InClusterConfig()
	if err != nil {
		kubeconfig := envOr("KUBECONFIG", os.Getenv("HOME")+"/.kube/config")
		cfg, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
		if err != nil {
			return nil, err
		}
	}
	return kubernetes.NewForConfig(cfg)
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
