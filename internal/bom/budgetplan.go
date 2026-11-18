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
)

// ErrNoBudget : le nœud n'a pas de budget cible, il n'y a rien à ramener dedans.
var ErrNoBudget = errors.New("node has no target budget")

// ErrPlanTooLarge : le sous-arbre a plus de pièces que le plan n'en évalue.
var ErrPlanTooLarge = errors.New("too many parts under the node to build a budget plan")

const (
	// maxPlanParts : plafond de pièces évaluées (mesuré : 2 705 pièces sous « Carrosserie ») ; au-delà, choisir un sous-ensemble.
	maxPlanParts = 5000
	// planExtraActions : actions proposées au-delà de celle qui couvre le dépassement.
	planExtraActions = 3
	// maxPlanActions : plafond d'actions rendues quand rien ne couvre le dépassement.
	maxPlanActions = 20
)

// PlanSaving : la meilleure économie d'une pièce du sous-arbre : l'offre moins chère de la même pièce ou d'une
// pièce alternative. Gain, CO2Delta et MassDelta sont par véhicule (quantité réelle comprise) ; les deltas
// sont « après − avant ».
type PlanSaving struct {
	NodeID                 int64
	NodeName               string
	FromPartID, ToPartID   int64
	FromOfferID, ToOfferID int64
	FromPart, ToPart       string
	FromSupplier           string
	ToSupplier             string
	FromMaterial           string // "" : matière non renseignée
	ToMaterial             string
	Gain                   decimal.Decimal
	CO2Delta, MassDelta    decimal.Decimal
	LeadDelta              int
	FromQuality, ToQuality *decimal.Decimal
}

// PlanAction : une même substitution (offre avant → offre après) regroupée sur tous les nœuds où elle s'applique.
type PlanAction struct {
	Nodes []PlanNode
	PlanSaving
	// Cumulative : gain cumulé de cette action et des précédentes ; Covers : première action qui couvre le dépassement.
	Cumulative decimal.Decimal
	Covers     bool
}

// PlanNode : un nœud touché par l'action.
type PlanNode struct {
	ID   int64
	Name string
}

// BudgetPlan : comment ramener un nœud dans son budget. Rien n'est appliqué : l'utilisateur applique les actions
// une à une (ReplaceNodePart), le plan se relit ensuite.
type BudgetPlan struct {
	NodeID   int64
	Currency money.Currency
	Budget   money.Money
	Cost     money.Money
	// Overrun : coût − budget, zéro si le nœud est dans son budget (plan vide).
	Overrun money.Money
	Actions []PlanAction
	// Remaining : ce qu'aucune action ne couvre (> 0 seulement quand Covered est faux).
	Remaining money.Money
	Covered   bool
}

// BuildPlanActions regroupe les économies par substitution, trie par gain décroissant (égalité : offre cible, puis
// premier nœud), cumule jusqu'à couvrir overrun et garde planExtraActions actions de plus. Rend aussi ce qui
// reste à trouver si l'ensemble des actions ne suffit pas (jamais négatif).
func BuildPlanActions(overrun decimal.Decimal, savings []PlanSaving) (actions []PlanAction, remaining decimal.Decimal) {
	type key struct{ from, to int64 }
	idx := map[key]int{}
	var groups []PlanAction
	for _, sv := range savings {
		k := key{sv.FromOfferID, sv.ToOfferID}
		i, ok := idx[k]
		if !ok {
			idx[k] = len(groups)
			groups = append(groups, PlanAction{PlanSaving: sv, Nodes: []PlanNode{{sv.NodeID, sv.NodeName}}})
			continue
		}
		g := &groups[i]
		g.Gain = g.Gain.Add(sv.Gain)
		g.CO2Delta = g.CO2Delta.Add(sv.CO2Delta)
		g.MassDelta = g.MassDelta.Add(sv.MassDelta)
		g.Nodes = append(g.Nodes, PlanNode{sv.NodeID, sv.NodeName})
	}
	slices.SortFunc(groups, func(a, b PlanAction) int {
		return cmp.Or(b.Gain.Cmp(a.Gain), cmp.Compare(a.ToOfferID, b.ToOfferID), cmp.Compare(a.NodeID, b.NodeID))
	})
	cum, covered, extra := decimal.Zero, false, 0
	for _, g := range groups {
		if len(actions) == maxPlanActions {
			break
		}
		if covered {
			if extra == planExtraActions {
				break
			}
			extra++
		}
		cum = cum.Add(g.Gain)
		g.Cumulative = cum
		if !covered && cum.Cmp(overrun) >= 0 {
			g.Covers, covered = true, true
		}
		actions = append(actions, g)
	}
	total := decimal.Zero
	for _, g := range groups {
		total = total.Add(g.Gain)
	}
	return actions, decimal.Max(overrun.Sub(total), decimal.Zero)
}

