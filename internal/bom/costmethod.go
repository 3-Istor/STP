package bom

import (
	"cmp"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/shopspring/decimal"

	"bomstudio/internal/money"
	"bomstudio/internal/pricing"
	"bomstudio/internal/supplier"
)

// Méthode de calcul du coût (ADR 0031) : une base (ce que le coût contient) × une politique de change.
// La stratégie qui en découle est choisie à un seul endroit, strategyFor (AGENTS §4).

// CostBasis : ce que le coût d'une pièce contient.
type CostBasis string

const (
	// BasisPurchase : cotation brute × quantité réelle, convertie (calcul historique, ADR 0007).
	BasisPurchase CostBasis = "purchase"
	// BasisLanded : achat − remise de contrat + douane + transport + emballage.
	BasisLanded CostBasis = "landed"
	// BasisFull : landed + outillage amorti sur le volume programme.
	BasisFull CostBasis = "full"
)

// FXPolicy : taux utilisé pour convertir dans la devise de la BOM.
type FXPolicy string

const (
	// FXLatest : dernier taux du contexte de prix (direct ou snapshot de prix).
	FXLatest FXPolicy = "latest"
	// FXBudget : taux budget de l'année de calcul, fixé par la trésorerie (table budget_rates).
	FXBudget FXPolicy = "budget"
)

// CostMethod est la méthode de calcul du coût d'une BOM. La valeur zéro vaut DefaultCostMethod.
type CostMethod struct {
	Basis CostBasis
	FX    FXPolicy
	// ProgramVolume : voitures sur la vie du programme, base d'amortissement de l'outillage ; nil si non saisi.
	ProgramVolume *int32
}

// DefaultCostMethod : purchase + latest, le calcul historique (aucune BOM existante ne change).
func DefaultCostMethod() CostMethod { return CostMethod{Basis: BasisPurchase, FX: FXLatest} }

// OrDefault remplace une méthode vide par DefaultCostMethod.
func (m CostMethod) OrDefault() CostMethod {
	if m.Basis == "" && m.FX == "" {
		d := DefaultCostMethod()
		d.ProgramVolume = m.ProgramVolume
		return d
	}
	return m
}

var (
	// ErrNoBudgetRate : aucun taux budget pour (année, devise → devise de la BOM). Jamais le dernier taux à la place.
	ErrNoBudgetRate = errors.New("no budget exchange rate for this year and pair")
	// ErrNoTooling : l'offre n'a pas d'outillage renseigné (NULL) ; « pas d'outillage » se déclare à 0.
	ErrNoTooling = errors.New("offer tooling not provided")
	// ErrNoProgramVolume : méthode full sans volume programme.
	ErrNoProgramVolume = errors.New("program volume not set")
	// ErrNoCostDate : aucune date de calcul (aucune cotation) ; contrats et taux budget ne sont pas datables.
	ErrNoCostDate = errors.New("no calculation date")
	// ErrNoCostFacts : faits de coût (douane, transport, emballage, outillage) non fournis pour l'offre.
	ErrNoCostFacts = errors.New("cost facts not loaded for offer")
)

// ValidateCostMethod refuse une base ou une politique inconnue, et full sans volume programme > 0.
func ValidateCostMethod(m CostMethod) error {
	if _, err := strategyFor(m); err != nil {
		return err
	}
	if m.ProgramVolume != nil && *m.ProgramVolume <= 0 {
		return fmt.Errorf("program volume %d must be > 0: %w", *m.ProgramVolume, ErrInvalidInput)
	}
	if m.Basis == BasisFull && m.ProgramVolume == nil {
		return fmt.Errorf("basis full needs a program volume (amortization base of tooling): %w", ErrInvalidInput)
	}
	return nil
}

// costStrategy : ce qu'une méthode met dans le coût d'une pièce, dans l'ordre d'application d'ADR 0031
// (prix → change → remise → douane → transport et emballage → outillage).
type costStrategy struct {
	fx        FXPolicy
	discount  bool // remise de contrat
	logistics bool // douane, transport, emballage
	tooling   bool // outillage ÷ volume programme
}

// strategyFor est le seul endroit où une méthode devient un calcul (AGENTS §4).
func strategyFor(m CostMethod) (costStrategy, error) {
	var s costStrategy
	switch m.FX {
	case FXLatest, FXBudget:
		s.fx = m.FX
	default:
		return s, fmt.Errorf("fx policy %q must be latest or budget: %w", m.FX, ErrInvalidInput)
	}
	switch m.Basis {
	case BasisPurchase:
	case BasisLanded:
		s.discount, s.logistics = true, true
	case BasisFull:
		s.discount, s.logistics, s.tooling = true, true, true
	default:
		return s, fmt.Errorf("cost basis %q must be purchase, landed or full: %w", m.Basis, ErrInvalidInput)
	}
	return s, nil
}

