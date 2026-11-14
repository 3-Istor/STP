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

var (
	// ErrInvalidTree : l'arbre fourni au calcul n'est pas un arbre (pas une seule racine, parent absent,
	// nœud inatteignable). Erreur d'entrée, pas une donnée manquante. Une pièce peut avoir des enfants (ADR 0008).
	ErrInvalidTree = errors.New("invalid bom tree")
	// ErrMissingCost : un descendant n'a pas de prix ou de taux ; le parent n'a pas de total de coût
	// (ADR 0007 règle 5). NodeTotals.MissingCost liste les pièces en défaut.
	ErrMissingCost = errors.New("cost unavailable: missing price or rate below")
	// ErrMissingLabor : une pièce du sous-arbre n'a pas d'heures de main-d'œuvre au catalogue ; le nœud n'a
	// ni heures ni ETP (ADR 0024). NodeTotals.MissingLabor liste les pièces en défaut.
	ErrMissingLabor = errors.New("labor hours unavailable: part without labor hours below")
	// ErrLaborNotFrozen : snapshot publié avant ADR 0024, ses heures n'ont pas été figées ; aucune heure ni
	// aucun ETP n'est reconstruit depuis le catalogue courant (le snapshot ne bouge pas).
	ErrLaborNotFrozen = errors.New("labor hours not frozen in this snapshot")
)

// CalcNode est un nœud tel que le calcul le voit, détaché de sa provenance (brouillon ou snapshot).
type CalcNode struct {
	ID       int64
	ParentID *int64 // nil : racine
	// Quantity : quantité par unité du parent.
	Quantity decimal.Decimal
	// ProductionDays : pièce → jours de production de la pièce ; sous-ensemble → les siens (ADR 0007 règle 4).
	ProductionDays int
	// Part : nil pour un sous-ensemble.
	Part *PartFacts
}

// PartFacts porte ce qu'une pièce apporte au calcul : son offre et ses attributs unitaires.
type PartFacts struct {
	PartID       int64 // sert au stock (ADR 0022)
	OfferID      int64
	LeadTimeDays int // délai de livraison de l'offre
	CO2Kg        decimal.Decimal
	MassKg       decimal.Decimal
	// LaborHours : heures de main-d'œuvre par unité ; nil si le catalogue ne les renseigne pas.
	LaborHours *decimal.Decimal
}

// PriceResult est le prix d'une offre, ou l'erreur qui explique son absence (ex. pricing.ErrNoQuote).
type PriceResult struct {
	Quote pricing.Quote
	Err   error
}

// RateResult est le taux d'une devise vers la devise de la BOM, ou l'erreur qui explique son absence.
type RateResult struct {
	Rate pricing.Rate
	Err  error
}

// TotalsInput rassemble tout ce dont dépend le calcul : rien d'autre (ni heure, ni réseau, ni base).
type TotalsInput struct {
	Nodes    []CalcNode
	Currency money.Currency // devise de référence de la BOM
	// Prices : par id d'offre. Une offre absente est traitée comme pricing.ErrNoQuote.
	Prices map[int64]PriceResult
	// Rates : par devise d'origine, vers Currency. Une devise absente est traitée comme pricing.ErrNoRate.
	Rates map[money.Currency]RateResult
	// PriceSnapshotID : nil = prix en direct ; sinon le snapshot de prix d'où viennent Prices et Rates.
	PriceSnapshotID *int64
	// Volume et AnnualHours servent aux ETP (ADR 0024) ; AnnualHours nil (ancien snapshot) → pas d'ETP.
	Volume      int32
	AnnualHours *decimal.Decimal
	// LaborNotFrozen : snapshot sans heures figées → chaque nœud porte ErrLaborNotFrozen.
	LaborNotFrozen bool
	// Stock : quantité en stock par pièce, propre à la BOM (ADR 0022) ; absente = aucun stock.
	Stock map[int64]decimal.Decimal
}

