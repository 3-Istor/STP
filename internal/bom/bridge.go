package bom

import (
	"cmp"
	"errors"
	"fmt"
	"slices"

	"github.com/shopspring/decimal"

	"bomstudio/internal/money"
)

// Pont de coût entre deux versions (ADR 0032) : on part de la version « de », on remplace une famille de faits
// à la fois par celle de la version « vers », dans un ordre fixe, et on rechiffre chaque marche avec
// ComputeTotals. Cause = coût(marche k) − coût(marche k−1) : la somme des causes est exactement l'écart.

// CauseKind : famille de faits remplacée à une marche du pont.
type CauseKind string

const (
	// CauseStructure : nœuds ajoutés ou retirés, quantités, attributs des pièces (masse…).
	CauseStructure CauseKind = "structure"
	// CauseOfferChange : un nœud apparié change d'offre (autre fournisseur).
	CauseOfferChange CauseKind = "offer_change"
	// CausePrice : même offre, autre cotation.
	CausePrice CauseKind = "price"
	// CauseFX : derniers taux et taux budget.
	CauseFX CauseKind = "fx"
	// CauseDiscount : remise retenue par offre (contrat expiré, palier…).
	CauseDiscount CauseKind = "discount"
	// CauseLogistics : douane, transport, emballage.
	CauseLogistics CauseKind = "logistics"
	// CauseTooling : outillage des offres et volume programme.
	CauseTooling CauseKind = "tooling"
	// CauseMethod : base et politique de change de la méthode de calcul.
	CauseMethod CauseKind = "method"
)

// BridgeOrder : ordre de remplacement, fixé par ADR 0032. Le changer change l'attribution des écarts.
var BridgeOrder = [...]CauseKind{CauseStructure, CauseOfferChange, CausePrice, CauseFX, CauseDiscount, CauseLogistics, CauseTooling, CauseMethod}

var (
	// ErrBridgeUnavailable : une des deux versions n'a pas de coût complet ; il n'y a pas d'écart à expliquer.
	ErrBridgeUnavailable = errors.New("cost bridge unavailable: a version has no complete cost")
	// ErrBridgeIncomplete : une marche intermédiaire manque d'un fait présent dans aucune des deux versions
	// (méthodes différentes) ; le pont n'est pas inventé.
	ErrBridgeIncomplete = errors.New("cost bridge incomplete: an intermediate step lacks a fact")
	// ErrBridgeMismatch : les extrémités du pont ne redonnent pas les coûts des versions (bug, jamais maquillé).
	ErrBridgeMismatch = errors.New("cost bridge does not reproduce the versions' costs")
)

// BridgeSide : une version telle que le calcul l'a vue (entrée et résultat), et le nom de ses sous-ensembles
// (appariement des nœuds).
type BridgeSide struct {
	Input  TotalsInput
	Totals Totals
	Names  map[int64]string
}

// NodePair : un nœud des deux versions ; FromID nil = ajouté, ToID nil = retiré.
type NodePair struct {
	FromID, ToID *int64
}

// BridgeNodeDelta : écart de coût propre d'un nœud pièce à une marche.
type BridgeNodeDelta struct {
	NodePair
	Delta money.Money
}

// maxStepNodes : nœuds gardés par marche (les plus gros écarts) ; NodeCount dit combien bougent en tout.
const maxStepNodes = 20

// BridgeStep : une marche du pont.
type BridgeStep struct {
	Kind      CauseKind
	Amount    money.Money // exact, dans la devise des versions
	Nodes     []BridgeNodeDelta
	NodeCount int
}

// ChangedPart : pièce ajoutée, retirée, changée d'offre ou de quantité ; Delta = coût propre vers − de.
type ChangedPart struct {
	NodePair
	Change             string // added | removed | offer | quantity | offer_quantity
	FromOffer, ToOffer *int64
	FromQty, ToQty     *decimal.Decimal // quantité par unité du parent
	Delta              money.Money
}

// BridgeDiff : les faits qui diffèrent, par famille (pour expliquer chaque cause).
type BridgeDiff struct {
	Added, Removed []NodePair // nœuds pièce et sous-ensembles
	QtyChanged     []NodePair
	OfferChanged   []NodePair
	PriceOffers    []int64          // offres présentes des deux côtés, cotation différente
	FXCurrencies   []money.Currency // dernier taux ou taux budget différent
	DiscountOffers []int64
	LogisticsOffer []int64
	ToolingOffers  []int64
	ProgramVolume  bool // volume programme différent
	Method         bool // base ou politique de change différente
}

