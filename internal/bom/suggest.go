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

// ErrArchived : la pièce ou l'offre est archivée (ADR 0009) : lisible, mais pas proposée ni ajoutée.
var ErrArchived = errors.New("archived in the catalog")

// Suggestions : alternatives proposées pour un nœud pièce, triées selon la stratégie qui s'y applique
// (ADR 0028). Rien n'est modifié : l'utilisateur applique une suggestion avec ReplaceNodePart.
type Suggestions struct {
	NodeID       int64
	Strategy     Strategy // "" : aucune stratégie dans la lignée, ordre par offre
	StrategyFrom int64    // nœud qui porte la stratégie (le nœud lui-même ou un ancêtre) ; 0 sans stratégie
	RealQuantity decimal.Decimal
	Currency     money.Currency
	// PriceSnapshotID : nil = prix en direct ; sinon les prix figés du brouillon (mêmes que ses totaux).
	PriceSnapshotID *int64
	Ranked          []Ranked
	Excluded        []Candidate
}

// Suggestions évalue, pour la quantité réelle du nœud, l'offre actuelle et chaque offre active de sa pièce et
// de ses alternatives déclarées, aux prix de la même source que les totaux du brouillon. Seul l'arbre est lu :
// une autre pièce incomplète de la BOM ne bloque pas les suggestions de celle-ci.
func (s *Service) Suggestions(ctx context.Context, bomID, nodeID int64) (Suggestions, error) {
	if s.listing == nil {
		return Suggestions{}, fmt.Errorf("suggestions for node %d in bom %d: %w", nodeID, bomID, ErrSuggestionsUnavailable)
	}
	b, nodes, err := s.Tree(ctx, bomID)
	if err != nil {
		return Suggestions{}, fmt.Errorf("suggestions for node %d in bom %d: %w", nodeID, bomID, err)
	}
	qty, n, err := realQuantity(nodes, nodeID)
	if err != nil {
		return Suggestions{}, fmt.Errorf("suggestions for node %d in bom %d: %w", nodeID, bomID, err)
	}
	if !n.IsPart() {
		return Suggestions{}, fmt.Errorf("suggestions for node %d in bom %d: %w", nodeID, bomID, ErrNotAPart)
	}
	strategy, from := EffectiveStrategy(nodes, nodeID)
	currency := money.Currency(b.Currency)

	cands, err := s.candidates(ctx, n, qty, currency, b.PriceSnapshotID)
	if err != nil {
		return Suggestions{}, fmt.Errorf("suggestions for node %d in bom %d: %w", nodeID, bomID, err)
	}
	ranked, excluded := Rank(strategy, cands)
	return Suggestions{
		NodeID: nodeID, Strategy: strategy, StrategyFrom: from, RealQuantity: qty, Currency: currency,
		PriceSnapshotID: b.PriceSnapshotID, Ranked: ranked, Excluded: excluded,
	}, nil
}

// realQuantity : produit des quantités de la racine au nœud (ADR 0007 règle 1), borné contre un cycle.
func realQuantity(nodes []Node, nodeID int64) (decimal.Decimal, Node, error) {
	byID := make(map[int64]Node, len(nodes))
	for _, n := range nodes {
		byID[n.ID] = n
	}
	n, ok := byID[nodeID]
	if !ok {
		return decimal.Decimal{}, Node{}, fmt.Errorf("node %d: %w", nodeID, ErrNodeNotFound)
	}
	qty := n.Quantity
	cur := n
	for steps := 0; cur.ParentID != nil; steps++ {
		p, ok := byID[*cur.ParentID]
		if !ok || steps > len(nodes) {
			return decimal.Decimal{}, Node{}, fmt.Errorf("node %d not under root: %w", nodeID, ErrInvalidTree)
		}
		qty = qty.Mul(p.Quantity)
		cur = p
	}
	return qty, n, nil
}

type pendingCandidate struct {
	part     catalog.Part
	offer    catalog.Offer
	excluded error // pièce ou offre actuelle archivée
}