// NodeTotals est le total d'un nœud et ce sur quoi il repose.
type NodeTotals struct {
	NodeID int64
	// RealQuantity : produit des quantités de la racine au nœud (ADR 0007 règle 1).
	RealQuantity decimal.Decimal
	// Cost : coût exact (non arrondi) dans la devise de la BOM ; nil si CostErr.
	Cost *money.Money
	// CostErr : pièce sans prix ou taux → ErrNoQuote / ErrNoRate enveloppée ; sinon, un descendant
	// sans coût → ErrMissingCost.
	CostErr error
	// MissingCost : pièces sans coût dans le sous-arbre (le nœud lui-même compris), par ordre de parcours.
	MissingCost []int64
	// CO2Kg, MassKg : contribution propre (pièce) + somme des enfants (ADR 0007 règle 3, ADR 0008 règle 3).
	CO2Kg  decimal.Decimal
	MassKg decimal.Decimal
	// LeadTimeDays : max(enfants) + délai propre (ADR 0008 règle 4). Délai propre : pièce → livraison +
	// production ; sous-ensemble → sa production.
	LeadTimeDays int
	// OwnCost : pour une pièce, sa contribution propre (prix converti × quantité réelle), sans ses enfants ;
	// nil pour un sous-ensemble ou si le prix ou le taux de la pièce manque (ADR 0008, panneau latéral).
	OwnCost *money.Money
	// OwnLeadTimeDays : délai propre du nœud, sans ses enfants.
	OwnLeadTimeDays int
	// Quote : pour une pièce, la cotation utilisée (source et date) ; nil sinon.
	Quote *pricing.Quote
	// Rate : pour une pièce convertie, le taux utilisé ; nil si même devise ou sans conversion.
	Rate *pricing.Rate
	// LaborHours : heures de main-d'œuvre par voiture du sous-arbre (heures × quantité réelle) ; nil si LaborErr.
	LaborHours *decimal.Decimal
	// LaborErr : ErrMissingLabor si une pièce du sous-arbre n'a pas d'heures ; MissingLabor les liste.
	LaborErr     error
	MissingLabor []int64
	// FTE : LaborHours × volume ÷ base horaire annuelle, à ftePrecision décimales (arrondi d'affichage en
	// sortie) ; nil sans heures ou sans base.
	FTE *decimal.Decimal
	// StockCovered : pour une pièce dont la BOM a du stock, quantité couverte (sur quantité réelle × volume) ;
	// nil si la pièce n'a pas de stock. InStock : couverture totale → délai propre nul (ADR 0022).
	StockCovered *decimal.Decimal
	InStock      bool
}

// Explanation dit sur quoi reposent tous les totaux d'un calcul.
type Explanation struct {
	Currency        money.Currency
	PriceSnapshotID *int64         // nil : prix en direct
	Rates           []pricing.Rate // taux effectivement utilisés, triés par paire
}

// Totals est le résultat d'un calcul : un total par nœud, dans l'ordre des ids.
type Totals struct {
	Nodes       []NodeTotals
	Explanation Explanation
	index       map[int64]int
}

// Node renvoie le total du nœud id.
func (t Totals) Node(id int64) (NodeTotals, bool) {
	i, ok := t.index[id]
	if !ok {
		return NodeTotals{}, false
	}
	return t.Nodes[i], true
}

