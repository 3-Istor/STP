package bom

import (
	"errors"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"bomstudio/internal/catalog"
	"bomstudio/internal/money"
	"bomstudio/internal/pricing"
)

// TestMakeOrBuy : QUAND je lance la simulation sur un sous-ensemble ALORS les deux coûts, l'écart et le volume
// d'équilibre sont affichés (make-or-buy, ADR 0026). Volume 1 000, sous-ensemble « Freins » × 2 par voiture,
// une pièce Disque × 2 par frein (20 € rendu : 18 achat + 0 douane + 1 transport + 1 emballage ; 0,5 h à 30 €/h).
func TestMakeOrBuy(t *testing.T) {
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	mny := func(a, c string) money.Money {
		v, err := money.Parse(a, c)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	mass, hours, rate := dec("1"), dec("0.5"), mny("30", "EUR")
	cat, err := catalog.NewStub([]catalog.Part{{ID: 1, Name: "Disque", MassKG: &mass, LaborHours: &hours, HourlyRate: &rate}},
		[]catalog.Offer{{ID: 10, PartID: 1, Currency: "EUR", OriginCountry: "PL"}})
	if err != nil {
		t.Fatal(err)
	}
	prices := pricing.NewStub([]pricing.Quote{{OfferID: 10, Price: mny("18", "EUR"), Source: "sim", QuotedAt: at}}, nil)
	logi := fakeLogistics{
		duties:    map[[2]string]decimal.Decimal{{"PL", "FR"}: dec("0"), {"CN", "FR"}: dec("10")},
		freight:   map[[2]string]money.Money{{"PL", "FR"}: mny("1", "EUR"), {"CN", "FR"}: mny("2", "EUR")},
		packaging: map[int64]money.Money{10: mny("1", "EUR")},
	}
	svc := NewService(newMemStore(), cat, prices).WithLogistics(logi)
	ctx := t.Context()
	fr := "FR"
	b, err := svc.CreateBOM(ctx, NewBOM{Name: "B", Currency: "EUR", Volume: 1000, DestinationCountry: &fr})
	if err != nil {
		t.Fatal(err)
	}
	_, nodes, _ := svc.Tree(ctx, b.ID)
	brakes, err := svc.AddNode(ctx, b.ID, NewNode{ParentID: nodes[0].ID, Name: "Freins", Quantity: dec("2")})
	if err != nil {
		t.Fatal(err)
	}
	pid, oid := int64(1), int64(10)
	if _, err := svc.AddNode(ctx, b.ID, NewNode{ParentID: brakes.ID, PartID: &pid, OfferID: &oid, Quantity: dec("2")}); err != nil {
		t.Fatal(err)
	}
	req := MakeOrBuyRequest{
		FixedCosts: mny("120000", "EUR"), LogisticsPerUnit: mny("2.5", "EUR"),
		Tier1Price: mny("80", "EUR"), Tier1Origin: "CN", Tier1Packaging: mny("1", "EUR"),
	}
	got, err := svc.MakeOrBuy(ctx, b.ID, brakes.ID, req)
	if err != nil {
		t.Fatal(err)
	}
	if got.Err != nil {
		t.Fatal(got.Err)
	}
	// Par voiture : 4 disques. Fabriquer = 120 000 / 1 000 (120) + 4 × 20 (80) + 4 × 0,5 h × 30 € (60) + 2 × 2,5 (5) = 265.
	// Acheter = 2 × (80 + 10 % de 80 + 2 kg × 2 € + 1) = 2 × 93 = 186. Écart 79 ; variable 145 ; marge 41 ;
	// équilibre ⌈120 000 ÷ 41⌉ = 2 927.
	eq := func(label string, m *money.Money, want string) {
		if m == nil || !m.Amount().Equal(dec(want)) {
			t.Errorf("%s %v, want %s", label, m, want)
		}
	}
	eq("make per car", got.MakePerCar, "265")
	eq("buy per car", got.BuyPerCar, "186")
	eq("difference", got.Difference, "79")
	if got.BreakEvenVolume == nil || *got.BreakEvenVolume != 2927 {
		t.Errorf("break-even %v, want 2927", got.BreakEvenVolume)
	}

	if _, err := svc.MakeOrBuy(ctx, b.ID, brakes.ID+1, req); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("simulating a part: err %v, want ErrInvalidInput", err)
	}
	bad := req
	bad.Tier1Origin = "china"
	if _, err := svc.MakeOrBuy(ctx, b.ID, brakes.ID, bad); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("bad origin: err %v, want ErrInvalidInput", err)
	}
	// Sans douane pour l'origine Tier 1 : pas de coût d'achat inventé, l'erreur est nommée.
	noDuty := req
	noDuty.Tier1Origin = "US"
	got, err = svc.MakeOrBuy(ctx, b.ID, brakes.ID, noDuty)
	if err != nil {
		t.Fatal(err)
	}
	if got.BuyPerCar != nil || got.BreakEvenVolume != nil || !errors.Is(got.Err, pricing.ErrNoCustomsDuty) {
		t.Errorf("buy %v break-even %v err %v, want none and ErrNoCustomsDuty", got.BuyPerCar, got.BreakEvenVolume, got.Err)
	}

	// Sous-ensemble vide : aucun coût de fabrication ni masse supposés nuls.
	empty, err := svc.AddNode(ctx, b.ID, NewNode{ParentID: nodes[0].ID, Name: "Vide", Quantity: dec("1")})
	if err != nil {
		t.Fatal(err)
	}
	got, err = svc.MakeOrBuy(ctx, b.ID, empty.ID, req)
	if err != nil {
		t.Fatal(err)
	}
	if got.MakePerCar != nil || got.BreakEvenVolume != nil || !errors.Is(got.Err, ErrEmptySubAssembly) {
		t.Errorf("empty sub-assembly: make %v break-even %v err %v, want ErrEmptySubAssembly", got.MakePerCar, got.BreakEvenVolume, got.Err)
	}
	// Coûts fixes en USD sans taux : fabriquer indisponible, acheter reste donné.
	usd := req
	usd.FixedCosts = mny("1000", "USD")
	got, err = svc.MakeOrBuy(ctx, b.ID, brakes.ID, usd)
	if err != nil {
		t.Fatal(err)
	}
	if got.MakePerCar != nil || got.BuyPerCar == nil || !errors.Is(got.Err, pricing.ErrNoRate) {
		t.Errorf("fixed costs in USD without rate: make %v buy %v err %v", got.MakePerCar, got.BuyPerCar, got.Err)
	}
	neg := req
	neg.LogisticsPerUnit = mny("-1", "EUR")
	if _, err := svc.MakeOrBuy(ctx, b.ID, brakes.ID, neg); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("negative amount: err %v, want ErrInvalidInput", err)
	}
}

