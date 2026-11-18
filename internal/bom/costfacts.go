package bom

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/shopspring/decimal"

	"bomstudio/internal/catalog"
	"bomstudio/internal/money"
	"bomstudio/internal/pricing"
	"bomstudio/internal/supplier"
)

// ErrCostSourcesUnavailable : la méthode demande des faits (contrats, tables, taux budget) que le service
// n'a pas reçus au câblage.
var ErrCostSourcesUnavailable = errors.New("cost method unavailable: cost sources not wired")

// ContractSource : les contrats d'un fournisseur, avec les offres qu'ils couvrent. *supplier.Service la satisfait.
type ContractSource interface {
	ListContracts(ctx context.Context, supplierID int64) ([]supplier.Contract, error)
}

// WithCostSources branche les faits de la méthode de calcul (ADR 0031) : paramètres d'offre, tables douane et
// transport, taux budget (params) et contrats (contracts).
func (s *Service) WithCostSources(params pricing.CostParamsStore, contracts ContractSource) *Service {
	s.costParams, s.contracts = params, contracts
	return s
}

// SetCostMethod change la méthode de calcul du coût du brouillon (un snapshot ne se modifie pas : il a sa
// propre méthode figée). full exige un volume programme.
func (s *Service) SetCostMethod(ctx context.Context, id int64, m CostMethod) (BOM, error) {
	if err := ValidateCostMethod(m); err != nil {
		return BOM{}, fmt.Errorf("set cost method of bom %d: %w", id, err)
	}
	out, err := s.store.SetCostMethod(ctx, id, m)
	if err != nil {
		return BOM{}, fmt.Errorf("set cost method of bom %d: %w", id, err)
	}
	return out, nil
}

