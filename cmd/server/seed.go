package main

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"bomstudio/internal/catalog"
	"bomstudio/internal/catalog/catalogstore"
	"bomstudio/internal/pricesim"
	"bomstudio/internal/pricing"
	"bomstudio/internal/pricing/pricingstore"
	"bomstudio/internal/sampledata"
	"bomstudio/internal/sampledata/seedstore"
	"bomstudio/internal/supplier"
	"bomstudio/internal/supplier/supplierstore"
)

// runSeed charge le jeu sample-dataset (ADR 0012, 0021) avec les mêmes paramètres SIM_* que le worker de
// prix : l'historique initial des offres est produit en process par le simulateur partagé (pricesim, le
// même que l'API de prix) et inséré dans la transaction du seed. Idempotent : relancer ne change rien.
func runSeed(ctx context.Context, pool *pgxpool.Pool) error {
	cfg, _, _, err := pricingConfigFromEnv()
	if err != nil {
		return fmt.Errorf("seed: %w", err)
	}
	ds, err := sampledata.Generate(cfg.Seed)
	if err != nil {
		return fmt.Errorf("seed: %w", err)
	}
	slog.Info("seed: loading sample dataset", "dataset_version", cfg.DatasetVersion, "seed", cfg.Seed,
		"as_of", cfg.InitialAsOf, "parts", len(ds.Parts), "offers", len(ds.Offers), "bom_nodes", ds.NodeCount())
	res, err := seedstore.Load(ctx, pool, ds,
		seedstore.Params{DatasetVersion: cfg.DatasetVersion, Seed: cfg.Seed, AsOf: cfg.InitialAsOf},
		func(ctx context.Context, tx pgx.Tx, offerIDs []int64) error {
			_, _, err := pricing.NewRuntime(pricesim.Simulator{}, pricingstore.NewTransactionalRuntime(tx, offerIDs), nil).Initialize(ctx, cfg)
			return err
		})
	if err != nil {
		return err
	}
	slog.Info("seed: done", "dataset_version", cfg.DatasetVersion, "created", res.Created, "offers", len(res.OfferIDs))
	return seedDemoVersions(ctx, pool, cfg, ds)
}

// seedDemoVersions applique les versions de démonstration (V1 publiée, brouillon B, pistes) par les services
// du domaine, avec les prix figés par la source de prix déjà initialisée. La publication est datée comme une
// publication par l'API (heure réelle, à la seconde), comme la création des brouillons.
func seedDemoVersions(ctx context.Context, pool *pgxpool.Pool, cfg pricing.RunConfig, ds sampledata.Dataset) error {
	plan, err := sampledata.Demo(ds)
	if err != nil {
		return fmt.Errorf("seed demo versions: %w", err)
	}
	prices := pricing.NewBOMPricing(pricing.NewReadService(pricingstore.NewReadStore(pool), cfg),
		pricing.NewSnapshotService(pricingstore.NewSnapshotStore(pool), cfg))
	cat := catalog.NewService(catalogstore.New(pool), time.Now)
	boms, snaps := domainServices(pool, cat, supplier.NewService(supplierstore.New(pool)), prices)
	res, err := sampledata.ApplyDemo(ctx, seedstore.DemoSteps{DB: pool, Version: cfg.DatasetVersion}, boms, snaps, plan, time.Now().UTC().Truncate(time.Second))
	if err != nil {
		return fmt.Errorf("seed demo versions: %w", err)
	}
	slog.Info("seed: demo versions", "snapshot_v1", res.SnapshotID, "draft_b", res.DraftID, "created", res.Created)
	return nil
}
