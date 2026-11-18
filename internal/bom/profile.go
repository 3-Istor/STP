package bom

import (
	"context"
	"errors"
	"fmt"
)

// Profils de véhicule (ADR 0036) : ce qui compte pour ce véhicule, en poids entiers sur cinq critères, qui
// classent les alternatives proposées. Rien ne change dans la BOM sans l'accord de l'utilisateur.

// Weights : poids des critères du classement. Ceux d'un profil sont entiers, entre 0 et 100, de somme 100 ;
// ceux d'une stratégie de nœud (strategyWeights) sont relatifs (seule leur proportion compte).
type Weights struct {
	Cost, CO2, LeadTime, Mass, Quality int
}

func (w Weights) sum() int { return w.Cost + w.CO2 + w.LeadTime + w.Mass + w.Quality }

// ProfilePreset : préréglage d'un profil ; ProfileCustom laisse les poids libres.
type ProfilePreset string

const (
	ProfileCityEco ProfilePreset = "city_eco"
	ProfileFamily  ProfilePreset = "family"
	ProfilePremium ProfilePreset = "premium"
	ProfileSport   ProfilePreset = "sport"
	ProfileCustom  ProfilePreset = "custom"
)

// VehicleProfile : profil d'une BOM (ou figé dans un snapshot).
type VehicleProfile struct {
	Preset  ProfilePreset
	Weights Weights
}

// ProfileDefinition : un préréglage proposé à l'utilisateur, avec ses poids (ceux de départ pour custom).
type ProfileDefinition struct {
	Preset  ProfilePreset
	Label   string
	Hint    string
	Weights Weights
}

// VehicleProfilePresets : les préréglages, dans l'ordre d'affichage (maquette I4-1, onglet Réglages).
func VehicleProfilePresets() []ProfileDefinition {
	return []ProfileDefinition{
		{ProfileCityEco, "Citadine éco", "Prix de vente bas : le coût domine.", Weights{Cost: 55, CO2: 15, LeadTime: 15, Mass: 5, Quality: 10}},
		{ProfileFamily, "Familiale", "Équilibre coût, délai et CO₂.", Weights{Cost: 40, CO2: 25, LeadTime: 20, Mass: 5, Quality: 10}},
		{ProfilePremium, "Premium", "La qualité fournisseur et le CO₂ comptent autant que le coût.",
			Weights{Cost: 20, CO2: 25, LeadTime: 10, Mass: 10, Quality: 35}},
		{ProfileSport, "Sportive", "La masse passe devant tout.", Weights{Cost: 20, CO2: 10, LeadTime: 10, Mass: 45, Quality: 15}},
		{ProfileCustom, "Personnalisé", "Réglez les poids vous-même (somme 100).", Weights{Cost: 30, CO2: 30, LeadTime: 20, Mass: 10, Quality: 10}},
	}
}

// NewVehicleProfile valide un profil saisi : préréglage connu ; un préréglage autre que custom impose ses poids
// (weights nil, ou égaux aux siens) ; custom exige des poids entiers entre 0 et 100 de somme 100.
func NewVehicleProfile(preset string, weights *Weights) (VehicleProfile, error) {
	for _, d := range VehicleProfilePresets() {
		if string(d.Preset) != preset {
			continue
		}
		if d.Preset != ProfileCustom {
			if weights != nil && *weights != d.Weights {
				return VehicleProfile{}, fmt.Errorf("preset %s has fixed weights %+v (use custom to change them): %w",
					preset, d.Weights, ErrInvalidInput)
			}
			return VehicleProfile{Preset: d.Preset, Weights: d.Weights}, nil
		}
		if weights == nil {
			return VehicleProfile{}, fmt.Errorf("custom profile needs weights: %w", ErrInvalidInput)
		}
		if err := validateWeights(*weights); err != nil {
			return VehicleProfile{}, err
		}
		return VehicleProfile{Preset: ProfileCustom, Weights: *weights}, nil
	}
	return VehicleProfile{}, fmt.Errorf("profile preset %q (want city_eco, family, premium, sport or custom): %w", preset, ErrInvalidInput)
}

func validateWeights(w Weights) error {
	for _, v := range []int{w.Cost, w.CO2, w.LeadTime, w.Mass, w.Quality} {
		if v < 0 || v > 100 {
			return fmt.Errorf("weight %d must be between 0 and 100: %w", v, ErrInvalidInput)
		}
	}
	if s := w.sum(); s != 100 {
		return fmt.Errorf("weights sum to %d, want 100: %w", s, ErrInvalidInput)
	}
	return nil
}

// ProfileStore : profils des brouillons. L'implémentation Postgres est bomstore.Store.
type ProfileStore interface {
	// VehicleProfile renvoie le profil de la BOM, nil si elle n'en a pas ; ErrBOMNotFound si elle n'existe pas.
	VehicleProfile(ctx context.Context, bomID int64) (*VehicleProfile, error)
	// SetVehicleProfile enregistre (ou remplace) le profil ; ErrBOMNotFound si la BOM n'existe pas.
	SetVehicleProfile(ctx context.Context, bomID int64, p VehicleProfile) (VehicleProfile, error)
}

// ErrProfilesUnavailable : le service a été câblé sans stockage des profils.
var ErrProfilesUnavailable = errors.New("vehicle profiles unavailable: no profile store")

// WithProfiles branche le stockage des profils de véhicule (ADR 0036).
func (s *Service) WithProfiles(p ProfileStore) *Service {
	s.profiles = p
	return s
}

// VehicleProfile renvoie le profil du brouillon (nil : aucun, le classement suit les stratégies de nœud).
func (s *Service) VehicleProfile(ctx context.Context, bomID int64) (*VehicleProfile, error) {
	if s.profiles == nil {
		return nil, fmt.Errorf("profile of bom %d: %w", bomID, ErrProfilesUnavailable)
	}
	p, err := s.profiles.VehicleProfile(ctx, bomID)
	if err != nil {
		return nil, fmt.Errorf("profile of bom %d: %w", bomID, err)
	}
	return p, nil
}

// SetVehicleProfile valide puis enregistre le profil du brouillon (un snapshot garde le sien, figé).
func (s *Service) SetVehicleProfile(ctx context.Context, bomID int64, preset string, weights *Weights) (VehicleProfile, error) {
	if s.profiles == nil {
		return VehicleProfile{}, fmt.Errorf("set profile of bom %d: %w", bomID, ErrProfilesUnavailable)
	}
	p, err := NewVehicleProfile(preset, weights)
	if err != nil {
		return VehicleProfile{}, fmt.Errorf("set profile of bom %d: %w", bomID, err)
	}
	out, err := s.profiles.SetVehicleProfile(ctx, bomID, p)
	if err != nil {
		return VehicleProfile{}, fmt.Errorf("set profile of bom %d: %w", bomID, err)
	}
	return out, nil
}
