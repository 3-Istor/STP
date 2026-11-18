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

// toolingPrecision : décimales gardées par la division de l'outillage (outillage × quantité ÷ (volume
// programme × quantité de l'offre)), seule division non finie du calcul ; l'arrondi ADR 0004 se fait en sortie.
const toolingPrecision = 28

// costCalc porte un calcul selon la méthode (ADR 0031) : la stratégie choisie par strategyFor, les taux
// utilisés et les remises résolues par offre. Pur : ni base, ni réseau, ni horloge.
type costCalc struct {
	in         TotalsInput
	method     CostMethod
	st         costStrategy
	cost       CostInput
	usedRates  map[money.Currency]pricing.Rate
	usedBudget map[money.Currency]pricing.Rate
	// offerQty : quantité par voiture de chaque offre dans la BOM (Σ quantités réelles de ses nœuds).
	offerQty  map[int64]decimal.Decimal
	discounts map[int64]discountResult
	// Valeurs partagées par les lignes et les nœuds du calcul (une allocation par taux, cotation ou remise, pas
	// une par nœud) ; lues seulement (v2-lot-g T8).
	rateRefs     map[pricing.Rate]*pricing.Rate
	quoteRefs    map[int64]*pricing.Quote
	discountRefs map[int64]*Discount
	partMemo     map[partLinesKey]*partLinesMemo
}

// rateRef : pointeur partagé vers un taux égal à r.
func (c *costCalc) rateRef(r pricing.Rate) *pricing.Rate {
	if p, ok := c.rateRefs[r]; ok {
		return p
	}
	p := &r
	c.rateRefs[r] = p
	return p
}

type discountResult struct {
	d   Discount
	err error
}

func newCostCalc(in TotalsInput) (*costCalc, error) {
	c := &costCalc{
		in: in, method: DefaultCostMethod(), usedRates: map[money.Currency]pricing.Rate{},
		usedBudget: map[money.Currency]pricing.Rate{}, offerQty: map[int64]decimal.Decimal{}, discounts: map[int64]discountResult{},
		rateRefs: map[pricing.Rate]*pricing.Rate{}, quoteRefs: map[int64]*pricing.Quote{}, discountRefs: map[int64]*Discount{},
		partMemo: map[partLinesKey]*partLinesMemo{},
	}
	if in.Cost != nil {
		c.cost = *in.Cost
		c.method = in.Cost.Method.OrDefault()
	}
	st, err := strategyFor(c.method)
	if err != nil {
		return nil, fmt.Errorf("cost method: %w", err)
	}
	c.st = st
	return c, nil
}

func (c *costCalc) explanation() Explanation {
	sorted := func(m map[money.Currency]pricing.Rate) []pricing.Rate {
		out := make([]pricing.Rate, 0, len(m))
		for _, r := range m {
			out = append(out, r)
		}
		slices.SortFunc(out, func(a, b pricing.Rate) int { return cmp.Or(cmp.Compare(a.From, b.From), cmp.Compare(a.To, b.To)) })
		return out
	}
	e := Explanation{
		Currency: c.in.Currency, PriceSnapshotID: c.in.PriceSnapshotID, Rates: sorted(c.usedRates),
		Method: c.method, BudgetRates: sorted(c.usedBudget),
	}
	if c.in.Cost != nil && !c.cost.AsOf.IsZero() {
		at := c.cost.AsOf
		e.CostAsOf = &at
	}
	return e
}

func (c *costCalc) resolvedDiscounts() map[int64]Discount {
	if len(c.discounts) == 0 {
		return nil
	}
	out := make(map[int64]Discount, len(c.discounts))
	for id, r := range c.discounts {
		if r.err == nil {
			out[id] = r.d
		}
	}
	return out
}