// BudgetPlan propose, pour un nœud au-dessus de son budget, les substitutions d'offre ou de pièce qui l'y
// ramènent, avec les mêmes candidates que Suggestions : la moins chère par pièce du sous-arbre (gain sur le prix
// d'achat × quantité réelle, quelle que soit la méthode de coût). Dans le budget : plan vide.
func (s *Service) BudgetPlan(ctx context.Context, bomID, nodeID int64) (BudgetPlan, error) {
	fail := func(err error) (BudgetPlan, error) {
		return BudgetPlan{}, fmt.Errorf("budget plan for node %d in bom %d: %w", nodeID, bomID, err)
	}
	if s.listing == nil {
		return fail(ErrSuggestionsUnavailable)
	}
	dc, err := s.DraftCost(ctx, bomID)
	if err != nil {
		return fail(err)
	}
	if _, ok := dc.Totals.Node(nodeID); !ok {
		return fail(fmt.Errorf("node %d: %w", nodeID, ErrNodeNotFound))
	}
	i := slices.IndexFunc(dc.Budgets, func(b TargetBudget) bool { return b.NodeID == nodeID })
	if i < 0 {
		return fail(ErrNoBudget)
	}
	sts, err := s.budgetStatuses(ctx, dc.BOM, dc.Nodes, dc.Totals, dc.Budgets[i:i+1])
	if err != nil {
		return fail(err)
	}
	st := sts[0]
	if st.Cost == nil || st.Delta == nil {
		return fail(fmt.Errorf("%w: %s", ErrMissingCost, st.Error))
	}
	currency := money.Currency(dc.BOM.Currency)
	zero, _ := money.New(decimal.Zero, string(currency))
	out := BudgetPlan{NodeID: nodeID, Currency: currency, Cost: *st.Cost, Overrun: zero, Remaining: zero, Covered: true}
	out.Budget = st.Target
	if st.Converted != nil {
		out.Budget = *st.Converted
	}
	if st.State != "over" {
		return out, nil
	}
	out.Overrun = *st.Delta

	parts := partsUnder(dc.Nodes, nodeID)
	if len(parts) > maxPlanParts {
		return fail(fmt.Errorf("%d parts, at most %d: %w", len(parts), maxPlanParts, ErrPlanTooLarge))
	}
	index, err := s.catalogIndex(ctx)
	if err != nil {
		return fail(err)
	}
	// Les offres à évaluer ne dépendent que du couple (pièce, offre) : une lecture par couple, un seul chargement des prix.
	type pair struct{ part, offer int64 }
	pending := map[pair][]pendingCandidate{}
	var lists [][]pendingCandidate
	for _, n := range parts {
		k := pair{*n.PartID, *n.OfferID}
		if _, ok := pending[k]; ok {
			continue
		}
		todo, err := s.pendingFor(ctx, n, index)
		if err != nil {
			return fail(err)
		}
		pending[k] = todo
		lists = append(lists, todo)
	}
	prices, rates, err := s.loadPendingPrices(ctx, lists, currency, dc.BOM.PriceSnapshotID)
	if err != nil {
		return fail(err)
	}
	var savings []PlanSaving
	byID := nodesByID(dc.Nodes)
	for _, n := range parts {
		qty, _, err := realQuantityIn(byID, n.ID)
		if err != nil {
			return fail(err)
		}
		cur, to, ok := bestSaving(evaluateAll(pending[pair{*n.PartID, *n.OfferID}], qty, currency, prices, rates))
		if !ok {
			continue
		}
		savings = append(savings, PlanSaving{
			NodeID: n.ID, NodeName: n.Name, FromPartID: cur.PartID, ToPartID: to.PartID,
			FromOfferID: cur.OfferID, ToOfferID: to.OfferID, Gain: cur.Cost.Amount().Sub(to.Cost.Amount()),
			CO2Delta: to.CO2Kg.Sub(cur.CO2Kg), MassDelta: to.MassKg.Sub(cur.MassKg), LeadDelta: to.LeadDays - cur.LeadDays,
			FromQuality: cur.Quality, ToQuality: to.Quality,
		})
	}
	actions, remaining := BuildPlanActions(out.Overrun.Amount(), savings)
	sup, err := s.supplierNames(ctx)
	if err != nil {
		return fail(err)
	}
	for i := range actions { // noms seulement pour les actions rendues
		a := &actions[i]
		if a.FromPart, a.FromSupplier, a.FromMaterial, err = s.describe(ctx, index, sup, a.FromPartID, a.FromOfferID); err != nil {
			return fail(err)
		}
		if a.ToPart, a.ToSupplier, a.ToMaterial, err = s.describe(ctx, index, sup, a.ToPartID, a.ToOfferID); err != nil {
			return fail(err)
		}
	}
	out.Actions = actions
	out.Covered = remaining.IsZero()
	if out.Remaining, err = money.New(remaining, string(currency)); err != nil {
		return fail(err)
	}
	return out, nil
}

