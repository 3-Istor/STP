package bom

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/shopspring/decimal"

	"bomstudio/internal/money"
	"bomstudio/internal/pricing"
)

// ErrAnalysisUnavailable : le service a été câblé sans la source qu'une analyse demande (historique des
// cotations, annuaire des fournisseurs).
var ErrAnalysisUnavailable = errors.New("analysis unavailable: source not wired")

// MaxPriceDrivers : nombre maximal de pièces demandées au classement.
const MaxPriceDrivers = 100

// OfferRange : fourchette des cotations d'une offre sur la fenêtre annuelle (ADR 0011) ; Err si elle manque.
type OfferRange struct {
	Min, Max *money.Money // dans la devise de l'offre
	Quotes   int          // cotations de la fenêtre
	Err      error
}

// DriverOffer : une offre retenue par la pièce dans la BOM et son amplitude par voiture.
type DriverOffer struct {
	OfferID  int64
	Quantity decimal.Decimal // Σ quantités réelles des nœuds de l'offre, par voiture
	Range    OfferRange
	// Amplitude : (max − min) × Quantity, convertie au taux de la méthode ; nil si Err.
	Amplitude *money.Money
	Err       error
}

// PriceDriver : pièce de la BOM et amplitude de son prix par voiture (somme de ses offres retenues).
type PriceDriver struct {
	PartID    int64
	Name      string
	Amplitude *money.Money // nil si une offre n'est pas évaluable (Err dit pourquoi)
	Offers    []DriverOffer
	Err       error
}

// PriceDrivers : pièces dont le prix pèse le plus (amplitude des cotations × quantité), et sur quoi le
// classement repose. Excluded : pièces non évaluables, jamais cachées.
type PriceDrivers struct {
	BOMID                  int64
	Currency               money.Currency
	WindowStart, WindowEnd time.Time
	Explanation            Explanation
	Drivers                []PriceDriver
	Excluded               []PriceDriver
}

// PriceDrivers classe les pièces du brouillon par amplitude de leurs cotations (max − min de l'offre retenue,
// fenêtre annuelle finissant à la date de calcul) × quantité réelle, convertie au taux de la méthode du coût.
// limit ∈ [1 ; MaxPriceDrivers].
func (s *Service) PriceDrivers(ctx context.Context, bomID int64, limit int) (PriceDrivers, error) {
	if limit < 1 || limit > MaxPriceDrivers {
		return PriceDrivers{}, fmt.Errorf("price drivers of bom %d: limit %d must be between 1 and %d: %w", bomID, limit, MaxPriceDrivers, ErrInvalidInput)
	}
	if s.history == nil {
		return PriceDrivers{}, fmt.Errorf("price drivers of bom %d: quote history: %w", bomID, ErrAnalysisUnavailable)
	}
	dc, err := s.DraftCost(ctx, bomID)
	if err != nil {
		return PriceDrivers{}, fmt.Errorf("price drivers: %w", err)
	}
	asOf := costAsOf(dc.Input)
	if asOf.IsZero() {
		return PriceDrivers{}, fmt.Errorf("price drivers of bom %d: %w", bomID, ErrNoCostDate)
	}
	start := pricing.AnnualWindowStart(asOf)
	ranges := map[int64]OfferRange{}
	names := map[int64]string{}
	for _, n := range dc.Nodes {
		if !n.IsPart() {
			continue
		}
		if _, ok := names[*n.PartID]; !ok {
			p, err := s.catalog.Part(ctx, *n.PartID)
			if err != nil {
				return PriceDrivers{}, fmt.Errorf("price drivers of bom %d: node %d: %w", bomID, n.ID, err)
			}
			names[p.ID] = p.Name
		}
		if _, ok := ranges[*n.OfferID]; ok {
			continue
		}
		quotes, err := s.history.ReadHistory(ctx, *n.OfferID, start, asOf)
		if err != nil {
			return PriceDrivers{}, fmt.Errorf("price drivers of bom %d: history of offer %d: %w", bomID, *n.OfferID, err)
		}
		ranges[*n.OfferID] = offerRange(*n.OfferID, asOf, quotes)
	}
	out, err := ComputePriceDrivers(dc.Input, dc.Totals, names, ranges, limit)
	if err != nil {
		return PriceDrivers{}, fmt.Errorf("price drivers of bom %d: %w", bomID, err)
	}
	out.BOMID, out.WindowStart, out.WindowEnd = bomID, start, asOf
	return out, nil
}