// ComputeTotals calcule les totaux de chaque nœud selon ADR 0007 et 0008, en O(n) : un parcours en profondeur
// itératif (pas de récursion), enfants par id croissant, puis une remontée des feuilles vers la racine.
// Aucun arrondi : les montants sont exacts, l'arrondi se fait en sortie (ADR 0004).
func ComputeTotals(in TotalsInput) (Totals, error) {
	nodes := slices.Clone(in.Nodes)
	slices.SortFunc(nodes, func(a, b CalcNode) int { return cmp.Compare(a.ID, b.ID) })

	index := make(map[int64]int, len(nodes))
	for i, n := range nodes {
		if _, dup := index[n.ID]; dup {
			return Totals{}, fmt.Errorf("node %d appears twice: %w", n.ID, ErrInvalidTree)
		}
		index[n.ID] = i
	}

	// Enfants de chaque nœud, par id croissant (nodes est trié) ; une seule racine.
	children := make([][]int, len(nodes))
	root := -1
	for i, n := range nodes {
		if n.ParentID == nil {
			if root >= 0 {
				return Totals{}, fmt.Errorf("nodes %d and %d are both roots: %w", nodes[root].ID, n.ID, ErrInvalidTree)
			}
			root = i
			continue
		}
		p, ok := index[*n.ParentID]
		if !ok {
			return Totals{}, fmt.Errorf("node %d: parent %d not in tree: %w", n.ID, *n.ParentID, ErrInvalidTree)
		}
		children[p] = append(children[p], i)
	}
	if root < 0 {
		return Totals{}, fmt.Errorf("no root: %w", ErrInvalidTree)
	}

	// Pré-ordre : un parent est toujours visité avant ses enfants.
	order := make([]int, 0, len(nodes))
	stack := []int{root}
	for len(stack) > 0 {
		i := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		order = append(order, i)
		for c := len(children[i]) - 1; c >= 0; c-- { // empilés à l'envers : dépilés par id croissant
			stack = append(stack, children[i][c])
		}
	}
	if len(order) != len(nodes) {
		return Totals{}, fmt.Errorf("%d nodes unreachable from root (cycle?): %w", len(nodes)-len(order), ErrInvalidTree)
	}

	out := make([]NodeTotals, len(nodes))
	zero, err := money.New(decimal.Zero, string(in.Currency))
	if err != nil {
		return Totals{}, fmt.Errorf("bom currency: %w", err)
	}
	usedRates := map[money.Currency]pricing.Rate{}

	// Quantité réelle, de la racine vers les feuilles.
	for _, i := range order {
		qty := nodes[i].Quantity
		if p := nodes[i].ParentID; p != nil {
			qty = out[index[*p]].RealQuantity.Mul(qty)
		}
		out[i] = NodeTotals{NodeID: nodes[i].ID, RealQuantity: qty}
	}
	allocateStock(nodes, out, in)

	// Remontée : ordre inverse du pré-ordre, chaque enfant est calculé avant son parent.
	// Chaque nœud = sa contribution propre (pièce) + celle de ses enfants (ADR 0007, ADR 0008).
	for k := len(order) - 1; k >= 0; k-- {
		i := order[k]
		n, t := nodes[i], &out[i]
		cost := zero
		var ownErr error
		t.OwnLeadTimeDays = n.ProductionDays
		if n.Part != nil {
			ownErr = partOwnTotals(t, n, in, usedRates)
			if ownErr == nil {
				cost = *t.OwnCost
			}
		}
		labor := decimal.Zero
		if n.Part != nil {
			if h := n.Part.LaborHours; h != nil {
				labor = h.Mul(t.RealQuantity)
			} else {
				t.MissingLabor = []int64{n.ID}
			}
		}
		maxChild := 0
		for _, c := range children[i] {
			ct := out[c]
			t.MissingLabor = append(t.MissingLabor, ct.MissingLabor...)
			if ct.LaborHours != nil {
				labor = labor.Add(*ct.LaborHours)
			}
			t.CO2Kg = t.CO2Kg.Add(ct.CO2Kg)
			t.MassKg = t.MassKg.Add(ct.MassKg)
			maxChild = max(maxChild, ct.LeadTimeDays)
			t.MissingCost = append(t.MissingCost, ct.MissingCost...)
			if ct.Cost != nil && len(t.MissingCost) == 0 {
				if cost, err = cost.Add(*ct.Cost); err != nil { // impossible : tout est dans in.Currency
					return Totals{}, fmt.Errorf("node %d: %w", n.ID, err)
				}
			}
		}
		t.LeadTimeDays = maxChild + t.OwnLeadTimeDays
		switch {
		case in.LaborNotFrozen:
			t.MissingLabor = nil
			t.LaborErr = fmt.Errorf("node %d: %w", n.ID, ErrLaborNotFrozen)
		case len(t.MissingLabor) > 0:
			t.LaborErr = fmt.Errorf("node %d: %d part(s) without labor hours: %w", n.ID, len(t.MissingLabor), ErrMissingLabor)
		default:
			t.LaborHours = &labor
			t.FTE = fte(labor, in.Volume, in.AnnualHours)
		}
		switch {
		case ownErr != nil:
			t.CostErr = fmt.Errorf("node %d: %w", n.ID, ownErr)
		case len(t.MissingCost) > 0:
			t.CostErr = fmt.Errorf("node %d: %d part(s) without cost: %w", n.ID, len(t.MissingCost), ErrMissingCost)
		default:
			t.Cost = &cost
		}
	}

	rates := make([]pricing.Rate, 0, len(usedRates))
	for _, r := range usedRates {
		rates = append(rates, r)
	}
	slices.SortFunc(rates, func(a, b pricing.Rate) int {
		return cmp.Or(cmp.Compare(a.From, b.From), cmp.Compare(a.To, b.To))
	})
	return Totals{
		Nodes:       out,
		Explanation: Explanation{Currency: in.Currency, PriceSnapshotID: in.PriceSnapshotID, Rates: rates},
		index:       index,
	}, nil
}