// partLines calcule les lignes du coût propre d'une pièce, dans l'ordre d'ADR 0031 : prix → change → remise
// → douane → transport et emballage → outillage. Toutes les composantes manquantes sont jointes dans l'erreur.
func (c *costCalc) partLines(t *NodeTotals, p *PartFacts) ([]CostLine, error) {
	pr, ok := c.in.Prices[p.OfferID]
	if !ok {
		return nil, fmt.Errorf("offer %d: %w", p.OfferID, pricing.ErrNoQuote)
	}
	if pr.Err != nil {
		return nil, fmt.Errorf("offer %d: %w", p.OfferID, pr.Err)
	}
	q := pr.Quote
	qp, ok := c.quoteRefs[p.OfferID]
	if !ok {
		qp = &q
		c.quoteRefs[p.OfferID] = qp
	}
	t.Quote = qp
	// Une ligne par nature au plus : capacité fixée d'emblée, sans recopie des lignes (lourdes) quand la tranche
	// grandit (v2-lot-g T8 : la moitié du temps du pont de coût sur 30 000 nœuds).
	lines := make([]CostLine, 0, numCostKinds)
	net, err := c.priceLines(t, q, &lines)
	if err != nil {
		return nil, err
	}
	var errs []error
	if c.st.discount {
		net, err = c.discountLine(p.OfferID, q.Price, net, &lines)
		errs = append(errs, err)
	}
	if c.st.logistics || c.st.tooling {
		oc, ok := c.cost.Offers[p.OfferID]
		if !ok {
			return nil, errors.Join(append(errs, fmt.Errorf("offer %d: %w", p.OfferID, ErrNoCostFacts))...)
		}
		if c.st.logistics {
			errs = append(errs, c.logisticsLines(t, p, oc, net, &lines)...)
		}
		if c.st.tooling {
			errs = append(errs, c.toolingLine(t, p.OfferID, oc, &lines))
		}
	}
	if err := errors.Join(errs...); err != nil {
		return nil, err
	}
	return lines, nil
}

// priceLines pose la ligne de prix (et l'écart de change en budget) et renvoie le prix converti au taux de la
// méthode. En budget, la ligne price est au dernier taux et la ligne fx porte l'écart budget − dernier ; sans
// dernier taux, price est au taux budget et fx est absente.
func (c *costCalc) priceLines(t *NodeTotals, q pricing.Quote, lines *[]CostLine) (money.Money, error) {
	gross := q.Price.Mul(t.RealQuantity)
	at := q.QuotedAt
	d := lineDetail{unit: q.Price, qty: t.RealQuantity, source: q.Source}
	if gross.Currency() == c.in.Currency {
		*lines = append(*lines, CostLine{Kind: KindPrice, Amount: gross, AsOf: &at, d: d})
		return gross, nil
	}
	d.converted = true
	if c.st.fx == FXLatest {
		conv, r, err := c.latest(gross)
		if err != nil {
			return money.Money{}, err
		}
		t.Rate = c.rateRef(r)
		d.rate = t.Rate
		*lines = append(*lines, CostLine{Kind: KindPrice, Amount: conv, AsOf: &at, d: d})
		return conv, nil
	}
	budget, br, err := c.budget(gross)
	if err != nil {
		return money.Money{}, err
	}
	t.Rate = c.rateRef(br)
	latest, lr, lerr := c.latest(gross)
	if lerr != nil { // pas de comparaison possible : le prix est au taux budget, sans ligne d'écart
		d.rate, d.noLatest = t.Rate, true
		*lines = append(*lines, CostLine{Kind: KindPrice, Amount: budget, AsOf: &at, d: d})
		return budget, nil
	}
	delta, err := money.New(budget.Amount().Sub(latest.Amount()), string(c.in.Currency))
	if err != nil {
		return money.Money{}, err
	}
	lat := lr.QuotedAt
	d.rate = c.rateRef(lr)
	fx := lineDetail{rate: t.Rate, other: d.rate}
	*lines = append(*lines, CostLine{Kind: KindPrice, Amount: latest, AsOf: &at, d: d}, CostLine{Kind: KindFX, Amount: delta, AsOf: &lat, d: fx})
	return budget, nil
}

// discountLine applique la remise retenue (% du prix converti) et renvoie le prix net. La ligne existe même
// sans remise (montant nul) pour en dire le motif (contrat expiré, sous le premier palier…).
func (c *costCalc) discountLine(offerID int64, price, net money.Money, lines *[]CostLine) (money.Money, error) {
	d, err := c.discount(offerID, price)
	if err != nil {
		return net, err
	}
	amount, err := money.New(net.Amount().Mul(d.Percent).Shift(-2).Neg(), string(c.in.Currency))
	if err != nil {
		return net, err
	}
	dp, ok := c.discountRefs[offerID]
	if !ok {
		dp = &d
		c.discountRefs[offerID] = dp
	}
	ld := lineDetail{discount: dp}
	if c.cost.Offers[offerID].Discount != nil { // remise figée : dire d'où
		ld.source = c.cost.FactsSource
	}
	at := c.cost.AsOf
	*lines = append(*lines, CostLine{Kind: KindDiscount, Amount: amount, AsOf: &at, d: ld})
	out, err := net.Add(amount)
	if err != nil {
		return net, err
	}
	return out, nil
}

