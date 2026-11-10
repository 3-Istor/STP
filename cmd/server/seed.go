package main

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"bomstudio/internal/pricesim"
	"bomstudio/internal/pricing"
	"bomstudio/internal/pricing/pricingstore"
	"bomstudio/internal/sampledata"
	"bomstudio/internal/sampledata/seedstore"
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
	return nil
}
