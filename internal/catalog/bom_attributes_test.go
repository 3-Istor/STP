package catalog

import (
	"errors"
	"strings"
	"testing"

	"github.com/shopspring/decimal"
)

func TestPartBOMAttributes(t *testing.T) {
	co2, mass, days := decimal.RequireFromString("2.5"), decimal.RequireFromString("0.8"), 3
	zero := 0
	tests := []struct {
		name    string
		part    Part
		wantErr string // attributs listés ; vide : pas d'erreur
	}{
		{"tous présents", Part{ID: 1, CO2KGPerUnit: &co2, MassKG: &mass, ProductionDays: &days}, ""},
		{"zéro explicite accepté", Part{ID: 1, CO2KGPerUnit: &co2, MassKG: &mass, ProductionDays: &zero}, ""},
		{"ancienne ligne sans attribut", Part{ID: 2}, "co2_kg_per_unit, mass_kg, production_days"},
		{"CO2 seul absent", Part{ID: 3, MassKG: &mass, ProductionDays: &days}, "co2_kg_per_unit"},
		{"production seule absente", Part{ID: 4, CO2KGPerUnit: &co2, MassKG: &mass}, "production_days"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.part.BOMAttributes()
			if tt.wantErr == "" {
				if err != nil {
					t.Fatal(err)
				}
				if !got.CO2KGPerUnit.Equal(*tt.part.CO2KGPerUnit) || !got.MassKG.Equal(*tt.part.MassKG) || got.ProductionDays != *tt.part.ProductionDays {
					t.Fatalf("attributes = %+v, want those of %+v", got, tt.part)
				}
				return
			}
			if !errors.Is(err, ErrMissingAttribute) || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("err = %v, want ErrMissingAttribute listing %q", err, tt.wantErr)
			}
		})
	}
}