// CostKind : nature d'une ligne de coût.
type CostKind string

const (
	KindPrice     CostKind = "price"
	KindFX        CostKind = "fx"
	KindDiscount  CostKind = "discount"
	KindCustoms   CostKind = "customs"
	KindFreight   CostKind = "freight"
	KindPackaging CostKind = "packaging"
	KindTooling   CostKind = "tooling"
)

// CostKinds : toutes les natures, dans l'ordre d'application (ordre des lignes en sortie).
var CostKinds = [numCostKinds]CostKind{KindPrice, KindFX, KindDiscount, KindCustoms, KindFreight, KindPackaging, KindTooling}

const numCostKinds = 7

func kindIndex(k CostKind) int { return slices.Index(CostKinds[:], k) }

// CostLine est une composante du coût d'une pièce, dans la devise de la BOM ; les lignes d'une pièce
// s'additionnent exactement en son coût propre. Libellé et source se construisent à la demande (Label,
// Source) : le calcul de 30 000 nœuds ne formate aucun texte.
type CostLine struct {
	Kind   CostKind
	Amount money.Money
	AsOf   *time.Time // date de la donnée (cotation, taux, date de calcul) ; nil pour une table sans date
	d      lineDetail
}

// lineDetail : données brutes d'une ligne, pour son libellé et sa source.
type lineDetail struct {
	qty         decimal.Decimal
	unit        money.Money // prix coté, tarif au kg, emballage, outillage
	mass        decimal.Decimal
	rate, other pricing.Rate // rate : taux appliqué ; other : dernier taux comparé (fx)
	converted   bool
	noLatest    bool
	pct         decimal.Decimal
	origin      string
	discount    Discount
	volume      int32
	offerQty    decimal.Decimal
	source      string // provenance de la cotation ou des faits (« tables », « snapshot N »)
	aggregate   bool   // ligne agrégée d'un sous-arbre
}

var kindNames = map[CostKind]string{
	KindPrice: "Prix d'achat", KindFX: "Écart de change (taux budget − dernier taux)", KindDiscount: "Remises de contrat",
	KindCustoms: "Douane", KindFreight: "Transport", KindPackaging: "Emballage", KindTooling: "Outillage amorti",
}

// Label décrit la ligne en français.
func (l CostLine) Label() string {
	d := l.d
	if d.aggregate {
		return kindNames[l.Kind] + " : somme des pièces du sous-ensemble"
	}
	switch l.Kind {
	case KindPrice:
		s := fmt.Sprintf("Prix coté %s × %s", d.unit, d.qty)
		switch {
		case d.noLatest:
			s += fmt.Sprintf(", au taux %s %s→%s %s (dernier taux indisponible)", d.rate.Source, d.rate.From, d.rate.To, d.rate.Rate)
		case d.converted:
			s += fmt.Sprintf(", au dernier taux %s→%s %s", d.rate.From, d.rate.To, d.rate.Rate)
		}
		return s
	case KindFX:
		return fmt.Sprintf("Écart de change : %s %s→%s %s au lieu du dernier taux %s", d.rate.Source, d.rate.From, d.rate.To,
			d.rate.Rate, d.other.Rate)
	case KindDiscount:
		return discountLabel(d.discount)
	case KindCustoms:
		return fmt.Sprintf("Douane (origine %s) : %s %% du prix net de remise", d.origin, d.pct)
	case KindFreight:
		return fmt.Sprintf("Transport (origine %s) : %s/kg × %s kg × %s", d.origin, d.unit, d.mass, d.qty)
	case KindPackaging:
		return fmt.Sprintf("Emballage %s × %s", d.unit, d.qty)
	case KindTooling:
		s := fmt.Sprintf("Outillage %s ÷ %d véhicules", d.unit, d.volume)
		if !d.qty.Equal(d.offerQty) {
			s += fmt.Sprintf(" × part du nœud %s / %s", d.qty, d.offerQty)
		}
		return s
	}
	return string(l.Kind)
}

// Source dit d'où vient la donnée de la ligne.
func (l CostLine) Source() string {
	d := l.d
	if d.aggregate {
		return "agrégat du sous-arbre"
	}
	switch l.Kind {
	case KindPrice:
		return d.source
	case KindFX:
		return d.rate.Source + " / " + d.other.Source
	case KindDiscount:
		s := "contrats"
		if d.discount.ContractID != nil {
			s = fmt.Sprintf("contrat %d", *d.discount.ContractID)
		}
		if d.source != "" {
			s += " (" + d.source + ")"
		}
		return s
	case KindCustoms:
		return "douane, " + d.source
	case KindFreight:
		return "transport, " + d.source
	case KindPackaging:
		return "emballage de l'offre, " + d.source
	case KindTooling:
		return "outillage de l'offre, " + d.source
	}
	return d.source
}

