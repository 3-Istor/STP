package snapshot

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/shopspring/decimal"

	"bomstudio/internal/bom"
	"bomstudio/internal/money"
)

// Comparaison de deux versions et pont de coût (ADR 0032). Le calcul est bom.ComputeBridge (pur) ; ce
// fichier lit les deux versions, puis traduit chaque marche en une phrase pour un acheteur, avec des noms.

// Types de référence d'une version.
const (
	RefBOM      = "bom"
	RefSnapshot = "snapshot"
)

// Ref désigne une version : un brouillon (bom:<id>) ou un snapshot publié (snapshot:<id>).
type Ref struct {
	Kind string
	ID   int64
}

func (r Ref) String() string { return r.Kind + ":" + strconv.FormatInt(r.ID, 10) }

// ParseRef lit « snapshot:<id> » ou « bom:<id> » ; autre chose → bom.ErrInvalidInput.
func ParseRef(s string) (Ref, error) {
	kind, raw, ok := strings.Cut(strings.TrimSpace(s), ":")
	id, err := strconv.ParseInt(raw, 10, 64)
	if !ok || (kind != RefBOM && kind != RefSnapshot) || err != nil || id <= 0 {
		return Ref{}, fmt.Errorf("version %q must be snapshot:<id> or bom:<id>: %w", s, bom.ErrInvalidInput)
	}
	return Ref{Kind: kind, ID: id}, nil
}

// VersionTotals : totaux de la racine d'une version.
type VersionTotals struct {
	Ref             Ref
	Label, Name     string
	Method          bom.CostMethod
	PriceSnapshotID *int64
	Cost            money.Money
	CO2Kg, MassKg   decimal.Decimal
	LeadTimeDays    int
}

// ComparisonDelta : vers − de.
type ComparisonDelta struct {
	Cost          money.Money
	CO2Kg, MassKg decimal.Decimal
	LeadTimeDays  int
}

// Cause : une marche du pont, expliquée. NodeIDs : nœuds de la version « vers » ; FromNodeIDs : nœuds retirés
// (ids de la version « de ») ; NodeCount : nombre total de pièces dont le coût bouge à cette marche.
type Cause struct {
	Kind        bom.CauseKind
	Amount      money.Money
	Explanation string
	NodeIDs     []int64
	FromNodeIDs []int64
	NodeCount   int
}

// ChangedPartView : pièce modifiée, avec son chemin lisible et l'avant / après.
type ChangedPartView struct {
	Change             string // added | removed | offer | quantity | offer_quantity
	Path               string
	NodeID, FromNodeID *int64
	PartID             int64
	Before, After      string
	Delta              money.Money
}

// Comparison : deux versions, l'écart, ses causes (Σ = Delta.Cost exactement) et les pièces modifiées.
type Comparison struct {
	From, To     VersionTotals
	Delta        ComparisonDelta
	Causes       []Cause
	ChangedParts []ChangedPartView
}

// WithSupplierNames branche la lecture des noms de fournisseurs (explications) ; sans, « fournisseur <id> ».
func (s *Service) WithSupplierNames(n bom.SupplierNames) *Service {
	s.suppliers = n
	return s
}

// side : une version lue pour la comparaison.
type side struct {
	ref    Ref
	totals VersionTotals
	bridge bom.BridgeSide
	nodes  map[int64]sideNode
}

type sideNode struct {
	parent  *int64
	name    string
	partID  *int64
	offerID *int64
	qty     decimal.Decimal
}

