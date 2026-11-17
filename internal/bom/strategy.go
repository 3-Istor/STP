package bom

import (
	"cmp"
	"errors"
	"fmt"
	"slices"

	"github.com/shopspring/decimal"

	"bomstudio/internal/money"
	"bomstudio/internal/pricing"
)

// Strategy : stratégie de décision attachée à un nœud (ADR 0028). Les pièces du sous-arbre suivent celle de
// leur ancêtre le plus proche (le nœud lui-même compris) qui en a une.
type Strategy string

const (
	StrategyCost     Strategy = "cost"      // coût minimum
	StrategyLeadTime Strategy = "lead_time" // délai minimum
	StrategyCO2      Strategy = "co2"       // CO2 minimum
	StrategyBalanced Strategy = "balanced"  // moyenne à poids égaux des trois scores normalisés
)

// ErrUnknownStrategy : la stratégie demandée n'est pas l'une des quatre (stratégies programmables : Won't).
var ErrUnknownStrategy = errors.New("unknown strategy")

// ParseStrategy valide une stratégie saisie.
func ParseStrategy(s string) (Strategy, error) {
	switch st := Strategy(s); st {
	case StrategyCost, StrategyLeadTime, StrategyCO2, StrategyBalanced:
		return st, nil
	}
	return "", fmt.Errorf("strategy %q (want cost, lead_time, co2 or balanced): %w", s, ErrUnknownStrategy)
}

// Candidate : une offre (de la pièce du nœud ou d'une alternative déclarée) évaluée pour la quantité réelle
// du nœud. Cost est converti dans la devise de la BOM ; une candidate incomplète porte Excluded.
type Candidate struct {
	PartID   int64
	OfferID  int64
	Current  bool // l'offre actuellement choisie par le nœud
	Cost     *money.Money
	LeadDays int // livraison de l'offre + production de la pièce
	CO2Kg    decimal.Decimal
	// MassKg : masse unitaire de la pièce × quantité réelle (critère masse d'un profil, ADR 0036).
	MassKg   decimal.Decimal
	HasFacts bool // LeadDays, CO2Kg et MassKg sont connus (faux si un attribut de la pièce manque)
	// Quality : note qualité de l'offre (plus haute = meilleure) ; nil si non renseignée.
	Quality *decimal.Decimal
	// Quote et Rate : cotation et taux utilisés pour Cost (explicabilité, AGENTS §3) ; Rate nil si même devise.
	Quote *pricing.Quote
	Rate  *pricing.Rate
	// Excluded : prix, taux ou attribut manquant → pas de score inventé (ADR 0028), la raison est donnée.
	Excluded error
}

// ErrNoQualityScore : la qualité pèse dans le classement mais l'offre n'a pas de note (ADR 0036) : la
// candidate est exclue avec cette raison, jamais notée 0.
var ErrNoQualityScore = errors.New("offer has no quality score")

// Ranked : candidate classée. Scores normalisés entre 0 (meilleur) et 1 sur les candidates comparées ; un
// critère de poids nul n'est pas évalué (score 0).
type Ranked struct {
	Candidate
	CostScore, LeadScore, CO2Score decimal.Decimal
	MassScore, QualityScore        decimal.Decimal
	Score                          decimal.Decimal // moyenne pondérée qui trie
}

// Rank classe les candidates complètes selon la stratégie ; les exclues sont rendues à part, par offre.
// Strategy vide : pas de tri imposé, ordre par offre (ADR 0028). Égalité de score → id d'offre croissant.
func Rank(strategy Strategy, cands []Candidate) (ranked []Ranked, excluded []Candidate) {
	return RankWeighted(strategyWeights(strategy), cands)
}