func discountLabel(d Discount) string {
	switch d.Reason {
	case "discount_applied":
		tier := ""
		if d.TierMin != nil {
			tier = fmt.Sprintf(" (palier ≥ %s u.)", d.TierMin)
		}
		return fmt.Sprintf("Remise de contrat %s %%%s", d.Percent, tier)
	case "contract_expired":
		return "Contrat expiré : remise non appliquée"
	case "contract_not_started":
		return "Contrat pas encore actif : remise non appliquée"
	case "below_first_tier":
		return "Quantité sous le premier palier : pas de remise"
	default:
		return "Aucun contrat actif : pas de remise"
	}
}

// discount renvoie la remise de l'offre : figée (snapshot) ou résolue depuis ses contrats pour la quantité
// commandée = quantité par voiture × volume de la BOM, à la date de calcul. Résolue une fois par offre.
func (c *costCalc) discount(offerID int64, price money.Money) (Discount, error) {
	if r, ok := c.discounts[offerID]; ok {
		return r.d, r.err
	}
	var r discountResult
	switch oc, ok := c.cost.Offers[offerID]; {
	case ok && oc.Discount != nil:
		r.d = *oc.Discount
	case c.cost.Frozen:
		r.err = fmt.Errorf("frozen discount of offer %d: %w", offerID, ErrNoCostFacts)
	case c.cost.AsOf.IsZero():
		r.err = fmt.Errorf("discount of offer %d: %w", offerID, ErrNoCostDate)
	default:
		qty := c.offerQty[offerID].Mul(decimal.NewFromInt32(c.in.Volume))
		r.d, r.err = resolveDiscount(offerID, c.cost.Contracts[offerID], qty, price, c.cost.AsOf)
	}
	c.discounts[offerID] = r
	return r.d, r.err
}

// logisticsLines : douane = % du prix net de remise ; transport = tarif au kg × masse × quantité réelle ;
// emballage = montant par unité × quantité réelle ; chaque montant converti au taux de la méthode.
func (c *costCalc) logisticsLines(t *NodeTotals, p *PartFacts, oc OfferCost, net money.Money, lines *[]CostLine) []error {
	var errs []error
	src := c.cost.FactsSource
	switch {
	case oc.CustomsErr != nil:
		errs = append(errs, fmt.Errorf("offer %d: %w", p.OfferID, oc.CustomsErr))
	case oc.CustomsPercent == nil:
		errs = append(errs, fmt.Errorf("offer %d: %w", p.OfferID, pricing.ErrNoCustomsDuty))
	default:
		m, err := money.New(net.Amount().Mul(*oc.CustomsPercent).Shift(-2), string(c.in.Currency))
		if err != nil {
			errs = append(errs, err)
			break
		}
		*lines = append(*lines, CostLine{Kind: KindCustoms, Amount: m, d: lineDetail{origin: oc.Origin, pct: *oc.CustomsPercent, source: src}})
	}
	switch {
	case oc.FreightErr != nil:
		errs = append(errs, fmt.Errorf("offer %d: %w", p.OfferID, oc.FreightErr))
	case oc.FreightPerKg == nil:
		errs = append(errs, fmt.Errorf("offer %d: %w", p.OfferID, pricing.ErrNoFreightRate))
	default:
		m, err := c.chosen(oc.FreightPerKg.Mul(p.MassKg).Mul(t.RealQuantity))
		if err != nil {
			errs = append(errs, err)
			break
		}
		*lines = append(*lines, CostLine{Kind: KindFreight, Amount: m,
			d: lineDetail{origin: oc.Origin, unit: *oc.FreightPerKg, mass: p.MassKg, qty: t.RealQuantity, source: src}})
	}
	switch {
	case oc.PackagingErr != nil:
		errs = append(errs, fmt.Errorf("offer %d: %w", p.OfferID, oc.PackagingErr))
	case oc.Packaging == nil:
		errs = append(errs, fmt.Errorf("offer %d: %w", p.OfferID, pricing.ErrNoPackaging))
	default:
		m, err := c.chosen(oc.Packaging.Mul(t.RealQuantity))
		if err != nil {
			errs = append(errs, err)
			break
		}
		*lines = append(*lines, CostLine{Kind: KindPackaging, Amount: m, d: lineDetail{unit: *oc.Packaging, qty: t.RealQuantity, source: src}})
	}
	return errs
}