// Bridge : coûts des deux versions, marches dans BridgeOrder (y compris nulles), pièces modifiées et faits.
type Bridge struct {
	From, To, Delta money.Money
	Steps           []BridgeStep
	ChangedParts    []ChangedPart
	Pairs           []NodePair // tous les nœuds : ceux de « vers » (par id), puis les retirés (par id)
	Diff            BridgeDiff
}

// ComputeBridge décompose l'écart de coût entre from et to (ADR 0032). Pur : mêmes entrées, même pont.
func ComputeBridge(from, to BridgeSide) (Bridge, error) {
	cur := to.Input.Currency
	if from.Input.Currency != cur {
		return Bridge{}, fmt.Errorf("cost bridge %s → %s: %w", from.Input.Currency, cur, money.ErrCurrencyMismatch)
	}
	fromCost, err := rootCost(from)
	if err != nil {
		return Bridge{}, fmt.Errorf("cost bridge, from version: %w", err)
	}
	toCost, err := rootCost(to)
	if err != nil {
		return Bridge{}, fmt.Errorf("cost bridge, to version: %w", err)
	}
	b := &bridger{from: from, to: to, cur: cur}
	b.match()

	out := Bridge{From: fromCost, To: toCost, Pairs: b.pairs()}
	if out.Delta, err = subMoney(toCost, fromCost); err != nil {
		return Bridge{}, err
	}
	out.Diff = b.diff()
	states := make([]Totals, len(BridgeOrder)+1)
	costs := make([]money.Money, len(BridgeOrder)+1)
	// Les marches à chiffrer sont indépendantes (calculs purs) : chiffrées en parallèle, résultats rangés par marche
	// (v2-lot-g T8) ; une marche identique à la précédente reprend son calcul.
	var todo []int
	for k := range states {
		if k == 0 || !unchanged(BridgeOrder[k-1], out.Diff) {
			todo = append(todo, k)
		}
	}
	errs := make([]error, len(states))
	parallel(len(todo), func(i int) {
		k := todo[i]
		states[k], errs[k] = ComputeTotals(b.state(k))
	})
	for k := range states {
		if k > 0 && unchanged(BridgeOrder[k-1], out.Diff) { // même entrée que la marche précédente : même calcul
			states[k], costs[k] = states[k-1], costs[k-1] //nolint:gosec // k > 0 : marche précédente
			continue
		}
		if errs[k] != nil {
			return Bridge{}, fmt.Errorf("cost bridge step %d: %w", k, errs[k])
		}
		root, _ := states[k].Node(b.rootID())
		if root.Cost == nil {
			return Bridge{}, fmt.Errorf("cost bridge step %d: %w: %w", k, ErrBridgeIncomplete, root.CostErr)
		}
		costs[k] = *root.Cost
	}
	if !costs[0].Amount().Equal(fromCost.Amount()) || !costs[len(costs)-1].Amount().Equal(toCost.Amount()) {
		return Bridge{}, fmt.Errorf("first step %s vs %s, last step %s vs %s: %w",
			costs[0], fromCost, costs[len(costs)-1], toCost, ErrBridgeMismatch)
	}
	for k, kind := range BridgeOrder {
		amount, err := subMoney(costs[k+1], costs[k])
		if err != nil {
			return Bridge{}, err
		}
		step := BridgeStep{Kind: kind, Amount: amount}
		step.Nodes, step.NodeCount = b.nodeDeltas(states[k], states[k+1])
		out.Steps = append(out.Steps, step)
	}
	if out.ChangedParts, err = b.changedParts(); err != nil {
		return Bridge{}, err
	}
	return out, nil
}

// unchanged dit si la famille kind est identique des deux côtés : la marche ne peut rien changer, on ne la
// rechiffre pas (le contrôle des extrémités reste fait). La structure est toujours rechiffrée (les attributs
// des pièces, comme la masse, ne sont pas comparés ici).
func unchanged(kind CauseKind, d BridgeDiff) bool {
	switch kind {
	case CauseOfferChange:
		return len(d.OfferChanged) == 0
	case CausePrice:
		return len(d.PriceOffers) == 0
	case CauseFX:
		return len(d.FXCurrencies) == 0
	case CauseDiscount:
		return len(d.DiscountOffers) == 0
	case CauseLogistics:
		return len(d.LogisticsOffer) == 0
	case CauseTooling:
		return len(d.ToolingOffers) == 0 && !d.ProgramVolume
	case CauseMethod:
		return !d.Method
	}
	return false
}

