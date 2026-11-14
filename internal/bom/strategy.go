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
	HasFacts bool // LeadDays et CO2Kg sont connus (faux si un attribut de la pièce manque)
	// Quote et Rate : cotation et taux utilisés pour Cost (explicabilité, AGENTS §3) ; Rate nil si même devise.
	Quote *pricing.Quote
	Rate  *pricing.Rate
	// Excluded : prix, taux ou attribut manquant → pas de score inventé (ADR 0028), la raison est donnée.
	Excluded error
}

// Ranked : candidate classée. Scores normalisés entre 0 (meilleur) et 1 sur les candidates comparées.
type Ranked struct {
	Candidate
	CostScore, LeadScore, CO2Score decimal.Decimal
	Score                          decimal.Decimal // score qui trie selon la stratégie
}

// Rank classe les candidates complètes selon la stratégie ; les exclues sont rendues à part, par offre.
// Strategy vide : pas de tri imposé, ordre par offre (ADR 0028). Égalité de score → id d'offre croissant.
func Rank(strategy Strategy, cands []Candidate) (ranked []Ranked, excluded []Candidate) {
	for _, c := range cands {
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
	score := scoreOf(strategy)
	for i := range ranked {
		ranked[i].Score = score(ranked[i])
	}
	slices.SortFunc(ranked, func(a, b Ranked) int {
		return cmp.Or(a.Score.Cmp(b.Score), cmp.Compare(a.OfferID, b.OfferID))
	})
	return ranked, excluded
}

// scoreOf : le seul endroit où une stratégie devient un calcul (AGENTS §4).
func scoreOf(s Strategy) func(Ranked) decimal.Decimal {
	switch s {
	case StrategyCost:
		return func(r Ranked) decimal.Decimal { return r.CostScore }
	case StrategyLeadTime:
		return func(r Ranked) decimal.Decimal { return r.LeadScore }
	case StrategyCO2:
		return func(r Ranked) decimal.Decimal { return r.CO2Score }
	case StrategyBalanced:
		three := decimal.NewFromInt(3)
		return func(r Ranked) decimal.Decimal {
			return r.CostScore.Add(r.LeadScore).Add(r.CO2Score).DivRound(three, ftePrecision)
		}
	default: // sans stratégie : tous à égalité, l'ordre par offre décide
		return func(Ranked) decimal.Decimal { return decimal.Zero }
	}
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
