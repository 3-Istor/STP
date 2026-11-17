package bom

import (
	"context"
	"errors"
	"testing"
	"time"

	"bomstudio/internal/catalog"
	"bomstudio/internal/money"
	"bomstudio/internal/pricing"
	"bomstudio/internal/supplier"
)

type fakeHistory map[int64][]pricing.HistoricalQuote

func (f fakeHistory) ReadHistory(_ context.Context, offerID int64, _, _ time.Time) ([]pricing.HistoricalQuote, error) {
	return f[offerID], nil
}

type fakeSuppliers []supplier.Supplier

func (f fakeSuppliers) ListSuppliers(context.Context) ([]supplier.Supplier, error) { return f, nil }

// TestAnalysesThroughTheService : les trois analyses lisent le brouillon par le service (catalogue, prix,
// budgets, historique, fournisseurs, contrats) et chiffrent avec son moteur ; une source absente → nommée.
func TestAnalysesThroughTheService(t *testing.T) {
	at := time.Date(2026, 4, 7, 12, 0, 0, 0, time.UTC)
	ten, one, mass := 10, dec("1"), dec("2")
	door := catalog.Part{ID: 1, Name: "Porte", Category: "Carrosserie", CO2KGPerUnit: &one, MassKG: &mass, ProductionDays: &ten}
	screw := catalog.Part{ID: 2, Name: "Vis", Category: "Fixations", CO2KGPerUnit: &one, MassKG: &one, ProductionDays: &ten}
	offers := []catalog.Offer{
		{ID: 10, PartID: 1, SupplierID: 1, Currency: "GBP", OriginCountry: "GB"},
		{ID: 20, PartID: 2, SupplierID: 2, Currency: "EUR", OriginCountry: "IT"},
		{ID: 21, PartID: 2, SupplierID: 1, Currency: "GBP", OriginCountry: "GB"},
	}
	cat, err := catalog.NewStub([]catalog.Part{door, screw}, offers)
	if err != nil {
		t.Fatal(err)
	}
	q := func(id int64, amount, cur string, when time.Time) pricing.Quote {
		m, _ := money.Parse(amount, cur)
		return pricing.Quote{OfferID: id, Price: m, Source: "sim", QuotedAt: when}
	}
	prices := pricing.NewStub([]pricing.Quote{q(10, "100", "GBP", at), q(20, "2", "EUR", at), q(21, "3", "GBP", at)},
		[]pricing.Rate{{From: "GBP", To: "EUR", Rate: dec("1.2"), Source: "sim", QuotedAt: at}})
	hist := fakeHistory{}
	for _, qq := range []pricing.Quote{q(10, "90", "GBP", at.AddDate(0, -2, 0)), q(10, "100", "GBP", at), q(20, "2", "EUR", at)} {
		hist[qq.OfferID] = append(hist[qq.OfferID], pricing.HistoricalQuote{ID: int64(len(hist[qq.OfferID]) + 1), OfferID: qq.OfferID,
			Price: qq.Price, Source: "sim", QuotedAt: qq.QuotedAt})
	}
	store := newMemStore()
	listing := fakeListing{parts: []catalog.PartWithOffers{{Part: door, Offers: offers[:1]}, {Part: screw, Offers: offers[1:]}}}
	svc := NewService(store, cat, prices).WithCatalogListing(listing).
		WithCostSources(nil, fakeContracts{1: {{ID: 7, SupplierID: 1, StartsOn: "2026-01-01", EndsOn: "2026-03-31", OfferIDs: []int64{10},
			Tiers: []supplier.Tier{{MinQuantity: dec("1"), DiscountPercent: dec("3")}}}}}).
		WithAnalysisSources(hist, fakeSuppliers{{ID: 1, Name: "Arden"}, {ID: 2, Name: "Valsusa"}})
	ctx := t.Context()
	b, err := svc.CreateBOM(ctx, NewBOM{Name: "B", Currency: "EUR", Volume: 100})
	if err != nil {
		t.Fatal(err)
	}
	_, nodes, _ := svc.Tree(ctx, b.ID)
	addPart := func(part, offer int64, qty string) {
		if _, err := svc.AddNode(ctx, b.ID, NewNode{ParentID: nodes[0].ID, PartID: &part, OfferID: &offer, Quantity: dec(qty)}); err != nil {
			t.Fatal(err)
		}
	}
	addPart(1, 10, "4")  // 100 GBP × 4 × 1.2 = 480
	addPart(2, 20, "10") // 20
	store.budgets = map[int64][]TargetBudget{b.ID: {{NodeID: nodes[0].ID, Amount: dec("500"), Currency: "EUR"}}}

	fx, err := svc.FXSensitivity(ctx, b.ID, dec("5"))
	if err != nil {
		t.Fatal(err)
	}
	// GBP : 480 sur 500 → choc +24, 524 ÷ 500 = 104.8 %.
	if len(fx.Currencies) != 1 || !fx.Currencies[0].Impact.Amount().Equal(dec("24")) ||
		fx.Currencies[0].BudgetPercentAfter.StringFixed(1) != "104.8" || fx.Budget == nil {
		t.Fatalf("fx %+v", fx)
	}

	drivers, err := svc.PriceDrivers(ctx, b.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	// Porte : (100 − 90) × 4 × 1.2 = 48 ; vis : une seule cotation, amplitude 0.
	if len(drivers.Drivers) != 2 || drivers.Drivers[0].PartID != 1 || !drivers.Drivers[0].Amplitude.Amount().Equal(dec("48")) ||
		!drivers.WindowEnd.Equal(at) {
		t.Fatalf("drivers %+v", drivers)
	}

	src, err := svc.Sourcing(ctx, b.ID)
	if err != nil {
		t.Fatal(err)
	}
	if src.ByCountry[0].Key != "GB" || len(src.Suppliers) != 2 || src.Suppliers[0].Name != "Arden" ||
		src.Suppliers[0].Contract.Status != "expired" || len(src.MonoSource) != 1 || src.MonoSource[0].PartID != 1 ||
		len(src.ExpiredContracts) != 1 || !src.ExpiredContracts[0].LostAmount.Amount().Equal(dec("14.4")) {
		t.Fatalf("sourcing %+v", src)
	}
	// Taux budget non câblés (costParams nil) : l'écart est nommé, pas inventé.
	if len(src.Rates) != 1 || !errors.Is(src.Rates[0].Err, ErrNoBudgetRate) {
		t.Fatalf("rates %+v", src.Rates)
	}

	bare := NewService(store, cat, prices)
	if _, err := bare.PriceDrivers(ctx, b.ID, 10); !errors.Is(err, ErrAnalysisUnavailable) {
		t.Fatalf("no history: %v", err)
	}
	if _, err := bare.Sourcing(ctx, b.ID); !errors.Is(err, ErrAnalysisUnavailable) {
		t.Fatalf("no suppliers: %v", err)
	}
	if _, err := svc.PriceDrivers(ctx, b.ID, 0); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("limit 0: %v", err)
	}
}
