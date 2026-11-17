package bom

import (
	"cmp"
	"context"
	"fmt"
	"maps"
	"slices"
	"time"

	"github.com/shopspring/decimal"

	"bomstudio/internal/catalog"
	"bomstudio/internal/money"
	"bomstudio/internal/pricing"
	"bomstudio/internal/supplier"
)

// TopSuppliers : nombre de fournisseurs du « poids des premiers » (vision V2 §2 : remplace l'indice HHI).
const TopSuppliers = 5

// CostShare : part du coût de la voiture portée par une clé (pays d'origine, devise d'origine).
type CostShare struct {
	Key          string
	Cost         money.Money
	SharePercent decimal.Decimal
	Parts        int // pièces distinctes
}

// ContractState : état, à la date de calcul, des contrats qui couvrent les offres retenues d'un fournisseur.
// Status : active | expired | not_started | none ; le contrat cité est le plus récent de cet état.
type ContractState struct {
	Status     string
	ContractID *int64
	EndsOn     string
}

// SupplierShare : part du coût par fournisseur des offres retenues.
type SupplierShare struct {
	SupplierID   int64
	Name         string
	Countries    []string // pays d'origine de ses offres retenues
	Currencies   []money.Currency
	Cost         money.Money
	SharePercent decimal.Decimal
	Parts        int
	Contract     ContractState
}

// MonoSourcePart : pièce de la BOM qui n'a qu'une offre active au catalogue (ou aucune, pièce archivée).
type MonoSourcePart struct {
	PartID       int64
	Name         string
	ActiveOffers int
	SupplierIDs  []int64 // fournisseurs des offres retenues dans la BOM
	Cost         money.Money
	SharePercent decimal.Decimal
}

// LostDiscount : remise d'un contrat expiré que l'offre aurait eue s'il était encore actif, par voiture.
type LostDiscount struct {
	PartID   int64
	Name     string
	OfferID  int64
	Quantity decimal.Decimal // commandée = quantité par voiture × volume de la BOM (comme ADR 0031)
	Percent  decimal.Decimal
	TierMin  *decimal.Decimal
	Amount   *money.Money // prix converti × quantité par voiture × % ; nil si Err
	Err      error
}

// ExpiredContract : contrat expiré à la date de calcul dont des offres sont encore utilisées sans contrat actif.
type ExpiredContract struct {
	ContractID   int64
	SupplierID   int64
	SupplierName string
	EndsOn       string
	Offers       []LostDiscount
	LostAmount   *money.Money // Σ des remises perdues ; nil si l'une n'est pas calculable
}

// RateGap : taux budget de l'année et dernier taux d'une devise, et effet sur la voiture de chiffrer au
// dernier taux plutôt qu'au taux budget (moteur des totaux : seul le taux de cette devise change).
type RateGap struct {
	Currency   money.Currency
	Budget     *pricing.Rate
	Latest     *pricing.Rate
	GapPercent *decimal.Decimal // (dernier − budget) ÷ budget × 100
	Effect     *money.Money     // coût au dernier taux − coût au taux budget
	Err        error
}

// Sourcing : vue d'ensemble de l'étape Sourcer, et sur quoi elle repose.
type Sourcing struct {
	BOMID            int64
	Currency         money.Currency
	AsOf             time.Time
	Total            money.Money
	ByCountry        []CostShare
	ByCurrency       []CostShare
	Suppliers        []SupplierShare // par coût décroissant
	TopSharePercent  decimal.Decimal // poids des TopSuppliers premiers
	MonoSource       []MonoSourcePart
	ExpiredContracts []ExpiredContract
	Rates            []RateGap
	Explanation      Explanation
}

// SourcingFacts : ce que la vue lit hors du calcul : catalogue, fournisseurs, contrats et taux budget.
type SourcingFacts struct {
	Offers       map[int64]catalog.Offer
	PartNames    map[int64]string
	ActiveOffers map[int64]int // offres actives au catalogue par pièce (absente : 0)
	Suppliers    map[int64]string
	Contracts    map[int64][]supplier.Contract // par fournisseur
	// BudgetRates : taux budget de l'année de calcul vers la devise de la BOM ; absents → RateGap.Err.
	BudgetRates map[money.Currency]decimal.Decimal
}

