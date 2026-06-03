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

	"github.com/Nechja/Aegis/apps/jot/internal/server"
	"github.com/Nechja/Aegis/apps/jot/internal/store"
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

	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           server.New(st),
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		log.Printf("jot listening on :%s", port)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("serve: %v", err)
		}
	}()

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
}