// partsUnder : les nœuds pièce du sous-arbre de rootID (lui compris), dans l'ordre des nœuds.
func partsUnder(nodes []Node, rootID int64) []Node {
	in := map[int64]bool{rootID: true}
	var out []Node
	for changed := true; changed; { // ponytail: O(n²) pire cas sur un arbre dans l'ordre inverse ; arbres < 10³ nœuds
		changed = false
		for _, n := range nodes {
			if !in[n.ID] && n.ParentID != nil && in[*n.ParentID] {
				in[n.ID], changed = true, true
			}
		}
	}
	for _, n := range nodes {
		if in[n.ID] && n.IsPart() {
			out = append(out, n)
		}
	}
	return out
}

// bestSaving : l'offre actuelle et la candidate complète la moins chère qui la bat ; égalité → plus petit id d'offre
// (cands est trié par offre).
func bestSaving(cands []Candidate) (cur, best Candidate, ok bool) {
	i := slices.IndexFunc(cands, func(c Candidate) bool { return c.Current })
	if i < 0 || cands[i].Excluded != nil || cands[i].Cost == nil {
		return cur, best, false
	}
	cur = cands[i]
	for _, c := range cands {
		if c.Current || c.Excluded != nil || c.Cost == nil || c.Cost.Amount().Cmp(cur.Cost.Amount()) >= 0 {
			continue
		}
		if !ok || c.Cost.Amount().Cmp(best.Cost.Amount()) < 0 {
			best, ok = c, true
		}
	}
	return cur, best, ok
}

// supplierNames : annuaire id → nom ; vide si le service est câblé sans annuaire.
func (s *Service) supplierNames(ctx context.Context) (map[int64]string, error) {
	out := map[int64]string{}
	if s.suppliers == nil {
		return out, nil
	}
	list, err := s.suppliers.ListSuppliers(ctx)
	if err != nil {
		return nil, err
	}
	for _, sp := range list {
		out[sp.ID] = sp.Name
	}
	return out, nil
}

// describe : nom de pièce, fournisseur et matière d'une offre ; pièce ou offre archivée lue par id.
func (s *Service) describe(ctx context.Context, index map[int64]catalog.PartWithOffers, sup map[int64]string, partID, offerID int64) (
	part, supplier, material string, err error,
) {
	p := index[partID]
	i := slices.IndexFunc(p.Offers, func(o catalog.Offer) bool { return o.ID == offerID })
	var offer catalog.Offer
	if i >= 0 {
		offer = p.Offers[i]
	} else {
		pp, err := s.catalog.Part(ctx, partID)
		if err != nil {
			return "", "", "", err
		}
		p.Part = pp
		if offer, err = s.catalog.Offer(ctx, offerID); err != nil {
			return "", "", "", err
		}
	}
	if p.Material != nil {
		material = *p.Material
	}
	return p.Name, sup[offer.SupplierID], material, nil
}
