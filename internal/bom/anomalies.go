package bom

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/shopspring/decimal"

	"bomstudio/internal/catalog"
	"bomstudio/internal/money"
	"bomstudio/internal/pricing"
)

// NodeAnomaly : verdict d'anomalie de coût pour un nœud pièce (cost-anomalies, ADR 0027).
type NodeAnomaly struct {
	NodeID int64
	pricing.Anomaly
}

// CostAnomalies : verdicts de tous les nœuds pièces d'un brouillon et sur quoi ils reposent.
type CostAnomalies struct {
	Threshold       decimal.Decimal
	Currency        money.Currency
	PriceSnapshotID *int64         // nil : prix en direct ; sinon aucune pièce n'est évaluable (ErrPopulationNotFrozen)
	Rates           []pricing.Rate // taux utilisés (et leur date), triés par devise d'origine
	Nodes           []NodeAnomaly
}

// CostAnomalies compare le prix unitaire de l'offre choisie par chaque nœud pièce à la médiane des pièces
// actives de sa catégorie, chacune à son offre la moins chère, tout converti dans la devise de la BOM aux
// prix de la même source que les totaux (ADR 0027). Rien n'est corrigé : la BOM n'est pas modifiée.
func (s *Service) CostAnomalies(ctx context.Context, bomID int64, threshold decimal.Decimal) (CostAnomalies, error) {
	out, _, _, err := s.anomaliesWith(ctx, bomID, s.anomalies, threshold)
	return out, err
}

// anomalyNode : ce que la détection a lu d'un nœud pièce, pour un regroupement par pièce (ADR 0037).
type anomalyNode struct {
	part catalog.Part
	node Node
}

