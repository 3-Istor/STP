package seedstore

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"bomstudio/internal/sampledata"
)

// DemoSteps enregistre les étapes de démonstration d'un dataset_version (table seed_demo_steps, insertion seule).
type DemoSteps struct {
	DB      DB
	Version string
}

var _ sampledata.DemoSteps = DemoSteps{}

// EntityIDs : clé logique → id pour un type d'entité du jeu chargé.
func (d DemoSteps) EntityIDs(ctx context.Context, kind string) (map[string]int64, error) {
	tx, err := d.DB.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()
	rows, err := tx.Query(ctx, `SELECT logical_key, entity_id FROM seed_entities WHERE dataset_version = $1 AND entity_kind = $2`,
		d.Version, kind)
	if err != nil {
		return nil, fmt.Errorf("seed %s %s ids: %w", d.Version, kind, err)
	}
	out := map[string]int64{}
	var key string
	var id int64
	if _, err := pgx.ForEachRow(rows, []any{&key, &id}, func() error {
		out[key] = id
		return nil
	}); err != nil {
		return nil, fmt.Errorf("seed %s %s ids: %w", d.Version, kind, err)
	}
	return out, nil
}

// Step dit si l'étape est déjà appliquée, et avec quel id.
func (d DemoSteps) Step(ctx context.Context, step string) (int64, bool, error) {
	tx, err := d.DB.Begin(ctx)
	if err != nil {
		return 0, false, err
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()
	var id int64
	err = tx.QueryRow(ctx, `SELECT entity_id FROM seed_demo_steps WHERE dataset_version = $1 AND step = $2`, d.Version, step).Scan(&id)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return 0, false, nil
	case err != nil:
		return 0, false, fmt.Errorf("seed %s step %s: %w", d.Version, step, err)
	}
	return id, true, nil
}

// RecordStep enregistre l'étape (une seule fois : clé primaire).
func (d DemoSteps) RecordStep(ctx context.Context, step string, id int64) error {
	tx, err := d.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()
	if _, err := tx.Exec(ctx, `INSERT INTO seed_demo_steps (dataset_version, step, entity_id) VALUES ($1, $2, $3)`, d.Version, step, id); err != nil {
		return fmt.Errorf("seed %s record step %s: %w", d.Version, step, err)
	}
	return tx.Commit(ctx)
}
