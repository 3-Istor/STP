package bom

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"bomstudio/internal/catalog"
	"bomstudio/internal/money"
	"bomstudio/internal/pricing"
)

// TestPartAnomalies : prix au kg par famille (ADR 0037), surcoût × quantité réelle, classement par impact,
// et la même BOM jugée par l'ancienne méthode (ADR 0027), toujours disponible.
func TestPartAnomalies(t *testing.T) {
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	var parts []catalog.Part
	var offers []catalog.Offer
	var quotes []pricing.Quote
	var listed []catalog.PartWithOffers
	add := func(id int64, mass, price string) {
		p := catalog.Part{ID: id, Name: fmt.Sprintf("F%d", id), Category: "Faisceaux"}
		if mass != "" {
			m := dec(mass)
			p.MassKG = &m
		}
		o := catalog.Offer{ID: id * 10, PartID: id, Currency: "EUR"}
		m, err := money.Parse(price, "EUR")
		if err != nil {
			t.Fatal(err)
		}
		parts, offers = append(parts, p), append(offers, o)
		quotes = append(quotes, pricing.Quote{OfferID: o.ID, Price: m, Source: "sim", QuotedAt: at})
		listed = append(listed, catalog.PartWithOffers{Part: p, Offers: []catalog.Offer{o}})
	}
	// Médiane €/kg de la famille : 18 (pièces 1 à 5, et 6 à 36 €/kg). Pièce 7 sans masse.
	add(1, "0.1", "1.80")
	add(2, "0.5", "9")
	add(3, "1", "18")
	add(4, "2", "36")
	add(5, "10", "180") // lourde, au prix de la famille : la médiane du prix unitaire la juge anormale
	add(6, "1", "36")   // 2 × la médiane au kg
	add(7, "", "5")
	cat, err := catalog.NewStub(parts, offers)
	if err != nil {
		t.Fatal(err)
	}
	svc := NewService(newMemStore(), cat, pricing.NewStub(quotes, nil)).WithCatalogListing(fakeListing{parts: listed})
	ctx := t.Context()
	b, err := svc.CreateBOM(ctx, NewBOM{Name: "B", Currency: "EUR", Volume: 1})
	if err != nil {
		t.Fatal(err)
	}
	_, nodes, _ := svc.Tree(ctx, b.ID)
	group, err := svc.AddNode(ctx, b.ID, NewNode{ParentID: nodes[0].ID, Name: "Portes", Quantity: dec("2")})
	if err != nil {
		t.Fatal(err)
	}
	node := func(parent, partID int64, qty string) {
		offerID := partID * 10
		if _, err := svc.AddNode(ctx, b.ID, NewNode{ParentID: parent, PartID: &partID, OfferID: &offerID, Quantity: dec(qty)}); err != nil {
			t.Fatal(err)
		}
	}
	node(group.ID, 6, "1")    // 2 par voiture
	node(nodes[0].ID, 6, "1") // + 1 : 3 par voiture
	node(nodes[0].ID, 5, "1") // lourde, normale au kg
	node(nodes[0].ID, 3, "1") // pile la médiane
	node(nodes[0].ID, 7, "1") // sans masse

	tests := []struct {
		name      string
		method    AnomalyMethod
		threshold *decimal.Decimal
		want      []string // "pièce:statut:impact" dans l'ordre
	}{
		// Pièce 6 : (36 − 18 × 1) × 3 = 54 ; pièce 5 : 180 − 18 × 10 = 0 ; pièce 3 : 0 ; pièce 7 : sans masse.
		{"€/kg (défaut)", AnomalyPerKg, nil, []string{"6:anomaly:54", "3:normal:0", "5:normal:0", "7:not_evaluable:-"}},
		// Médiane des prix unitaires = 18 (1.80, 9, 18, 36, 36, 180, 5 → 18) ; pièce 5 : 180 ≥ 3 × 18.
		{"médiane du prix unitaire (ADR 0027)", AnomalyMedian, nil, []string{"5:anomaly:162", "6:normal:54", "3:normal:0", "7:normal:-13"}},
		{"€/kg au seuil 3 : la pièce 6 redevient normale", AnomalyPerKg, ptrDec("3"),
			[]string{"6:normal:54", "3:normal:0", "5:normal:0", "7:not_evaluable:-"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := svc.PartAnomalies(ctx, b.ID, tt.method, tt.threshold)
			if err != nil {
				t.Fatal(err)
			}
			var out []string
			for _, p := range got.Parts {
				impact := "-"
				if p.Impact != nil {
					impact = p.Impact.Amount().String()
				}
				out = append(out, fmt.Sprintf("%d:%s:%s", p.PartID, p.Status, impact))
			}
			if fmt.Sprint(out) != fmt.Sprint(tt.want) {
				t.Fatalf("parts %v\nwant  %v", out, tt.want)
			}
		})
	}
	got, _ := svc.PartAnomalies(ctx, b.ID, AnomalyPerKg, nil)
	if p := got.Parts[0]; len(p.NodeIDs) != 2 || !p.Quantity.Equal(dec("3")) || p.Name != "F6" ||
		!p.PerKg.Amount().Equal(dec("36")) || !p.MedianPerKg.Amount().Equal(dec("18")) {
		t.Fatalf("part 6 %+v", p)
	}
	if p := got.Parts[3]; !errors.Is(p.Err, pricing.ErrNoMass) {
		t.Fatalf("part 7 reason %v, want ErrNoMass", p.Err)
	}
	if _, err := svc.PartAnomalies(ctx, b.ID, "ai", nil); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("unknown method: %v", err)
	}
}

func ptrDec(s string) *decimal.Decimal { d := dec(s); return &d }
