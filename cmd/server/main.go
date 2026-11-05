// Command server démarre l'API BOM Studio.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"bomstudio/internal/catalog"
	"bomstudio/internal/catalog/catalogstore"
	"bomstudio/internal/httpapi"
	"bomstudio/internal/pricing"
	"bomstudio/internal/pricing/pricingstore"
	"bomstudio/internal/supplier"
	"bomstudio/internal/supplier/supplierstore"
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

	// Voir ADR 0006 : seule la composition importe l'adapter de stockage PostgreSQL.
	pool, err := pgxpool.New(ctx, envOr("DATABASE_URL", "postgres://bom:bom@localhost:5432/bomstudio?sslmode=disable"))
	if err != nil {
		return fmt.Errorf("configure database: %w", err)
	}
	defer pool.Close()
	worker, err := configurePricing(pool)
	if err != nil {
		return fmt.Errorf("configure pricing source: %w", err)
	}
	if worker != nil {
		defer worker.signals.Close()
		workerCtx, cancelWorker := context.WithCancel(ctx)
		workerDone := make(chan struct{})
		go func() { defer close(workerDone); worker.run(workerCtx) }()
		// Arrêter et attendre le worker avant pool.Close, même si ListenAndServe échoue.
		defer func() { cancelWorker(); <-workerDone }()
	}
	catalogService := catalog.NewService(catalogstore.New(pool), time.Now)
	supplierService := supplier.NewService(supplierstore.New(pool))
	historyService := pricing.NewHistoryService(pricingstore.New(pool))
	var reads *pricing.ReadService
	var snapshots *pricing.SnapshotService
	var signals *httpapi.PricingHub
	if worker != nil {
		reads = worker.reads
		snapshots = worker.snapshots
		signals = worker.signals
	}

	addr := ":" + envOr("PORT", "8080")
	srv := &http.Server{
		Addr:              addr,
		BaseContext:       func(net.Listener) context.Context { return ctx },
		Handler:           httpapi.NewRouter(httpapi.Services{Catalog: catalogService, Supplier: supplierService, History: historyService, Pricing: reads, PriceSnapshots: snapshots, PricingSignals: signals}),
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
