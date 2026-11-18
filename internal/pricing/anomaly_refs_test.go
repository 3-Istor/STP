package pricing

import (
	"errors"
	"strings"
	"testing"

	"github.com/shopspring/decimal"

	"bomstudio/internal/money"
)

func eurs(t *testing.T, amounts ...string) []money.Money {
	t.Helper()
	out := make([]money.Money, len(amounts))
	for i, a := range amounts {
		out[i] = *eur(t, a)
	}
	return out
}

// TestOfferSpreadDetector : coût de l'offre retenue comparé à la médiane des offres de la même pièce (ADR 0041).
func TestOfferSpreadDetector(t *testing.T) {
	tests := []struct {
		name       string
		subject    func(t *testing.T) CostSample
		wantStatus AnomalyStatus
		wantMedian string
		wantExcess string
		wantErr    error
	}{
		{"offre retenue 30 % au-dessus de la médiane des offres : anomalie", func(t *testing.T) CostSample {
			return CostSample{PartID: 1, Cost: eur(t, "130"), OfferCosts: eurs(t, "100", "100", "130")}
		}, AnomalyDetected, "100", "30", nil},
		{"pile au seuil 1,25 : anomalie", func(t *testing.T) CostSample {
			return CostSample{PartID: 1, Cost: eur(t, "125"), OfferCosts: eurs(t, "100", "100", "125")}
		}, AnomalyDetected, "100", "25", nil},
		{"juste sous le seuil : normal", func(t *testing.T) CostSample {
			return CostSample{PartID: 1, Cost: eur(t, "124.99"), OfferCosts: eurs(t, "100", "100", "124.99")}
		}, AnomalyNormal, "100", "24.99", nil},
		{"offre la moins chère : normale, écart négatif", func(t *testing.T) CostSample {
			return CostSample{PartID: 1, Cost: eur(t, "80"), OfferCosts: eurs(t, "80", "100", "120")}
		}, AnomalyNormal, "100", "-20", nil},
		{"deux offres : médiane = moyenne des deux", func(t *testing.T) CostSample {
			return CostSample{PartID: 1, Cost: eur(t, "150"), OfferCosts: eurs(t, "100", "150")}
		}, AnomalyNormal, "125", "25", nil},
		{"une seule offre : non évaluable", func(t *testing.T) CostSample {
			return CostSample{PartID: 1, Cost: eur(t, "150"), OfferCosts: eurs(t, "150"), MissingOffers: 2}
		}, AnomalyNotEvaluable, "", "", ErrTooFewOffers},
		{"offre retenue sans prix : non évaluable, jamais normale", func(t *testing.T) CostSample {
			return CostSample{PartID: 1, Err: ErrNoQuote, OfferCosts: eurs(t, "100", "100")}
		}, AnomalyNotEvaluable, "", "", ErrNoQuote},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := OfferSpreadDetector{MinOffers: 2}.Detect([]CostSample{tt.subject(t)}, nil, DefaultOfferSpreadThreshold)
			if err != nil {
				t.Fatal(err)
			}
			a := got[0]
			if a.Status != tt.wantStatus {
				t.Fatalf("status %s (%v), want %s", a.Status, a.Err, tt.wantStatus)
			}
			if tt.wantErr != nil {
				if !errors.Is(a.Err, tt.wantErr) || a.Median != nil || a.Excess != nil {
					t.Fatalf("err %v median %v, want %v and nothing computed", a.Err, a.Median, tt.wantErr)
				}
				return
			}
			if !a.Median.Amount().Equal(decimal.RequireFromString(tt.wantMedian)) || !a.Excess.Amount().Equal(decimal.RequireFromString(tt.wantExcess)) {
				t.Fatalf("median %s excess %s, want %s and %s", a.Median, a.Excess, tt.wantMedian, tt.wantExcess)
			}
		})
	}
	if _, err := (OfferSpreadDetector{MinOffers: 2}).Detect(nil, nil, decimal.NewFromInt(1)); !errors.Is(err, ErrInvalidThreshold) {
		t.Fatalf("threshold 1: %v", err)
	}
}

