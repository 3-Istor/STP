package bomstore_test

import (
	"errors"
	"testing"

	"bomstudio/internal/bom"
	"bomstudio/internal/bom/bomstore"
)

// ADR 0036 : le profil se relit tel qu'enregistré ; la base refuse une somme ≠ 100 même sans le domaine.
func TestStoreVehicleProfile(t *testing.T) {
	tx := testDB(t)
	ctx := t.Context()
	s := bomstore.New(tx)
	b, err := s.CreateBOM(ctx, bom.NewBOM{Name: "P", Currency: "EUR", Volume: 10})
	if err != nil {
		t.Fatal(err)
	}
	if p, err := s.VehicleProfile(ctx, b.ID); err != nil || p != nil {
		t.Fatalf("no profile yet: %+v, %v", p, err)
	}
	tests := []struct {
		name    string
		p       bom.VehicleProfile
		wantErr bool
	}{
		{"familiale", bom.VehicleProfile{Preset: bom.ProfileFamily, Weights: bom.Weights{Cost: 40, CO2: 25, LeadTime: 20, Mass: 5, Quality: 10}}, false},
		{"remplacé par un profil personnalisé", bom.VehicleProfile{Preset: bom.ProfileCustom, Weights: bom.Weights{Mass: 100}}, false},
		{"somme 90 : refus de la base", bom.VehicleProfile{Preset: bom.ProfileCustom, Weights: bom.Weights{Mass: 90}}, true},
		{"préréglage inconnu : refus de la base", bom.VehicleProfile{Preset: "suv", Weights: bom.Weights{Mass: 100}}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sp := savepoint(t, tx)
			got, err := bomstore.New(sp).SetVehicleProfile(ctx, b.ID, tt.p)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil {
				return
			}
			read, err := bomstore.New(sp).VehicleProfile(ctx, b.ID)
			if err != nil || got != tt.p || read == nil || *read != tt.p {
				t.Fatalf("set %+v, read %+v, %v; want %+v", got, read, err, tt.p)
			}
		})
	}
	if _, err := s.VehicleProfile(ctx, 1<<40); !errors.Is(err, bom.ErrBOMNotFound) {
		t.Errorf("get on unknown bom: %v", err)
	}
	if _, err := s.SetVehicleProfile(ctx, 1<<40, tests[0].p); !errors.Is(err, bom.ErrBOMNotFound) {
		t.Errorf("set on unknown bom: %v", err)
	}
}