// Sourcing construit la vue d'ensemble de l'étape Sourcer du brouillon.
func (s *Service) Sourcing(ctx context.Context, bomID int64) (Sourcing, error) {
	if s.suppliers == nil || s.contracts == nil || s.listing == nil {
		return Sourcing{}, fmt.Errorf("sourcing of bom %d: suppliers, contracts or catalog listing: %w", bomID, ErrAnalysisUnavailable)
	}
	dc, err := s.DraftCost(ctx, bomID)
	if err != nil {
		return Sourcing{}, fmt.Errorf("sourcing: %w", err)
	}
	f, err := s.sourcingFacts(ctx, dc)
	if err != nil {
		return Sourcing{}, fmt.Errorf("sourcing of bom %d: %w", bomID, err)
	}
	out, err := ComputeSourcing(dc.Input, dc.Totals, rootOf(dc.Nodes), f)
	if err != nil {
		return Sourcing{}, fmt.Errorf("sourcing of bom %d: %w", bomID, err)
	}
	out.BOMID = bomID
	return out, nil
}

func (s *Service) sourcingFacts(ctx context.Context, dc DraftCost) (SourcingFacts, error) {
	f := SourcingFacts{Offers: map[int64]catalog.Offer{}, PartNames: map[int64]string{}, ActiveOffers: map[int64]int{},
		Suppliers: map[int64]string{}, Contracts: map[int64][]supplier.Contract{}}
	all, err := s.listing.ListParts(ctx, catalog.Filters{})
	if err != nil {
		return f, err
	}
	for _, p := range all {
		f.PartNames[p.ID] = p.Name
		for _, o := range p.Offers {
			if o.ArchivedAt == nil {
				f.ActiveOffers[p.ID]++
				f.Offers[o.ID] = o
			}
		}
	}
	for _, n := range dc.Nodes { // pièce ou offre archivée : lue par id
		if !n.IsPart() {
			continue
		}
		if _, ok := f.PartNames[*n.PartID]; !ok {
			p, err := s.catalog.Part(ctx, *n.PartID)
			if err != nil {
				return f, fmt.Errorf("node %d: %w", n.ID, err)
			}
			f.PartNames[p.ID] = p.Name
		}
		if _, ok := f.Offers[*n.OfferID]; !ok {
			o, err := s.catalog.Offer(ctx, *n.OfferID)
			if err != nil {
				return f, fmt.Errorf("node %d: %w", n.ID, err)
			}
			f.Offers[o.ID] = o
		}
	}
	sups, err := s.suppliers.ListSuppliers(ctx)
	if err != nil {
		return f, err
	}
	for _, sp := range sups {
		f.Suppliers[sp.ID] = sp.Name
	}
	for _, id := range offerIDs(dc.Nodes) {
		sup := f.Offers[id].SupplierID
		if _, ok := f.Contracts[sup]; ok {
			continue
		}
		cs, err := s.contracts.ListContracts(ctx, sup)
		if err != nil {
			return f, fmt.Errorf("contracts of supplier %d: %w", sup, err)
		}
		f.Contracts[sup] = cs
	}
	if ci := dc.Input.Cost; ci != nil && ci.BudgetRates != nil {
		f.BudgetRates = ci.BudgetRates
	} else if asOf := costAsOf(dc.Input); s.costParams != nil && !asOf.IsZero() {
		if f.BudgetRates, err = s.costParams.BudgetRates(ctx, asOf.Year(), money.Currency(dc.BOM.Currency)); err != nil {
			return f, err
		}
	}
	return f, nil
}

