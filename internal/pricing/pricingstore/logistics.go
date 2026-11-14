package pricingstore

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"

	"bomstudio/internal/money"
	"bomstudio/internal/pricing"
	"bomstudio/internal/pricing/pricingdb"
)

// Logistics implémente pricing.LogisticsStore (douane, transport, emballage ; ADR 0025).
type Logistics struct {
	queries *pricingdb.Queries
}

var _ pricing.LogisticsStore = (*Logistics)(nil)

func NewLogistics(db pricingdb.DBTX) *Logistics { return &Logistics{queries: pricingdb.New(db)} }

func (l *Logistics) CustomsDuty(ctx context.Context, origin, destination string) (pricing.CustomsDuty, error) {
	r, err := l.queries.GetCustomsDuty(ctx, pricingdb.GetCustomsDutyParams{OriginCountry: origin, DestinationCountry: destination})
	if errors.Is(err, pgx.ErrNoRows) {
		return pricing.CustomsDuty{}, fmt.Errorf("customs %s→%s: %w", origin, destination, pricing.ErrNoCustomsDuty)
	}
	if err != nil {
		return pricing.CustomsDuty{}, fmt.Errorf("customs %s→%s: %w", origin, destination, err)
	}
	return toDuty(r), nil
}

func (l *Logistics) FreightRate(ctx context.Context, origin, destination string) (pricing.FreightRate, error) {
	r, err := l.queries.GetFreightRate(ctx, pricingdb.GetFreightRateParams{OriginCountry: origin, DestinationCountry: destination})
	if errors.Is(err, pgx.ErrNoRows) {
		return pricing.FreightRate{}, fmt.Errorf("freight %s→%s: %w", origin, destination, pricing.ErrNoFreightRate)
	}
	if err != nil {
		return pricing.FreightRate{}, fmt.Errorf("freight %s→%s: %w", origin, destination, err)
	}
	return toFreight(r)
}

func (l *Logistics) OfferPackaging(ctx context.Context, offerID int64) (money.Money, error) {
	r, err := l.queries.GetOfferPackaging(ctx, offerID)
	if errors.Is(err, pgx.ErrNoRows) {
		return money.Money{}, fmt.Errorf("packaging of offer %d: %w", offerID, pricing.ErrUnknownOffer)
	}
	if err != nil {
		return money.Money{}, fmt.Errorf("packaging of offer %d: %w", offerID, err)
	}
	if !r.PackagingAmount.Valid || !r.PackagingCurrency.Valid {
		return money.Money{}, fmt.Errorf("offer %d: %w", offerID, pricing.ErrNoPackaging)
	}
	return money.New(r.PackagingAmount.Decimal, r.PackagingCurrency.String)
}

func (l *Logistics) ListCustomsDuties(ctx context.Context) ([]pricing.CustomsDuty, error) {
	rows, err := l.queries.ListCustomsDuties(ctx)
	if err != nil {
		return nil, fmt.Errorf("list customs duties: %w", err)
	}
	out := make([]pricing.CustomsDuty, len(rows))
	for i, r := range rows {
		out[i] = toDuty(r)
	}
	return out, nil
}

func (l *Logistics) ListFreightRates(ctx context.Context) ([]pricing.FreightRate, error) {
	rows, err := l.queries.ListFreightRates(ctx)
	if err != nil {
		return nil, fmt.Errorf("list freight rates: %w", err)
	}
	out := make([]pricing.FreightRate, len(rows))
	for i, r := range rows {
		if out[i], err = toFreight(r); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func (l *Logistics) SetCustomsDuty(ctx context.Context, d pricing.CustomsDuty) (pricing.CustomsDuty, error) {
	r, err := l.queries.UpsertCustomsDuty(ctx, pricingdb.UpsertCustomsDutyParams{
		OriginCountry: d.Origin, DestinationCountry: d.Destination, DutyPercent: d.Percent,
	})
	if err != nil {
		return pricing.CustomsDuty{}, fmt.Errorf("set customs %s→%s: %w", d.Origin, d.Destination, err)
	}
	return toDuty(r), nil
}

func (l *Logistics) SetFreightRate(ctx context.Context, f pricing.FreightRate) (pricing.FreightRate, error) {
	r, err := l.queries.UpsertFreightRate(ctx, pricingdb.UpsertFreightRateParams{
		OriginCountry: f.Origin, DestinationCountry: f.Destination, AmountPerKg: f.PerKg.Amount(), Currency: string(f.PerKg.Currency()),
	})
	if err != nil {
		return pricing.FreightRate{}, fmt.Errorf("set freight %s→%s: %w", f.Origin, f.Destination, err)
	}
	return toFreight(r)
}

func (l *Logistics) SetOfferPackaging(ctx context.Context, offerID int64, m money.Money) (money.Money, error) {
	r, err := l.queries.SetOfferPackaging(ctx, pricingdb.SetOfferPackagingParams{
		ID: offerID, PackagingAmount: decimal.NullDecimal{Decimal: m.Amount(), Valid: true},
		PackagingCurrency: pgtype.Text{String: string(m.Currency()), Valid: true},
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return money.Money{}, fmt.Errorf("set packaging of offer %d: %w", offerID, pricing.ErrUnknownOffer)
	}
	if err != nil {
		return money.Money{}, fmt.Errorf("set packaging of offer %d: %w", offerID, err)
	}
	return money.New(r.PackagingAmount.Decimal, r.PackagingCurrency.String)
}

func toDuty(r pricingdb.CustomsDuty) pricing.CustomsDuty {
	return pricing.CustomsDuty{Origin: r.OriginCountry, Destination: r.DestinationCountry, Percent: r.DutyPercent}
}

func toFreight(r pricingdb.FreightRate) (pricing.FreightRate, error) {
	m, err := money.New(r.AmountPerKg, r.Currency)
	if err != nil {
		return pricing.FreightRate{}, fmt.Errorf("freight %s→%s: %w", r.OriginCountry, r.DestinationCountry, err)
	}
	return pricing.FreightRate{Origin: r.OriginCountry, Destination: r.DestinationCountry, PerKg: m}, nil
}

func (l *Logistics) ClearOfferPackaging(ctx context.Context, offerID int64) error {
	n, err := l.queries.ClearOfferPackaging(ctx, offerID)
	if err != nil {
		return fmt.Errorf("clear packaging of offer %d: %w", offerID, err)
	}
	if n == 0 {
		return fmt.Errorf("clear packaging of offer %d: %w", offerID, pricing.ErrUnknownOffer)
	}
	return nil
}

func (l *Logistics) DeleteCustomsDuty(ctx context.Context, origin, destination string) error {
	if err := l.queries.DeleteCustomsDuty(ctx, pricingdb.DeleteCustomsDutyParams{OriginCountry: origin, DestinationCountry: destination}); err != nil {
		return fmt.Errorf("delete customs %s→%s: %w", origin, destination, err)
	}
	return nil
}

func (l *Logistics) DeleteFreightRate(ctx context.Context, origin, destination string) error {
	if err := l.queries.DeleteFreightRate(ctx, pricingdb.DeleteFreightRateParams{OriginCountry: origin, DestinationCountry: destination}); err != nil {
		return fmt.Errorf("delete freight %s→%s: %w", origin, destination, err)
	}
	return nil
}