func (s *Service) loadSide(ctx context.Context, ref Ref, labels map[string]string) (side, error) {
	out := side{ref: ref, nodes: map[int64]sideNode{}}
	names := map[int64]string{}
	switch ref.Kind {
	case RefBOM:
		dc, err := s.boms.DraftCost(ctx, ref.ID)
		if err != nil {
			return side{}, err
		}
		for _, n := range dc.Nodes {
			out.nodes[n.ID] = sideNode{parent: n.ParentID, name: n.Name, partID: n.PartID, offerID: n.OfferID, qty: n.Quantity}
			if !n.IsPart() {
				names[n.ID] = n.Name
			}
		}
		out.bridge = bom.BridgeSide{Input: dc.Input, Totals: dc.Totals, Names: names}
		out.totals = VersionTotals{Name: dc.BOM.Name, Method: dc.BOM.CostMethod.OrDefault(), PriceSnapshotID: dc.BOM.PriceSnapshotID}
	default:
		p, err := s.Get(ctx, ref.ID)
		if err != nil {
			return side{}, err
		}
		for _, n := range p.Nodes {
			sn := sideNode{parent: n.ParentKey, name: n.Name, qty: n.Quantity}
			if n.Part != nil {
				sn.partID, sn.offerID = &n.Part.PartID, &n.Part.OfferID
			} else {
				names[n.Key] = n.Name
			}
			out.nodes[n.Key] = sn
		}
		out.bridge = bom.BridgeSide{Input: p.Input, Totals: p.Totals, Names: names}
		m := bom.DefaultCostMethod()
		if p.Snapshot.CostMethod != nil {
			m = *p.Snapshot.CostMethod
		}
		out.totals = VersionTotals{Name: p.Snapshot.Name, Method: m, PriceSnapshotID: &p.Snapshot.PriceSnapshotID}
	}
	out.totals.Ref, out.totals.Label = ref, labels[ref.String()]
	root, ok := out.bridge.Totals.Node(bom.CalcRootID(out.bridge.Input.Nodes))
	if !ok {
		return side{}, fmt.Errorf("%s: no root totals: %w", ref, bom.ErrInvalidTree)
	}
	if root.Cost == nil {
		return side{}, fmt.Errorf("%s: %w: %w", ref, bom.ErrBridgeUnavailable, root.CostErr)
	}
	out.totals.Cost, out.totals.CO2Kg, out.totals.MassKg, out.totals.LeadTimeDays = *root.Cost, root.CO2Kg, root.MassKg, root.LeadTimeDays
	return out, nil
}

// Compare compare deux versions : totaux, écart, causes de l'écart de coût (pont, ADR 0032) et pièces modifiées.
func (s *Service) Compare(ctx context.Context, from, to Ref) (Comparison, error) {
	labels := map[string]string{}
	for _, r := range []Ref{from, to} {
		f, err := s.familyOf(ctx, r)
		if err != nil {
			return Comparison{}, fmt.Errorf("compare %s → %s: %w", from, to, err)
		}
		for k, v := range f.labels {
			labels[k] = v
		}
	}
	a, err := s.loadSide(ctx, from, labels)
	if err != nil {
		return Comparison{}, fmt.Errorf("compare %s → %s: %w", from, to, err)
	}
	z, err := s.loadSide(ctx, to, labels)
	if err != nil {
		return Comparison{}, fmt.Errorf("compare %s → %s: %w", from, to, err)
	}
	br, err := bom.ComputeBridge(a.bridge, z.bridge)
	if err != nil {
		return Comparison{}, fmt.Errorf("compare %s → %s: %w", from, to, err)
	}
	out := Comparison{From: a.totals, To: z.totals, Delta: ComparisonDelta{
		Cost: br.Delta, CO2Kg: z.totals.CO2Kg.Sub(a.totals.CO2Kg), MassKg: z.totals.MassKg.Sub(a.totals.MassKg),
		LeadTimeDays: z.totals.LeadTimeDays - a.totals.LeadTimeDays,
	}}
	nm := &namer{s: s, ctx: ctx, parts: map[int64]string{}, offers: map[int64]string{}}
	for _, step := range br.Steps {
		if step.Amount.Amount().IsZero() {
			continue // une cause nulle n'explique rien ; la somme reste exacte
		}
		c := Cause{Kind: step.Kind, Amount: step.Amount, NodeCount: step.NodeCount}
		for _, n := range step.Nodes {
			if n.ToID != nil {
				c.NodeIDs = append(c.NodeIDs, *n.ToID)
			} else if n.FromID != nil {
				c.FromNodeIDs = append(c.FromNodeIDs, *n.FromID)
			}
		}
		if c.Explanation, err = explain(nm, step, br, a, z); err != nil {
			return Comparison{}, fmt.Errorf("compare %s → %s: %w", from, to, err)
		}
		out.Causes = append(out.Causes, c)
	}
	for _, cp := range br.ChangedParts {
		v, err := changedPartView(nm, cp, a, z)
		if err != nil {
			return Comparison{}, fmt.Errorf("compare %s → %s: %w", from, to, err)
		}
		out.ChangedParts = append(out.ChangedParts, v)
	}
	return out, nil
}