// RankWeighted classe les candidates par la moyenne pondérée de leurs scores normalisés (ADR 0036) :
// Σ poids × score ÷ Σ poids. Poids tous nuls : ordre par offre. Si la qualité pèse, une candidate sans note est
// exclue (ErrNoQualityScore).
func RankWeighted(w Weights, cands []Candidate) (ranked []Ranked, excluded []Candidate) {
	for _, c := range cands {
		if c.Excluded == nil && w.Quality > 0 && c.Quality == nil {
			c.Excluded = fmt.Errorf("offer %d: %w", c.OfferID, ErrNoQualityScore)
		}
		if c.Excluded != nil {
			excluded = append(excluded, c)
			continue
		}
		ranked = append(ranked, Ranked{Candidate: c})
	}
	slices.SortFunc(excluded, func(a, b Candidate) int { return cmp.Compare(a.OfferID, b.OfferID) })
	if len(ranked) == 0 {
		return ranked, excluded
	}
	normalize(ranked, func(r Ranked) decimal.Decimal { return r.Cost.Amount() }, func(r *Ranked, s decimal.Decimal) { r.CostScore = s })
	normalize(ranked, func(r Ranked) decimal.Decimal { return decimal.NewFromInt(int64(r.LeadDays)) },
		func(r *Ranked, s decimal.Decimal) { r.LeadScore = s })
	normalize(ranked, func(r Ranked) decimal.Decimal { return r.CO2Kg }, func(r *Ranked, s decimal.Decimal) { r.CO2Score = s })
	if w.Mass > 0 {
		normalize(ranked, func(r Ranked) decimal.Decimal { return r.MassKg }, func(r *Ranked, s decimal.Decimal) { r.MassScore = s })
	}
	if w.Quality > 0 { // plus haute = meilleure : on normalise l'opposé
		normalize(ranked, func(r Ranked) decimal.Decimal { return r.Quality.Neg() }, func(r *Ranked, s decimal.Decimal) { r.QualityScore = s })
	}
	for i := range ranked {
		ranked[i].Score = weightedScore(w, ranked[i])
	}
	slices.SortFunc(ranked, func(a, b Ranked) int {
		return cmp.Or(a.Score.Cmp(b.Score), cmp.Compare(a.OfferID, b.OfferID))
	})
	return ranked, excluded
}

// strategyWeights : le seul endroit où une stratégie de nœud devient un calcul (AGENTS §4) — des poids.
// « balanced » est le cas à poids égaux sur coût, délai et CO2 (ADR 0028, ADR 0036).
func strategyWeights(s Strategy) Weights {
	switch s {
	case StrategyCost:
		return Weights{Cost: 1}
	case StrategyLeadTime:
		return Weights{LeadTime: 1}
	case StrategyCO2:
		return Weights{CO2: 1}
	case StrategyBalanced:
		return Weights{Cost: 1, LeadTime: 1, CO2: 1}
	default: // sans stratégie : tous à égalité, l'ordre par offre décide
		return Weights{}
	}
}

// weightedScore : Σ poids × score ÷ Σ poids, à ftePrecision décimales ; 0 si tous les poids sont nuls.
func weightedScore(w Weights, r Ranked) decimal.Decimal {
	total := w.sum()
	if total == 0 {
		return decimal.Zero
	}
	sum := r.CostScore.Mul(decimal.NewFromInt(int64(w.Cost))).
		Add(r.LeadScore.Mul(decimal.NewFromInt(int64(w.LeadTime)))).
		Add(r.CO2Score.Mul(decimal.NewFromInt(int64(w.CO2)))).
		Add(r.MassScore.Mul(decimal.NewFromInt(int64(w.Mass)))).
		Add(r.QualityScore.Mul(decimal.NewFromInt(int64(w.Quality))))
	return sum.DivRound(decimal.NewFromInt(int64(total)), ftePrecision)
}

// normalize pose (v − min) ÷ (max − min) ; si toutes les valeurs sont égales, 0 pour toutes (ADR 0028).
func normalize(rs []Ranked, value func(Ranked) decimal.Decimal, set func(*Ranked, decimal.Decimal)) {
	lo, hi := value(rs[0]), value(rs[0])
	for _, r := range rs[1:] {
		v := value(r)
		lo, hi = decimal.Min(lo, v), decimal.Max(hi, v)
	}
	span := hi.Sub(lo)
	for i := range rs {
		if span.IsZero() {
			set(&rs[i], decimal.Zero)
			continue
		}
		set(&rs[i], value(rs[i]).Sub(lo).DivRound(span, ftePrecision))
	}
}

// EffectiveStrategy : stratégie du nœud ou de son ancêtre le plus proche qui en a une ; from = ce nœud.
// Sans stratégie dans la lignée : "" et from = 0.
func EffectiveStrategy(nodes []Node, nodeID int64) (s Strategy, from int64) {
	byID := make(map[int64]Node, len(nodes))
	for _, n := range nodes {
		byID[n.ID] = n
	}
	for steps, cur := 0, nodeID; steps <= len(nodes); steps++ { // borne : pas de boucle même sur un arbre invalide
		n, ok := byID[cur]
		if !ok {
			return "", 0
		}
		if n.Strategy != nil {
			return *n.Strategy, n.ID
		}
		if n.ParentID == nil {
			return "", 0
		}
		cur = *n.ParentID
	}
	return "", 0
}
