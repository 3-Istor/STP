package bom

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/shopspring/decimal"

	"bomstudio/internal/money"
	"bomstudio/internal/pricing"
)

// maxCompared : offres comparées côte à côte par requête.
const maxCompared = 20

// ErrLogisticsUnavailable : le service a été câblé sans les tables du coût rendu.
var ErrLogisticsUnavailable = errors.New("landed cost unavailable: no logistics tables")

// OfferLanded : coût rendu d'une unité d'une offre, composante par composante (ADR 0025).
type OfferLanded struct {
	OfferID     int64
	Origin      string
	Destination *string
	pricing.Landed
	// Sur quoi repose le résultat (AGENTS §3) : valeurs des tables retenues (lues en direct, non figées au
	// snapshot de prix) et taux utilisés avec leur date.
	DutyPercent  *decimal.Decimal
	FreightPerKg *money.Money
	PackagingPer *money.Money
	Rates        []pricing.Rate
}

// LandedComparison : offres d'une même pièce comparées côte à côte, dans la devise de la BOM, aux prix et
// taux de la même source que ses totaux (direct ou snapshot de prix).
type LandedComparison struct {
	PartID          int64
	Currency        money.Currency
	PriceSnapshotID *int64
	Offers          []OfferLanded
}

// LandedCosts calcule le coût rendu unitaire de chaque offre de offerIDs (toutes de la pièce partID) pour la
// BOM bomID : achat + douane (origine de l'offre → destination de la BOM) + transport au kg + emballage.
// Une composante absente est une erreur nommée de cette composante, jamais un zéro.
func (s *Service) LandedCosts(ctx context.Context, bomID, partID int64, offerIDs []int64) (LandedComparison, error) {
	if s.logistics == nil {
		return LandedComparison{}, fmt.Errorf("landed costs of part %d: %w", partID, ErrLogisticsUnavailable)
	}
	if len(offerIDs) == 0 || len(offerIDs) > maxCompared {
		return LandedComparison{}, fmt.Errorf("landed costs of part %d: compare 1 to %d offers, not %d: %w",
			partID, maxCompared, len(offerIDs), ErrInvalidInput)
	}
	b, err := s.store.GetBOM(ctx, bomID)
	if err != nil {
		return LandedComparison{}, fmt.Errorf("landed costs of part %d: %w", partID, err)
	}
	part, err := s.catalog.Part(ctx, partID)
	if err != nil {
		return LandedComparison{}, fmt.Errorf("landed costs of part %d: %w", partID, err)
	}
	ids := slices.Clone(offerIDs)
	slices.Sort(ids)
	ids = slices.Compact(ids)
	for _, id := range ids {
		if err := s.checkOfferSellsPart(ctx, id, partID); err != nil {
			return LandedComparison{}, fmt.Errorf("landed costs of part %d: %w", partID, err)
		}
	}
	currency := money.Currency(b.Currency)
	prices, _, err := LoadPrices(ctx, s.pricing, b.PriceSnapshotID, ids, currency)
	if err != nil {
		return LandedComparison{}, fmt.Errorf("landed costs of part %d: %w", partID, err)
	}

	out := LandedComparison{PartID: partID, Currency: currency, PriceSnapshotID: b.PriceSnapshotID}
	for _, id := range ids {
		o, err := s.catalog.Offer(ctx, id)
		if err != nil {
			return LandedComparison{}, fmt.Errorf("landed costs of part %d: %w", partID, err)
		}
		used := map[money.Currency]pricing.Rate{}
		var fatal error
		in := pricing.LandedInput{MassKg: part.MassKG, Convert: s.recordingConverter(ctx, b.PriceSnapshotID, currency, used, &fatal)}
		if pr := prices[id]; pr.Err != nil {
			in.PurchaseErr = pr.Err
		} else {
			q := pr.Quote.Price
			in.Purchase = &q
		}
		if err := s.landedTables(ctx, &in, o.OriginCountry, b.DestinationCountry, id); err != nil {
			return LandedComparison{}, fmt.Errorf("landed costs of part %d: %w", partID, err)
		}
		landed := pricing.LandedCost(in)
		if fatal != nil { // panne de lecture d'un taux : erreur, pas une composante « manquante »
			return LandedComparison{}, fmt.Errorf("landed costs of part %d: %w", partID, fatal)
		}
		out.Offers = append(out.Offers, OfferLanded{
			OfferID: id, Origin: o.OriginCountry, Destination: b.DestinationCountry, Landed: landed,
			DutyPercent: in.Duty, FreightPerKg: in.FreightPerKg, PackagingPer: in.Packaging, Rates: sortedRates(used),
		})
	}
	return out, nil
}

