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
	if s.listing == nil {
		return CostAnomalies{}, fmt.Errorf("cost anomalies of bom %d: %w", bomID, ErrSuggestionsUnavailable)
	}
	b, nodes, err := s.Tree(ctx, bomID)
	if err != nil {
		return CostAnomalies{}, fmt.Errorf("cost anomalies of bom %d: %w", bomID, err)
	}
	currency := money.Currency(b.Currency)
	out := CostAnomalies{Threshold: threshold, Currency: currency, PriceSnapshotID: b.PriceSnapshotID}
	if b.PriceSnapshotID != nil {
		// Le snapshot de prix d'une BOM ne fige que ses propres offres : la population de la catégorie n'y est
		// pas. Plutôt qu'une médiane faussée (ou lue en direct en silence), chaque pièce est non évaluable.
		for _, n := range nodes {
			if n.IsPart() {
				out.Nodes = append(out.Nodes, NodeAnomaly{NodeID: n.ID, Anomaly: pricing.Anomaly{
					PartID: *n.PartID, Status: pricing.AnomalyNotEvaluable, Err: pricing.ErrPopulationNotFrozen,
				}})
			}
		}
		return out, nil
	}
	all, err := s.listing.ListParts(ctx, catalog.Filters{})
	if err != nil {
		return CostAnomalies{}, fmt.Errorf("cost anomalies of bom %d: %w", bomID, err)
	}

	// Prix en direct de chaque offre, un par un : une offre illisible (sans cotation, données invalides,
	// entité inconnue côté prix) écarte seulement sa pièce de la médiane, avec la raison ; une panne échoue.
	quotes := map[int64]PriceResult{}
	var fromCurrencies []money.Currency
	readOffer := func(id int64) error {
		if _, done := quotes[id]; done {
			return nil
		}
		q, err := s.pricing.OfferPrice(ctx, id)
		switch {
		case errors.Is(err, pricing.ErrNoQuote), errors.Is(err, pricing.ErrInvalidHistoryData),
			errors.Is(err, pricing.ErrPriceEntityNotFound), errors.Is(err, pricing.ErrInvalidSourceData):
			quotes[id] = PriceResult{Err: err}
		case err != nil:
			return err
		default:
			quotes[id] = PriceResult{Quote: q}
			if c := q.Price.Currency(); c != currency {
				fromCurrencies = append(fromCurrencies, c)
			}
		}
		return nil
	}
	for _, p := range all {
		for _, o := range p.Offers {
			if o.ArchivedAt == nil {
				if err := readOffer(o.ID); err != nil {
					return CostAnomalies{}, fmt.Errorf("cost anomalies of bom %d: %w", bomID, err)
				}
			}
		}
	}
	for _, id := range offerIDs(nodes) {
		if err := readOffer(id); err != nil {
			return CostAnomalies{}, fmt.Errorf("cost anomalies of bom %d: %w", bomID, err)
		}
	}
	slices.Sort(fromCurrencies)
	rates := map[money.Currency]RateResult{}
	for _, from := range slices.Compact(fromCurrencies) {
		r, err := s.pricing.Rate(ctx, from, currency)
		switch {
		case errors.Is(err, pricing.ErrNoRate):
			rates[from] = RateResult{Err: err}
		case err != nil:
			return CostAnomalies{}, fmt.Errorf("cost anomalies of bom %d: %w", bomID, err)
		default:
			rates[from] = RateResult{Rate: r}
			out.Rates = append(out.Rates, r)
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
	categories := make(map[int64]string, len(all))
	for _, p := range all {
		categories[p.ID] = p.Category
		sample := pricing.CostSample{PartID: p.ID, Category: p.Category, Err: pricing.ErrNoQuote}
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
	var subjectNodes []int64
	for _, n := range nodes {
		if !n.IsPart() {
			continue
		}
		cat, ok := categories[*n.PartID]
		if !ok { // pièce archivée : hors population active, on lit sa catégorie au catalogue
			p, err := s.catalog.Part(ctx, *n.PartID)
			if err != nil {
				return CostAnomalies{}, fmt.Errorf("cost anomalies of bom %d: node %d: %w", bomID, n.ID, err)
			}
			cat = p.Category
		}
		c, err := unit(*n.OfferID)
		subjects = append(subjects, pricing.CostSample{PartID: *n.PartID, Category: cat, Cost: c, Err: err})
		subjectNodes = append(subjectNodes, n.ID)
	}
	verdicts, err := s.anomalies.Detect(subjects, population, threshold)
	if err != nil {
		return CostAnomalies{}, fmt.Errorf("cost anomalies of bom %d: %w", bomID, err)
	}
	for i, v := range verdicts {
		out.Nodes = append(out.Nodes, NodeAnomaly{NodeID: subjectNodes[i], Anomaly: v})
	}
	slices.SortFunc(out.Nodes, func(a, b NodeAnomaly) int { return cmp.Compare(a.NodeID, b.NodeID) })
	return out, nil
}
