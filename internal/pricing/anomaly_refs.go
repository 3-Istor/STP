package pricing

import (
	"cmp"
	"fmt"
	"slices"

	"github.com/shopspring/decimal"

	"bomstudio/internal/money"
)

// Méthodes d'anomalies « offres » et « historique » (ADR 0041) : la référence n'est pas une population de
// pièces du catalogue mais les autres prix de la même pièce, donc aucun mélange entre véhicules.

// DefaultOfferSpreadThreshold : l'offre retenue coûte 25 % de plus que la médiane des offres de sa pièce.
var DefaultOfferSpreadThreshold = decimal.RequireFromString("1.25")

// DefaultHistoryThreshold : le prix coté est 15 % au-dessus de la médiane de ses 12 derniers mois.
var DefaultHistoryThreshold = decimal.RequireFromString("1.15")

// OfferSpreadDetector : anomalie si le coût de l'offre retenue ≥ seuil × médiane des offres actives de la même
// pièce, seulement si la pièce a au moins MinOffers offres chiffrées. Sa population est ignorée.
type OfferSpreadDetector struct {
	MinOffers int
}

var _ AnomalyDetector = OfferSpreadDetector{}

// Detect : un verdict par sujet, dans l'ordre des sujets.
func (d OfferSpreadDetector) Detect(subjects, _ []CostSample, threshold decimal.Decimal) ([]Anomaly, error) {
	if threshold.LessThanOrEqual(decimal.NewFromInt(1)) {
		return nil, fmt.Errorf("threshold %s: %w", threshold, ErrInvalidThreshold)
	}
	out := make([]Anomaly, len(subjects))
	for i, s := range subjects {
		a := Anomaly{
			PartID: s.PartID, Category: s.Category, Status: AnomalyNotEvaluable, Cost: s.Cost,
			Peers: len(s.OfferCosts), Missing: s.MissingOffers,
		}
		switch {
		case s.Err != nil || s.Cost == nil:
			a.Err = cmp.Or(s.Err, ErrNoQuote)
		case len(s.OfferCosts) < d.MinOffers:
			a.Err = fmt.Errorf("%d of %d required (%d without price or rate, %d archived): %w", len(s.OfferCosts), d.MinOffers, s.MissingOffers, s.ArchivedOffers, ErrTooFewOffers)
		default:
			costs := slices.Clone(s.OfferCosts)
			slices.SortFunc(costs, func(x, y money.Money) int { return x.Amount().Cmp(y.Amount()) })
			if err := sameCurrency(costs); err != nil {
				return nil, fmt.Errorf("part %d offers: %w", s.PartID, err)
			}
			m := median(costs)
			if m.Currency() != s.Cost.Currency() {
				return nil, fmt.Errorf("part %d: cost in %s, median in %s: %w", s.PartID, s.Cost.Currency(), m.Currency(), money.ErrCurrencyMismatch)
			}
			if !m.Amount().IsPositive() {
				a.Err = ErrNonPositiveMedian
				break
			}
			r := s.Cost.Amount().DivRound(m.Amount(), ratioPrecision)
			excess, err := s.Cost.Add(m.Mul(decimal.NewFromInt(-1)))
			if err != nil {
				return nil, err
			}
			a.Median, a.Ratio, a.Excess = &m, &r, &excess
			a.Status = AnomalyNormal
			if s.Cost.Amount().GreaterThanOrEqual(threshold.Mul(m.Amount())) {
				a.Status = AnomalyDetected
			}
		}
		out[i] = a
	}
	return out, nil
}

// HistoryDetector : anomalie si le prix coté de l'offre retenue ≥ seuil × médiane de ses cotations des 12
// derniers mois, seulement s'il y en a au moins MinQuotes. Comparaison dans la devise de l'offre ; la
// référence est ensuite exprimée dans la devise commune au taux courant du coût (sans changer le rapport).
type HistoryDetector struct {
	MinQuotes int
}

var _ AnomalyDetector = HistoryDetector{}

// Detect : un verdict par sujet, dans l'ordre des sujets. La population est ignorée.
func (d HistoryDetector) Detect(subjects, _ []CostSample, threshold decimal.Decimal) ([]Anomaly, error) {
	if threshold.LessThanOrEqual(decimal.NewFromInt(1)) {
		return nil, fmt.Errorf("threshold %s: %w", threshold, ErrInvalidThreshold)
	}
	out := make([]Anomaly, len(subjects))
	for i, s := range subjects {
		a := Anomaly{PartID: s.PartID, Category: s.Category, Status: AnomalyNotEvaluable, Cost: s.Cost}
		if s.Trail != nil {
			a.Peers = len(s.Trail.Past)
		}
		switch {
		case s.Err != nil || s.Cost == nil:
			a.Err = cmp.Or(s.Err, ErrNoQuote)
		case s.Trail == nil || len(s.Trail.Past) < d.MinQuotes:
			other := 0
			if s.Trail != nil {
				other = s.Trail.OtherCurrency
			}
			a.Err = fmt.Errorf("%d of %d required (%d in another currency): %w", a.Peers, d.MinQuotes, other, ErrTooFewQuotes)
		default:
			past := slices.Clone(s.Trail.Past)
			slices.SortFunc(past, func(x, y money.Money) int { return x.Amount().Cmp(y.Amount()) })
			if err := sameCurrency(past); err != nil {
				return nil, fmt.Errorf("part %d quotes: %w", s.PartID, err)
			}
			m := median(past)
			if m.Currency() != s.Trail.Current.Currency() {
				return nil, fmt.Errorf("part %d: price in %s, history in %s: %w", s.PartID, s.Trail.Current.Currency(), m.Currency(), money.ErrCurrencyMismatch)
			}
			if !m.Amount().IsPositive() || !s.Trail.Current.Amount().IsPositive() {
				a.Err = ErrNonPositiveMedian
				break
			}
			r := s.Trail.Current.Amount().DivRound(m.Amount(), ratioPrecision)
			// Coût × médiane ÷ prix coté : multiplier avant de diviser garde l'exactitude (120 × 100 ÷ 120 = 100).
			expected, err := money.New(s.Cost.Amount().Mul(m.Amount()).DivRound(s.Trail.Current.Amount(), ratioPrecision), string(s.Cost.Currency()))
			if err != nil {
				return nil, err
			}
			excess, err := s.Cost.Add(expected.Mul(decimal.NewFromInt(-1)))
			if err != nil {
				return nil, err
			}
			a.Median, a.Ratio, a.Excess = &expected, &r, &excess
			a.Status = AnomalyNormal
			if s.Trail.Current.Amount().GreaterThanOrEqual(threshold.Mul(m.Amount())) {
				a.Status = AnomalyDetected
			}
		}
		out[i] = a
	}
	return out, nil
}
