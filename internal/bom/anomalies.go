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
	out, _, _, err := s.anomaliesWith(ctx, bomID, anomalySpec{detector: s.anomalies}, threshold)
	return out, err
}

// anomalyNode : ce que la détection a lu d'un nœud pièce, pour un regroupement par pièce (ADR 0037).
type anomalyNode struct {
	part catalog.Part
	node Node
}

// familyOf : catégorie × matière de la pièce ; vide si l'une des deux manque (jamais devinée).
func familyOf(p catalog.Part) string {
	if p.Category == "" || p.Material == nil {
		return ""
	}
	return p.Category + " · " + *p.Material
}

// anomaliesWith juge chaque nœud pièce avec le détecteur de spec ; population et sujets portent coût unitaire converti
// et masse de la pièce. Rend aussi la pièce et le nœud de chaque verdict, et l'arbre lu.
func (s *Service) anomaliesWith(ctx context.Context, bomID int64, spec anomalySpec, threshold decimal.Decimal) (
	CostAnomalies, []anomalyNode, []Node, error,
) {
	if s.listing == nil {
		return CostAnomalies{}, nil, nil, fmt.Errorf("cost anomalies of bom %d: %w", bomID, ErrSuggestionsUnavailable)
	}
	b, nodes, err := s.Tree(ctx, bomID)
	if err != nil {
		return CostAnomalies{}, nil, nil, fmt.Errorf("cost anomalies of bom %d: %w", bomID, err)
	}
	detector, need := spec.detector, spec.needs
	if detector == nil {
		return CostAnomalies{}, nil, nil, fmt.Errorf("cost anomalies of bom %d: no detector: %w", bomID, ErrInvalidInput)
	}
	if need.history && s.history == nil {
		return CostAnomalies{}, nil, nil, fmt.Errorf("cost anomalies of bom %d: %w", bomID, ErrSuggestionsUnavailable)
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
		if need.family {
			sample.Family = familyOf(p.Part)
		}
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

	// Méthode « offres » : coût de chaque offre active de la pièce ; une offre sans prix ou sans taux est comptée
	// à part (Missing), jamais remplacée par un prix.
	offerCosts := map[int64][]money.Money{}
	offerMissing, offerArchived := map[int64]int{}, map[int64]int{}
	activeParts := map[int64]struct{}{}
	if need.offers {
		for _, p := range all {
			activeParts[p.ID] = struct{}{}
			for _, o := range p.Offers {
				if o.ArchivedAt != nil {
					offerArchived[p.ID]++
					continue
				}
				if c, err := unit(o.ID); err != nil {
					offerMissing[p.ID]++
				} else {
					offerCosts[p.ID] = append(offerCosts[p.ID], *c)
				}
			}
		}
	}
	// Méthode « historique » : cotations des 12 derniers mois de chaque offre retenue, lues en une seule requête.
	trails := map[int64]*pricing.PriceTrail{}
	if need.history {
		var windows []pricing.HistoryWindow
		seenOffer := map[int64]bool{}
		for _, n := range nodes {
			if !n.IsPart() || seenOffer[*n.OfferID] || quotes[*n.OfferID].Err != nil {
				continue
			}
			seenOffer[*n.OfferID] = true
			at := quotes[*n.OfferID].Quote.QuotedAt
			windows = append(windows, pricing.HistoryWindow{OfferID: *n.OfferID, From: pricing.AnnualWindowStart(at), To: at})
		}
		hist, err := s.history.ReadWindows(ctx, windows)
		if err != nil {
			return CostAnomalies{}, nil, nil, fmt.Errorf("cost anomalies of bom %d: history: %w", bomID, err)
		}
		for _, w := range windows {
			trails[w.OfferID] = trailFrom(quotes[w.OfferID].Quote, hist[w.OfferID])
		}
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
		sample := pricing.CostSample{PartID: *n.PartID, Category: p.Category, Cost: c, MassKg: p.MassKG, Err: err}
		if need.family {
			sample.Family = familyOf(p)
		}
		if need.offers {
			sample.OfferCosts, sample.MissingOffers, sample.ArchivedOffers = offerCosts[p.ID], offerMissing[p.ID], offerArchived[p.ID]
			if _, active := activeParts[p.ID]; !active && err == nil {
				sample.Err = pricing.ErrPartArchived // hors population active : ses offres ne sont pas comparées
			}
		}
		if need.history && err == nil {
			sample.Trail = trails[*n.OfferID]
		}
		subjects = append(subjects, sample)
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

// trailFrom : la cotation courante d'une offre et ses cotations passées de la fenêtre de 12 mois, dans la devise
// de l'offre. La cotation courante n'est pas dans le passé ; une cotation dans une autre devise est écartée mais
// comptée (OtherCurrency), jamais convertie.
func trailFrom(current pricing.Quote, hist []pricing.HistoricalQuote) *pricing.PriceTrail {
	start := pricing.AnnualWindowStart(current.QuotedAt)
	t := &pricing.PriceTrail{Current: current.Price}
	for _, h := range hist {
		if h.ID == current.ID || h.QuotedAt.Before(start) {
			continue
		}
		if h.Price.Currency() != current.Price.Currency() {
			t.OtherCurrency++
			continue
		}
		t.Past = append(t.Past, h.Price)
	}
	return t
}
