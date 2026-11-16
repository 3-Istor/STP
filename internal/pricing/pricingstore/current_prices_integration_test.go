package pricingstore_test

import (
	"testing"

	"bomstudio/internal/pricing/pricingdb"
)

// ReadCurrentPrices (LATERAL, une sous-requête par offre) : dernière cotation de chaque offre à as_of,
// cotations futures ignorées, égalité d'horodatage tranchée par l'id, offre sans cotation ou inconnue absente.
// Même contrat que l'ancien DISTINCT ON ; seul le coût change (une ligne lue par offre, pas tout l'historique).
func TestReadCurrentPricesLatestPerOffer(t *testing.T) {
	tx := historyDB(t)
	a, b, silent := historyOffer(t, tx), historyOffer(t, tx), historyOffer(t, tx)
	insertQuote(t, tx, a, "100", "2026-01-01T00:00:00Z", "old")
	insertQuote(t, tx, a, "120", "2026-06-01T00:00:00Z", "tie-first")
	tieLast := insertQuote(t, tx, a, "125", "2026-06-01T00:00:00Z", "tie-last")
	insertQuote(t, tx, a, "999", "2027-01-01T00:00:00Z", "future")
	current := insertQuote(t, tx, b, "50", "2026-03-01T00:00:00Z", "current")
	insertQuote(t, tx, b, "40", "2026-02-01T00:00:00Z", "older-inserted-later")

	tests := []struct {
		name   string
		offers []int64
		asOf   string
		want   map[int64]int64 // offre → id de cotation attendu
	}{
		{"latest per offer, future excluded, tie by id", []int64{a, b, silent, -1}, "2026-11-05T00:00:00Z", map[int64]int64{a: tieLast.ID, b: current.ID}},
		{"as_of before every quote", []int64{a, b}, "2025-01-01T00:00:00Z", map[int64]int64{}},
		{"duplicate ids read once", []int64{b, b}, "2026-11-05T00:00:00Z", map[int64]int64{b: current.ID}},
		{"empty lot", nil, "2026-11-05T00:00:00Z", map[int64]int64{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rows, err := pricingdb.New(tx).ReadCurrentPrices(t.Context(), pricingdb.ReadCurrentPricesParams{OfferIds: tt.offers, AsOf: quoteTime(t, tt.asOf)})
			if err != nil {
				t.Fatal(err)
			}
			got := map[int64]int64{}
			for _, r := range rows {
				if _, dup := got[r.OfferID]; dup {
					t.Fatalf("offer %d returned twice", r.OfferID)
				}
				got[r.OfferID] = r.ID
			}
			if len(got) != len(tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
			for offer, id := range tt.want {
				if got[offer] != id {
					t.Errorf("offer %d: quote %d, want %d", offer, got[offer], id)
				}
			}
		})
	}
}
