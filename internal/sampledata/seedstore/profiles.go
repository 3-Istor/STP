package seedstore

import (
	"context"
	"fmt"
	"maps"
	"slices"

	"github.com/jackc/pgx/v5"

	"bomstudio/internal/sampledata"
)

// insertProfiles pose les profils de démo (ADR 0036) sur les BOM du jeu chargé sous version. Un profil déjà
// présent (choisi depuis par l'utilisateur, ou seed précédent) est gardé : relancer le seed ne change rien.
func insertProfiles(ctx context.Context, tx pgx.Tx, version string, profiles map[string]sampledata.VehicleProfile) error {
	for _, key := range slices.Sorted(maps.Keys(profiles)) {
		p := profiles[key]
		if _, err := tx.Exec(ctx, `INSERT INTO bom_vehicle_profiles
			(bom_id, preset, weight_cost, weight_co2, weight_lead_time, weight_mass, weight_quality)
			SELECT entity_id, $3, $4, $5, $6, $7, $8 FROM seed_entities
			WHERE dataset_version = $1 AND entity_kind = 'bom' AND logical_key = $2
			ON CONFLICT (bom_id) DO NOTHING`,
			version, key, p.Preset, p.Cost, p.CO2, p.LeadTime, p.Mass, p.Quality); err != nil {
			return fmt.Errorf("vehicle profile of %s: %w", key, err)
		}
	}
	return nil
}