// ComputeSourcing est le calcul pur. Le coût d'une pièce est son coût propre selon la méthode (OwnCost) ; la
// somme des pièces est le coût de la racine. Une pièce sans coût fait échouer la vue (ErrMissingCost).
func ComputeSourcing(in TotalsInput, t Totals, rootID int64, f SourcingFacts) (Sourcing, error) {
	total, err := rootCost(t, rootID)
	if err != nil {
		return Sourcing{}, err
	}
	asOf := costAsOf(in)
	out := Sourcing{Currency: in.Currency, AsOf: asOf, Total: total, Explanation: t.Explanation}
	share := func(m money.Money) decimal.Decimal {
		if !total.Amount().IsPositive() {
			return decimal.Zero
		}
		return m.Amount().Mul(hundred).DivRound(total.Amount(), percentPrecision)
	}

	type acc struct {
		cost  decimal.Decimal
		parts map[int64]bool
		keys  map[string]bool
		curs  map[money.Currency]bool
	}
	newAcc := func() *acc {
		return &acc{parts: map[int64]bool{}, keys: map[string]bool{}, curs: map[money.Currency]bool{}}
	}
	countries, currencies, suppliers := map[string]*acc{}, map[string]*acc{}, map[int64]*acc{}
	partCost, partSuppliers := map[int64]decimal.Decimal{}, map[int64]map[int64]bool{}
	offerQty := map[int64]decimal.Decimal{}
	for _, n := range in.Nodes {
		if n.Part == nil {
			continue
		}
		nt, _ := t.Node(n.ID)
		if nt.OwnCost == nil {
			return Sourcing{}, fmt.Errorf("node %d: %w", n.ID, cmp.Or(nt.CostErr, ErrMissingCost))
		}
		o, ok := f.Offers[n.Part.OfferID]
		if !ok {
			return Sourcing{}, fmt.Errorf("offer %d: %w", n.Part.OfferID, catalog.ErrOfferNotFound)
		}
		c, pid := nt.OwnCost.Amount(), n.Part.PartID
		for _, a := range []*acc{get(countries, o.OriginCountry, newAcc), get(currencies, string(o.Currency), newAcc), get(suppliers, o.SupplierID, newAcc)} {
			a.cost = a.cost.Add(c)
			a.parts[pid] = true
		}
		suppliers[o.SupplierID].keys[o.OriginCountry] = true
		suppliers[o.SupplierID].curs[o.Currency] = true
		partCost[pid] = partCost[pid].Add(c)
		if partSuppliers[pid] == nil {
			partSuppliers[pid] = map[int64]bool{}
		}
		partSuppliers[pid][o.SupplierID] = true
		offerQty[o.ID] = offerQty[o.ID].Add(nt.RealQuantity)
	}
	mk := func(amount decimal.Decimal) money.Money { m, _ := money.New(amount, string(in.Currency)); return m }
	shares := func(m map[string]*acc) []CostShare {
		out := make([]CostShare, 0, len(m))
		for k, a := range m {
			c := mk(a.cost)
			out = append(out, CostShare{Key: k, Cost: c, SharePercent: share(c), Parts: len(a.parts)})
		}
		slices.SortFunc(out, func(a, b CostShare) int { return cmp.Or(b.Cost.Amount().Cmp(a.Cost.Amount()), cmp.Compare(a.Key, b.Key)) })
		return out
	}
	out.ByCountry, out.ByCurrency = shares(countries), shares(currencies)

	for id, a := range suppliers {
		c := mk(a.cost)
		sh := SupplierShare{SupplierID: id, Name: f.Suppliers[id], Cost: c, SharePercent: share(c), Parts: len(a.parts),
			Countries: slices.Sorted(maps.Keys(a.keys)), Currencies: slices.Sorted(maps.Keys(a.curs))}
		if sh.Contract, err = contractState(f.Contracts[id], usedOffersOf(id, f.Offers, offerQty), asOf); err != nil {
			return Sourcing{}, fmt.Errorf("supplier %d: %w", id, err)
		}
		out.Suppliers = append(out.Suppliers, sh)
	}
	slices.SortFunc(out.Suppliers, func(a, b SupplierShare) int {
		return cmp.Or(b.Cost.Amount().Cmp(a.Cost.Amount()), cmp.Compare(a.SupplierID, b.SupplierID))
	})
	for _, s := range out.Suppliers[:min(TopSuppliers, len(out.Suppliers))] {
		out.TopSharePercent = out.TopSharePercent.Add(s.SharePercent)
	}

	for _, pid := range slices.Sorted(maps.Keys(partCost)) {
		if f.ActiveOffers[pid] <= 1 {
			c := mk(partCost[pid])
			out.MonoSource = append(out.MonoSource, MonoSourcePart{PartID: pid, Name: f.PartNames[pid], ActiveOffers: f.ActiveOffers[pid],
				SupplierIDs: slices.Sorted(maps.Keys(partSuppliers[pid])), Cost: c, SharePercent: share(c)})
		}
	}
	slices.SortFunc(out.MonoSource, func(a, b MonoSourcePart) int {
		return cmp.Or(b.Cost.Amount().Cmp(a.Cost.Amount()), cmp.Compare(a.PartID, b.PartID))
	})

	if out.ExpiredContracts, err = expiredContracts(in, f, offerQty, asOf); err != nil {
		return Sourcing{}, err
	}
	if out.Rates, err = rateGaps(in, rootID, f.BudgetRates, asOf); err != nil {
		return Sourcing{}, err
	}
	return out, nil
}

