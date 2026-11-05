package pricingstore

import (
	"errors"
	"strings"
	"testing"
	"time"

	"bomstudio/internal/money"
	"bomstudio/internal/pricesim"
	"bomstudio/internal/pricing"
)

// L'adapter BOMPricing (ADR 0020) branche le gel et les lectures figées de la BOM sur le vrai
// SnapshotService : mêmes prix qu'en direct au gel, puis plus jamais modifiés.
func TestBOMPricingFreezesBOMScopeAndReadsFrozenFacts(t *testing.T) {
	pool := runtimeDB(t)
	offer, part := runtimeOffer(t, pool)
	other, _ := runtimeOffer(t, pool)
	cfg := runtimeConfig()
	if _, _, err := pricing.NewRuntime(pricesim.Simulator{}, &RuntimeStore{pool: pool}, nil).Initialize(t.Context(), cfg); err != nil {
		t.Fatal(err)
	}
	newBOM := func(currency string) int64 {
		var bom, root int64
		if err := pool.QueryRow(t.Context(), "INSERT INTO boms(name,currency,volume) VALUES ('adapter',$1,1) RETURNING id", currency).Scan(&bom); err != nil {
			t.Fatal(err)
		}
		if err := pool.QueryRow(t.Context(), "INSERT INTO bom_nodes(bom_id,name) VALUES ($1,'root') RETURNING id", bom).Scan(&root); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(t.Context(), "INSERT INTO bom_nodes(bom_id,parent_id,part_id,offer_id) VALUES ($1,$2,$3,$4)", bom, root, part, offer); err != nil {
			t.Fatal(err)
		}
		return bom
	}
	adapter := pricing.NewBOMPricing(pricing.NewReadService(NewReadStore(pool), cfg), pricing.NewSnapshotService(NewSnapshotStore(pool), cfg))
	wallClock := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC) // temps réel : ignoré, la source est en temps simulé

	eur := newBOM("EUR")
	live, err := adapter.OfferPrice(t.Context(), offer)
	if err != nil {
		t.Fatal(err)
	}
	snapID, err := adapter.FreezePrices(t.Context(), eur, []int64{offer}, []money.Currency{"EUR"}, wallClock)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(t.Context(), "INSERT INTO price_quotes(offer_id,amount,currency,source,quoted_at) VALUES ($1,999,'USD','later',$2)", offer, cfg.InitialAsOf); err != nil {
		t.Fatal(err)
	}
	// Un adapter neuf relit la base (pas son cache) : le snapshot doit tenir sans lui.
	fresh := pricing.NewBOMPricing(pricing.NewReadService(NewReadStore(pool), cfg), pricing.NewSnapshotService(NewSnapshotStore(pool), cfg))
	for name, p := range map[string]*pricing.BOMPricing{"cache": adapter, "base": fresh} {
		t.Run("lecture figée "+name, func(t *testing.T) {
			q, err := p.SnapshotOfferPrice(t.Context(), snapID, offer)
			if err != nil || q.ID != live.ID || !q.Price.Amount().Equal(live.Price.Amount()) || q.Price.Currency() != "USD" {
				t.Fatalf("frozen quote = %+v (%v), want live quote %+v at freeze time", q, err, live)
			}
			r, err := p.SnapshotRate(t.Context(), snapID, "USD", "EUR")
			if err != nil || r.From != "USD" || r.To != "EUR" || !r.Rate.IsPositive() {
				t.Fatalf("frozen rate = %+v (%v)", r, err)
			}
		})
	}

	tests := []struct {
		name string
		err  func() error
		want error
		msg  string
	}{
		{"pas d'inversion implicite d'un taux figé", func() error { _, err := adapter.SnapshotRate(t.Context(), snapID, "EUR", "USD"); return err }, pricing.ErrNoRate, "EUR->USD"},
		{"offre hors du snapshot", func() error { _, err := adapter.SnapshotOfferPrice(t.Context(), snapID, other); return err }, pricing.ErrNoQuote, ""},
		{"snapshot inconnu", func() error { _, err := fresh.SnapshotOfferPrice(t.Context(), snapID+1000, offer); return err }, pricing.ErrPriceSnapshotNotFound, ""},
		{"offre attendue absente de la BOM", func() error {
			_, err := adapter.FreezePrices(t.Context(), newBOM("EUR"), []int64{offer, other}, []money.Currency{"EUR"}, wallClock)
			return err
		}, pricing.ErrNoQuote, ""},
		{"SI aucun taux vers la devise de la BOM ALORS refus qui liste la paire", func() error {
			_, err := adapter.FreezePrices(t.Context(), newBOM("JPY"), []int64{offer}, []money.Currency{"JPY"}, wallClock)
			return err
		}, pricing.ErrNoRate, "USD->JPY"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.err()
			if !errors.Is(err, tt.want) || !strings.Contains(err.Error(), tt.msg) {
				t.Fatalf("err = %v, want %v mentioning %q", err, tt.want, tt.msg)
			}
		})
	}
}
