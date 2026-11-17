// Command server démarre l'API BOM Studio. `server seed` charge le jeu sample-dataset puis s'arrête
// (voir seed.go, ADR 0012 et 0021).
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

	"bomstudio/internal/bom"
	"bomstudio/internal/bom/bomstore"
	"bomstudio/internal/catalog"
	"bomstudio/internal/catalog/catalogstore"
	"bomstudio/internal/httpapi"
	"bomstudio/internal/pricing"
	"bomstudio/internal/pricing/pricingstore"
	"bomstudio/internal/snapshot"
	"bomstudio/internal/snapshot/snapshotstore"
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

	// Voir ADR 0009 : seule la composition importe l'adapter de stockage PostgreSQL.
	pool, err := pgxpool.New(ctx, envOr("DATABASE_URL", "postgres://bom:bom@localhost:5432/bomstudio?sslmode=disable"))
	if err != nil {
		return fmt.Errorf("configure database: %w", err)
	}
	defer pool.Close()
	if len(os.Args) > 1 && os.Args[1] == "seed" {
		return runSeed(ctx, pool)
	}
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

	// BOM : vrai catalogue et prix de live-price-feed (adapter pricing.BOMPricing, voir ADR 0020).
	// Sans PRICE_API_URL, aucun prix : la BOM tourne sur un stub vide (figer et publier échouent, nommément).
	var prices pricing.Pricing
	if reads != nil && snapshots != nil {
		prices = pricing.NewBOMPricing(reads, snapshots)
	} else {
		prices = pricing.NewStub(nil, nil)
		slog.Warn("PRICE_API_URL is not set: BOM prices are an empty in-memory stub")
	}
	logistics := pricing.NewLogisticsService(pricingstore.NewLogistics(pool))
	boms, bomSnapshots := domainServices(pool, catalogService, supplierService, prices)

	handler := httpapi.NewRouter(httpapi.Services{
		Catalog: catalogService, Supplier: supplierService, History: historyService,
		Pricing: reads, PriceSnapshots: snapshots, PricingSignals: signals,
		BOMs: boms, Snapshots: bomSnapshots, Logistics: logistics,
	})
	addr := ":" + envOr("PORT", "8080")
	srv := &http.Server{
		Addr:              addr,
		BaseContext:       func(net.Listener) context.Context { return ctx },
		Handler:           handler,
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

// domainServices câble les domaines bom et snapshot sur la base et une source de prix (serveur et seed).
func domainServices(pool *pgxpool.Pool, cat *catalog.Service, sup *supplier.Service, prices pricing.Pricing) (*bom.Service, *snapshot.Service) {
	logisticsStore := pricingstore.NewLogistics(pool)
	boms := bom.NewService(bomstore.New(pool), cat, prices).
		WithCatalogListing(cat).WithLogistics(pricing.NewLogisticsService(logisticsStore).Store()).
		WithCostSources(logisticsStore, sup).            // méthode de calcul du coût, ADR 0031
		WithIdeas(bomstore.New(pool)).                   // pistes de réduction
		WithProfiles(bomstore.New(pool)).                // profils de véhicule, ADR 0036
		WithAnalysisSources(pricingstore.New(pool), sup) // analyses du lot E (historique, fournisseurs)
	snaps := snapshot.NewService(snapshotstore.New(pool), boms, cat, prices).
		WithSupplierNames(sup) // explications de la comparaison, ADR 0032
	return boms, snaps
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