func get[K comparable, V any](m map[K]*V, k K, mk func() *V) *V {
	v, ok := m[k]
	if !ok {
		v = mk()
		m[k] = v
	}
	return v
}

// usedOffersOf : offres retenues dans la BOM vendues par le fournisseur, triées.
func usedOffersOf(supplierID int64, offers map[int64]catalog.Offer, used map[int64]decimal.Decimal) []int64 {
	var out []int64
	for id := range used {
		if offers[id].SupplierID == supplierID {
			out = append(out, id)
		}
	}
	slices.Sort(out)
	return out
}

// contractState : actif s'il existe un contrat actif sur une offre utilisée ; sinon expiré, pas commencé, aucun.
func contractState(contracts []supplier.Contract, offers []int64, asOf time.Time) (ContractState, error) {
	best := map[string]*supplier.Contract{}
	for _, c := range contracts {
		if !slices.ContainsFunc(offers, func(id int64) bool { return slices.Contains(c.OfferIDs, id) }) {
			continue
		}
		st, err := supplier.ContractStatus(c, asOf)
		if err != nil {
			return ContractState{}, err
		}
		if b := best[st]; b == nil || c.EndsOn > b.EndsOn || (c.EndsOn == b.EndsOn && c.ID > b.ID) {
			best[st] = &c
		}
	}
	for _, st := range []string{"active", "expired", "not_started"} {
		if c := best[st]; c != nil {
			id := c.ID
			return ContractState{Status: st, ContractID: &id, EndsOn: c.EndsOn}, nil
		}
	}
	return ContractState{Status: "none"}, nil
}

// expiredContracts : pour chaque offre retenue sans contrat actif mais couverte par un contrat expiré (le plus
// récent, comme la remise d'ADR 0031), la remise que ce contrat donnerait à la quantité commandée, évaluée à
// son dernier jour de validité, appliquée au prix converti au taux de la méthode.
func expiredContracts(in TotalsInput, f SourcingFacts, offerQty map[int64]decimal.Decimal, asOf time.Time) ([]ExpiredContract, error) {
	if asOf.IsZero() {
		return nil, nil
	}
	calc, err := newCostCalc(in)
	if err != nil {
		return nil, err
	}
	byContract := map[int64]*ExpiredContract{}
	for _, oid := range slices.Sorted(maps.Keys(offerQty)) {
		o := f.Offers[oid]
		var last *supplier.Contract
		active := false
		for _, c := range f.Contracts[o.SupplierID] {
			if !slices.Contains(c.OfferIDs, oid) {
				continue
			}
			st, err := supplier.ContractStatus(c, asOf)
			if err != nil {
				return nil, err
			}
			switch {
			case st == "active":
				active = true
			case st == "expired" && (last == nil || c.EndsOn > last.EndsOn || (c.EndsOn == last.EndsOn && c.ID > last.ID)):
				last = &c
			}
		}
		if active || last == nil {
			continue
		}
		ec := get(byContract, last.ID, func() *ExpiredContract {
			return &ExpiredContract{ContractID: last.ID, SupplierID: o.SupplierID, SupplierName: f.Suppliers[o.SupplierID], EndsOn: last.EndsOn}
		})
		ec.Offers = append(ec.Offers, lostDiscount(calc, in, o, *last, offerQty[oid], f.PartNames[o.PartID]))
	}
	out := make([]ExpiredContract, 0, len(byContract))
	for _, id := range slices.Sorted(maps.Keys(byContract)) {
		ec := byContract[id]
		var sum *money.Money
		for i, l := range ec.Offers {
			if l.Amount == nil {
				sum = nil
				break
			}
			if i == 0 {
				v := *l.Amount
				sum = &v
				continue
			}
			v, err := sum.Add(*l.Amount)
			if err != nil {
				return nil, err
			}
			sum = &v
		}
		ec.LostAmount = sum
		out = append(out, *ec)
	}
	return out, nil
}

