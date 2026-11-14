package pricing

import (
	"cmp"
	"errors"
	"fmt"
	"slices"

	"github.com/shopspring/decimal"

	"bomstudio/internal/money"
)

// Détection des anomalies de coût (cost-anomalies, ADR 0027). L'interface permettra de remplacer la règle
// statistique par un modèle IA sans toucher aux appelants (AGENTS §3 « IA dans le produit »).

var (
	// ErrTooFewPeers : la catégorie compte moins de pièces évaluables que le minimum (5 par défaut).
	ErrTooFewPeers = errors.New("too few comparable parts in category")
	// ErrNoCategory : la pièce n'a pas de catégorie, aucune comparaison possible.
	ErrNoCategory = errors.New("part has no category")
	// ErrInvalidThreshold : le seuil doit être un décimal > 1.
	ErrInvalidThreshold = errors.New("anomaly threshold must be > 1")
	// ErrNonPositiveMedian : médiane nulle, aucun rapport calculable.
	ErrNonPositiveMedian = errors.New("category median is not positive")
	// ErrPopulationNotFrozen : la BOM a des prix figés ; son snapshot de prix ne contient que ses propres offres,
	// pas celles des autres pièces de la catégorie : la comparaison n'est pas possible à ces prix.
	ErrPopulationNotFrozen = errors.New("category prices are not frozen with this bom")
)

// DefaultAnomalyThreshold : 3× la médiane de la catégorie (docs/features.md, cost-anomalies).
var DefaultAnomalyThreshold = decimal.NewFromInt(3)

// CostSample : coût unitaire d'une pièce dans une devise commune, ou l'erreur qui explique son absence.
type CostSample struct {
	PartID   int64
	Category string
	Cost     *money.Money
	Err      error
}

// AnomalyStatus : verdict d'une pièce. NotEvaluable n'est jamais lu comme « normal ».
type AnomalyStatus string

const (
	AnomalyNormal       AnomalyStatus = "normal"
	AnomalyDetected     AnomalyStatus = "anomaly"
	AnomalyNotEvaluable AnomalyStatus = "not_evaluable"
)

// Anomaly : verdict et sur quoi il repose (médiane, nombre de pièces comparées, rapport au seuil).
type Anomaly struct {
	PartID   int64
	Category string
	Status   AnomalyStatus
	Cost     *money.Money
	Median   *money.Money
	Ratio    *decimal.Decimal // Cost ÷ Median, exact à ratioPrecision décimales
	Peers    int              // pièces de la catégorie qui ont un coût (la pièce elle-même si elle est active)
	Missing  int              // pièces de la catégorie écartées faute de prix ou de taux (jamais cachées)
	Err      error            // pourquoi la pièce n'est pas évaluable
}

// AnomalyDetector juge chaque sujet par rapport à la population (même devise pour tous les coûts).
type AnomalyDetector interface {
	Detect(subjects, population []CostSample, threshold decimal.Decimal) ([]Anomaly, error)
}

// MedianDetector : anomalie si coût ≥ seuil × médiane de la catégorie, seulement si la catégorie compte au
// moins MinPeers pièces évaluables (ADR 0027). Médiane d'un nombre pair = moyenne exacte des deux centrales.
type MedianDetector struct {
	MinPeers int
}

var _ AnomalyDetector = MedianDetector{}

const ratioPrecision = 28

// Detect : un verdict par sujet, dans l'ordre des sujets. Les erreurs de la population (prix absent) retirent
// seulement la pièce concernée de la médiane ; celle d'un sujet le rend non évaluable.
func (d MedianDetector) Detect(subjects, population []CostSample, threshold decimal.Decimal) ([]Anomaly, error) {
	if threshold.LessThanOrEqual(decimal.NewFromInt(1)) {
		return nil, fmt.Errorf("threshold %s: %w", threshold, ErrInvalidThreshold)
	}
	byCategory := map[string][]money.Money{}
	missing := map[string]int{}
	for _, p := range population {
		switch {
		case p.Category == "":
		case p.Err == nil && p.Cost != nil:
			byCategory[p.Category] = append(byCategory[p.Category], *p.Cost)
		default:
			missing[p.Category]++
		}
	}
	categories := make([]string, 0, len(byCategory))
	for c := range byCategory {
		categories = append(categories, c)
	}
	slices.Sort(categories) // message d'erreur déterministe
	medians := map[string]money.Money{}
	for _, c := range categories {
		costs := byCategory[c]
		slices.SortFunc(costs, func(a, b money.Money) int { return a.Amount().Cmp(b.Amount()) })
		if err := sameCurrency(costs); err != nil {
			return nil, fmt.Errorf("category %q: %w", c, err)
		}
		medians[c] = median(costs)
	}
	out := make([]Anomaly, len(subjects))
	for i, s := range subjects {
		a := Anomaly{
			PartID: s.PartID, Category: s.Category, Status: AnomalyNotEvaluable, Cost: s.Cost,
			Peers: len(byCategory[s.Category]), Missing: missing[s.Category],
		}
		m, hasMedian := medians[s.Category]
		switch {
		case s.Category == "":
			a.Err = ErrNoCategory
		case s.Err != nil || s.Cost == nil:
			a.Err = cmp.Or(s.Err, ErrNoQuote)
		case a.Peers < d.MinPeers || !hasMedian:
			a.Err = fmt.Errorf("%d of %d required (%d without price or rate): %w", a.Peers, d.MinPeers, a.Missing, ErrTooFewPeers)
		case !m.Amount().IsPositive():
			a.Err = ErrNonPositiveMedian
		default:
			if m.Currency() != s.Cost.Currency() {
				return nil, fmt.Errorf("part %d: cost in %s, median in %s: %w", s.PartID, s.Cost.Currency(), m.Currency(), money.ErrCurrencyMismatch)
			}
			r := s.Cost.Amount().DivRound(m.Amount(), ratioPrecision)
			a.Median, a.Ratio = &m, &r
			a.Status = AnomalyNormal
			if s.Cost.Amount().GreaterThanOrEqual(threshold.Mul(m.Amount())) {
				a.Status = AnomalyDetected
			}
		}
		out[i] = a
	}
	return out, nil
}

func median(sorted []money.Money) money.Money {
	n := len(sorted)
	if n%2 == 1 {
		return sorted[n/2]
	}
	sum := sorted[n/2-1].Amount().Add(sorted[n/2].Amount())
	// ÷ 2 a au plus une décimale de plus que la somme : DivRound à cette précision est exact.
	m, _ := money.New(sum.DivRound(decimal.NewFromInt(2), -sum.Exponent()+1), string(sorted[0].Currency()))
	return m
}

func sameCurrency(costs []money.Money) error {
	for _, c := range costs[1:] {
		if c.Currency() != costs[0].Currency() {
			return fmt.Errorf("%s and %s: %w", costs[0].Currency(), c.Currency(), money.ErrCurrencyMismatch)
		}
	}
	return nil
}