func subMoney(a, b money.Money) (money.Money, error) {
	if a.Currency() != b.Currency() {
		return money.Money{}, fmt.Errorf("%s − %s: %w", a, b, money.ErrCurrencyMismatch)
	}
	return money.New(a.Amount().Sub(b.Amount()), string(a.Currency()))
}

func rootCost(s BridgeSide) (money.Money, error) {
	for _, n := range s.Input.Nodes {
		if n.ParentID != nil {
			continue
		}
		t, ok := s.Totals.Node(n.ID)
		if !ok {
			return money.Money{}, fmt.Errorf("root %d has no totals: %w", n.ID, ErrInvalidTree)
		}
		if t.Cost == nil {
			return money.Money{}, fmt.Errorf("%w: %w", ErrBridgeUnavailable, t.CostErr)
		}
		return *t.Cost, nil
	}
	return money.Money{}, fmt.Errorf("no root: %w", ErrInvalidTree)
}

// bridger porte l'appariement des deux arbres et construit les états intermédiaires.
type bridger struct {
	from, to       BridgeSide
	cur            money.Currency
	fromNodes      map[int64]CalcNode
	toNodes        map[int64]CalcNode
	fromToTo       map[int64]int64 // nœud apparié : id « de » → id « vers »
	toToFrom       map[int64]int64
	unified        map[int64]int64 // id « de » → id dans un état à structure « de » (apparié : id « vers » ; retiré : −id)
	fromIDs, toIDs []int64         // triés
}

func indexNodes(nodes []CalcNode) (map[int64]CalcNode, []int64, map[int64][]int64, int64) {
	m := make(map[int64]CalcNode, len(nodes))
	ids := make([]int64, 0, len(nodes))
	children := map[int64][]int64{}
	var root int64
	for _, n := range nodes {
		m[n.ID] = n
		ids = append(ids, n.ID)
	}
	slices.Sort(ids)
	for _, id := range ids {
		n := m[id]
		if n.ParentID == nil {
			root = id
			continue
		}
		children[*n.ParentID] = append(children[*n.ParentID], id)
	}
	return m, ids, children, root
}

// segment : ce qui identifie un nœud parmi ses frères d'une version à l'autre.
func segment(n CalcNode, names map[int64]string) string {
	if n.Part != nil {
		return fmt.Sprintf("p:%d", n.Part.PartID)
	}
	return "a:" + names[n.ID]
}

// match apparie les nœuds par chemin depuis la racine : même segment, rang parmi les frères de même segment
// (ordre des ids). Les racines sont toujours appariées.
func (b *bridger) match() {
	var fromChildren, toChildren map[int64][]int64
	var fromRoot, toRoot int64
	b.fromNodes, b.fromIDs, fromChildren, fromRoot = indexNodes(b.from.Input.Nodes)
	b.toNodes, b.toIDs, toChildren, toRoot = indexNodes(b.to.Input.Nodes)
	b.fromToTo, b.toToFrom = map[int64]int64{}, map[int64]int64{}
	queue := [][2]int64{{fromRoot, toRoot}}
	for len(queue) > 0 {
		p := queue[0]
		queue = queue[1:]
		b.fromToTo[p[0]], b.toToFrom[p[1]] = p[1], p[0]
		bySeg := map[string][]int64{}
		for _, c := range toChildren[p[1]] {
			s := segment(b.toNodes[c], b.to.Names)
			bySeg[s] = append(bySeg[s], c)
		}
		for _, c := range fromChildren[p[0]] {
			s := segment(b.fromNodes[c], b.from.Names)
			if cand := bySeg[s]; len(cand) > 0 {
				queue = append(queue, [2]int64{c, cand[0]})
				bySeg[s] = cand[1:]
			}
		}
	}
	b.unified = make(map[int64]int64, len(b.fromIDs))
	for _, id := range b.fromIDs {
		if t, ok := b.fromToTo[id]; ok {
			b.unified[id] = t
		} else {
			b.unified[id] = -id
		}
	}
}

func (b *bridger) rootID() int64 {
	for _, id := range b.toIDs {
		if b.toNodes[id].ParentID == nil {
			return id
		}
	}
	return 0
}

func (b *bridger) pairs() []NodePair {
	out := make([]NodePair, 0, len(b.toIDs))
	for _, id := range b.toIDs {
		p := NodePair{ToID: ptr(id)}
		if f, ok := b.toToFrom[id]; ok {
			p.FromID = ptr(f)
		}
		out = append(out, p)
	}
	for _, id := range b.fromIDs {
		if _, ok := b.fromToTo[id]; !ok {
			out = append(out, NodePair{FromID: ptr(id)})
		}
	}
	return out
}

