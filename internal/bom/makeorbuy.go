package bom

import (
	"context"
	"errors"
	"fmt"

	"github.com/shopspring/decimal"

	"bomstudio/internal/money"
	"bomstudio/internal/pricing"
)

// MakeOrBuyRequest : saisie de la simulation (ADR 0026, rien n'est stocké). Montants en devise quelconque,
// convertis dans la devise de la BOM aux taux du même contexte que ses totaux.
type MakeOrBuyRequest struct {
	FixedCosts       money.Money // coûts fixes totaux de la fabrication interne, répartis sur le volume
	LogisticsPerUnit money.Money // logistique interne par unité du sous-ensemble
	Tier1Price       money.Money // prix d'achat d'une unité du sous-ensemble chez l'équipementier
	Tier1Origin      string      // pays d'origine de l'offre Tier 1 (douane, transport)
	Tier1Packaging   money.Money // emballage d'une unité achetée
}

// MakeOrBuyResult : la simulation et sur quoi elle repose. La BOM n'est pas modifiée (Won't : décision auto).
type MakeOrBuyResult struct {
	NodeID          int64
	RealQuantity    decimal.Decimal // unités du sous-ensemble par voiture
	Volume          int32
	Currency        money.Currency
	PriceSnapshotID *int64
	Make, Buy       []pricing.CostLine // composantes par voiture
	// Rates : taux utilisés pour les conversions de la simulation (hors coût rendu des pièces, détaillé par
	// LandedCosts), lus une fois chacun, avec leur date.
	Rates []pricing.Rate
	pricing.MakeOrBuy
}

// ErrEmptySubAssembly : le sous-ensemble n'a aucune pièce : rien à fabriquer, ni masse pour le transport Tier 1.
var ErrEmptySubAssembly = errors.New("sub-assembly has no part")

