package snapshot

import (
	"context"
	"fmt"

	"bomstudio/internal/bom"
)

// Profile renvoie le profil de véhicule figé à la publication (ADR 0036) ; nil si la BOM n'en avait pas.
// Seul l'en-tête est lu : aucun total n'est recalculé.
func (s *Service) Profile(ctx context.Context, id int64) (*bom.VehicleProfile, error) {
	h, err := s.store.Get(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("profile of snapshot %d: %w", id, err)
	}
	return h.Profile, nil
}
