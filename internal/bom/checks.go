package bom

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"bomstudio/internal/catalog"
	"bomstudio/internal/money"
	"bomstudio/internal/pricing"
	"bomstudio/internal/supplier"
)

// Contrôles avant publication (ADR 0033) : une fonction pure par règle, sur une entrée que le service lit.
// block empêche la publication ; warn exige une justification écrite ; ok n'exige rien.

// CheckLevel : ok | warn | block.
type CheckLevel string

const (
	CheckOK    CheckLevel = "ok"
	CheckWarn  CheckLevel = "warn"
	CheckBlock CheckLevel = "block"
)

// Codes des contrôles, dans l'ordre d'affichage.
const (
	CheckOffersActive     = "offers_active"
	CheckFXComplete       = "fx_complete"
	CheckPricesFrozen     = "prices_frozen"
	CheckMethodComplete   = "method_complete"
	CheckTargetDate       = "target_date"
	CheckBudgets          = "budgets"
	CheckExpiredContracts = "expired_contracts"
)

// PublicationCheck : résultat d'une règle. NodeIDs : nœuds concernés (triés).
type PublicationCheck struct {
	Code    string
	Level   CheckLevel
	Title   string
	Detail  string
	NodeIDs []int64
}

// ExpiredContractUse : offre utilisée dont aucun contrat n'est actif et dont un contrat a expiré à la date de calcul.
type ExpiredContractUse struct {
	OfferID      int64
	ContractID   int64
	SupplierName string
	EndsOn       string // AAAA-MM-JJ
	NodeIDs      []int64
}

// ChecksInput : tout ce dont dépendent les règles. Rien d'autre (ni heure, ni base).
type ChecksInput struct {
	Nodes           []Node
	Totals          Totals
	Archived        map[int64]bool // nœud pièce dont la pièce ou l'offre est archivée
	PriceSnapshotID *int64
	Planning        Planning
	Budgets         []BudgetStatus
	RootID          int64
	Names           map[int64]string // nom des sous-ensembles budgétés
	// Contracts : offres utilisées sous contrat expiré seulement (vide = aucune).
	Contracts []ExpiredContractUse
	AsOf      *time.Time
	// ContractsUnavailable : la source de contrats n'est pas câblée.
	ContractsUnavailable bool
}

// RunChecks applique toutes les règles, dans l'ordre d'affichage.
func RunChecks(in ChecksInput) []PublicationCheck {
	return []PublicationCheck{
		checkOffersActive(in), checkFXComplete(in), checkPricesFrozen(in), checkMethodComplete(in),
		checkTargetDate(in), checkBudgets(in), checkExpiredContracts(in),
	}
}

// partCostErrors : nœuds pièce sans coût propre, triés, avec leur erreur.
func partCostErrors(in ChecksInput, match func(error) bool) []int64 {
	var out []int64
	for _, n := range in.Nodes {
		if !n.IsPart() {
			continue
		}
		t, ok := in.Totals.Node(n.ID)
		if !ok || t.OwnCost != nil || t.CostErr == nil {
			continue
		}
		if match(t.CostErr) {
			out = append(out, n.ID)
		}
	}
	return out
}

func isPriceErr(err error) bool { return errors.Is(err, pricing.ErrNoQuote) }
func isFXErr(err error) bool {
	return errors.Is(err, pricing.ErrNoRate) || errors.Is(err, ErrNoBudgetRate)
}

// checkOffersActive : chaque pièce a une offre non archivée (pièce comprise) et une cotation.
func checkOffersActive(in ChecksInput) PublicationCheck {
	c := PublicationCheck{Code: CheckOffersActive, Title: "Toutes les pièces ont une offre active"}
	ids := partCostErrors(in, isPriceErr)
	for id, archived := range in.Archived {
		if archived && !slices.Contains(ids, id) {
			ids = append(ids, id)
		}
	}
	slices.Sort(ids)
	if len(ids) > 0 {
		c.Level, c.NodeIDs = CheckBlock, ids
		c.Detail = fmt.Sprintf("%d pièce(s) sans offre active ou sans cotation", len(ids))
		return c
	}
	c.Level, c.Detail = CheckOK, fmt.Sprintf("%d nœuds", len(in.Nodes))
	return c
}

