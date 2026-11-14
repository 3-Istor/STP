package pricingstore_test

import (
	"errors"
	"testing"

	"github.com/shopspring/decimal"

	"bomstudio/internal/money"
	"bomstudio/internal/pricing"
	"bomstudio/internal/pricing/pricingstore"
)

// ADR 0025 : tables de douane et de transport, emballage d'une offre ; une ligne absente est une erreur
// nommée, jamais un zéro.
func TestLogisticsStore(t *testing.T) {
	tx := historyDB(t)
	ctx := t.Context()
	l := pricingstore.NewLogistics(tx)

	if _, err := l.CustomsDuty(ctx, "CN", "FR"); !errors.Is(err, pricing.ErrNoCustomsDuty) {
		t.Fatalf("missing duty: err %v, want ErrNoCustomsDuty", err)
	}
	if _, err := l.FreightRate(ctx, "CN", "FR"); !errors.Is(err, pricing.ErrNoFreightRate) {
		t.Fatalf("missing freight: err %v, want ErrNoFreightRate", err)
	}
	if _, err := l.SetCustomsDuty(ctx, pricing.CustomsDuty{Origin: "CN", Destination: "FR", Percent: decimal.RequireFromString("4.5")}); err != nil {
		t.Fatal(err)
	}
	if _, err := l.SetCustomsDuty(ctx, pricing.CustomsDuty{Origin: "CN", Destination: "FR", Percent: decimal.RequireFromString("3")}); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	d, err := l.CustomsDuty(ctx, "CN", "FR")
	if err != nil || !d.Percent.Equal(decimal.NewFromInt(3)) {
		t.Fatalf("duty %+v, %v ; want 3 %%", d, err)
	}
	perKg, _ := money.Parse("0.80", "EUR")
	if _, err := l.SetFreightRate(ctx, pricing.FreightRate{Origin: "CN", Destination: "FR", PerKg: perKg}); err != nil {
		t.Fatal(err)
	}
	f, err := l.FreightRate(ctx, "CN", "FR")
	if err != nil || !f.PerKg.Amount().Equal(decimal.RequireFromString("0.8")) || f.PerKg.Currency() != "EUR" {
		t.Fatalf("freight %+v, %v", f, err)
	}

	var supplier, part, offer int64
	if err := tx.QueryRow(ctx, `INSERT INTO suppliers (name) VALUES ('S') RETURNING id`).Scan(&supplier); err != nil {
		t.Fatal(err)
	}
	if err := tx.QueryRow(ctx, `INSERT INTO parts (name) VALUES ('P') RETURNING id`).Scan(&part); err != nil {
		t.Fatal(err)
	}
	if err := tx.QueryRow(ctx, `INSERT INTO offers (part_id, supplier_id, currency, price_min, price_max, lead_time_days, origin_country)
		VALUES ($1, $2, 'EUR', 1, 2, 3, 'CN') RETURNING id`, part, supplier).Scan(&offer); err != nil {
		t.Fatal(err)
	}
	if _, err := l.OfferPackaging(ctx, offer); !errors.Is(err, pricing.ErrNoPackaging) {
		t.Fatalf("missing packaging: err %v, want ErrNoPackaging", err)
	}
	pk, _ := money.Parse("1.25", "USD")
	if _, err := l.SetOfferPackaging(ctx, offer, pk); err != nil {
		t.Fatal(err)
	}
	if got, err := l.OfferPackaging(ctx, offer); err != nil || got.Currency() != "USD" || !got.Amount().Equal(decimal.RequireFromString("1.25")) {
		t.Fatalf("packaging %v, %v", got, err)
	}
	if _, err := l.SetOfferPackaging(ctx, 999999999, pk); !errors.Is(err, pricing.ErrUnknownOffer) {
		t.Fatalf("unknown offer: err %v, want ErrUnknownOffer", err)
	}
}
