package bom

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"bomstudio/internal/catalog"
	"bomstudio/internal/money"
	"bomstudio/internal/pricing"
)

// TestCostAnomalies : le prix unitaire de l'offre choisie dans la BOM, converti dans la devise de la BOM,
// comparé à la médiane des pièces de la catégorie à leur offre la moins chère (ADR 0027).
func TestCostAnomalies(t *testing.T) {
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	var parts []catalog.Part
	var offers []catalog.Offer
	var quotes []pricing.Quote
	var listed []catalog.PartWithOffers
	addPart := func(id int64, category string, prices ...string) {
		p := catalog.Part{ID: id, Name: fmt.Sprintf("P%d", id), Category: category}
		pw := catalog.PartWithOffers{Part: p}
		for i, amount := range prices {
			o := catalog.Offer{ID: id*10 + int64(i), PartID: id, Currency: "USD"}
			m, err := money.Parse(amount, "USD")
			if err != nil {
				t.Fatal(err)
			}
			offers = append(offers, o)
			pw.Offers = append(pw.Offers, o)
			quotes = append(quotes, pricing.Quote{OfferID: o.ID, Price: m, Source: "sim", QuotedAt: at})
		}
		parts = append(parts, p)
		listed = append(listed, pw)
	}
	// Freins : médiane des moins chères = 100 USD. La pièce 1 est choisie à son offre chère (400 USD).
	addPart(1, "Freins", "100", "400")
	addPart(2, "Freins", "90")
	addPart(3, "Freins", "100")
	addPart(4, "Freins", "110")
	addPart(5, "Freins", "120") // offre 50 : illisible dans le second cas
	addPart(6, "Vis", "1")      // catégorie de 1 pièce : non évaluable
	cat, err := catalog.NewStub(parts, offers)
	if err != nil {
		t.Fatal(err)
	}
	prices := pricing.NewStub(quotes, []pricing.Rate{{From: "USD", To: "EUR", Rate: dec("0.5"), Source: "sim", QuotedAt: at}})
	svc := NewService(newMemStore(), cat, prices).WithCatalogListing(fakeListing{parts: listed})
	ctx := t.Context()
	b, err := svc.CreateBOM(ctx, NewBOM{Name: "B", Currency: "EUR", Volume: 1})
	if err != nil {
		t.Fatal(err)
	}
	_, nodes, _ := svc.Tree(ctx, b.ID)
	add := func(partID, offerID int64) int64 {
		n, err := svc.AddNode(ctx, b.ID, NewNode{ParentID: nodes[0].ID, PartID: &partID, OfferID: &offerID, Quantity: dec("4")})
		if err != nil {
			t.Fatal(err)
		}
		return n.ID
	}
	expensive, cheap, screw := add(1, 11), add(2, 20), add(6, 60)

	got, err := svc.CostAnomalies(ctx, b.ID, pricing.DefaultAnomalyThreshold)
	if err != nil {
		t.Fatal(err)
	}
	byNode := map[int64]NodeAnomaly{}
	for _, n := range got.Nodes {
		byNode[n.NodeID] = n
	}
	tests := []struct {
		node       int64
		wantStatus pricing.AnomalyStatus
		wantCost   string
		wantErr    error
	}{
		{expensive, pricing.AnomalyDetected, "200.00 EUR", nil}, // 400 USD × 0,5 ; médiane 50 EUR ; 4×
		{cheap, pricing.AnomalyNormal, "45.00 EUR", nil},
		{screw, pricing.AnomalyNotEvaluable, "0.50 EUR", pricing.ErrTooFewPeers},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("node %d", tt.node), func(t *testing.T) {
			a := byNode[tt.node]
			if a.Status != tt.wantStatus || !errors.Is(a.Err, tt.wantErr) {
				t.Fatalf("status %s err %v, want %s %v", a.Status, a.Err, tt.wantStatus, tt.wantErr)
			}
			if a.Cost == nil || a.Cost.Round().String() != tt.wantCost {
				t.Fatalf("unit cost %v, want %s", a.Cost, tt.wantCost)
			}
		})
	}
	if r := byNode[expensive].Ratio; r == nil || !r.Equal(dec("4")) {
		t.Fatalf("ratio %v, want 4", r)
	}
	if len(got.Rates) != 1 || got.Rates[0].From != "USD" {
		t.Fatalf("rates used %+v, want USD→EUR", got.Rates)
	}

	// Revue ADR 0027 : une offre de la population illisible côté prix écarte sa pièce (comptée dans Missing)
	// sans faire échouer la requête.
	broken := &brokenOffer{Pricing: prices, offerID: 50, err: fmt.Errorf("offer 50: %w", pricing.ErrInvalidHistoryData)}
	svc2 := NewService(svc.store, cat, broken).WithCatalogListing(fakeListing{parts: listed})
	got, err = svc2.CostAnomalies(ctx, b.ID, pricing.DefaultAnomalyThreshold)
	if err != nil {
		t.Fatalf("an unreadable population offer fails the whole request: %v", err)
	}
	for _, n := range got.Nodes {
		if n.NodeID == expensive && (n.Status != pricing.AnomalyNotEvaluable || n.Missing != 1) {
			t.Fatalf("node %d: status %s missing %d, want not evaluable (4 of 5) with 1 missing", n.NodeID, n.Status, n.Missing)
		}
	}

	// Une BOM aux prix figés n'a pas la population dans son snapshot : non évaluable, nommément.
	if _, err := svc.FreezePrices(ctx, b.ID, at); err != nil {
		t.Fatal(err)
	}
	got, err = svc.CostAnomalies(ctx, b.ID, pricing.DefaultAnomalyThreshold)
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range got.Nodes {
		if n.Status != pricing.AnomalyNotEvaluable || !errors.Is(n.Err, pricing.ErrPopulationNotFrozen) {
			t.Fatalf("frozen bom node %d: %s %v, want ErrPopulationNotFrozen", n.NodeID, n.Status, n.Err)
		}
	}
}

// brokenOffer : source de prix dont une offre renvoie une erreur autre que ErrNoQuote.
type brokenOffer struct {
	pricing.Pricing
	offerID int64
	err     error
}

func (b *brokenOffer) OfferPrices(ctx context.Context, ids []int64) (map[int64]pricing.QuoteResult, error) {
	out, err := b.Pricing.OfferPrices(ctx, ids)
	if err != nil {
		return nil, err
	}
	if _, asked := out[b.offerID]; asked {
		out[b.offerID] = pricing.QuoteResult{Err: b.err}
	}
	return out, nil
}
