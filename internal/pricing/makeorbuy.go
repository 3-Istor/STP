package pricing

import (
	"errors"
	"fmt"

	"github.com/shopspring/decimal"

	"bomstudio/internal/money"
)

// Make or buy d'un sous-ensemble (ADR 0026) : évaluateur pur, sans base ni horloge. Aucune décision n'est prise
// (Won't : décision automatique) ; le résultat donne les deux coûts, l'écart et le volume d'équilibre.

var (
	// ErrInvalidVolume : le volume doit être > 0 pour répartir les coûts fixes.
	ErrInvalidVolume = errors.New("volume must be > 0")
	// ErrBreakEvenOutOfRange : le volume d'équilibre dépasse ce qu'un entier représente (marge infime).
	ErrBreakEvenOutOfRange = errors.New("break-even volume out of range")
)

// CostLine : une composante nommée d'un coût par voiture, déjà dans la devise de la BOM, ou son erreur.
type CostLine struct {
	Label string
	Component
}

// MakeOrBuyInput : coûts fixes totaux (répartis sur le volume) et composantes variables par voiture.
type MakeOrBuyInput struct {
	Volume       int32
	FixedCosts   money.Money // montant total, devise de la BOM
	FixedErr     error       // coûts fixes non convertibles : fabriquer/voiture indisponible, le reste est donné
	MakeVariable []CostLine  // par voiture : coût rendu des pièces, main-d'œuvre, logistique
	Buy          []CostLine  // par voiture : prix Tier 1, douane, transport, emballage
}

// MakeOrBuy : coûts par voiture au volume donné, écart (fabriquer − acheter) et volume d'équilibre.
type MakeOrBuy struct {
	FixedPerCar     *money.Money
	MakeVariable    *money.Money
	MakePerCar      *money.Money
	BuyPerCar       *money.Money
	Difference      *money.Money // > 0 : fabriquer coûte plus cher que d'acheter à ce volume
	BreakEvenVolume *int64       // plus petit volume entier à partir duquel fabriquer coûte au plus autant
	NeverProfitable bool         // acheter ≤ coût variable de fabrication : aucun volume ne rend la fabrication rentable
	Err             error        // composantes manquantes, jointes ; les totaux concernés sont nil
}

// EvaluateMakeOrBuy : fabriquer/voiture = fixes ÷ volume + variable ; volume d'équilibre = ⌈fixes ÷ (acheter −
// variable)⌉ si acheter > variable. Toutes les divisions restent décimales (aucun float).
func EvaluateMakeOrBuy(in MakeOrBuyInput) MakeOrBuy {
	var out MakeOrBuy
	if in.Volume <= 0 {
		out.Err = fmt.Errorf("volume %d: %w", in.Volume, ErrInvalidVolume)
		return out
	}
	cur := string(in.FixedCosts.Currency())
	var errF error
	if in.FixedErr != nil {
		errF = fmt.Errorf("fixed costs: %w", in.FixedErr)
	} else {
		fixed, err := money.New(in.FixedCosts.Amount().DivRound(decimal.NewFromInt32(in.Volume), 28), cur)
		if err != nil {
			errF = err
		} else {
			out.FixedPerCar = &fixed
		}
	}
	variable, errV := sum(in.MakeVariable, cur)
	buy, errB := sum(in.Buy, cur)
	out.Err = errors.Join(errF, errV, errB)
	if errV == nil {
		out.MakeVariable = &variable
		if errF == nil {
			m, _ := out.FixedPerCar.Add(variable)
			out.MakePerCar = &m
		}
	}
	if errB == nil {
		out.BuyPerCar = &buy
	}
	if out.Err != nil {
		return out
	}
	d, _ := out.MakePerCar.Add(buy.Mul(decimal.NewFromInt(-1)))
	out.Difference = &d
	margin := buy.Amount().Sub(variable.Amount())
	if margin.IsZero() && in.FixedCosts.Amount().IsZero() { // mêmes coûts à tout volume : « au plus autant » dès 1
		one := int64(1)
		out.BreakEvenVolume = &one
		return out
	}
	if !margin.IsPositive() {
		out.NeverProfitable = true
		return out
	}
	// Quotient entier et reste exacts (QuoRem à 0 décimale) : plafond sans arrondi intermédiaire.
	v, rem := in.FixedCosts.Amount().QuoRem(margin, 0)
	if rem.IsPositive() {
		v = v.Add(decimal.NewFromInt(1))
	}
	if v.LessThan(decimal.NewFromInt(1)) {
		v = decimal.NewFromInt(1)
	}
	if v.GreaterThan(decimal.NewFromInt(1 << 62)) {
		out.Err = fmt.Errorf("break-even volume %s: %w", v, ErrBreakEvenOutOfRange)
		return out
	}
	be := v.IntPart()
	out.BreakEvenVolume = &be
	return out
}

func sum(lines []CostLine, cur string) (money.Money, error) {
	total, _ := money.New(decimal.Zero, cur)
	var errs []error
	for _, l := range lines {
		if l.Err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", l.Label, l.Err))
			continue
		}
		t, err := total.Add(*l.Amount)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", l.Label, err))
			continue
		}
		total = t
	}
	return total, errors.Join(errs...)
}