// checkFXComplete : aucune conversion manquante (dernier taux ou taux budget).
func checkFXComplete(in ChecksInput) PublicationCheck {
	c := PublicationCheck{Code: CheckFXComplete, Title: "Aucune conversion de devise manquante"}
	if ids := partCostErrors(in, isFXErr); len(ids) > 0 {
		c.Level, c.NodeIDs = CheckBlock, ids
		c.Detail = fmt.Sprintf("%d pièce(s) sans taux de change", len(ids))
		return c
	}
	cur := map[money.Currency]bool{in.Totals.Explanation.Currency: true}
	for _, r := range append(slices.Clone(in.Totals.Explanation.Rates), in.Totals.Explanation.BudgetRates...) {
		cur[r.From] = true
	}
	c.Level, c.Detail = CheckOK, fmt.Sprintf("%d devise(s)", len(cur))
	return c
}

// checkPricesFrozen : un snapshot de prix choisi ; en direct, la publication en créera un (à justifier).
func checkPricesFrozen(in ChecksInput) PublicationCheck {
	c := PublicationCheck{Code: CheckPricesFrozen, Title: "Prix figés dans un snapshot"}
	if in.PriceSnapshotID != nil {
		c.Level, c.Detail = CheckOK, fmt.Sprintf("snapshot #%d", *in.PriceSnapshotID)
		return c
	}
	c.Level, c.Detail = CheckWarn, "prix en direct : un snapshot de prix sera créé à la publication"
	return c
}

// checkMethodComplete : toutes les composantes de la méthode (remise, douane, transport, emballage,
// outillage…) sont présentes ; les prix et taux manquants relèvent des deux contrôles précédents.
func checkMethodComplete(in ChecksInput) PublicationCheck {
	m := in.Totals.Explanation.Method.OrDefault()
	c := PublicationCheck{Code: CheckMethodComplete, Title: "Composantes de la méthode présentes"}
	ids := partCostErrors(in, func(err error) bool { return !isPriceErr(err) && !isFXErr(err) })
	if len(ids) > 0 {
		t, _ := in.Totals.Node(ids[0])
		c.Level, c.NodeIDs = CheckBlock, ids
		c.Detail = fmt.Sprintf("%d pièce(s) incomplètes, ex. %s", len(ids), t.CostErr)
		return c
	}
	c.Level, c.Detail = CheckOK, fmt.Sprintf("méthode %s · %s", m.Basis, m.FX)
	return c
}

// checkTargetDate : la disponibilité tient la date cible (warn sinon) ; sans date cible, rien à tenir.
func checkTargetDate(in ChecksInput) PublicationCheck {
	p := in.Planning
	c := PublicationCheck{Code: CheckTargetDate, Title: "Date cible tenue", Level: CheckOK}
	switch {
	case p.TargetMet == nil || p.AllowedDays == nil:
		c.Detail = fmt.Sprintf("%d j, pas de date cible", p.LeadTimeDays)
	case *p.TargetMet:
		c.Detail = fmt.Sprintf("%d j pour %d j", p.LeadTimeDays, *p.AllowedDays)
	default:
		c.Level = CheckWarn
		c.Detail = fmt.Sprintf("%d j pour %d j : %d j de retard", p.LeadTimeDays, *p.AllowedDays, p.LeadTimeDays-*p.AllowedDays)
		for _, l := range p.Late {
			c.NodeIDs = append(c.NodeIDs, l.NodeID)
		}
		slices.Sort(c.NodeIDs)
	}
	return c
}