// candidates : l'offre actuelle du nœud (toujours, même archivée : exclue avec la raison) et chaque offre active
// de sa pièce et de ses alternatives non archivées.
func (s *Service) candidates(ctx context.Context, n Node, qty decimal.Decimal, currency money.Currency, snapID *int64) ([]Candidate, error) {
	alts, err := s.listing.Alternatives(ctx, *n.PartID)
	if err != nil {
		return nil, err
	}
	all, err := s.listing.ListParts(ctx, catalog.Filters{})
	if err != nil {
		return nil, err
	}
	byPart := make(map[int64]catalog.PartWithOffers, len(all))
	for _, p := range all {
		byPart[p.ID] = p
	}
	partIDs := []int64{*n.PartID}
	for _, a := range alts {
		if a.ArchivedAt == nil {
			partIDs = append(partIDs, a.ID)
		}
	}
	slices.Sort(partIDs)
	partIDs = slices.Compact(partIDs)

	// L'offre actuelle, lue par id : elle reste visible si la pièce ou l'offre a été archivée depuis.
	curPart, err := s.catalog.Part(ctx, *n.PartID)
	if err != nil {
		return nil, err
	}
	curOffer, err := s.catalog.Offer(ctx, *n.OfferID)
	if err != nil {
		return nil, err
	}
	todo := []pendingCandidate{{part: curPart, offer: curOffer}}
	if curPart.ArchivedAt != nil || curOffer.ArchivedAt != nil {
		todo[0].excluded = fmt.Errorf("current offer %d: %w", curOffer.ID, ErrArchived)
	}
	for _, pid := range partIDs {
		p, ok := byPart[pid]
		if !ok {
			continue // pièce archivée : absente de la liste active, pas de candidate
		}
		for _, o := range p.Offers {
			if o.ArchivedAt == nil && o.ID != curOffer.ID {
				todo = append(todo, pendingCandidate{part: p.Part, offer: o})
			}
		}
	}
	offers := make([]int64, len(todo))
	for i, t := range todo {
		offers[i] = t.offer.ID
	}
	slices.Sort(offers)
	prices, rates, err := LoadPrices(ctx, s.pricing, snapID, slices.Compact(offers), currency)
	if err != nil {
		return nil, err
	}
	out := make([]Candidate, 0, len(todo))
	for _, t := range todo {
		out = append(out, evaluate(t, qty, currency, prices, rates, curOffer.ID))
	}
	slices.SortFunc(out, func(a, b Candidate) int { return cmp.Compare(a.OfferID, b.OfferID) })
	return out, nil
}

func evaluate(t pendingCandidate, qty decimal.Decimal, currency money.Currency, prices map[int64]PriceResult,
	rates map[money.Currency]RateResult, current int64,
) Candidate {
	c := Candidate{PartID: t.part.ID, OfferID: t.offer.ID, Current: t.offer.ID == current}
	attrs, err := t.part.BOMAttributes()
	if err != nil {
		c.Excluded = err
		return c
	}
	lead, co2 := t.offer.LeadTimeDays+attrs.ProductionDays, attrs.CO2KGPerUnit.Mul(qty)
	c.LeadDays, c.CO2Kg, c.HasFacts = lead, co2, true
	if pr := prices[t.offer.ID]; pr.Err == nil {
		q := pr.Quote
		c.Quote = &q
	}
	cost, rate, err := convertedCost(prices[t.offer.ID], rates, qty, currency)
	c.Rate = rate
	switch {
	case t.excluded != nil:
		c.Excluded = t.excluded
	case err != nil:
		c.Excluded = fmt.Errorf("offer %d: %w", t.offer.ID, err)
	default:
		c.Cost = &cost
	}
	return c
}

// convertedCost : prix × quantité, converti dans la devise de la BOM, et le taux utilisé (nil : même devise) ;
// prix ou taux absent → erreur nommée.
func convertedCost(pr PriceResult, rates map[money.Currency]RateResult, qty decimal.Decimal, to money.Currency) (
	money.Money, *pricing.Rate, error,
) {
	if pr.Err != nil {
		return money.Money{}, nil, pr.Err
	}
	if pr.Quote.Price.Currency() == "" {
		return money.Money{}, nil, pricing.ErrNoQuote
	}
	cost := pr.Quote.Price.Mul(qty)
	from := cost.Currency()
	if from == to {
		return cost, nil, nil
	}
	rr, ok := rates[from]
	if !ok {
		return money.Money{}, nil, fmt.Errorf("rate %s→%s: %w", from, to, pricing.ErrNoRate)
	}
	if rr.Err != nil {
		return money.Money{}, nil, rr.Err
	}
	if rr.Rate.From != from || rr.Rate.To != to {
		return money.Money{}, nil, fmt.Errorf("rate for %s→%s is %s→%s: %w", from, to, rr.Rate.From, rr.Rate.To, pricing.ErrNoRate)
	}
	r := rr.Rate
	m, err := money.New(cost.Amount().Mul(r.Rate), string(to))
	return m, &r, err
}