// namer lit (une fois) les noms de pièces et le fournisseur de chaque offre.
type namer struct {
	s      *Service
	ctx    context.Context //nolint:containedctx // durée de vie d'un seul appel à Compare
	parts  map[int64]string
	offers map[int64]string
}

func (n *namer) part(id int64) (string, error) {
	if v, ok := n.parts[id]; ok {
		return v, nil
	}
	p, err := n.s.catalog.Part(n.ctx, id)
	if err != nil {
		return "", err
	}
	n.parts[id] = p.Name
	return p.Name, nil
}

// supplier : nom du fournisseur de l'offre (ou « offre <id> » si les noms ne sont pas câblés).
func (n *namer) supplier(offerID int64) (string, error) {
	if v, ok := n.offers[offerID]; ok {
		return v, nil
	}
	o, err := n.s.catalog.Offer(n.ctx, offerID)
	if err != nil {
		return "", err
	}
	name := fmt.Sprintf("fournisseur %d", o.SupplierID)
	if n.s.suppliers != nil {
		sup, err := n.s.suppliers.Supplier(n.ctx, o.SupplierID)
		if err != nil {
			return "", err
		}
		name = sup.Name
	}
	n.offers[offerID] = name
	return name, nil
}

func (sd side) path(n *namer, id int64) (string, error) {
	var parts []string
	for cur, ok := sd.nodes[id]; ok && cur.parent != nil; cur, ok = sd.nodes[*cur.parent] {
		label := cur.name
		if cur.partID != nil {
			var err error
			if label, err = n.part(*cur.partID); err != nil {
				return "", err
			}
		}
		parts = append(parts, label)
	}
	slices.Reverse(parts)
	return strings.Join(parts, " > "), nil
}

func changedPartView(n *namer, cp bom.ChangedPart, a, z side) (ChangedPartView, error) {
	v := ChangedPartView{Change: cp.Change, NodeID: cp.ToID, FromNodeID: cp.FromID, Before: "—", After: "—", Delta: cp.Delta}
	var err error
	describe := func(offer *int64, qty *decimal.Decimal) (string, error) {
		sup, err := n.supplier(*offer)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("%s · %s u.", sup, qty), nil
	}
	if cp.FromOffer != nil {
		if v.Before, err = describe(cp.FromOffer, cp.FromQty); err != nil {
			return v, err
		}
	}
	if cp.ToOffer != nil {
		if v.After, err = describe(cp.ToOffer, cp.ToQty); err != nil {
			return v, err
		}
	}
	if cp.ToID != nil {
		v.Path, err = z.path(n, *cp.ToID)
		v.PartID = *z.nodes[*cp.ToID].partID
	} else {
		v.Path, err = a.path(n, *cp.FromID)
		v.PartID = *a.nodes[*cp.FromID].partID
	}
	return v, err
}

// currencyNames : article et nom des devises courantes ; une autre devise garde son code.
var currencyNames = map[money.Currency][2]string{
	"EUR": {"l'", "euro"}, "USD": {"le ", "dollar"}, "GBP": {"la ", "livre"}, "JPY": {"le ", "yen"},
	"CNY": {"le ", "yuan"}, "KRW": {"le ", "won"}, "CHF": {"le ", "franc suisse"}, "PLN": {"le ", "zloty"},
	"MXN": {"le ", "peso mexicain"}, "SEK": {"la ", "couronne suédoise"}, "CZK": {"la ", "couronne tchèque"},
}

func currencyName(c money.Currency, article bool) string {
	n, ok := currencyNames[c]
	if !ok {
		return string(c)
	}
	if article {
		return n[0] + n[1]
	}
	return n[1]
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	r := []rune(s)
	return strings.ToUpper(string(r[0])) + string(r[1:])
}

func joinFr(items []string) string {
	switch len(items) {
	case 0:
		return ""
	case 1:
		return items[0]
	}
	return strings.Join(items[:len(items)-1], ", ") + " et " + items[len(items)-1]
}

// counted : « Haut-parleur 13 cm ×4, Vis ×2 » (ordre d'apparition).
func counted(names []string) string {
	order, count := []string{}, map[string]int{}
	for _, n := range names {
		if count[n] == 0 {
			order = append(order, n)
		}
		count[n]++
	}
	out := make([]string, len(order))
	for i, n := range order {
		out[i] = n
		if count[n] > 1 {
			out[i] += fmt.Sprintf(" ×%d", count[n])
		}
	}
	if len(out) > 4 {
		return strings.Join(out[:4], ", ") + fmt.Sprintf(" et %d autres", len(out)-4)
	}
	return strings.Join(out, ", ")
}

