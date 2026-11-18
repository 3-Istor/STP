package bom

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"bomstudio/internal/catalog"
	"bomstudio/internal/money"
	"bomstudio/internal/pricing"
)

func usdMoney(t *testing.T, a string) money.Money {
	t.Helper()
	m, err := money.Parse(a, "USD")
	if err != nil {
		t.Fatal(err)
	}
	return m
}

// trailFrom : la cotation courante est exclue du passé, la fenêtre de 12 mois est respectée, une cotation dans
// une autre devise est écartée mais comptée (jamais convertie, jamais silencieuse).
func TestTrailFrom(t *testing.T) {
	at := time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)
	current := pricing.Quote{ID: 9, OfferID: 1, Price: usdMoney(t, "120"), QuotedAt: at}
	eur, _ := money.Parse("100", "EUR")
	hist := []pricing.HistoricalQuote{
		{ID: 1, Price: usdMoney(t, "100"), QuotedAt: at.AddDate(0, -13, 0)}, // hors fenêtre : la référence d'avant
		{ID: 2, Price: usdMoney(t, "101"), QuotedAt: at.AddDate(0, -6, 0)},
		{ID: 3, Price: eur, QuotedAt: at.AddDate(0, -5, 0)},
		{ID: 9, Price: usdMoney(t, "120"), QuotedAt: at}, // la cotation courante elle-même
	}
	got := trailFrom(current, hist)
	if len(got.Past) != 1 || !got.Past[0].Amount().Equal(dec("101")) || got.OtherCurrency != 1 || !got.Current.Amount().Equal(dec("120")) {
		t.Fatalf("trail %+v, want one USD past quote (101), one other-currency, current 120", got)
	}
}

// countingHistory compte les lectures : l'historique de toutes les offres retenues se lit en une fois.
type countingHistory struct {
	calls   int
	windows int
	past    func(offerID int64, at time.Time) []pricing.HistoricalQuote
}

func (c *countingHistory) ReadHistory(context.Context, int64, time.Time, time.Time) ([]pricing.HistoricalQuote, error) {
	return nil, errors.New("one query per offer: use ReadWindows")
}

func (c *countingHistory) ReadWindows(_ context.Context, ws []pricing.HistoryWindow) (map[int64][]pricing.HistoricalQuote, error) {
	c.calls++
	c.windows += len(ws)
	out := map[int64][]pricing.HistoricalQuote{}
	for _, w := range ws {
		out[w.OfferID] = c.past(w.OfferID, w.To)
	}
	return out, nil
}

func TestHistoryAnomaliesReadTrailsInOneQuery(t *testing.T) {
	at := time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)
	var parts []catalog.Part
	var offers []catalog.Offer
	var quotes []pricing.Quote
	var listed []catalog.PartWithOffers
	for id := int64(1); id <= 3; id++ {
		p := catalog.Part{ID: id, Name: fmt.Sprintf("P%d", id), Category: "Freins"}
		o := catalog.Offer{ID: id * 10, PartID: id, Currency: "EUR"}
		m, _ := money.Parse("100", "EUR")
		parts, offers = append(parts, p), append(offers, o)
		quotes = append(quotes, pricing.Quote{ID: id, OfferID: o.ID, Price: m, Source: "sim", QuotedAt: at})
		listed = append(listed, catalog.PartWithOffers{Part: p, Offers: []catalog.Offer{o}})
	}
	cat, err := catalog.NewStub(parts, offers)
	if err != nil {
		t.Fatal(err)
	}
	hist := &countingHistory{past: func(offerID int64, at time.Time) []pricing.HistoricalQuote {
		var h []pricing.HistoricalQuote
		for k := 1; k <= 5; k++ {
			m, _ := money.Parse("100", "EUR")
			h = append(h, pricing.HistoricalQuote{ID: 100 + int64(k), OfferID: offerID, Price: m, QuotedAt: at.AddDate(0, 0, -k*10)})
		}
		return h
	}}
	svc := NewService(newMemStore(), cat, pricing.NewStub(quotes, nil)).WithCatalogListing(fakeListing{parts: listed}).WithAnalysisSources(hist, nil)
	ctx := t.Context()
	b, err := svc.CreateBOM(ctx, NewBOM{Name: "B", Currency: "EUR", Volume: 1})
	if err != nil {
		t.Fatal(err)
	}
	_, nodes, _ := svc.Tree(ctx, b.ID)
	for id := int64(1); id <= 3; id++ {
		partID, offerID := id, id*10
		if _, err := svc.AddNode(ctx, b.ID, NewNode{ParentID: nodes[0].ID, PartID: &partID, OfferID: &offerID, Quantity: dec("1")}); err != nil {
			t.Fatal(err)
		}
	}
	got, err := svc.PartAnomalies(ctx, b.ID, AnomalyHistory, nil)
	if err != nil {
		t.Fatal(err)
	}
	if hist.calls != 1 || hist.windows != 3 {
		t.Fatalf("history reads: %d calls for %d windows, want 1 call for 3", hist.calls, hist.windows)
	}
	for _, p := range got.Parts {
		if p.Status != pricing.AnomalyNormal {
			t.Errorf("part %d: %s (%v), want normal", p.PartID, p.Status, p.Err)
		}
	}
	// Sans source d'historique : erreur nommée, pas un zéro.
	bare := NewService(newMemStore(), cat, pricing.NewStub(quotes, nil)).WithCatalogListing(fakeListing{parts: listed})
	b2, _ := bare.CreateBOM(ctx, NewBOM{Name: "B", Currency: "EUR", Volume: 1})
	if _, err := bare.PartAnomalies(ctx, b2.ID, AnomalyHistory, nil); !errors.Is(err, ErrSuggestionsUnavailable) {
		t.Fatalf("history nil: %v, want ErrSuggestionsUnavailable", err)
	}
}

