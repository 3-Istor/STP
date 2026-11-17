package pricing

import (
	"errors"
	"testing"

	"github.com/shopspring/decimal"
)

// TestPerKgDetector : prix au kg comparé à la médiane €/kg de la famille, seuil 2× par défaut (ADR 0037).
func TestPerKgDetector(t *testing.T) {
	kg := func(s string) *decimal.Decimal { d := decimal.RequireFromString(s); return &d }
	// Famille « Faisceaux » : 18 €/kg partout, avec des masses très différentes (la médiane du prix unitaire,
	// ADR 0027, mettrait le faisceau lourd en anomalie ; le prix au kg non).
	family := func(t *testing.T) []CostSample {
		return []CostSample{
			{PartID: 1, Category: "Faisceaux", Cost: eur(t, "1.80"), MassKg: kg("0.1")},
			{PartID: 2, Category: "Faisceaux", Cost: eur(t, "9"), MassKg: kg("0.5")},
			{PartID: 3, Category: "Faisceaux", Cost: eur(t, "18"), MassKg: kg("1")},
			{PartID: 4, Category: "Faisceaux", Cost: eur(t, "36"), MassKg: kg("2")},
			{PartID: 5, Category: "Faisceaux", Cost: eur(t, "90"), MassKg: kg("5")},
		}
	}
	tests := []struct {
		name       string
		subject    func(t *testing.T) CostSample
		population func(t *testing.T) []CostSample
		wantStatus AnomalyStatus
		wantPerKg  string
		wantExcess string // coût − médiane €/kg × masse
		wantErr    error
	}{
		{"lourd mais au prix de la famille : normal", func(t *testing.T) CostSample {
			return CostSample{PartID: 5, Category: "Faisceaux", Cost: eur(t, "90"), MassKg: kg("5")}
		}, family, AnomalyNormal, "18", "0", nil},
		{"QUAND le prix au kg vaut 2 × la médiane de la famille ALORS anomalie, surcoût = 36 − 18 × 1", func(t *testing.T) CostSample {
			return CostSample{PartID: 9, Category: "Faisceaux", Cost: eur(t, "36"), MassKg: kg("1")}
		}, family, AnomalyDetected, "36", "18", nil},
		{"juste sous 2× : normal", func(t *testing.T) CostSample {
			return CostSample{PartID: 9, Category: "Faisceaux", Cost: eur(t, "35.99"), MassKg: kg("1")}
		}, family, AnomalyNormal, "35.99", "17.99", nil},
		{"pièce sans masse : non évaluable, jamais 0 €/kg", func(t *testing.T) CostSample {
			return CostSample{PartID: 9, Category: "Faisceaux", Cost: eur(t, "36")}
		}, family, AnomalyNotEvaluable, "", "", ErrNoMass},
		{"pièce de masse nulle : non évaluable", func(t *testing.T) CostSample {
			return CostSample{PartID: 9, Category: "Faisceaux", Cost: eur(t, "36"), MassKg: kg("0")}
		}, family, AnomalyNotEvaluable, "", "", ErrNoMass},
		{"famille dont une pièce n'a pas de masse : 4 pièces évaluables < 5", func(t *testing.T) CostSample {
			return CostSample{PartID: 9, Category: "Faisceaux", Cost: eur(t, "36"), MassKg: kg("1")}
		}, func(t *testing.T) []CostSample {
			f := family(t)
			f[0].MassKg = nil
			return f
		}, AnomalyNotEvaluable, "", "", ErrTooFewPeers},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := PerKgDetector{MinPeers: 5}.Detect([]CostSample{tt.subject(t)}, tt.population(t), DefaultPerKgThreshold)
			if err != nil {
				t.Fatal(err)
			}
			a := got[0]
			if a.Status != tt.wantStatus {
				t.Fatalf("status %s (%v), want %s", a.Status, a.Err, tt.wantStatus)
			}
			if tt.wantErr != nil {
				if !errors.Is(a.Err, tt.wantErr) || a.PerKg != nil || a.Excess != nil {
					t.Fatalf("err %v per kg %v, want %v and nothing computed", a.Err, a.PerKg, tt.wantErr)
				}
				return
			}
			if !a.PerKg.Amount().Equal(decimal.RequireFromString(tt.wantPerKg)) || !a.MedianPerKg.Amount().Equal(decimal.RequireFromString("18")) ||
				!a.Excess.Amount().Equal(decimal.RequireFromString(tt.wantExcess)) {
				t.Fatalf("per kg %s median %s excess %s, want %s, 18, %s", a.PerKg, a.MedianPerKg, a.Excess, tt.wantPerKg, tt.wantExcess)
			}
		})
	}
	if _, err := (PerKgDetector{MinPeers: 5}).Detect(nil, nil, decimal.NewFromInt(1)); !errors.Is(err, ErrInvalidThreshold) {
		t.Fatalf("threshold 1: %v", err)
	}
}