func lostDiscount(calc *costCalc, in TotalsInput, o catalog.Offer, c supplier.Contract, perCar decimal.Decimal, name string) LostDiscount {
	l := LostDiscount{PartID: o.PartID, Name: name, OfferID: o.ID, Quantity: perCar.Mul(decimal.NewFromInt32(in.Volume))}
	pr, ok := in.Prices[o.ID]
	if !ok || pr.Err != nil {
		l.Err = fmt.Errorf("offer %d: %w", o.ID, cmp.Or(pr.Err, pricing.ErrNoQuote))
		return l
	}
	end, err := time.Parse(time.DateOnly, c.EndsOn)
	if err != nil {
		l.Err = fmt.Errorf("contract %d ends_on %q: %w", c.ID, c.EndsOn, ErrInvalidInput)
		return l
	}
	d, err := resolveDiscount(o.ID, []supplier.Contract{c}, l.Quantity, pr.Quote.Price, end)
	if err != nil {
		l.Err = err
		return l
	}
	l.Percent, l.TierMin = d.Percent, d.TierMin
	gross, err := calc.chosen(pr.Quote.Price.Mul(perCar))
	if err != nil {
		l.Err = fmt.Errorf("offer %d: %w", o.ID, err)
		return l
	}
	amount := gross.Mul(d.Percent.Shift(-2))
	l.Amount = &amount
	return l
}

// rateGaps : pour chaque devise convertie, effet sur la voiture du dernier taux au lieu du taux budget, par le
// moteur des totaux en taux budget (les devises sans taux budget y gardent leur dernier taux : seul l'écart de
// la devise étudiée change entre les deux calculs).
func rateGaps(in TotalsInput, rootID int64, budget map[money.Currency]decimal.Decimal, asOf time.Time) ([]RateGap, error) {
	var curs []money.Currency
	for c, rr := range in.Rates {
		if rr.Err == nil {
			curs = append(curs, c)
		}
	}
	for c := range budget {
		curs = append(curs, c)
	}
	slices.Sort(curs)
	curs = slices.Compact(curs)
	if len(curs) == 0 {
		return nil, nil
	}
	mixed := map[money.Currency]decimal.Decimal{}
	for _, c := range curs {
		if r, ok := budget[c]; ok {
			mixed[c] = r
		} else if rr := in.Rates[c]; rr.Err == nil && rr.Rate.Rate.IsPositive() {
			mixed[c] = rr.Rate.Rate
		}
	}
	withBudget := func(rates map[money.Currency]decimal.Decimal) TotalsInput {
		out := in
		var ci CostInput
		if in.Cost != nil {
			ci = *in.Cost
		} else {
			ci = CostInput{Method: DefaultCostMethod(), FactsSource: "tables"}
		}
		ci.Method.FX, ci.AsOf, ci.BudgetRates = FXBudget, asOf, rates
		out.Cost = &ci
		return out
	}
	total := func(rates map[money.Currency]decimal.Decimal) (money.Money, error) {
		t, err := ComputeTotals(withBudget(rates))
		if err != nil {
			return money.Money{}, err
		}
		return rootCost(t, rootID)
	}
	var base *money.Money
	out := make([]RateGap, 0, len(curs))
	for _, c := range curs {
		g := RateGap{Currency: c}
		if r, ok := budget[c]; ok {
			br := budgetRate(c, in.Currency, asOf.Year(), r)
			g.Budget = &br
		}
		if rr, ok := in.Rates[c]; ok && rr.Err == nil {
			lr := rr.Rate
			g.Latest = &lr
		}
		switch {
		case g.Budget == nil:
			g.Err = fmt.Errorf("budget rate %d %s→%s: %w", asOf.Year(), c, in.Currency, ErrNoBudgetRate)
		case g.Latest == nil:
			g.Err = fmt.Errorf("rate %s→%s: %w", c, in.Currency, pricing.ErrNoRate)
		}
		if g.Err != nil {
			out = append(out, g)
			continue
		}
		gap := g.Latest.Rate.Sub(g.Budget.Rate).Mul(hundred).DivRound(g.Budget.Rate, percentPrecision)
		g.GapPercent = &gap
		if base == nil {
			b, err := total(mixed)
			if err != nil {
				return nil, fmt.Errorf("cost at budget rates: %w", err)
			}
			base = &b
		}
		swapped := maps.Clone(mixed)
		swapped[c] = g.Latest.Rate
		latest, err := total(swapped)
		if err != nil {
			return nil, fmt.Errorf("cost with the latest %s rate: %w", c, err)
		}
		effect, err := latest.Add(base.Mul(decimal.NewFromInt(-1)))
		if err != nil {
			return nil, err
		}
		g.Effect = &effect
		out = append(out, g)
	}
	return out, nil
}

