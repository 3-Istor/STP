package pricing

import (
	"errors"
	"testing"

	"github.com/shopspring/decimal"

	"bomstudio/internal/money"
)

func eur(t *testing.T, amount string) *money.Money {
	t.Helper()
	m, err := money.Parse(amount, "EUR")
	if err != nil {
		t.Fatal(err)
	}
	return &m
}

// TestMedianDetector : écart à la médiane de la catégorie, seuil 3× par défaut, au moins 5 pièces (ADR 0027).
func TestMedianDetector(t *testing.T) {
	freins := func(t *testing.T, costs ...string) []CostSample {
		out := make([]CostSample, len(costs))
		for i, c := range costs {
			out[i] = CostSample{PartID: int64(i + 1), Category: "Freins", Cost: eur(t, c)}
		}
		return out
	}
	three := DefaultAnomalyThreshold
	tests := []struct {
		name       string
		subject    func(t *testing.T) CostSample
		population func(t *testing.T) []CostSample
		threshold  decimal.Decimal
		wantStatus AnomalyStatus
		wantRatio  string
		wantMedian string
		wantErr    error
	}{
		{
			name:       "QUAND une pièce coûte 3 fois la médiane de sa catégorie ALORS elle est une anomalie, écart affiché",
			subject:    func(t *testing.T) CostSample { return CostSample{PartID: 9, Category: "Freins", Cost: eur(t, "300")} },
			population: func(t *testing.T) []CostSample { return freins(t, "90", "100", "100", "110", "300") },
			threshold:  three, wantStatus: AnomalyDetected, wantRatio: "3", wantMedian: "100",
		},
		{
			name: "juste sous le seuil : normale",
			subject: func(t *testing.T) CostSample {
				return CostSample{PartID: 9, Category: "Freins", Cost: eur(t, "299.99")}
			},
			population: func(t *testing.T) []CostSample { return freins(t, "90", "100", "100", "110", "299.99") },
			threshold:  three, wantStatus: AnomalyNormal, wantMedian: "100",
		},
		{
			name:       "nombre pair : médiane = moyenne exacte des deux valeurs centrales",
			subject:    func(t *testing.T) CostSample { return CostSample{PartID: 9, Category: "Freins", Cost: eur(t, "500")} },
			population: func(t *testing.T) []CostSample { return freins(t, "10", "100", "101", "200", "300", "500") },
			threshold:  three, wantStatus: AnomalyDetected, wantMedian: "150.5",
		},
		{
			name:       "SI la catégorie compte moins de 5 pièces ALORS non évaluable, jamais « normale »",
			subject:    func(t *testing.T) CostSample { return CostSample{PartID: 9, Category: "Freins", Cost: eur(t, "900")} },
			population: func(t *testing.T) []CostSample { return freins(t, "100", "100", "100", "900") },
			threshold:  three, wantStatus: AnomalyNotEvaluable, wantErr: ErrTooFewPeers,
		},
		{
			name:    "une pièce de la population sans prix ne compte pas dans la médiane",
			subject: func(t *testing.T) CostSample { return CostSample{PartID: 9, Category: "Freins", Cost: eur(t, "400")} },
			population: func(t *testing.T) []CostSample {
				return append(freins(t, "100", "100", "100", "400"), CostSample{PartID: 50, Category: "Freins", Err: ErrNoQuote})
			},
			threshold: three, wantStatus: AnomalyNotEvaluable, wantErr: ErrTooFewPeers,
		},
		{
			name:       "sujet sans prix : non évaluable avec la raison",
			subject:    func(t *testing.T) CostSample { return CostSample{PartID: 9, Category: "Freins", Err: ErrNoRate} },
			population: func(t *testing.T) []CostSample { return freins(t, "1", "1", "1", "1", "1") },
			threshold:  three, wantStatus: AnomalyNotEvaluable, wantErr: ErrNoRate,
		},
		{
			name:       "pièce sans catégorie : non évaluable",
			subject:    func(t *testing.T) CostSample { return CostSample{PartID: 9, Cost: eur(t, "1")} },
			population: func(t *testing.T) []CostSample { return freins(t, "1", "1", "1", "1", "1") },
			threshold:  three, wantStatus: AnomalyNotEvaluable, wantErr: ErrNoCategory,
		},
		{
			name:       "seuil paramétrable : 1,5×",
			subject:    func(t *testing.T) CostSample { return CostSample{PartID: 9, Category: "Freins", Cost: eur(t, "150")} },
			population: func(t *testing.T) []CostSample { return freins(t, "90", "100", "100", "110", "150") },
			threshold:  decimal.RequireFromString("1.5"), wantStatus: AnomalyDetected, wantRatio: "1.5",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := MedianDetector{MinPeers: 5}.Detect([]CostSample{tt.subject(t)}, tt.population(t), tt.threshold)
			if err != nil {
				t.Fatal(err)
			}
			a := got[0]
			if a.Status != tt.wantStatus {
				t.Errorf("status %s, want %s (err %v)", a.Status, tt.wantStatus, a.Err)
			}
			if !errors.Is(a.Err, tt.wantErr) {
				t.Errorf("err %v, want %v", a.Err, tt.wantErr)
			}
			if tt.wantRatio != "" && (a.Ratio == nil || !a.Ratio.Equal(decimal.RequireFromString(tt.wantRatio))) {
				t.Errorf("ratio %v, want %s", a.Ratio, tt.wantRatio)
			}
			if tt.wantMedian != "" && (a.Median == nil || !a.Median.Amount().Equal(decimal.RequireFromString(tt.wantMedian))) {
				t.Errorf("median %v, want %s", a.Median, tt.wantMedian)
			}
		})
	}
}

func TestMedianDetectorRejectsBadThreshold(t *testing.T) {
	for _, th := range []string{"1", "0.5", "-3"} {
		if _, err := (MedianDetector{MinPeers: 5}).Detect(nil, nil, decimal.RequireFromString(th)); !errors.Is(err, ErrInvalidThreshold) {
			t.Errorf("threshold %s: err %v, want ErrInvalidThreshold", th, err)
		}
	}
}
