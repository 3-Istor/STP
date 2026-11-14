package bom

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"bomstudio/internal/catalog"
	"bomstudio/internal/money"
	"bomstudio/internal/pricing"
)

// fakeLogistics : tables en mémoire pour LandedCosts.
type fakeLogistics struct {
	pricing.LogisticsStore
	duties    map[[2]string]decimal.Decimal
	freight   map[[2]string]money.Money
	packaging map[int64]money.Money
}

func (f fakeLogistics) CustomsDuty(_ context.Context, o, d string) (pricing.CustomsDuty, error) {
	p, ok := f.duties[[2]string{o, d}]
	if !ok {
		return pricing.CustomsDuty{}, fmt.Errorf("%s→%s: %w", o, d, pricing.ErrNoCustomsDuty)
	}
	return pricing.CustomsDuty{Origin: o, Destination: d, Percent: p}, nil
}

func (f fakeLogistics) FreightRate(_ context.Context, o, d string) (pricing.FreightRate, error) {
	m, ok := f.freight[[2]string{o, d}]
	if !ok {
		return pricing.FreightRate{}, fmt.Errorf("%s→%s: %w", o, d, pricing.ErrNoFreightRate)
	}
	return pricing.FreightRate{Origin: o, Destination: d, PerKg: m}, nil
}

func (f fakeLogistics) OfferPackaging(_ context.Context, id int64) (money.Money, error) {
	m, ok := f.packaging[id]
	if !ok {
		return money.Money{}, pricing.ErrNoPackaging
	}
	return m, nil
}

// TestLandedCosts : QUAND je compare une pièce de Chine et une pièce d'Europe de l'Est ALORS chaque composante
// du coût rendu est détaillée, dans la devise de la BOM (total-landed-cost, ADR 0025).
func TestLandedCosts(t *testing.T) {
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	mass := dec("2")
	cat, err := catalog.NewStub([]catalog.Part{{ID: 1, Name: "Jante", MassKG: &mass}, {ID: 2, Name: "Autre"}},
		[]catalog.Offer{
			{ID: 10, PartID: 1, Currency: "USD", OriginCountry: "CN"},
			{ID: 11, PartID: 1, Currency: "EUR", OriginCountry: "PL"},
			{ID: 20, PartID: 2, Currency: "EUR", OriginCountry: "PL"},
		})
	if err != nil {
		t.Fatal(err)
	}
	mny := func(a, c string) money.Money {
		v, err := money.Parse(a, c)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	prices := pricing.NewStub(
		[]pricing.Quote{{OfferID: 10, Price: mny("50", "USD"), Source: "sim", QuotedAt: at}, {OfferID: 11, Price: mny("60", "EUR"), Source: "sim", QuotedAt: at}},
		[]pricing.Rate{{From: "USD", To: "EUR", Rate: dec("0.9"), Source: "sim", QuotedAt: at}},
	)
	logi := fakeLogistics{
		duties:    map[[2]string]decimal.Decimal{{"CN", "FR"}: dec("10"), {"PL", "FR"}: dec("0")},
		freight:   map[[2]string]money.Money{{"CN", "FR"}: mny("3", "USD"), {"PL", "FR"}: mny("0.5", "EUR")},
		packaging: map[int64]money.Money{10: mny("2", "EUR")}, // l'offre 11 n'a pas d'emballage renseigné
	}
	svc := NewService(newMemStore(), cat, prices).WithLogistics(logi)
	ctx := t.Context()
	b, err := svc.CreateBOM(ctx, NewBOM{Name: "B", Currency: "EUR", Volume: 1})
	if err != nil {
		t.Fatal(err)
	}

	got, err := svc.LandedCosts(ctx, b.ID, 1, []int64{11, 10})
	if err != nil {
		t.Fatal(err)
	}
	for _, o := range got.Offers {
		if o.Customs.Amount != nil || !errors.Is(o.Customs.Err, pricing.ErrNoDestination) {
			t.Fatalf("offer %d without destination: customs %v (%v), want ErrNoDestination", o.OfferID, o.Customs.Amount, o.Customs.Err)
		}
	}

	if _, err := svc.SetDestinationCountry(ctx, b.ID, ptrStr("fr")); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("lowercase destination: err %v, want ErrInvalidInput", err)
	}
	if _, err := svc.SetDestinationCountry(ctx, b.ID, ptrStr("FR")); err != nil {
		t.Fatal(err)
	}
	got, err = svc.LandedCosts(ctx, b.ID, 1, []int64{11, 10})
	if err != nil {
		t.Fatal(err)
	}
	summary := func(o OfferLanded) string {
		c := func(x pricing.Component) string {
			if x.Amount == nil {
				return "—"
			}
			return x.Amount.Round().String()
		}
		total := "—"
		if o.Total != nil {
			total = o.Total.Round().String()
		}
		return fmt.Sprintf("%d %s: %s + %s + %s + %s = %s", o.OfferID, o.Origin, c(o.Purchase), c(o.Customs), c(o.Freight), c(o.Packaging), total)
	}
	want := []string{
		// 50 USD → 45 € ; douane 10 % = 4,50 € ; 3 USD/kg × 2 kg → 5,40 € ; emballage 2 €
		"10 CN: 45.00 EUR + 4.50 EUR + 5.40 EUR + 2.00 EUR = 56.90 EUR",
		"11 PL: 60.00 EUR + 0.00 EUR + 1.00 EUR + — = —",
	}
	for i, o := range got.Offers {
		if s := summary(o); s != want[i] {
			t.Errorf("offer %d: %s\nwant      %s", i, s, want[i])
		}
	}
	if r := got.Offers[0].Rates; len(r) != 1 || r[0].From != "USD" || r[0].To != "EUR" {
		t.Errorf("offer 10 rates %+v, want the USD→EUR rate used", r)
	}
	if d := got.Offers[0].DutyPercent; d == nil || !d.Equal(dec("10")) {
		t.Errorf("offer 10 duty %v, want 10 %%", d)
	}
	if !errors.Is(got.Offers[1].Err, pricing.ErrNoPackaging) {
		t.Errorf("offer 11 err %v, want ErrNoPackaging", got.Offers[1].Err)
	}
	if _, err := svc.LandedCosts(ctx, b.ID, 1, []int64{20}); !errors.Is(err, ErrOfferNotForPart) {
		t.Fatalf("offer of another part: err %v, want ErrOfferNotForPart", err)
	}
}

func ptrStr(s string) *string { return &s }