// MakeOrBuy simule fabriquer ou acheter le sous-ensemble nodeID, par voiture, au volume de la BOM :
// fabriquer = coûts fixes ÷ volume + coût rendu des pièces du sous-arbre + heures × taux horaire + logistique ;
// acheter = prix Tier 1 + douane + transport (masse du sous-arbre) + emballage.
func (s *Service) MakeOrBuy(ctx context.Context, bomID, nodeID int64, req MakeOrBuyRequest) (MakeOrBuyResult, error) {
	if s.logistics == nil {
		return MakeOrBuyResult{}, fmt.Errorf("make or buy node %d: %w", nodeID, ErrLogisticsUnavailable)
	}
	if err := pricing.ValidCountry(req.Tier1Origin); err != nil {
		return MakeOrBuyResult{}, fmt.Errorf("make or buy node %d: %w: %w", nodeID, err, ErrInvalidInput)
	}
	for _, m := range []money.Money{req.FixedCosts, req.LogisticsPerUnit, req.Tier1Price, req.Tier1Packaging} {
		if err := pricing.ValidateAmount(m); err != nil {
			return MakeOrBuyResult{}, fmt.Errorf("make or buy node %d: %w: %w", nodeID, err, ErrInvalidInput)
		}
	}
	b, nodes, err := s.Tree(ctx, bomID)
	if err != nil {
		return MakeOrBuyResult{}, fmt.Errorf("make or buy node %d: %w", nodeID, err)
	}
	quantities, err := realQuantities(nodes)
	if err != nil {
		return MakeOrBuyResult{}, fmt.Errorf("make or buy node %d: %w", nodeID, err)
	}
	n, ok := nodeByID(nodes, nodeID)
	if !ok {
		return MakeOrBuyResult{}, fmt.Errorf("make or buy node %d: %w", nodeID, ErrNodeNotFound)
	}
	qty := quantities[nodeID]
	if n.IsPart() {
		return MakeOrBuyResult{}, fmt.Errorf("make or buy node %d: a part is bought by its offer, simulate a sub-assembly: %w",
			nodeID, ErrInvalidInput)
	}
	currency := money.Currency(b.Currency)
	used := map[money.Currency]pricing.Rate{}
	var fatal error
	convert := s.recordingConverter(ctx, b.PriceSnapshotID, currency, used, &fatal)
	out := MakeOrBuyResult{NodeID: nodeID, RealQuantity: qty, Volume: b.Volume, Currency: currency, PriceSnapshotID: b.PriceSnapshotID}

	parts, labor, mass, err := s.subtreeMakeCosts(ctx, b, nodes, quantities, nodeID, convert)
	if err != nil {
		return MakeOrBuyResult{}, fmt.Errorf("make or buy node %d: %w", nodeID, err)
	}
	logistics := perCar(convert, req.LogisticsPerUnit, qty)
	out.Make = []pricing.CostLine{
		{Label: "parts_landed", Component: parts}, {Label: "labor", Component: labor}, {Label: "logistics", Component: logistics},
	}

	// Achat : coût rendu d'une unité Tier 1 (masse d'une unité = masse du sous-arbre par voiture ÷ quantité).
	in := pricing.LandedInput{Purchase: &req.Tier1Price, Packaging: &req.Tier1Packaging, Convert: convert}
	if mass.Err != nil {
		in.MassKg = nil
	} else {
		perUnit := mass.Value.DivRound(qty, 28)
		in.MassKg = &perUnit
	}
	if err := s.tier1Tables(ctx, &in, req.Tier1Origin, b.DestinationCountry); err != nil {
		return MakeOrBuyResult{}, fmt.Errorf("make or buy node %d: %w", nodeID, err)
	}
	if mass.Err != nil {
		in.FreightErr = errors.Join(in.FreightErr, mass.Err) // garde aussi « pas de destination »
	}
	landed := pricing.LandedCost(in)
	out.Buy = []pricing.CostLine{
		{Label: "tier1_price", Component: scale(landed.Purchase, qty)}, {Label: "customs", Component: scale(landed.Customs, qty)},
		{Label: "freight", Component: scale(landed.Freight, qty)}, {Label: "packaging", Component: scale(landed.Packaging, qty)},
	}

	fixed, fixedErr := convert(req.FixedCosts)
	if fixedErr != nil {
		fixed, _ = money.New(decimal.Zero, b.Currency) // ignoré : FixedErr rend fabriquer/voiture indisponible
	}
	if fatal != nil { // panne de lecture d'un taux : erreur, pas une composante « manquante »
		return MakeOrBuyResult{}, fmt.Errorf("make or buy node %d: %w", nodeID, fatal)
	}
	out.MakeOrBuy = pricing.EvaluateMakeOrBuy(pricing.MakeOrBuyInput{
		Volume: b.Volume, FixedCosts: fixed, FixedErr: fixedErr, MakeVariable: out.Make, Buy: out.Buy,
	})
	out.Rates = sortedRates(used)
	return out, nil
}

type decimalOrErr struct {
	Value decimal.Decimal
	Err   error
}

// subtreeMakeCosts : par voiture, coût rendu des pièces du sous-arbre (offres choisies), main-d'œuvre (heures ×
// taux horaire de la pièce) et masse. Une donnée absente rend la composante indisponible, nommément.
func (s *Service) subtreeMakeCosts(ctx context.Context, b BOM, nodes []Node, quantities map[int64]decimal.Decimal, rootID int64,
	convert func(money.Money) (money.Money, error),
) (
	parts, labor pricing.Component, mass decimalOrErr, err error,
) {
	children := map[int64][]Node{}
	for _, n := range nodes {
		if n.ParentID != nil {
			children[*n.ParentID] = append(children[*n.ParentID], n)
		}
	}
	zero, _ := money.New(decimal.Zero, b.Currency)
	partsTotal, laborTotal := zero, zero
	var partsErrs, laborErrs, massErrs []error
	count := 0
	stack := append([]Node(nil), children[rootID]...)
	for len(stack) > 0 {
		n := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		stack = append(stack, children[n.ID]...)
		if !n.IsPart() {
			continue
		}
		count++
		q := quantities[n.ID]
		cmp, lerr := s.LandedCosts(ctx, b.ID, *n.PartID, []int64{*n.OfferID})
		if lerr != nil {
			return pricing.Component{}, pricing.Component{}, decimalOrErr{}, lerr
		}
		if t := cmp.Offers[0].Total; t != nil {
			partsTotal, _ = partsTotal.Add(t.Mul(q))
		} else {
			partsErrs = append(partsErrs, fmt.Errorf("node %d: %w", n.ID, cmp.Offers[0].Err))
		}
		p, perr := s.catalog.Part(ctx, *n.PartID)
		if perr != nil {
			return pricing.Component{}, pricing.Component{}, decimalOrErr{}, perr
		}
		if p.MassKG == nil {
			massErrs = append(massErrs, fmt.Errorf("node %d: %w", n.ID, pricing.ErrNoMass))
		} else {
			mass.Value = mass.Value.Add(p.MassKG.Mul(q))
		}
		if p.LaborHours == nil || p.HourlyRate == nil {
			laborErrs = append(laborErrs, fmt.Errorf("node %d: %w", n.ID, ErrMissingLabor))
			continue
		}
		rate, cerr := convert(*p.HourlyRate)
		if cerr != nil {
			laborErrs = append(laborErrs, fmt.Errorf("node %d: %w", n.ID, cerr))
			continue
		}
		laborTotal, _ = laborTotal.Add(rate.Mul(p.LaborHours.Mul(q)))
	}
	if count == 0 { // rien à fabriquer : aucun coût, ni masse, n'est supposé nul
		empty := pricing.Component{Err: ErrEmptySubAssembly}
		return empty, empty, decimalOrErr{Err: ErrEmptySubAssembly}, nil
	}
	parts, labor = component(partsTotal, partsErrs), component(laborTotal, laborErrs)
	mass.Err = errors.Join(massErrs...)
	return parts, labor, mass, nil
}