// allocateStock répartit le stock de chaque pièce entre ses nœuds, par id de nœud croissant (déterministe,
// ADR 0022) : chaque nœud demande quantité réelle × volume. nodes est trié par id.
func allocateStock(nodes []CalcNode, out []NodeTotals, in TotalsInput) {
	if len(in.Stock) == 0 {
		return
	}
	left := make(map[int64]decimal.Decimal, len(in.Stock))
	for partID, q := range in.Stock {
		left[partID] = q
	}
	volume := decimal.NewFromInt32(in.Volume)
	for i, n := range nodes {
		if n.Part == nil {
			continue
		}
		avail, ok := left[n.Part.PartID]
		if !ok {
			continue
		}
		need := out[i].RealQuantity.Mul(volume)
		covered := decimal.Min(avail, need)
		left[n.Part.PartID] = avail.Sub(covered)
		out[i].StockCovered = &covered
		out[i].InStock = need.IsPositive() && covered.Equal(need)
	}
}

// ftePrecision : décimales gardées par la division des ETP (le quotient n'est pas toujours fini) ; l'arrondi
// d'affichage à 2 décimales se fait en sortie, bien au-delà de cette précision.
const ftePrecision = 28

// fte = heures par voiture × volume ÷ base horaire annuelle (ADR 0024) ; nil sans base horaire positive.
func fte(hours decimal.Decimal, volume int32, annualHours *decimal.Decimal) *decimal.Decimal {
	if annualHours == nil || !annualHours.IsPositive() {
		return nil
	}
	v := hours.Mul(decimal.NewFromInt32(volume)).DivRound(*annualHours, ftePrecision)
	return &v
}

// partOwnTotals pose la contribution propre d'une pièce, avant celle de ses enfants : coût = prix × quantité
// réelle, converti dans la devise de la BOM ; CO2 et masse = valeur unitaire × quantité réelle ;
// délai propre = livraison + production. Sans prix ou taux, la pièce se liste dans MissingCost (en tête :
// ordre de parcours) et l'erreur est renvoyée.
func partOwnTotals(t *NodeTotals, n CalcNode, in TotalsInput, usedRates map[money.Currency]pricing.Rate) error {
	p := n.Part
	t.CO2Kg = p.CO2Kg.Mul(t.RealQuantity)
	t.MassKg = p.MassKg.Mul(t.RealQuantity)
	t.OwnLeadTimeDays = p.LeadTimeDays + n.ProductionDays
	if t.InStock { // déjà fabriquée et disponible : ni livraison ni production (ADR 0022)
		t.OwnLeadTimeDays = 0
	}

	cost, err := partCost(t, p.OfferID, in, usedRates)
	if err != nil {
		t.MissingCost = []int64{n.ID}
		return err
	}
	t.OwnCost = &cost
	return nil
}

func partCost(t *NodeTotals, offerID int64, in TotalsInput, usedRates map[money.Currency]pricing.Rate) (money.Money, error) {
	pr, ok := in.Prices[offerID]
	if !ok {
		return money.Money{}, fmt.Errorf("offer %d: %w", offerID, pricing.ErrNoQuote)
	}
	if pr.Err != nil {
		return money.Money{}, fmt.Errorf("offer %d: %w", offerID, pr.Err)
	}
	q := pr.Quote
	t.Quote = &q
	cost := q.Price.Mul(t.RealQuantity)
	from := cost.Currency()
	if from == in.Currency {
		return cost, nil
	}
	rr, ok := in.Rates[from]
	if !ok {
		return money.Money{}, fmt.Errorf("rate %s→%s: %w", from, in.Currency, pricing.ErrNoRate)
	}
	if rr.Err != nil {
		return money.Money{}, fmt.Errorf("rate %s→%s: %w", from, in.Currency, rr.Err)
	}
	if rr.Rate.From != from || rr.Rate.To != in.Currency {
		return money.Money{}, fmt.Errorf("rate for %s→%s is %s→%s: %w", from, in.Currency, rr.Rate.From, rr.Rate.To, pricing.ErrNoRate)
	}
	r := rr.Rate
	t.Rate = &r
	usedRates[from] = r
	return money.New(cost.Amount().Mul(r.Rate), string(in.Currency))
}