func ptr[T any](v T) *T { return &v }

// pairOf retrouve les ids d'origine d'un id d'état.
func (b *bridger) pairOf(stateID int64) NodePair {
	if stateID < 0 {
		return NodePair{FromID: ptr(-stateID)}
	}
	p := NodePair{ToID: ptr(stateID)}
	if f, ok := b.toToFrom[stateID]; ok {
		p.FromID = ptr(f)
	}
	return p
}

// after dit si la famille kind est déjà prise dans « vers » à la marche k (0 = tout « de »).
func after(k int, kind CauseKind) bool {
	return slices.Index(BridgeOrder[:], kind) < k
}

// state construit l'entrée du calcul à la marche k.
func (b *bridger) state(k int) TotalsInput {
	offerTo := after(k, CauseOfferChange)
	var nodes []CalcNode
	if after(k, CauseStructure) {
		nodes = make([]CalcNode, 0, len(b.toIDs))
		for _, id := range b.toIDs {
			n := b.toNodes[id]
			if n.Part != nil && !offerTo {
				if f, ok := b.toToFrom[id]; ok && b.fromNodes[f].Part != nil {
					p := *n.Part
					p.OfferID = b.fromNodes[f].Part.OfferID
					n.Part = &p
				}
			}
			nodes = append(nodes, n)
		}
	} else {
		nodes = make([]CalcNode, 0, len(b.fromIDs))
		for _, id := range b.fromIDs {
			n := b.fromNodes[id]
			n.ID = b.unified[id]
			if n.ParentID != nil {
				n.ParentID = ptr(b.unified[*n.ParentID])
			}
			if n.Part != nil && offerTo {
				if t, ok := b.fromToTo[id]; ok && b.toNodes[t].Part != nil {
					p := *n.Part
					p.OfferID = b.toNodes[t].Part.OfferID
					n.Part = &p
				}
			}
			nodes = append(nodes, n)
		}
	}
	offers := map[int64]bool{}
	for _, n := range nodes {
		if n.Part != nil {
			offers[n.Part.OfferID] = true
		}
	}
	pick := func(kind CauseKind) (BridgeSide, BridgeSide) {
		if after(k, kind) {
			return b.to, b.from
		}
		return b.from, b.to
	}

	prices := make(map[int64]PriceResult, len(offers))
	p1, p2 := pick(CausePrice)
	for o := range offers {
		if r, ok := p1.Input.Prices[o]; ok && r.Err == nil {
			prices[o] = r
		} else if r, ok := p2.Input.Prices[o]; ok && r.Err == nil {
			prices[o] = r
		}
	}
	f1, f2 := pick(CauseFX)
	rates := map[money.Currency]RateResult{}
	for _, s := range []BridgeSide{f2, f1} { // f1 en dernier : il l'emporte
		for c, r := range s.Input.Rates {
			if r.Err == nil {
				rates[c] = r
			}
		}
	}

	m1, _ := pick(CauseMethod)
	t1, t2 := pick(CauseTooling)
	method := sideMethod(m1)
	method.ProgramVolume = sideMethod(t1).ProgramVolume
	if method.ProgramVolume == nil {
		method.ProgramVolume = sideMethod(t2).ProgramVolume
	}
	ci := &CostInput{Method: method, Frozen: true, FactsSource: "pont de coût"}
	if c := f1.Input.Cost; c != nil && !c.AsOf.IsZero() {
		ci.AsOf = c.AsOf
	} else if c := f2.Input.Cost; c != nil {
		ci.AsOf = c.AsOf
	}
	for _, s := range []BridgeSide{f2, f1} {
		if s.Input.Cost == nil {
			continue
		}
		for c, r := range s.Input.Cost.BudgetRates {
			if ci.BudgetRates == nil {
				ci.BudgetRates = map[money.Currency]decimal.Decimal{}
			}
			ci.BudgetRates[c] = r
		}
	}
	l1, l2 := pick(CauseLogistics)
	d1, d2 := pick(CauseDiscount)
	ci.Offers = make(map[int64]OfferCost, len(offers))
	for o := range offers {
		var oc OfferCost
		if f, ok := offerFacts(l1, l2, o); ok {
			oc.Origin, oc.CustomsPercent, oc.CustomsErr = f.Origin, f.CustomsPercent, f.CustomsErr
			oc.FreightPerKg, oc.FreightErr, oc.Packaging, oc.PackagingErr = f.FreightPerKg, f.FreightErr, f.Packaging, f.PackagingErr
		}
		if f, ok := offerFacts(t1, t2, o); ok {
			oc.Tooling, oc.ToolingErr = f.Tooling, f.ToolingErr
		}
		if d, ok := b.discountOf(d1, o); ok {
			oc.Discount = &d
		} else if d, ok := b.discountOf(d2, o); ok {
			oc.Discount = &d
		}
		ci.Offers[o] = oc
	}
	return TotalsInput{
		Nodes: nodes, Currency: b.cur, Prices: prices, Rates: rates, Volume: b.to.Input.Volume,
		PriceSnapshotID: b.to.Input.PriceSnapshotID, Cost: ci,
	}
}