func (s *Service) tier1Tables(ctx context.Context, in *pricing.LandedInput, origin string, dest *string) error {
	if dest == nil {
		in.DutyErr, in.FreightErr = pricing.ErrNoDestination, pricing.ErrNoDestination
		return nil
	}
	d, err := s.logistics.CustomsDuty(ctx, origin, *dest)
	switch {
	case errors.Is(err, pricing.ErrNoCustomsDuty):
		in.DutyErr = err
	case err != nil:
		return err
	default:
		in.Duty = &d.Percent
	}
	f, err := s.logistics.FreightRate(ctx, origin, *dest)
	switch {
	case errors.Is(err, pricing.ErrNoFreightRate):
		in.FreightErr = err
	case err != nil:
		return err
	default:
		in.FreightPerKg = &f.PerKg
	}
	return nil
}

func component(total money.Money, errs []error) pricing.Component {
	if err := errors.Join(errs...); err != nil {
		return pricing.Component{Err: err}
	}
	return pricing.Component{Amount: &total}
}

func perCar(convert func(money.Money) (money.Money, error), perUnit money.Money, qty decimal.Decimal) pricing.Component {
	c, err := convert(perUnit)
	if err != nil {
		return pricing.Component{Err: err}
	}
	v := c.Mul(qty)
	return pricing.Component{Amount: &v}
}

func scale(c pricing.Component, qty decimal.Decimal) pricing.Component {
	if c.Err != nil {
		return c
	}
	v := c.Amount.Mul(qty)
	return pricing.Component{Amount: &v}
}

// realQuantities : quantité réelle de chaque nœud (produit des quantités depuis la racine), en une passe
// descendante ; ErrInvalidTree si un nœud n'est pas sous la racine.
func realQuantities(nodes []Node) (map[int64]decimal.Decimal, error) {
	children := map[int64][]Node{}
	var root *Node
	for i := range nodes {
		if nodes[i].ParentID == nil {
			root = &nodes[i]
			continue
		}
		children[*nodes[i].ParentID] = append(children[*nodes[i].ParentID], nodes[i])
	}
	if root == nil {
		return nil, fmt.Errorf("no root: %w", ErrInvalidTree)
	}
	out := map[int64]decimal.Decimal{root.ID: root.Quantity}
	stack := []Node{*root}
	for len(stack) > 0 {
		n := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		for _, c := range children[n.ID] {
			out[c.ID] = out[n.ID].Mul(c.Quantity)
			stack = append(stack, c)
		}
	}
	if len(out) != len(nodes) {
		return nil, fmt.Errorf("%d nodes unreachable from root: %w", len(nodes)-len(out), ErrInvalidTree)
	}
	return out, nil
}

func nodeByID(nodes []Node, id int64) (Node, bool) {
	for _, n := range nodes {
		if n.ID == id {
			return n, true
		}
	}
	return Node{}, false
}