// costInput lit ce que la méthode du brouillon demande, et seulement cela : date de calcul (cotation la plus
// récente utilisée, temps simulé : jamais l'heure du serveur), taux budget de son année, contrats, tables
// douane et transport, emballage et outillage des offres. Une donnée absente devient l'erreur nommée de sa
// composante ; une panne de lecture est l'erreur de l'appel.
func (s *Service) costInput(ctx context.Context, b BOM, offers map[int64]catalog.Offer, prices map[int64]PriceResult) (*CostInput, error) {
	m := b.CostMethod.OrDefault()
	st, err := strategyFor(m)
	if err != nil {
		return nil, err
	}
	ci := &CostInput{Method: m, FactsSource: "tables"}
	if m == DefaultCostMethod() || (m.Basis == BasisPurchase && m.FX == FXLatest) {
		return ci, nil // calcul historique : rien d'autre à lire
	}
	ci.AsOf = QuotesAsOf(prices)
	needsTables := st.logistics || st.tooling
	if s.costParams == nil || (st.discount && s.contracts == nil) {
		return nil, ErrCostSourcesUnavailable
	}
	if st.fx == FXBudget && !ci.AsOf.IsZero() {
		if ci.BudgetRates, err = s.costParams.BudgetRates(ctx, ci.AsOf.Year(), money.Currency(b.Currency)); err != nil {
			return nil, err
		}
	}
	ids := make([]int64, 0, len(offers))
	for id := range offers {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	if st.discount {
		if ci.Contracts, err = s.offerContracts(ctx, ids, offers); err != nil {
			return nil, err
		}
	}
	if needsTables {
		if ci.Offers, err = s.offerCosts(ctx, b.DestinationCountry, ids); err != nil {
			return nil, err
		}
	}
	return ci, nil
}

// AllContracts : tous les contrats en une lecture ; *supplier.Service la satisfait. Facultatif : sans, les contrats
// sont lus fournisseur par fournisseur (ContractSource).
type AllContracts interface {
	ListAllContracts(ctx context.Context) ([]supplier.Contract, error)
}

// contractsBySupplier : contrats de chaque fournisseur de suppliers, en une lecture si la source le permet
// (v2-lot-g T8), sinon un appel par fournisseur. Un fournisseur sans contrat a une liste vide.
func (s *Service) contractsBySupplier(ctx context.Context, suppliers []int64) (map[int64][]supplier.Contract, error) {
	out := make(map[int64][]supplier.Contract, len(suppliers))
	if all, ok := s.contracts.(AllContracts); ok {
		cs, err := all.ListAllContracts(ctx)
		if err != nil {
			return nil, err
		}
		for _, sup := range suppliers {
			out[sup] = []supplier.Contract{}
		}
		for _, c := range cs {
			if _, wanted := out[c.SupplierID]; wanted {
				out[c.SupplierID] = append(out[c.SupplierID], c)
			}
		}
		return out, nil
	}
	for _, sup := range suppliers {
		if _, done := out[sup]; done {
			continue
		}
		cs, err := s.contracts.ListContracts(ctx, sup)
		if err != nil {
			return nil, fmt.Errorf("contracts of supplier %d: %w", sup, err)
		}
		out[sup] = cs
	}
	return out, nil
}

// offerContracts : contrats couvrant chaque offre (contractsBySupplier).
func (s *Service) offerContracts(ctx context.Context, ids []int64, offers map[int64]catalog.Offer) (map[int64][]supplier.Contract, error) {
	sups := make([]int64, 0, len(ids))
	for _, id := range ids {
		sups = append(sups, offers[id].SupplierID)
	}
	slices.Sort(sups)
	bySupplier, err := s.contractsBySupplier(ctx, slices.Compact(sups))
	if err != nil {
		return nil, err
	}
	out := make(map[int64][]supplier.Contract, len(ids))
	for _, id := range ids {
		for _, c := range bySupplier[offers[id].SupplierID] {
			if slices.Contains(c.OfferIDs, id) {
				out[id] = append(out[id], c)
			}
		}
	}
	return out, nil
}

// offerCosts : douane et transport (origine de l'offre → destination de la BOM), emballage et outillage.
func (s *Service) offerCosts(ctx context.Context, dest *string, ids []int64) (map[int64]OfferCost, error) {
	params, err := s.costParams.OfferCostParams(ctx, ids)
	if err != nil {
		return nil, err
	}
	duties, err := s.costParams.ListCustomsDuties(ctx)
	if err != nil {
		return nil, err
	}
	freights, err := s.costParams.ListFreightRates(ctx)
	if err != nil {
		return nil, err
	}
	type lane struct{ from, to string }
	duty := make(map[lane]decimal.Decimal, len(duties))
	for _, d := range duties {
		duty[lane{d.Origin, d.Destination}] = d.Percent
	}
	freight := make(map[lane]money.Money, len(freights))
	for _, f := range freights {
		freight[lane{f.Origin, f.Destination}] = f.PerKg
	}
	out := make(map[int64]OfferCost, len(ids))
	for _, id := range ids {
		p, ok := params[id]
		if !ok {
			return nil, fmt.Errorf("cost params of offer %d: %w", id, pricing.ErrUnknownOffer)
		}
		oc := OfferCost{Origin: p.Origin, Packaging: p.Packaging, Tooling: p.Tooling}
		if p.Packaging == nil {
			oc.PackagingErr = pricing.ErrNoPackaging
		}
		if p.Tooling == nil {
			oc.ToolingErr = ErrNoTooling
		}
		if dest == nil {
			oc.CustomsErr, oc.FreightErr = pricing.ErrNoDestination, pricing.ErrNoDestination
		} else {
			l := lane{p.Origin, *dest}
			if d, ok := duty[l]; ok {
				oc.CustomsPercent = &d
			} else {
				oc.CustomsErr = fmt.Errorf("customs %s→%s: %w", l.from, l.to, pricing.ErrNoCustomsDuty)
			}
			if f, ok := freight[l]; ok {
				oc.FreightPerKg = &f
			} else {
				oc.FreightErr = fmt.Errorf("freight %s→%s: %w", l.from, l.to, pricing.ErrNoFreightRate)
			}
		}
		out[id] = oc
	}
	return out, nil
}

// extraRateCurrencies : devises des faits (transport, emballage, outillage) à convertir au dernier taux et
// absentes des taux déjà lus.
func extraRateCurrencies(ci *CostInput, to money.Currency, have map[money.Currency]RateResult) []money.Currency {
	set := map[money.Currency]bool{}
	add := func(m *money.Money) {
		if m == nil || m.Currency() == to {
			return
		}
		if _, ok := have[m.Currency()]; !ok {
			set[m.Currency()] = true
		}
	}
	for _, oc := range ci.Offers {
		add(oc.FreightPerKg)
		add(oc.Packaging)
		add(oc.Tooling)
	}
	out := make([]money.Currency, 0, len(set))
	for c := range set {
		out = append(out, c)
	}
	slices.Sort(out)
	return out
}

// AddFactRates ajoute à rates les derniers taux des devises des faits de ci (transport, emballage, outillage)
// qui n'y sont pas encore, lus en direct ou dans le snapshot de prix snapID.
func AddFactRates(ctx context.Context, p pricing.Pricing, snapID *int64, ci *CostInput, to money.Currency, rates map[money.Currency]RateResult) error {
	return addRates(ctx, p, snapID, extraRateCurrencies(ci, to, rates), to, rates)
}

// addRates lit (en direct ou dans le snapshot de prix) les taux des devises from vers to et les ajoute à rates ;
// un taux absent reste pricing.ErrNoRate pour le nœud qui en aurait besoin.
func addRates(ctx context.Context, p pricing.Pricing, snapID *int64, from []money.Currency, to money.Currency, rates map[money.Currency]RateResult) error {
	if len(from) == 0 {
		return nil
	}
	found, err := loadRates(ctx, p, snapID, from, to)
	if err != nil {
		return err
	}
	for _, c := range from {
		r := found[c]
		if r.Err != nil && !errors.Is(r.Err, pricing.ErrNoRate) {
			return r.Err
		}
		rates[c] = r
	}
	return nil
}

// DraftCost : tout ce qu'un calcul de brouillon a lu (une seule lecture, Draft) et produit ; la publication en
// fige les faits.
type DraftCost struct {
	BOM      BOM
	Nodes    []Node
	Stock    []PartStock
	Schedule *Schedule
	Budgets  []TargetBudget
	Input    TotalsInput
	Totals   Totals
	draft    Draft // état lu, BOM d'origine (sans substitution du snapshot de prix)
}

// Draft rend l'état du brouillon sur lequel repose ce calcul (BOM d'origine, prix non substitués).
func (dc DraftCost) Draft() Draft { return dc.draft }

// DraftCost calcule les totaux du brouillon selon sa méthode et rend aussi l'entrée du calcul.
func (s *Service) DraftCost(ctx context.Context, bomID int64) (DraftCost, error) {
	d, err := s.store.Draft(ctx, bomID)
	if err != nil {
		return DraftCost{}, fmt.Errorf("cost of bom %d: %w", bomID, err)
	}
	return s.CostOfDraft(ctx, d, nil)
}

// DraftCostWithPrices calcule le brouillon avec les prix et taux du snapshot de prix priceSnapshotID au lieu
// des siens : la publication chiffre exactement ce qu'elle fige.
func (s *Service) DraftCostWithPrices(ctx context.Context, bomID, priceSnapshotID int64) (DraftCost, error) {
	d, err := s.store.Draft(ctx, bomID)
	if err != nil {
		return DraftCost{}, fmt.Errorf("cost of bom %d: %w", bomID, err)
	}
	return s.CostOfDraft(ctx, d, &priceSnapshotID)
}

// CostOfDraft chiffre un état de brouillon déjà lu, sans relire le brouillon : avec ses propres prix
// (priceSnapshotID nil) ou ceux du snapshot de prix priceSnapshotID. DraftCost.BOM porte alors ce snapshot
// de prix ; DraftCost.Draft() rend l'état d'origine.
func (s *Service) CostOfDraft(ctx context.Context, d Draft, priceSnapshotID *int64) (DraftCost, error) {
	b := d.BOM
	if priceSnapshotID != nil {
		b.PriceSnapshotID = priceSnapshotID
	}
	in, err := s.totalsInput(ctx, b, d.Nodes, d.Stock)
	if err != nil {
		return DraftCost{}, fmt.Errorf("cost of bom %d: %w", b.ID, err)
	}
	t, err := ComputeTotals(in)
	if err != nil {
		return DraftCost{}, fmt.Errorf("cost of bom %d: %w", b.ID, err)
	}
	return DraftCost{BOM: b, Nodes: d.Nodes, Stock: d.Stock, Schedule: d.Schedule, Budgets: d.Budgets, Input: in, Totals: t, draft: d}, nil
}

// Breakdown : lignes de coût d'un nœud (ADR 0031). Pièce sans enfant : ses lignes détaillées ; sous-ensemble
// (ou pièce avec enfants) : une ligne agrégée par nature sur tout le sous-arbre. Σ lignes = Total.
type Breakdown struct {
	NodeID   int64
	Currency money.Currency
	Method   CostMethod
	AsOf     *time.Time
	Lines    []CostLine
	Total    money.Money
}

// CostBreakdown construit la ventilation du nœud nodeID ; un nœud sans coût renvoie son erreur nommée.
func CostBreakdown(t Totals, nodeID int64, hasChildren bool) (Breakdown, error) {
	n, ok := t.Node(nodeID)
	if !ok {
		return Breakdown{}, fmt.Errorf("node %d: %w", nodeID, ErrNodeNotFound)
	}
	if n.Cost == nil {
		return Breakdown{}, fmt.Errorf("cost breakdown: %w", n.CostErr)
	}
	out := Breakdown{NodeID: nodeID, Currency: t.Explanation.Currency, Method: t.Explanation.Method, AsOf: t.Explanation.CostAsOf, Total: *n.Cost}
	if n.OwnLines != nil && !hasChildren {
		out.Lines = n.OwnLines
		return out, nil
	}
	for k, m := range n.KindTotals {
		if m != nil {
			out.Lines = append(out.Lines, CostLine{Kind: CostKinds[k], Amount: *m, d: lineDetail{aggregate: true}})
		}
	}
	return out, nil
}

// CostBreakdown : ventilation d'un nœud du brouillon selon sa méthode.
func (s *Service) CostBreakdown(ctx context.Context, bomID, nodeID int64) (Breakdown, error) {
	dc, err := s.DraftCost(ctx, bomID)
	if err != nil {
		return Breakdown{}, err
	}
	found, hasChildren := false, false
	for _, n := range dc.Nodes {
		found = found || n.ID == nodeID
		hasChildren = hasChildren || (n.ParentID != nil && *n.ParentID == nodeID)
	}
	if !found {
		return Breakdown{}, fmt.Errorf("cost breakdown of node %d in bom %d: %w", nodeID, bomID, ErrNodeNotFound)
	}
	out, err := CostBreakdown(dc.Totals, nodeID, hasChildren)
	if err != nil {
		return Breakdown{}, fmt.Errorf("cost breakdown of node %d in bom %d: %w", nodeID, bomID, err)
	}
	return out, nil
}