func sideMethod(s BridgeSide) CostMethod {
	if s.Input.Cost == nil {
		return DefaultCostMethod()
	}
	return s.Input.Cost.Method.OrDefault()
}

func offerFacts(first, second BridgeSide, offerID int64) (OfferCost, bool) {
	for _, s := range []BridgeSide{first, second} {
		if s.Input.Cost == nil {
			continue
		}
		if f, ok := s.Input.Cost.Offers[offerID]; ok {
			return f, true
		}
	}
	return OfferCost{}, false
}

// discountOf : remise résolue par le calcul de la version (contrats à sa date, ou figée au snapshot).
func (b *bridger) discountOf(s BridgeSide, offerID int64) (Discount, bool) {
	d, ok := s.Totals.Discounts[offerID]
	return d, ok
}

// nodeDeltas : nœuds pièce dont le coût propre bouge entre deux marches, les plus gros écarts d'abord.
func (b *bridger) nodeDeltas(prev, next Totals) ([]BridgeNodeDelta, int) {
	own := func(t Totals) map[int64]decimal.Decimal {
		m := make(map[int64]decimal.Decimal, len(t.Nodes))
		for _, n := range t.Nodes {
			if n.OwnCost != nil {
				m[n.NodeID] = n.OwnCost.Amount()
			}
		}
		return m
	}
	a, z := own(prev), own(next)
	ids := make([]int64, 0, len(z))
	for id := range a {
		ids = append(ids, id)
	}
	for id := range z {
		if _, ok := a[id]; !ok {
			ids = append(ids, id)
		}
	}
	type d struct {
		id int64
		v  decimal.Decimal
	}
	var moved []d
	for _, id := range ids {
		if v := z[id].Sub(a[id]); !v.IsZero() {
			moved = append(moved, d{id, v})
		}
	}
	slices.SortFunc(moved, func(x, y d) int { return cmp.Or(y.v.Abs().Cmp(x.v.Abs()), cmp.Compare(x.id, y.id)) })
	out := make([]BridgeNodeDelta, 0, min(len(moved), maxStepNodes))
	for _, m := range moved[:min(len(moved), maxStepNodes)] {
		amount, _ := money.New(m.v, string(b.cur)) // devise déjà validée
		out = append(out, BridgeNodeDelta{NodePair: b.pairOf(m.id), Delta: amount})
	}
	return out, len(moved)
}