// offerRange : min et max de la fenêtre annuelle (pricing.CalculateStatistics). Une fenêtre sans référence
// antérieure garde ses extrêmes ; sans cotation dans la fenêtre, l'erreur nommée est rendue.
func offerRange(offerID int64, asOf time.Time, quotes []pricing.HistoricalQuote) OfferRange {
	st, err := pricing.CalculateStatistics(offerID, asOf, quotes)
	if st.Minimum == nil || st.Maximum == nil {
		if err == nil {
			err = pricing.ErrNoWindow
		}
		return OfferRange{Err: fmt.Errorf("offer %d quotes: %w", offerID, err)}
	}
	return OfferRange{Min: st.Minimum, Max: st.Maximum, Quotes: len(st.History)}
}

// ComputePriceDrivers est le calcul pur : quantités réelles des totaux, conversion au taux de la méthode (le
// même que le coût), tri par amplitude décroissante puis id de pièce, limite appliquée aux pièces évaluées.
func ComputePriceDrivers(in TotalsInput, t Totals, names map[int64]string, ranges map[int64]OfferRange, limit int) (PriceDrivers, error) {
	calc, err := newCostCalc(in)
	if err != nil {
		return PriceDrivers{}, err
	}
	type key struct{ part, offer int64 }
	qty := map[key]decimal.Decimal{}
	for _, n := range in.Nodes {
		if n.Part == nil {
			continue
		}
		nt, ok := t.Node(n.ID)
		if !ok {
			return PriceDrivers{}, fmt.Errorf("node %d: %w", n.ID, ErrNodeNotFound)
		}
		k := key{n.Part.PartID, n.Part.OfferID}
		qty[k] = qty[k].Add(nt.RealQuantity)
	}
	keys := make([]key, 0, len(qty))
	for k := range qty {
		keys = append(keys, k)
	}
	slices.SortFunc(keys, func(a, b key) int { return cmp.Or(cmp.Compare(a.part, b.part), cmp.Compare(a.offer, b.offer)) })

	byPart := map[int64]*PriceDriver{}
	var parts []int64
	for _, k := range keys {
		d, ok := byPart[k.part]
		if !ok {
			d = &PriceDriver{PartID: k.part, Name: names[k.part]}
			byPart[k.part] = d
			parts = append(parts, k.part)
		}
		o := DriverOffer{OfferID: k.offer, Quantity: qty[k], Range: ranges[k.offer]}
		switch r := o.Range; {
		case r.Err != nil:
			o.Err = r.Err
		case r.Min == nil || r.Max == nil:
			o.Err = fmt.Errorf("offer %d: %w", k.offer, pricing.ErrNoWindow)
		default:
			spread, err := r.Max.Add(r.Min.Mul(decimal.NewFromInt(-1)))
			if err != nil {
				o.Err = err
				break
			}
			conv, err := calc.chosen(spread.Mul(o.Quantity))
			if err != nil {
				o.Err = fmt.Errorf("offer %d: %w", k.offer, err)
				break
			}
			o.Amplitude = &conv
		}
		d.Offers = append(d.Offers, o)
	}
	out := PriceDrivers{Currency: in.Currency, Explanation: calc.explanation()}
	for _, pid := range parts {
		d := byPart[pid]
		var errs []error
		total := money.Money{}
		for _, o := range d.Offers {
			if o.Err != nil {
				errs = append(errs, o.Err)
				continue
			}
			if total.Currency() == "" {
				total = *o.Amplitude
				continue
			}
			if total, err = total.Add(*o.Amplitude); err != nil {
				return PriceDrivers{}, err
			}
		}
		if d.Err = errors.Join(errs...); d.Err != nil {
			out.Excluded = append(out.Excluded, *d)
			continue
		}
		d.Amplitude = &total
		out.Drivers = append(out.Drivers, *d)
	}
	slices.SortFunc(out.Drivers, func(a, b PriceDriver) int {
		return cmp.Or(b.Amplitude.Amount().Cmp(a.Amplitude.Amount()), cmp.Compare(a.PartID, b.PartID))
	})
	if len(out.Drivers) > limit {
		out.Drivers = out.Drivers[:limit]
	}
	return out, nil
}