// checkBudgets : sous-systèmes (nœuds budgétés autres que la racine) au-dessus de leur budget.
func checkBudgets(in ChecksInput) PublicationCheck {
	c := PublicationCheck{Code: CheckBudgets, Title: "Sous-systèmes dans leur budget", Level: CheckOK}
	var over []string
	n := 0
	for _, b := range in.Budgets {
		if b.NodeID == in.RootID {
			continue
		}
		n++
		if b.State != "over" || b.Cost == nil {
			continue
		}
		c.NodeIDs = append(c.NodeIDs, b.NodeID)
		label := in.Names[b.NodeID]
		if label == "" {
			label = fmt.Sprintf("nœud %d", b.NodeID)
		}
		if b.Target.Amount().IsPositive() {
			label += fmt.Sprintf(" %s %%", b.Cost.Amount().Div(b.Target.Amount()).Mul(decimal.NewFromInt(100)).Round(0))
		}
		over = append(over, label)
	}
	if len(over) > 0 {
		c.Level = CheckWarn
		c.Title = fmt.Sprintf("%d sous-système(s) au-dessus du budget", len(over))
		c.Detail = strings.Join(over, ", ") + " : justification requise"
		return c
	}
	c.Detail = fmt.Sprintf("%d budget(s)", n)
	return c
}

// checkExpiredContracts : une offre utilisée n'a plus que des contrats expirés à la date de calcul.
func checkExpiredContracts(in ChecksInput) PublicationCheck {
	c := PublicationCheck{Code: CheckExpiredContracts, Title: "Aucun contrat expiré encore utilisé", Level: CheckOK}
	switch {
	case in.ContractsUnavailable:
		c.Level, c.Detail = CheckWarn, "contrats non consultables"
		return c
	case len(in.Contracts) == 0:
		c.Detail = "aucune offre sous contrat expiré"
		if in.AsOf != nil {
			c.Detail += " au " + in.AsOf.UTC().Format("02/01/2006")
		}
		return c
	}
	seen := map[string]bool{}
	var who []string
	for _, u := range in.Contracts {
		c.NodeIDs = append(c.NodeIDs, u.NodeIDs...)
		label := u.SupplierName
		if label == "" {
			label = fmt.Sprintf("contrat %d", u.ContractID)
		}
		if t, err := time.Parse(time.DateOnly, u.EndsOn); err == nil {
			label += ", " + t.Format("02/01")
		}
		if !seen[label] {
			seen[label] = true
			who = append(who, label)
		}
	}
	slices.Sort(c.NodeIDs)
	c.NodeIDs = slices.Compact(c.NodeIDs)
	c.Level, c.Title = CheckBlock, "Contrat expiré encore utilisé"
	c.Detail = strings.Join(who, " ; ")
	return c
}

// ExpiredContractUses : pour chaque offre utilisée (nœuds par offre), le contrat expiré le plus récent si
// aucun contrat ne la couvre plus à asOf. Pur ; ordre par offre.
func ExpiredContractUses(nodesByOffer map[int64][]int64, contracts map[int64][]supplier.Contract, asOf time.Time) ([]ExpiredContractUse, error) {
	offers := make([]int64, 0, len(nodesByOffer))
	for o := range nodesByOffer {
		offers = append(offers, o)
	}
	slices.Sort(offers)
	var out []ExpiredContractUse
	for _, o := range offers {
		var active bool
		var last *supplier.Contract
		for _, c := range contracts[o] {
			st, err := supplier.ContractStatus(c, asOf)
			if err != nil {
				return nil, fmt.Errorf("contract %d: %w", c.ID, err)
			}
			switch st {
			case "active":
				active = true
			case "expired":
				if last == nil || c.EndsOn > last.EndsOn || (c.EndsOn == last.EndsOn && c.ID > last.ID) {
					cc := c
					last = &cc
				}
			}
		}
		if active || last == nil {
			continue
		}
		ids := slices.Clone(nodesByOffer[o])
		slices.Sort(ids)
		out = append(out, ExpiredContractUse{OfferID: o, ContractID: last.ID, EndsOn: last.EndsOn, NodeIDs: ids})
	}
	return out, nil
}