// toolingLine amortit l'outillage de l'offre sur le volume programme et le répartit entre ses nœuds au
// prorata de la quantité réelle : outillage × quantité du nœud ÷ (volume programme × quantité de l'offre).
func (c *costCalc) toolingLine(t *NodeTotals, offerID int64, oc OfferCost, lines *[]CostLine) error {
	pv := c.method.ProgramVolume
	switch {
	case pv == nil || *pv <= 0:
		return fmt.Errorf("tooling of offer %d: %w", offerID, ErrNoProgramVolume)
	case oc.ToolingErr != nil:
		return fmt.Errorf("offer %d: %w", offerID, oc.ToolingErr)
	case oc.Tooling == nil:
		return fmt.Errorf("offer %d: %w", offerID, ErrNoTooling)
	}
	conv, err := c.chosen(*oc.Tooling)
	if err != nil {
		return err
	}
	offerQty := c.offerQty[offerID]
	if !offerQty.IsPositive() {
		return fmt.Errorf("tooling of offer %d: no quantity in bom: %w", offerID, ErrInvalidTree)
	}
	share := conv.Amount().Mul(t.RealQuantity).DivRound(decimal.NewFromInt32(*pv).Mul(offerQty), toolingPrecision)
	m, err := money.New(share, string(c.in.Currency))
	if err != nil {
		return err
	}
	*lines = append(*lines, CostLine{Kind: KindTooling, Amount: m,
		d: lineDetail{unit: *oc.Tooling, volume: *pv, qty: t.RealQuantity, offerQty: offerQty, source: c.cost.FactsSource}})
	return nil
}

// chosen convertit au taux de la méthode (dernier taux ou taux budget).
func (c *costCalc) chosen(m money.Money) (money.Money, error) {
	if m.Currency() == c.in.Currency {
		return m, nil
	}
	if c.st.fx == FXBudget {
		out, _, err := c.budget(m)
		return out, err
	}
	out, _, err := c.latest(m)
	return out, err
}

// latest convertit au dernier taux du contexte de prix (pricing.ErrNoRate s'il manque) et le note.
func (c *costCalc) latest(m money.Money) (money.Money, pricing.Rate, error) {
	from := m.Currency()
	rr, ok := c.in.Rates[from]
	if !ok {
		return money.Money{}, pricing.Rate{}, fmt.Errorf("rate %s→%s: %w", from, c.in.Currency, pricing.ErrNoRate)
	}
	if rr.Err != nil {
		return money.Money{}, pricing.Rate{}, fmt.Errorf("rate %s→%s: %w", from, c.in.Currency, rr.Err)
	}
	if rr.Rate.From != from || rr.Rate.To != c.in.Currency {
		return money.Money{}, pricing.Rate{}, fmt.Errorf("rate for %s→%s is %s→%s: %w", from, c.in.Currency, rr.Rate.From, rr.Rate.To, pricing.ErrNoRate)
	}
	r := rr.Rate
	c.usedRates[from] = r
	out, err := money.New(m.Amount().Mul(r.Rate), string(c.in.Currency))
	return out, r, err
}

// budget convertit au taux budget de l'année de calcul (ErrNoBudgetRate s'il manque, jamais le dernier taux).
func (c *costCalc) budget(m money.Money) (money.Money, pricing.Rate, error) {
	from := m.Currency()
	if c.cost.AsOf.IsZero() {
		return money.Money{}, pricing.Rate{}, fmt.Errorf("budget rate %s→%s: %w", from, c.in.Currency, ErrNoCostDate)
	}
	year := c.cost.AsOf.UTC().Year()
	rate, ok := c.cost.BudgetRates[from]
	if !ok {
		return money.Money{}, pricing.Rate{}, fmt.Errorf("budget rate %d %s→%s: %w", year, from, c.in.Currency, ErrNoBudgetRate)
	}
	r := budgetRate(from, c.in.Currency, year, rate)
	c.usedBudget[from] = r
	out, err := money.New(m.Amount().Mul(rate), string(c.in.Currency))
	return out, r, err
}