// anomaliesWith juge chaque nœud pièce avec detector ; population et sujets portent coût unitaire converti
// et masse de la pièce. Rend aussi la pièce et le nœud de chaque verdict, et l'arbre lu.
func (s *Service) anomaliesWith(ctx context.Context, bomID int64, detector pricing.AnomalyDetector, threshold decimal.Decimal) (
	CostAnomalies, []anomalyNode, []Node, error,
) {
	if s.listing == nil {
		return CostAnomalies{}, nil, nil, fmt.Errorf("cost anomalies of bom %d: %w", bomID, ErrSuggestionsUnavailable)
	}
	b, nodes, err := s.Tree(ctx, bomID)
	if err != nil {
		return CostAnomalies{}, nil, nil, fmt.Errorf("cost anomalies of bom %d: %w", bomID, err)
	}
	if detector == nil {
		return CostAnomalies{}, nil, nil, fmt.Errorf("cost anomalies of bom %d: no detector: %w", bomID, ErrInvalidInput)
	}
	currency := money.Currency(b.Currency)
	out := CostAnomalies{Threshold: threshold, Currency: currency, PriceSnapshotID: b.PriceSnapshotID}
	if b.PriceSnapshotID != nil {
		// Le snapshot de prix d'une BOM ne fige que ses propres offres : la population de la catégorie n'y est
		// pas. Plutôt qu'une médiane faussée (ou lue en direct en silence), chaque pièce est non évaluable.
		var seen []anomalyNode
		read := map[int64]catalog.Part{}
		for _, n := range nodes {
			if n.IsPart() {
				p, ok := read[*n.PartID]
				if !ok {
					var err error
					if p, err = s.catalog.Part(ctx, *n.PartID); err != nil {
						return CostAnomalies{}, nil, nil, fmt.Errorf("cost anomalies of bom %d: node %d: %w", bomID, n.ID, err)
					}
					read[p.ID] = p
				}
				out.Nodes = append(out.Nodes, NodeAnomaly{NodeID: n.ID, Anomaly: pricing.Anomaly{
					PartID: *n.PartID, Category: p.Category, Status: pricing.AnomalyNotEvaluable, Err: pricing.ErrPopulationNotFrozen,
				}})
				seen = append(seen, anomalyNode{part: p, node: n})
			}
		}
		return out, seen, nodes, nil
	}
	all, err := s.listing.ListParts(ctx, catalog.Filters{})
	if err != nil {
		return CostAnomalies{}, nil, nil, fmt.Errorf("cost anomalies of bom %d: %w", bomID, err)
	}

	// Prix en direct de toutes les offres actives du catalogue et de celles de la BOM, en une seule lecture :
	// une offre illisible (sans cotation, données invalides, entité inconnue côté prix) écarte seulement sa
	// pièce de la médiane, avec la raison ; une panne échoue.
	var ids []int64
	for _, p := range all {
		for _, o := range p.Offers {
			if o.ArchivedAt == nil {
				ids = append(ids, o.ID)
			}
		}
	}
	ids = append(ids, offerIDs(nodes)...)
	slices.Sort(ids)
	ids = slices.Compact(ids)
	quotes, err := s.pricing.OfferPrices(ctx, ids)
	if err != nil {
		return CostAnomalies{}, nil, nil, fmt.Errorf("cost anomalies of bom %d: %w", bomID, err)
	}
	var fromCurrencies []money.Currency
	for _, id := range ids {
		q := quotes[id]
		switch {
		case q.Err == nil:
			if c := q.Quote.Price.Currency(); c != currency {
				fromCurrencies = append(fromCurrencies, c)
			}
		case errors.Is(q.Err, pricing.ErrNoQuote), errors.Is(q.Err, pricing.ErrInvalidHistoryData),
			errors.Is(q.Err, pricing.ErrPriceEntityNotFound), errors.Is(q.Err, pricing.ErrInvalidSourceData):
			quotes[id] = PriceResult{Err: q.Err}
		default:
			return CostAnomalies{}, nil, nil, fmt.Errorf("cost anomalies of bom %d: offer %d: %w", bomID, id, q.Err)
		}
	}
	slices.Sort(fromCurrencies)
	fromCurrencies = slices.Compact(fromCurrencies)
	pairs := make([]pricing.FXPair, 0, len(fromCurrencies))
	for _, from := range fromCurrencies {
		pairs = append(pairs, pricing.FXPair{Base: from, Quote: currency})
	}
	found, err := s.pricing.PairRates(ctx, pairs)
	if err != nil {
		return CostAnomalies{}, nil, nil, fmt.Errorf("cost anomalies of bom %d: %w", bomID, err)
	}
	rates := map[money.Currency]RateResult{}
	for _, from := range fromCurrencies {
		r := found[pricing.FXPair{Base: from, Quote: currency}]
		switch {
		case errors.Is(r.Err, pricing.ErrNoRate):
			rates[from] = RateResult{Err: r.Err}
		case r.Err != nil:
			return CostAnomalies{}, nil, nil, fmt.Errorf("cost anomalies of bom %d: %w", bomID, r.Err)
		default:
			rates[from] = RateResult{Rate: r.Rate}
			out.Rates = append(out.Rates, r.Rate)
		}
	}
	one := decimal.NewFromInt(1)
	unit := func(offerID int64) (*money.Money, error) {
		c, _, err := convertedCost(quotes[offerID], rates, one, currency)
		if err != nil {
			return nil, fmt.Errorf("offer %d: %w", offerID, err)
		}
		return &c, nil
	}

	// Population : chaque pièce active à son offre la moins chère (après conversion), égalité → id d'offre.
	// Une pièce dont une offre est sans prix ou sans taux n'est pas jugée sur une autre offre plus chère :
	// elle est écartée avec la raison (sinon la médiane serait tirée vers le haut en silence).
	population := make([]pricing.CostSample, 0, len(all))
	catalogParts := make(map[int64]catalog.Part, len(all))
	for _, p := range all {
		catalogParts[p.ID] = p.Part
		sample := pricing.CostSample{PartID: p.ID, Category: p.Category, MassKg: p.MassKG, Err: pricing.ErrNoQuote}
		for _, o := range p.Offers {
			if o.ArchivedAt != nil {
				continue
			}
			c, err := unit(o.ID)
			if err != nil {
				sample.Cost, sample.Err = nil, err
				break
			}
			if sample.Cost == nil || c.Amount().LessThan(sample.Cost.Amount()) {
				sample.Cost, sample.Err = c, nil
			}
		}
		population = append(population, sample)
	}

	var subjects []pricing.CostSample
	var judged []anomalyNode
	for _, n := range nodes {
		if !n.IsPart() {
			continue
		}
		p, ok := catalogParts[*n.PartID]
		if !ok { // pièce archivée : hors population active, on la lit au catalogue (une fois)
			var err error
			if p, err = s.catalog.Part(ctx, *n.PartID); err != nil {
				return CostAnomalies{}, nil, nil, fmt.Errorf("cost anomalies of bom %d: node %d: %w", bomID, n.ID, err)
			}
			catalogParts[p.ID] = p
		}
		c, err := unit(*n.OfferID)
		subjects = append(subjects, pricing.CostSample{PartID: *n.PartID, Category: p.Category, Cost: c, MassKg: p.MassKG, Err: err})
		judged = append(judged, anomalyNode{part: p, node: n})
	}
	verdicts, err := detector.Detect(subjects, population, threshold)
	if err != nil {
		return CostAnomalies{}, nil, nil, fmt.Errorf("cost anomalies of bom %d: %w", bomID, err)
	}
	for i, v := range verdicts {
		out.Nodes = append(out.Nodes, NodeAnomaly{NodeID: judged[i].node.ID, Anomaly: v})
	}
	// nodes est trié par id (Store.Tree) : out.Nodes et judged le sont aussi, dans le même ordre.
	slices.SortFunc(out.Nodes, func(a, b NodeAnomaly) int { return cmp.Compare(a.NodeID, b.NodeID) })
	return out, judged, nodes, nil
}
