package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Nechja/Aegis/apps/glist/internal/ai"
	"github.com/Nechja/Aegis/apps/glist/internal/server"
	"github.com/Nechja/Aegis/apps/glist/internal/store"
)

func main() {
	pgURL := os.Getenv("PG_URL")
	if pgURL == "" {
		log.Fatal("PG_URL not set")
	}
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	st, err := store.Open(ctx, pgURL)
	if err != nil {
		log.Fatalf("store: %v", err)
	}
	defer st.Close()

	if err := st.Migrate(ctx); err != nil {
		log.Fatalf("migrate: %v", err)
	}

	ol := ai.NewOllama(os.Getenv("OLLAMA_URL"), os.Getenv("OLLAMA_MODEL"))
	if ol != nil {
		log.Printf("ollama: %s model=%s", ol.URL, ol.Model)
	} else {
		log.Printf("ollama: disabled (OLLAMA_URL or OLLAMA_MODEL not set)")
	}

	// Cheer pool is seeded with hardcoded fallbacks. Pool refills happen
	// lazily as Pick drains the pool — no boot-time prewarm (it created
	// Ollama contention that slowed user-triggered SSE streams).
	cheer := ai.NewCheer(ol)

	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           server.New(st, ol, cheer),
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		log.Printf("glist listening on :%s", port)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("serve: %v", err)
		}
	}()

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
}
