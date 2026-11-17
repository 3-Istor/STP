package snapshotstore

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"bomstudio/internal/bom"
	"bomstudio/internal/snapshot/snapshotdb"
)

// Profil de véhicule figé (ADR 0036, migration 00014).

// insertProfile copie le profil dans la transaction de la publication ; nil : rien à figer.
func insertProfile(ctx context.Context, q *snapshotdb.Queries, id int64, p *bom.VehicleProfile) error {
	if p == nil {
		return nil
	}
	w := p.Weights
	if err := q.InsertSnapshotVehicleProfile(ctx, snapshotdb.InsertSnapshotVehicleProfileParams{
		SnapshotID: id, Preset: string(p.Preset), WeightCost: int16(w.Cost), WeightCo2: int16(w.CO2), // #nosec G115 -- 0..100 validé
		WeightLeadTime: int16(w.LeadTime), WeightMass: int16(w.Mass), WeightQuality: int16(w.Quality), // #nosec G115 -- 0..100 validé
	}); err != nil {
		return fmt.Errorf("vehicle profile: %w", err)
	}
	return nil
}

// profile relit le profil figé ; nil si le snapshot n'en a pas.
func (s *Store) profile(ctx context.Context, id int64) (*bom.VehicleProfile, error) {
	r, err := s.q.GetSnapshotVehicleProfile(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("vehicle profile: %w", err)
	}
	return &bom.VehicleProfile{Preset: bom.ProfilePreset(r.Preset), Weights: bom.Weights{
		Cost: int(r.WeightCost), CO2: int(r.WeightCo2), LeadTime: int(r.WeightLeadTime), Mass: int(r.WeightMass), Quality: int(r.WeightQuality),
	}}, nil
}