// Sous-ensemble imbriqué à quantité > 1 et pièce parente d'une autre pièce (ADR 0008) : chaque pièce est
// comptée une fois, à sa quantité réelle.
func TestMakeOrBuyNested(t *testing.T) {
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	mny := func(a, c string) money.Money {
		v, err := money.Parse(a, c)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	one, h, rate := dec("1"), dec("0.5"), mny("30", "EUR")
	cat, err := catalog.NewStub([]catalog.Part{
		{ID: 1, Name: "Roue", MassKG: &one, LaborHours: &h, HourlyRate: &rate},
		{ID: 2, Name: "Écrou", MassKG: &one, LaborHours: &h, HourlyRate: &rate},
	}, []catalog.Offer{{ID: 10, PartID: 1, Currency: "EUR", OriginCountry: "PL"}, {ID: 20, PartID: 2, Currency: "EUR", OriginCountry: "PL"}})
	if err != nil {
		t.Fatal(err)
	}
	prices := pricing.NewStub([]pricing.Quote{
		{OfferID: 10, Price: mny("18", "EUR"), Source: "sim", QuotedAt: at}, {OfferID: 20, Price: mny("0", "EUR"), Source: "sim", QuotedAt: at},
	}, nil)
	logi := fakeLogistics{
		duties:    map[[2]string]decimal.Decimal{{"PL", "FR"}: dec("0"), {"CN", "FR"}: dec("0")},
		freight:   map[[2]string]money.Money{{"PL", "FR"}: mny("1", "EUR"), {"CN", "FR"}: mny("2", "EUR")},
		packaging: map[int64]money.Money{10: mny("1", "EUR"), 20: mny("1", "EUR")},
	}
	svc := NewService(newMemStore(), cat, prices).WithLogistics(logi)
	ctx := t.Context()
	fr := "FR"
	b, _ := svc.CreateBOM(ctx, NewBOM{Name: "B", Currency: "EUR", Volume: 1, DestinationCountry: &fr})
	_, nodes, _ := svc.Tree(ctx, b.ID)
	add := func(n NewNode) Node {
		out, err := svc.AddNode(ctx, b.ID, n)
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	a := add(NewNode{ParentID: nodes[0].ID, Name: "A", Quantity: dec("2")})
	s := add(NewNode{ParentID: a.ID, Name: "S", Quantity: dec("3")})
	p1, o1, p2, o2 := int64(1), int64(10), int64(2), int64(20)
	wheel := add(NewNode{ParentID: s.ID, PartID: &p1, OfferID: &o1, Quantity: dec("4")})
	add(NewNode{ParentID: wheel.ID, PartID: &p2, OfferID: &o2, Quantity: dec("5")})
	got, err := svc.MakeOrBuy(ctx, b.ID, s.ID, MakeOrBuyRequest{
		FixedCosts: mny("0", "EUR"), LogisticsPerUnit: mny("0", "EUR"), Tier1Price: mny("0", "EUR"),
		Tier1Origin: "CN", Tier1Packaging: mny("0", "EUR"),
	})
	if err != nil {
		t.Fatal(err)
	}
	// S : 6 par voiture ; roues 24 × (18 + 0 + 1 + 1) = 480 ; écrous 120 × (0 + 0 + 1 + 1) = 240 ;
	// main-d'œuvre 144 × 0,5 h × 30 € = 2 160 ; transport Tier 1 = 144 kg × 2 € = 288.
	if p := got.Make[0].Amount; p == nil || !p.Amount().Equal(dec("720")) {
		t.Errorf("parts landed %v, want 720", p)
	}
	if l := got.Make[1].Amount; l == nil || !l.Amount().Equal(dec("2160")) {
		t.Errorf("labor %v, want 2160", l)
	}
	if f := got.Buy[2].Amount; f == nil || !f.Amount().Round(10).Equal(dec("288")) {
		t.Errorf("tier1 freight %v, want 288", f)
	}
}
