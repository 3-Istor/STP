package bomstore

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"bomstudio/internal/bom"
	"bomstudio/internal/bom/bomdb"
)

// Profils de véhicule (ADR 0036, migration 00014).

const fkProfileBOM = "bom_vehicle_profiles_bom_id_fkey"

var _ bom.ProfileStore = (*Store)(nil)

// VehicleProfile renvoie le profil de la BOM, nil si elle n'en a pas ; bom.ErrBOMNotFound si elle n'existe pas.
func (s *Store) VehicleProfile(ctx context.Context, bomID int64) (*bom.VehicleProfile, error) {
	row, err := s.q.GetVehicleProfile(ctx, bomID)
	if errors.Is(err, pgx.ErrNoRows) {
		if _, err := s.GetBOM(ctx, bomID); err != nil {
			return nil, err
		}
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("profile of bom %d: %w", bomID, err)
	}
	p := toProfile(bomdb.UpsertVehicleProfileRow(row))
	return &p, nil
}

// SetVehicleProfile enregistre (ou remplace) le profil ; la base vérifie aussi poids et somme (CHECK).
func (s *Store) SetVehicleProfile(ctx context.Context, bomID int64, p bom.VehicleProfile) (bom.VehicleProfile, error) {
	w := p.Weights
	row, err := s.q.UpsertVehicleProfile(ctx, bomdb.UpsertVehicleProfileParams{
		BomID: bomID, Preset: string(p.Preset), WeightCost: int16(w.Cost), WeightCo2: int16(w.CO2), // #nosec G115 -- 0..100 validé
		WeightLeadTime: int16(w.LeadTime), WeightMass: int16(w.Mass), WeightQuality: int16(w.Quality), // #nosec G115 -- 0..100 validé
	})
	if isConstraint(err, fkProfileBOM) {
		err = bom.ErrBOMNotFound
	}
	if err != nil {
		return bom.VehicleProfile{}, fmt.Errorf("set profile of bom %d: %w", bomID, err)
	}
	return toProfile(row), nil
}

func toProfile(r bomdb.UpsertVehicleProfileRow) bom.VehicleProfile {
	return bom.VehicleProfile{Preset: bom.ProfilePreset(r.Preset), Weights: bom.Weights{
		Cost: int(r.WeightCost), CO2: int(r.WeightCo2), LeadTime: int(r.WeightLeadTime), Mass: int(r.WeightMass), Quality: int(r.WeightQuality),
	}}
}