// Discount est la remise retenue pour une offre : calculée depuis les contrats (brouillon) ou figée (snapshot).
type Discount struct {
	Percent    decimal.Decimal
	Reason     string // discount_applied, no_active_contract, contract_expired, contract_not_started, below_first_tier
	ContractID *int64
	TierMin    *decimal.Decimal
}

// OfferCost : faits de coût d'une offre hors prix. Une composante absente porte son erreur nommée
// (pricing.ErrNoCustomsDuty, ErrNoFreightRate, ErrNoPackaging, ErrNoDestination, ErrNoTooling), jamais un zéro.
type OfferCost struct {
	Origin         string
	CustomsPercent *decimal.Decimal
	CustomsErr     error
	FreightPerKg   *money.Money
	FreightErr     error
	Packaging      *money.Money
	PackagingErr   error
	Tooling        *money.Money
	ToolingErr     error
	// Discount : remise figée (snapshot) ; nil → calculée depuis CostInput.Contracts.
	Discount *Discount
}

// CostInput : ce dont dépend une méthode autre que le calcul historique. Rien d'autre (ni heure, ni base).
type CostInput struct {
	Method CostMethod
	// AsOf : date de calcul (état des contrats, année du taux budget) ; zéro → ErrNoCostDate si nécessaire.
	AsOf time.Time
	// BudgetRates : taux budget de l'année de AsOf vers la devise de la BOM, par devise d'origine.
	BudgetRates map[money.Currency]decimal.Decimal
	// Offers : faits de coût par offre (douane, transport, emballage, outillage, remise figée).
	Offers map[int64]OfferCost
	// Contracts : contrats couvrant chaque offre (brouillon) ; ignorés si la remise de l'offre est figée.
	Contracts map[int64][]supplier.Contract
	// FactsSource : provenance des faits (« tables » ou « snapshot N »), reprise dans les lignes.
	FactsSource string
	// Frozen : faits figés d'un snapshot ; une remise non figée est alors une erreur, jamais recalculée.
	Frozen bool
}

// resolveDiscount évalue les contrats de l'offre (supplier.EvaluateContracts : palier atteint sur toute la
// quantité, bornes inclusives) pour la quantité commandée à asOf. Un contrat expiré n'est pas appliqué et
// le dit (contract_expired) ; plusieurs contrats actifs → supplier.ErrSelectionRequired.
func resolveDiscount(offerID int64, contracts []supplier.Contract, qty decimal.Decimal, price money.Money, asOf time.Time) (Discount, error) {
	if !qty.IsPositive() {
		return Discount{}, fmt.Errorf("offer %d: ordered quantity %s must be > 0: %w", offerID, qty, ErrInvalidInput)
	}
	ev, err := supplier.EvaluateContracts(contracts, supplier.EvaluationInput{
		OfferID: offerID, AsOf: asOf, UnitPrice: price, Quantity: qty.String(),
	})
	if err != nil {
		return Discount{}, fmt.Errorf("discount of offer %d: %w", offerID, err)
	}
	pct, err := decimal.NewFromString(ev.DiscountPercent)
	if err != nil {
		return Discount{}, fmt.Errorf("discount of offer %d: %w", offerID, err)
	}
	d := Discount{Percent: pct, Reason: ev.Reason, ContractID: ev.AppliedContractID}
	if ev.Tier != nil {
		m := ev.Tier.MinQuantity
		d.TierMin = &m
	}
	if ev.Reason == "no_active_contract" {
		// Le motif dit pourquoi : un contrat de l'offre a expiré (le plus récent, déterministe).
		var expired []supplier.Contract
		for _, c := range contracts {
			if !slices.Contains(c.OfferIDs, offerID) {
				continue
			}
			if st, err := supplier.ContractStatus(c, asOf); err == nil && st == "expired" {
				expired = append(expired, c)
			}
		}
		if len(expired) > 0 {
			last := slices.MaxFunc(expired, func(a, b supplier.Contract) int {
				return cmp.Or(cmp.Compare(a.EndsOn, b.EndsOn), cmp.Compare(a.ID, b.ID))
			})
			id := last.ID
			d.Reason, d.ContractID = "contract_expired", &id
		}
	}
	return d, nil
}

// budgetRate présente un taux budget comme un taux daté : 1er janvier de son année, source « budget ».
func budgetRate(from, to money.Currency, year int, rate decimal.Decimal) pricing.Rate {
	return pricing.Rate{From: from, To: to, Rate: rate, Source: fmt.Sprintf("budget %d", year),
		QuotedAt: time.Date(year, time.January, 1, 0, 0, 0, 0, time.UTC)}
}