func plural(n int, one, many string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, one)
	}
	return fmt.Sprintf("%d %s", n, many)
}

// explain écrit la phrase d'une cause pour un acheteur, à partir des faits qui diffèrent (bom.BridgeDiff).
func explain(n *namer, step bom.BridgeStep, br bom.Bridge, a, z side) (string, error) {
	d := br.Diff
	switch step.Kind {
	case bom.CauseStructure:
		return explainStructure(n, d, a, z)
	case bom.CauseOfferChange:
		return explainOffers(n, d, a, z)
	case bom.CausePrice:
		s := "Nouvelles cotations sur " + plural(len(d.PriceOffers), "offre", "offres")
		if len(step.Nodes) > 0 && step.Nodes[0].ToID != nil {
			if p := z.nodes[*step.Nodes[0].ToID].partID; p != nil {
				name, err := n.part(*p)
				if err != nil {
					return "", err
				}
				s += ", surtout " + name
			}
		}
		return s + ".", nil
	case bom.CauseFX:
		return explainFX(d, step, a, z), nil
	case bom.CauseDiscount:
		return explainDiscounts(n, d, a, z)
	case bom.CauseLogistics:
		return "Douane, transport ou emballage modifiés sur " + plural(len(d.LogisticsOffer), "offre", "offres") + ".", nil
	case bom.CauseTooling:
		s := "Outillage modifié sur " + plural(len(d.ToolingOffers), "offre", "offres")
		if d.ProgramVolume {
			fv, tv := a.totals.Method.ProgramVolume, z.totals.Method.ProgramVolume
			if fv != nil && tv != nil {
				s += fmt.Sprintf(", volume programme %d → %d véhicules", *fv, *tv)
			}
		}
		return s + ".", nil
	case bom.CauseMethod:
		fm, tm := a.totals.Method, z.totals.Method
		return fmt.Sprintf("Méthode de calcul passée de %s · %s à %s · %s.", fm.Basis, fm.FX, tm.Basis, tm.FX), nil
	}
	return "", fmt.Errorf("unknown cause %q", step.Kind)
}

func explainStructure(n *namer, d bom.BridgeDiff, a, z side) (string, error) {
	var added, removed []string
	for _, p := range d.Added {
		if pid := z.nodes[*p.ToID].partID; pid != nil {
			name, err := n.part(*pid)
			if err != nil {
				return "", err
			}
			added = append(added, name)
		}
	}
	for _, p := range d.Removed {
		if pid := a.nodes[*p.FromID].partID; pid != nil {
			name, err := n.part(*pid)
			if err != nil {
				return "", err
			}
			removed = append(removed, name)
		}
	}
	var parts []string
	if len(added) > 0 {
		parts = append(parts, fmt.Sprintf("%s (%s)", plural(len(added), "pièce ajoutée", "pièces ajoutées"), counted(added)))
	}
	if len(removed) > 0 {
		parts = append(parts, fmt.Sprintf("%s (%s)", plural(len(removed), "pièce retirée", "pièces retirées"), counted(removed)))
	}
	if len(d.QtyChanged) > 0 {
		parts = append(parts, plural(len(d.QtyChanged), "quantité modifiée", "quantités modifiées"))
	}
	if len(parts) == 0 {
		return "Attributs de pièces modifiés (masse) : le transport bouge.", nil
	}
	return capitalize(strings.Join(parts, " ; ")) + ".", nil
}

func explainOffers(n *namer, d bom.BridgeDiff, a, z side) (string, error) {
	type key struct {
		part     string
		from, to string
	}
	var order []key
	count := map[key]int{}
	for _, p := range d.OfferChanged {
		fn, tn := a.nodes[*p.FromID], z.nodes[*p.ToID]
		part, err := n.part(*tn.partID)
		if err != nil {
			return "", err
		}
		from, err := n.supplier(*fn.offerID)
		if err != nil {
			return "", err
		}
		to, err := n.supplier(*tn.offerID)
		if err != nil {
			return "", err
		}
		k := key{part, from, to}
		if count[k] == 0 {
			order = append(order, k)
		}
		count[k]++
	}
	var out []string
	for i, k := range order {
		if i == 3 {
			out = append(out, fmt.Sprintf("et %d autres changements", len(order)-3))
			break
		}
		s := fmt.Sprintf("%s : %s → %s", k.part, k.from, k.to)
		if count[k] > 1 {
			s += fmt.Sprintf(" (%d nœuds)", count[k])
		}
		out = append(out, s)
	}
	if len(out) == 0 {
		return "Changements d'offres.", nil
	}
	return strings.Join(out, " ; ") + " (outillage et faits de la nouvelle offre compris).", nil
}