// SupplierNames : nom d'un fournisseur ; *supplier.Service la satisfait (facultatif, pour les libellés).
type SupplierNames interface {
	Supplier(ctx context.Context, id int64) (supplier.Supplier, error)
}

// PublicationChecks évalue les contrôles avant publication du brouillon (ADR 0033) sur ses totaux selon sa méthode.
func (s *Service) PublicationChecks(ctx context.Context, bomID int64) ([]PublicationCheck, error) {
	in, err := s.checksInput(ctx, bomID)
	if err != nil {
		return nil, fmt.Errorf("publication checks of bom %d: %w", bomID, err)
	}
	return RunChecks(in), nil
}

func (s *Service) checksInput(ctx context.Context, bomID int64) (ChecksInput, error) {
	dc, err := s.DraftCost(ctx, bomID)
	if err != nil {
		return ChecksInput{}, err
	}
	sch, err := s.store.Schedule(ctx, bomID)
	if err != nil {
		return ChecksInput{}, err
	}
	plan, err := PlanSchedule(dc.Input.Nodes, dc.Totals, sch)
	if err != nil {
		return ChecksInput{}, err
	}
	targets, err := s.store.TargetBudgets(ctx, bomID)
	if err != nil {
		return ChecksInput{}, err
	}
	budgets, err := EvaluateTargetBudgets(dc.Totals, targets)
	if err != nil {
		return ChecksInput{}, err
	}
	in := ChecksInput{
		Nodes: dc.Nodes, Totals: dc.Totals, PriceSnapshotID: dc.BOM.PriceSnapshotID, Planning: plan, Budgets: budgets,
		Archived: map[int64]bool{}, Names: map[int64]string{},
	}
	parts := map[int64]catalog.Part{}
	offers := map[int64]catalog.Offer{}
	nodesByOffer := map[int64][]int64{}
	for _, n := range dc.Nodes {
		if n.ParentID == nil {
			in.RootID = n.ID
		}
		if !n.IsPart() {
			in.Names[n.ID] = n.Name
			continue
		}
		nodesByOffer[*n.OfferID] = append(nodesByOffer[*n.OfferID], n.ID)
		p, ok := parts[*n.PartID]
		if !ok {
			if p, err = s.catalog.Part(ctx, *n.PartID); err != nil {
				return ChecksInput{}, fmt.Errorf("node %d: %w", n.ID, err)
			}
			parts[p.ID] = p
		}
		o, ok := offers[*n.OfferID]
		if !ok {
			if o, err = s.catalog.Offer(ctx, *n.OfferID); err != nil {
				return ChecksInput{}, fmt.Errorf("node %d: %w", n.ID, err)
			}
			offers[o.ID] = o
		}
		if p.ArchivedAt != nil || o.ArchivedAt != nil {
			in.Archived[n.ID] = true
		}
	}
	in.AsOf = dc.Totals.Explanation.CostAsOf
	if in.AsOf == nil { // méthode historique : date de la cotation la plus récente utilisée
		for _, t := range dc.Totals.Nodes {
			if t.Quote != nil && (in.AsOf == nil || t.Quote.QuotedAt.After(*in.AsOf)) {
				at := t.Quote.QuotedAt.UTC()
				in.AsOf = &at
			}
		}
	}
	if s.contracts == nil || in.AsOf == nil {
		in.ContractsUnavailable = s.contracts == nil
		return in, nil
	}
	ids := make([]int64, 0, len(offers))
	for id := range offers {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	contracts, err := s.offerContracts(ctx, ids, offers)
	if err != nil {
		return ChecksInput{}, err
	}
	if in.Contracts, err = ExpiredContractUses(nodesByOffer, contracts, *in.AsOf); err != nil {
		return ChecksInput{}, err
	}
	names, _ := s.contracts.(SupplierNames)
	for i, u := range in.Contracts {
		if names == nil {
			break
		}
		if sup, err := names.Supplier(ctx, offers[u.OfferID].SupplierID); err == nil {
			in.Contracts[i].SupplierName = sup.Name
		}
	}
	return in, nil
}
