package pricing

import (
	"cmp"
	"fmt"
	"maps"
	"slices"

	"github.com/shopspring/decimal"

	"bomstudio/internal/money"
)

// DefaultPerKgThreshold : seuil par défaut du prix au kg (2 × la médiane de la famille, ADR 0037).
var DefaultPerKgThreshold = decimal.NewFromInt(2)

// PerKgDetector : anomalie si le prix au kg de la pièce ≥ seuil × médiane des prix au kg de sa famille
// (catégorie), seulement si la famille compte au moins MinPeers pièces évaluables (ADR 0037). Une pièce sans
// masse (ou de masse nulle) n'a pas de prix au kg : écartée de la médiane, non évaluable comme sujet.
//
// ByFamily (méthode « matière », ADR 0041) : la famille est CostSample.Family (catégorie × matière) au lieu de la
// catégorie ; une pièce sans famille est non évaluable (ErrNoMaterial).
type PerKgDetector struct {
	MinPeers int
	ByFamily bool
}

// key : clé de regroupement d'une pièce.
func (d PerKgDetector) key(s CostSample) string {
	if d.ByFamily {
		return s.Family
	}
	return s.Category
}

var _ AnomalyDetector = PerKgDetector{}

// perKg : coût ÷ masse, à ratioPrecision décimales ; ErrNoMass si la masse manque ou n'est pas positive.
func perKg(s CostSample) (money.Money, error) {
	if s.Err != nil || s.Cost == nil {
		return money.Money{}, cmp.Or(s.Err, ErrNoQuote)
	}
	if s.MassKg == nil || !s.MassKg.IsPositive() {
		return money.Money{}, fmt.Errorf("part %d: mass missing or zero, price per kg not computable: %w", s.PartID, ErrNoMass)
	}
	return money.New(s.Cost.Amount().DivRound(*s.MassKg, ratioPrecision), string(s.Cost.Currency()))
}

// Detect : un verdict par sujet, dans l'ordre des sujets (mêmes règles de population qu'ADR 0027).
func (d PerKgDetector) Detect(subjects, population []CostSample, threshold decimal.Decimal) ([]Anomaly, error) {
	if threshold.LessThanOrEqual(decimal.NewFromInt(1)) {
		return nil, fmt.Errorf("threshold %s: %w", threshold, ErrInvalidThreshold)
	}
	byCategory := map[string][]money.Money{}
	missing := map[string]int{}
	for _, p := range population {
		k := d.key(p)
		if k == "" {
			continue
		}
		v, err := perKg(p)
		if err != nil {
			missing[k]++
			continue
		}
		byCategory[k] = append(byCategory[k], v)
	}
	medians := map[string]money.Money{}
	for _, c := range slices.Sorted(maps.Keys(byCategory)) {
		values := byCategory[c]
		slices.SortFunc(values, func(a, b money.Money) int { return a.Amount().Cmp(b.Amount()) })
		if err := sameCurrency(values); err != nil {
			return nil, fmt.Errorf("category %q: %w", c, err)
		}
		medians[c] = median(values)
	}
	out := make([]Anomaly, len(subjects))
	for i, s := range subjects {
		k := d.key(s)
		a := Anomaly{
			PartID: s.PartID, Category: s.Category, Status: AnomalyNotEvaluable, Cost: s.Cost,
			Peers: len(byCategory[k]), Missing: missing[k],
		}
		if d.ByFamily {
			a.Family = k
		}
		m, hasMedian := medians[k]
		v, vErr := perKg(s)
		switch {
		case d.ByFamily && k == "":
			a.Err = ErrNoMaterial
		case s.Category == "":
			a.Err = ErrNoCategory
		case vErr != nil:
			a.Err = vErr
		case a.Peers < d.MinPeers || !hasMedian:
			a.Err = fmt.Errorf("%d of %d required (%d without price, rate or mass): %w", a.Peers, d.MinPeers, a.Missing, ErrTooFewPeers)
		case !m.Amount().IsPositive():
			a.Err = ErrNonPositiveMedian
		default:
			if m.Currency() != v.Currency() {
				return nil, fmt.Errorf("part %d: cost in %s, median in %s: %w", s.PartID, v.Currency(), m.Currency(), money.ErrCurrencyMismatch)
			}
			r := v.Amount().DivRound(m.Amount(), ratioPrecision)
			expected := m.Mul(*s.MassKg) // coût d'une pièce de cette masse au prix médian de la famille
			excess, err := s.Cost.Add(expected.Mul(decimal.NewFromInt(-1)))
			if err != nil {
				return nil, err
			}
			a.PerKg, a.MedianPerKg, a.Median, a.Ratio, a.Excess = &v, &m, &expected, &r, &excess
			a.Status = AnomalyNormal
			if v.Amount().GreaterThanOrEqual(threshold.Mul(m.Amount())) {
				a.Status = AnomalyDetected
			}
		}
		out[i] = a
	}
	return out, nil
}