func explainFX(d bom.BridgeDiff, step bom.BridgeStep, a, z side) string {
	cur := z.bridge.Input.Currency
	budget := z.totals.Method.FX == bom.FXBudget && a.totals.Method.FX == bom.FXBudget
	var up, down []string
	for _, c := range d.FXCurrencies {
		var fr, tr decimal.Decimal
		var ok bool
		if budget && a.bridge.Input.Cost != nil && z.bridge.Input.Cost != nil {
			fr, ok = a.bridge.Input.Cost.BudgetRates[c]
			if t, ok2 := z.bridge.Input.Cost.BudgetRates[c]; ok && ok2 {
				tr = t
			} else {
				ok = false
			}
		}
		if !ok {
			f, ok1 := a.bridge.Input.Rates[c]
			t, ok2 := z.bridge.Input.Rates[c]
			if !ok1 || !ok2 || f.Err != nil || t.Err != nil {
				continue
			}
			fr, tr = f.Rate.Rate, t.Rate.Rate
		}
		switch tr.Cmp(fr) {
		case 1:
			up = append(up, currencyName(c, true))
		case -1:
			down = append(down, currencyName(c, true))
		}
	}
	verb := func(names []string, one, many string) string {
		if len(names) == 1 {
			return joinFr(names) + " " + one
		}
		return joinFr(names) + " " + many
	}
	var parts []string
	if len(up) > 0 {
		parts = append(parts, verb(up, "a monté", "ont monté"))
	}
	if len(down) > 0 {
		parts = append(parts, verb(down, "a baissé", "ont baissé"))
	}
	what := "Les taux de change ont bougé"
	if len(parts) > 0 {
		what = capitalize(strings.Join(parts, ", "))
	}
	if budget {
		what += " (taux budget)"
	}
	// Part du coût de « vers » payée hors devise de la BOM : prix et faits convertis des pièces.
	foreign, total := decimal.Zero, z.totals.Cost.Amount()
	for _, t := range z.bridge.Totals.Nodes {
		if t.Quote != nil && t.Quote.Price.Currency() != cur && t.OwnCost != nil {
			foreign = foreign.Add(t.OwnCost.Amount())
		}
	}
	if pct, ok := bom.PercentOf(foreign, total); ok && foreign.IsPositive() { // texte : arrondi half-even ici, une fois
		return fmt.Sprintf("%s : %s %% du coût est payé hors %s.", what, pct.RoundBank(0), currencyName(cur, false))
	}
	return what + "."
}

func explainDiscounts(n *namer, d bom.BridgeDiff, a, z side) (string, error) {
	type agg struct {
		count   int
		maxLost decimal.Decimal
	}
	expired := map[string]*agg{}
	var order []string
	other := 0
	for _, o := range d.DiscountOffers {
		fd, td := a.bridge.Totals.Discounts[o], z.bridge.Totals.Discounts[o]
		if td.Reason != "contract_expired" || fd.Reason == "contract_expired" {
			other++
			continue
		}
		sup, err := n.supplier(o)
		if err != nil {
			return "", err
		}
		g, ok := expired[sup]
		if !ok {
			g = &agg{}
			expired[sup] = g
			order = append(order, sup)
		}
		g.count++
		g.maxLost = decimal.Max(g.maxLost, fd.Percent)
	}
	var out []string
	for _, sup := range order {
		g := expired[sup]
		out = append(out, fmt.Sprintf("Contrat %s expiré : la remise (jusqu'à %s %%) ne s'applique plus sur %s",
			sup, strings.Replace(g.maxLost.String(), ".", ",", 1), plural(g.count, "offre", "offres")))
	}
	if other > 0 {
		out = append(out, "Remise modifiée (palier ou contrat) sur "+plural(other, "offre", "offres"))
	}
	if len(out) == 0 {
		return "Remises modifiées.", nil
	}
	return strings.Join(out, " ; ") + ".", nil
}