// landedTables lit douane, transport et emballage ; une ligne absente devient l'erreur de sa composante, toute
// autre erreur (panne) est renvoyée.
func (s *Service) landedTables(ctx context.Context, in *pricing.LandedInput, origin string, dest *string, offerID int64) error {
	if dest == nil {
		in.DutyErr, in.FreightErr = pricing.ErrNoDestination, pricing.ErrNoDestination
	} else {
		d, err := s.logistics.CustomsDuty(ctx, origin, *dest)
		switch {
		case errors.Is(err, pricing.ErrNoCustomsDuty):
			in.DutyErr = err
		case err != nil:
			return err
		default:
			in.Duty = &d.Percent
		}
		f, err := s.logistics.FreightRate(ctx, origin, *dest)
		switch {
		case errors.Is(err, pricing.ErrNoFreightRate):
			in.FreightErr = err
		case err != nil:
			return err
		default:
			in.FreightPerKg = &f.PerKg
		}
	}
	p, err := s.logistics.OfferPackaging(ctx, offerID)
	switch {
	case errors.Is(err, pricing.ErrNoPackaging):
		in.PackagingErr = err
	case err != nil:
		return err
	default:
		in.Packaging = &p
	}
	return nil
}

// recordingConverter convertit un montant dans la devise de la BOM avec les taux du même contexte que ses totaux
// (en direct, ou ceux du snapshot de prix ; taux absent → pricing.ErrNoRate) et note chaque taux utilisé dans
// used (si non nil) : un même taux n'est lu qu'une fois par appel, toutes les composantes partagent sa valeur.
// Une erreur autre que pricing.ErrNoRate (panne) est aussi notée dans *fatal pour que l'appelant échoue.
func (s *Service) recordingConverter(ctx context.Context, snapID *int64, to money.Currency, used map[money.Currency]pricing.Rate,
	fatal *error,
) func(money.Money) (money.Money, error) {
	return func(m money.Money) (money.Money, error) {
		if r, ok := used[m.Currency()]; ok {
			return money.New(m.Amount().Mul(r.Rate), string(to))
		}
		if m.Currency() == to {
			return m, nil
		}
		r, err := loadRate(ctx, s.pricing, snapID, m.Currency(), to)
		if err != nil && !errors.Is(err, pricing.ErrNoRate) && fatal != nil && *fatal == nil {
			*fatal = err
		}
		if err != nil {
			return money.Money{}, fmt.Errorf("rate %s→%s: %w", m.Currency(), to, err)
		}
		if r.From != m.Currency() || r.To != to {
			return money.Money{}, fmt.Errorf("rate for %s→%s is %s→%s: %w", m.Currency(), to, r.From, r.To, pricing.ErrNoRate)
		}
		if used != nil {
			used[m.Currency()] = r
		}
		return money.New(m.Amount().Mul(r.Rate), string(to))
	}
}

func sortedRates(used map[money.Currency]pricing.Rate) []pricing.Rate {
	out := make([]pricing.Rate, 0, len(used))
	for _, r := range used {
		out = append(out, r)
	}
	slices.SortFunc(out, func(a, b pricing.Rate) int { return cmp.Compare(a.From, b.From) })
	return out
}