// TestHistoryDetector : prix coté comparé à la médiane des 12 derniers mois, dans la devise de l'offre (ADR 0041).
func TestHistoryDetector(t *testing.T) {
	usd := func(amount string) money.Money {
		m, err := money.Parse(amount, "USD")
		if err != nil {
			t.Fatal(err)
		}
		return m
	}
	usds := func(amounts ...string) []money.Money {
		out := make([]money.Money, len(amounts))
		for i, a := range amounts {
			out[i] = usd(a)
		}
		return out
	}
	tests := []struct {
		name       string
		subject    func(t *testing.T) CostSample
		wantStatus AnomalyStatus
		wantRatio  string
		wantExcess string // en EUR : coût converti − coût converti à la médiane
		wantErr    error
	}{
		{"prix 20 % au-dessus de sa médiane annuelle : anomalie, écart converti au taux du coût", func(t *testing.T) CostSample {
			// 120 USD = 110 EUR (taux 0,9167…) ; référence 100 USD → 91,6667… EUR ; on prend un coût rond.
			return CostSample{PartID: 1, Cost: eur(t, "120"), Trail: &PriceTrail{Current: usd("120"), Past: usds("100", "100", "100", "100", "100")}}
		}, AnomalyDetected, "1.2", "20", nil},
		{"change sans effet : le verdict ne dépend que de la devise de l'offre", func(t *testing.T) CostSample {
			return CostSample{PartID: 1, Cost: eur(t, "500"), Trail: &PriceTrail{Current: usd("100"), Past: usds("100", "101", "99", "100", "100")}}
		}, AnomalyNormal, "1", "0", nil},
		{"juste sous 1,15 : normal", func(t *testing.T) CostSample {
			return CostSample{PartID: 1, Cost: eur(t, "114.99"), Trail: &PriceTrail{Current: usd("114.99"), Past: usds("100", "100", "100", "100", "100")}}
		}, AnomalyNormal, "1.1499", "14.99", nil},
		{"moins de 5 cotations : non évaluable", func(t *testing.T) CostSample {
			return CostSample{PartID: 1, Cost: eur(t, "120"), Trail: &PriceTrail{Current: usd("120"), Past: usds("100", "100")}}
		}, AnomalyNotEvaluable, "", "", ErrTooFewQuotes},
		{"historique non lu : non évaluable", func(t *testing.T) CostSample {
			return CostSample{PartID: 1, Cost: eur(t, "120")}
		}, AnomalyNotEvaluable, "", "", ErrTooFewQuotes},
		{"offre sans prix : non évaluable", func(t *testing.T) CostSample {
			return CostSample{PartID: 1, Err: ErrNoQuote}
		}, AnomalyNotEvaluable, "", "", ErrNoQuote},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := HistoryDetector{MinQuotes: 5}.Detect([]CostSample{tt.subject(t)}, nil, DefaultHistoryThreshold)
			if err != nil {
				t.Fatal(err)
			}
			a := got[0]
			if a.Status != tt.wantStatus {
				t.Fatalf("status %s (%v), want %s", a.Status, a.Err, tt.wantStatus)
			}
			if tt.wantErr != nil {
				if !errors.Is(a.Err, tt.wantErr) || a.Ratio != nil {
					t.Fatalf("err %v ratio %v, want %v and nothing computed", a.Err, a.Ratio, tt.wantErr)
				}
				return
			}
			if !a.Ratio.Equal(decimal.RequireFromString(tt.wantRatio)) {
				t.Fatalf("ratio %s, want %s", a.Ratio, tt.wantRatio)
			}
		})
	}
	// L'écart est exprimé dans la devise commune : coût 120 EUR pour un prix coté de 120 → référence 100 EUR.
	got, err := HistoryDetector{MinQuotes: 5}.Detect([]CostSample{{PartID: 1, Cost: eur(t, "120"),
		Trail: &PriceTrail{Current: usd("120"), Past: usds("100", "100", "100", "100", "100")}}}, nil, DefaultHistoryThreshold)
	if err != nil || !got[0].Excess.Amount().Equal(decimal.RequireFromString("20")) || got[0].Excess.Currency() != "EUR" {
		t.Fatalf("excess %v err %v, want 20 EUR", got[0].Excess, err)
	}
}

