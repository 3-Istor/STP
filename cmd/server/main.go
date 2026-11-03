// Command server démarre l'API BOM Studio.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"bomstudio/internal/bom"
	"bomstudio/internal/bom/bomstore"
	"bomstudio/internal/catalog"
	"bomstudio/internal/httpapi"
	"bomstudio/internal/pricing"
	"bomstudio/internal/snapshot"
	"bomstudio/internal/snapshot/snapshotstore"
)

func main() {
	if err := run(); err != nil {
		slog.Error("server stopped", "err", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		return errors.New("DATABASE_URL is not set")
	}
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer pool.Close()
	pingCtx, cancelPing := context.WithTimeout(ctx, 5*time.Second)
	defer cancelPing()
	if err := pool.Ping(pingCtx); err != nil {
		return fmt.Errorf("ping database: %w", err)
	}

	// Catalogue et prix : stubs vides en attendant leurs implémentations Postgres (NLPF-67, NLPF-68).
	// D'ici là, seuls les sous-ensembles s'ajoutent (une offre est inconnue du stub) et figer ou publier
	// une BOM échoue (le snapshot de prix du stub n'existe pas en base).
	cat, err := catalog.NewStub(nil, nil)
	if err != nil {
		return fmt.Errorf("catalog: %w", err)
	}
	prices := pricing.NewStub(nil, nil)
	slog.Warn("catalog and pricing are empty in-memory stubs until NLPF-67 and NLPF-68")

	boms := bom.NewService(bomstore.New(pool), cat, prices)
	snapshots := snapshot.NewService(snapshotstore.New(pool), boms, cat, prices)

	addr := ":" + envOr("PORT", "8080")
	srv := &http.Server{
		Addr:              addr,
		Handler:           httpapi.NewRouter(httpapi.Deps{BOMs: boms, Snapshots: snapshots}),
		ReadHeaderTimeout: 5 * time.Second,
	}

	errc := make(chan error, 1)
	go func() {
		slog.Info("listening", "addr", addr)
		errc <- srv.ListenAndServe()
	}()

	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return err
	}
	if err := <-errc; !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