// Méthode « offres » : une offre retenue archivée n'est pas comparée et le dit ; une pièce archivée est nommée
// archivée, pas « 0 of 2 required ».
func TestOfferAnomaliesNameArchivedOfferAndPart(t *testing.T) {
	at := time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)
	gone := at.AddDate(0, -1, 0)
	p1, p2 := catalog.Part{ID: 1, Name: "P1", Category: "Freins"}, catalog.Part{ID: 2, Name: "P2", Category: "Freins"}
	o10, o11 := catalog.Offer{ID: 10, PartID: 1, Currency: "EUR"}, catalog.Offer{ID: 11, PartID: 1, Currency: "EUR", ArchivedAt: &gone}
	o20, o21 := catalog.Offer{ID: 20, PartID: 2, Currency: "EUR"}, catalog.Offer{ID: 21, PartID: 2, Currency: "EUR"}
	cat, err := catalog.NewStub([]catalog.Part{p1, p2}, []catalog.Offer{o10, o11, o20, o21})
	if err != nil {
		t.Fatal(err)
	}
	var quotes []pricing.Quote
	for i, o := range []catalog.Offer{o10, o11, o20, o21} {
		m, _ := money.Parse("100", "EUR")
		quotes = append(quotes, pricing.Quote{ID: int64(i + 1), OfferID: o.ID, Price: m, Source: "sim", QuotedAt: at})
	}
	// La pièce 2 est archivée : le listing ne rend que les pièces actives.
	listing := fakeListing{parts: []catalog.PartWithOffers{{Part: p1, Offers: []catalog.Offer{o10, o11}}}}
	svc := NewService(newMemStore(), cat, pricing.NewStub(quotes, nil)).WithCatalogListing(listing)
	ctx := t.Context()
	b, _ := svc.CreateBOM(ctx, NewBOM{Name: "B", Currency: "EUR", Volume: 1})
	_, nodes, _ := svc.Tree(ctx, b.ID)
	for _, pick := range [][2]int64{{1, 11}, {2, 20}} {
		partID, offerID := pick[0], pick[1]
		if _, err := svc.AddNode(ctx, b.ID, NewNode{ParentID: nodes[0].ID, PartID: &partID, OfferID: &offerID, Quantity: dec("1")}); err != nil {
			t.Fatal(err)
		}
	}
	got, err := svc.PartAnomalies(ctx, b.ID, AnomalyOffers, nil)
	if err != nil {
		t.Fatal(err)
	}
	by := map[int64]PartAnomaly{}
	for _, p := range got.Parts {
		by[p.PartID] = p
	}
	if !errors.Is(by[1].Err, pricing.ErrTooFewOffers) || !strings.Contains(by[1].Err.Error(), "1 archived") {
		t.Errorf("part 1 reason %v, want ErrTooFewOffers naming 1 archived offer", by[1].Err)
	}
	if !errors.Is(by[2].Err, pricing.ErrPartArchived) {
		t.Errorf("part 2 reason %v, want ErrPartArchived", by[2].Err)
	}
}

// Une méthode d'anomalie dit en un seul endroit son détecteur, son seuil et ce qu'elle lit en plus.
func TestAnomalySpecNeeds(t *testing.T) {
	svc := NewService(newMemStore(), nil, nil)
	tests := []struct {
		method AnomalyMethod
		want   anomalyNeeds
	}{
		{AnomalyMedian, anomalyNeeds{}},
		{AnomalyPerKg, anomalyNeeds{}},
		{AnomalyOffers, anomalyNeeds{offers: true}},
		{AnomalyHistory, anomalyNeeds{history: true}},
		{AnomalyMaterial, anomalyNeeds{family: true}},
	}
	for _, tt := range tests {
		spec, err := svc.anomalySpecFor(tt.method)
		if err != nil || spec.needs != tt.want || spec.detector == nil {
			t.Errorf("%s: %+v (%v), want needs %+v", tt.method, spec, err, tt.want)
		}
	}
	if _, err := svc.anomalySpecFor("nope"); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("unknown method: %v", err)
	}
}