// TestPerKgDetectorByFamily : méthode « matière » — famille catégorie × matière, pièce sans matière non évaluable.
func TestPerKgDetectorByFamily(t *testing.T) {
	kg := func(s string) *decimal.Decimal { d := decimal.RequireFromString(s); return &d }
	// Même catégorie, deux matières : l'acier à 10 €/kg, le carbone à 100 €/kg.
	pop := []CostSample{}
	for i, c := range []string{"10", "10", "10", "10", "10"} {
		pop = append(pop, CostSample{PartID: int64(i + 1), Category: "Caisse", Family: "Caisse · acier", Cost: eur(t, c), MassKg: kg("1")})
	}
	for i, c := range []string{"100", "100", "100", "100", "100"} {
		pop = append(pop, CostSample{PartID: int64(i + 10), Category: "Caisse", Family: "Caisse · carbone", Cost: eur(t, c), MassKg: kg("1")})
	}
	tests := []struct {
		name       string
		subject    CostSample
		wantStatus AnomalyStatus
		wantFamily string
		wantErr    error
	}{
		{"carbone à 100 €/kg parmi le carbone : normal", CostSample{PartID: 20, Category: "Caisse", Family: "Caisse · carbone", Cost: eur(t, "100"), MassKg: kg("1")}, AnomalyNormal, "Caisse · carbone", nil},
		{"acier à 100 €/kg parmi l'acier : anomalie", CostSample{PartID: 21, Category: "Caisse", Family: "Caisse · acier", Cost: eur(t, "100"), MassKg: kg("1")}, AnomalyDetected, "Caisse · acier", nil},
		{"pièce sans matière : non évaluable", CostSample{PartID: 22, Category: "Caisse", Cost: eur(t, "100"), MassKg: kg("1")}, AnomalyNotEvaluable, "", ErrNoMaterial},
		{"famille de 0 pair : non évaluable", CostSample{PartID: 23, Category: "Caisse", Family: "Caisse · inox", Cost: eur(t, "100"), MassKg: kg("1")}, AnomalyNotEvaluable, "Caisse · inox", ErrTooFewPeers},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := PerKgDetector{MinPeers: 5, ByFamily: true}.Detect([]CostSample{tt.subject}, pop, DefaultPerKgThreshold)
			if err != nil {
				t.Fatal(err)
			}
			a := got[0]
			if a.Status != tt.wantStatus || a.Family != tt.wantFamily || (tt.wantErr != nil && !errors.Is(a.Err, tt.wantErr)) {
				t.Fatalf("status %s family %q err %v, want %s %q %v", a.Status, a.Family, a.Err, tt.wantStatus, tt.wantFamily, tt.wantErr)
			}
		})
	}
}

// Une raison « trop peu de… » dit ce qui a été écarté : cotations en autre devise, offres archivées.
func TestTooFewReasonsNameWhatWasDropped(t *testing.T) {
	usd := func(a string) money.Money {
		m, err := money.Parse(a, "USD")
		if err != nil {
			t.Fatal(err)
		}
		return m
	}
	h, err := HistoryDetector{MinQuotes: 5}.Detect([]CostSample{{PartID: 1, Cost: eur(t, "120"),
		Trail: &PriceTrail{Current: usd("120"), Past: []money.Money{usd("100"), usd("100")}, OtherCurrency: 3}}}, nil, DefaultHistoryThreshold)
	if err != nil || !errors.Is(h[0].Err, ErrTooFewQuotes) || !strings.Contains(h[0].Err.Error(), "3 in another currency") {
		t.Fatalf("history reason %v (%v), want ErrTooFewQuotes naming 3 quotes in another currency", h[0].Err, err)
	}
	o, err := OfferSpreadDetector{MinOffers: 2}.Detect([]CostSample{{PartID: 1, Cost: eur(t, "150"), OfferCosts: eurs(t, "150"), ArchivedOffers: 2}}, nil, DefaultOfferSpreadThreshold)
	if err != nil || !errors.Is(o[0].Err, ErrTooFewOffers) || !strings.Contains(o[0].Err.Error(), "2 archived") {
		t.Fatalf("offers reason %v (%v), want ErrTooFewOffers naming 2 archived offers", o[0].Err, err)
	}
	// Pièce archivée : hors population active, jamais « 0 of 2 required ».
	a, err := OfferSpreadDetector{MinOffers: 2}.Detect([]CostSample{{PartID: 1, Cost: eur(t, "150"), Err: ErrPartArchived}}, nil, DefaultOfferSpreadThreshold)
	if err != nil || !errors.Is(a[0].Err, ErrPartArchived) {
		t.Fatalf("archived part reason %v (%v), want ErrPartArchived", a[0].Err, err)
	}
}