// diff relève les faits qui diffèrent entre les deux versions, famille par famille (listes triées).
func (b *bridger) diff() BridgeDiff {
	var d BridgeDiff
	for _, p := range b.pairs() {
		switch {
		case p.FromID == nil:
			d.Added = append(d.Added, p)
		case p.ToID == nil:
			d.Removed = append(d.Removed, p)
		default:
			f, t := b.fromNodes[*p.FromID], b.toNodes[*p.ToID]
			if !f.Quantity.Equal(t.Quantity) {
				d.QtyChanged = append(d.QtyChanged, p)
			}
			if f.Part != nil && t.Part != nil && f.Part.OfferID != t.Part.OfferID {
				d.OfferChanged = append(d.OfferChanged, p)
			}
		}
	}
	for o, fr := range b.from.Input.Prices {
		if tr, ok := b.to.Input.Prices[o]; ok && fr.Err == nil && tr.Err == nil &&
			(fr.Quote.Price.Currency() != tr.Quote.Price.Currency() || !fr.Quote.Price.Amount().Equal(tr.Quote.Price.Amount())) {
			d.PriceOffers = append(d.PriceOffers, o)
		}
	}
	slices.Sort(d.PriceOffers)
	fxSet := map[money.Currency]bool{}
	for c, fr := range b.from.Input.Rates {
		if tr, ok := b.to.Input.Rates[c]; ok && fr.Err == nil && tr.Err == nil && !fr.Rate.Rate.Equal(tr.Rate.Rate) {
			fxSet[c] = true
		}
	}
	if fc, tc := b.from.Input.Cost, b.to.Input.Cost; fc != nil && tc != nil {
		for c, fr := range fc.BudgetRates {
			if tr, ok := tc.BudgetRates[c]; ok && !fr.Equal(tr) {
				fxSet[c] = true
			}
		}
		for o, ff := range fc.Offers {
			tf, ok := tc.Offers[o]
			if !ok {
				continue
			}
			if !eqDec(ff.CustomsPercent, tf.CustomsPercent) || !eqMoney(ff.FreightPerKg, tf.FreightPerKg) || !eqMoney(ff.Packaging, tf.Packaging) {
				d.LogisticsOffer = append(d.LogisticsOffer, o)
			}
			if !eqMoney(ff.Tooling, tf.Tooling) {
				d.ToolingOffers = append(d.ToolingOffers, o)
			}
		}
	}
	for c := range fxSet {
		d.FXCurrencies = append(d.FXCurrencies, c)
	}
	slices.Sort(d.FXCurrencies)
	slices.Sort(d.LogisticsOffer)
	slices.Sort(d.ToolingOffers)
	for o, fd := range b.from.Totals.Discounts {
		if td, ok := b.to.Totals.Discounts[o]; ok && (!fd.Percent.Equal(td.Percent) || fd.Reason != td.Reason) {
			d.DiscountOffers = append(d.DiscountOffers, o)
		}
	}
	slices.Sort(d.DiscountOffers)
	fm, tm := sideMethod(b.from), sideMethod(b.to)
	d.Method = fm.Basis != tm.Basis || fm.FX != tm.FX
	d.ProgramVolume = !eqInt32(fm.ProgramVolume, tm.ProgramVolume)
	return d
}

func eqDec(a, b *decimal.Decimal) bool {
	return (a == nil) == (b == nil) && (a == nil || a.Equal(*b))
}

func eqMoney(a, b *money.Money) bool {
	return (a == nil) == (b == nil) && (a == nil || (a.Currency() == b.Currency() && a.Amount().Equal(b.Amount())))
}

func eqInt32(a, b *int32) bool { return (a == nil) == (b == nil) && (a == nil || *a == *b) }

// changedParts : pièces ajoutées, retirées, changées d'offre ou de quantité, avec l'écart de leur coût propre
// tel que chaque version le calcule.
func (b *bridger) changedParts() ([]ChangedPart, error) {
	ownCost := func(t Totals, id int64) decimal.Decimal {
		if n, ok := t.Node(id); ok && n.OwnCost != nil {
			return n.OwnCost.Amount()
		}
		return decimal.Zero
	}
	var out []ChangedPart
	for _, p := range b.pairs() {
		var f, t *CalcNode
		if p.FromID != nil {
			n := b.fromNodes[*p.FromID]
			f = &n
		}
		if p.ToID != nil {
			n := b.toNodes[*p.ToID]
			t = &n
		}
		if (f == nil || f.Part == nil) && (t == nil || t.Part == nil) {
			continue
		}
		c := ChangedPart{NodePair: p}
		before, afterCost := decimal.Zero, decimal.Zero
		if f != nil && f.Part != nil {
			c.FromOffer, c.FromQty = ptr(f.Part.OfferID), ptr(f.Quantity)
			before = ownCost(b.from.Totals, f.ID)
		}
		if t != nil && t.Part != nil {
			c.ToOffer, c.ToQty = ptr(t.Part.OfferID), ptr(t.Quantity)
			afterCost = ownCost(b.to.Totals, t.ID)
		}
		switch {
		case c.FromOffer == nil:
			c.Change = "added"
		case c.ToOffer == nil:
			c.Change = "removed"
		default:
			offer, qty := *c.FromOffer != *c.ToOffer, !c.FromQty.Equal(*c.ToQty)
			switch {
			case offer && qty:
				c.Change = "offer_quantity"
			case offer:
				c.Change = "offer"
			case qty:
				c.Change = "quantity"
			default:
				continue
			}
		}
		delta, err := money.New(afterCost.Sub(before), string(b.cur))
		if err != nil {
			return nil, err
		}
		c.Delta = delta
		out = append(out, c)
	}
	return out, nil
}
