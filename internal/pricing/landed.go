package pricing

import (
	"context"
	"errors"
	"fmt"
	"regexp"

	"github.com/shopspring/decimal"

	"bomstudio/internal/money"
)

// Coût total rendu (total-landed-cost, ADR 0025) : achat + douane + transport + emballage, chaque composante
// convertie dans la devise de la BOM avec les taux du même contexte que ses totaux.

var (
	// ErrNoCustomsDuty : aucune ligne de douane pour (origine, destination) ; jamais une douane à 0 implicite.
	ErrNoCustomsDuty = errors.New("no customs duty for this origin and destination")
	// ErrNoFreightRate : aucun coût de transport pour (origine, destination).
	ErrNoFreightRate = errors.New("no freight rate for this origin and destination")
	// ErrNoPackaging : l'offre n'a pas de coût d'emballage renseigné.
	ErrNoPackaging = errors.New("offer has no packaging cost")
	// ErrNoDestination : la BOM n'a pas de pays de destination.
	ErrNoDestination = errors.New("bom has no destination country")
	// ErrNoMass : la pièce n'a pas de masse, le transport au kg n'est pas calculable.
	ErrNoMass = errors.New("part has no mass")
	// ErrUnknownOffer : l'offre dont on règle l'emballage n'existe pas.
	ErrUnknownOffer = errors.New("offer not found")
	// ErrInvalidLogistics : saisie invalide d'une table (pays, pourcentage, montant).
	ErrInvalidLogistics = errors.New("invalid logistics input")
)

// CustomsDuty : droit de douane en % du prix d'achat, par (origine, destination).
type CustomsDuty struct {
	Origin, Destination string
	Percent             decimal.Decimal
}

// FreightRate : coût de transport par kg, par (origine, destination).
type FreightRate struct {
	Origin, Destination string
	PerKg               money.Money
}

// LogisticsStore : tables paramétrables et emballage des offres. L'implémentation Postgres vit dans pricingstore.
type LogisticsStore interface {
	CustomsDuty(ctx context.Context, origin, destination string) (CustomsDuty, error) // ErrNoCustomsDuty
	FreightRate(ctx context.Context, origin, destination string) (FreightRate, error) // ErrNoFreightRate
	OfferPackaging(ctx context.Context, offerID int64) (money.Money, error)           // ErrNoPackaging
	ListCustomsDuties(ctx context.Context) ([]CustomsDuty, error)
	ListFreightRates(ctx context.Context) ([]FreightRate, error)
	SetCustomsDuty(ctx context.Context, d CustomsDuty) (CustomsDuty, error)
	SetFreightRate(ctx context.Context, f FreightRate) (FreightRate, error)
	SetOfferPackaging(ctx context.Context, offerID int64, m money.Money) (money.Money, error)
	ClearOfferPackaging(ctx context.Context, offerID int64) error
	DeleteCustomsDuty(ctx context.Context, origin, destination string) error
	DeleteFreightRate(ctx context.Context, origin, destination string) error
}

// LogisticsService valide les saisies des tables avant de les écrire.
type LogisticsService struct{ store LogisticsStore }

func NewLogisticsService(s LogisticsStore) *LogisticsService { return &LogisticsService{store: s} }

var countryCode = regexp.MustCompile(`^[A-Z]{2}$`)

// ValidCountry : code ISO 3166-1 alpha-2 en majuscules.
func ValidCountry(c string) error {
	if !countryCode.MatchString(c) {
		return fmt.Errorf("country %q must be ISO 3166-1 alpha-2 (two capital letters): %w", c, ErrInvalidLogistics)
	}
	return nil
}

var maxDutyPercent = decimal.NewFromInt(1000)

func (s *LogisticsService) Store() LogisticsStore { return s.store }

func (s *LogisticsService) ListCustomsDuties(ctx context.Context) ([]CustomsDuty, error) {
	return s.store.ListCustomsDuties(ctx)
}

func (s *LogisticsService) ListFreightRates(ctx context.Context) ([]FreightRate, error) {
	return s.store.ListFreightRates(ctx)
}

// SetCustomsDuty : pourcentage dans [0, 1000].
func (s *LogisticsService) SetCustomsDuty(ctx context.Context, d CustomsDuty) (CustomsDuty, error) {
	if err := errors.Join(ValidCountry(d.Origin), ValidCountry(d.Destination)); err != nil {
		return CustomsDuty{}, err
	}
	if d.Percent.IsNegative() || d.Percent.GreaterThan(maxDutyPercent) || !d.Percent.Equal(d.Percent.Truncate(6)) {
		return CustomsDuty{}, fmt.Errorf("duty %s%% must be in [0, 1000] with at most 6 decimals: %w", d.Percent, ErrInvalidLogistics)
	}
	return s.store.SetCustomsDuty(ctx, d)
}

// SetFreightRate : montant par kg ≥ 0.
func (s *LogisticsService) SetFreightRate(ctx context.Context, f FreightRate) (FreightRate, error) {
	if err := errors.Join(ValidCountry(f.Origin), ValidCountry(f.Destination), validAmount(f.PerKg)); err != nil {
		return FreightRate{}, err
	}
	return s.store.SetFreightRate(ctx, f)
}

// SetOfferPackaging : montant par unité ≥ 0.
func (s *LogisticsService) SetOfferPackaging(ctx context.Context, offerID int64, m money.Money) (money.Money, error) {
	if err := validAmount(m); err != nil {
		return money.Money{}, err
	}
	return s.store.SetOfferPackaging(ctx, offerID, m)
}

// ClearOfferPackaging remet l'emballage de l'offre à « non renseigné ».
func (s *LogisticsService) ClearOfferPackaging(ctx context.Context, offerID int64) error {
	return s.store.ClearOfferPackaging(ctx, offerID)
}

// DeleteCustomsDuty retire une ligne de douane (sans effet si absente).
func (s *LogisticsService) DeleteCustomsDuty(ctx context.Context, origin, destination string) error {
	if err := errors.Join(ValidCountry(origin), ValidCountry(destination)); err != nil {
		return err
	}
	return s.store.DeleteCustomsDuty(ctx, origin, destination)
}

// DeleteFreightRate retire une ligne de transport (sans effet si absente).
func (s *LogisticsService) DeleteFreightRate(ctx context.Context, origin, destination string) error {
	if err := errors.Join(ValidCountry(origin), ValidCountry(destination)); err != nil {
		return err
	}
	return s.store.DeleteFreightRate(ctx, origin, destination)
}

var maxLogisticsAmount = decimal.New(1, 12)

// ValidateAmount : montant saisi dans [0, 1e12[ avec au plus 6 décimales (tables logistiques, simulations).
func ValidateAmount(m money.Money) error { return validAmount(m) }

func validAmount(m money.Money) error {
	a := m.Amount()
	if a.IsNegative() || a.GreaterThanOrEqual(maxLogisticsAmount) || !a.Equal(a.Truncate(6)) {
		return fmt.Errorf("amount %s must be in [0, 1e12[ with at most 6 decimals: %w", m, ErrInvalidLogistics)
	}
	return nil
}

// LandedInput : ce dont dépend le coût rendu d'une unité. Les erreurs des composantes absentes sont portées
// telles quelles (ErrNoCustomsDuty, ErrNoFreightRate, ErrNoPackaging, ErrNoMass, ErrNoQuote) ; Convert
// convertit un montant dans la devise de la BOM (ErrNoRate).
type LandedInput struct {
	Purchase     *money.Money
	PurchaseErr  error
	Duty         *decimal.Decimal // % du prix d'achat
	DutyErr      error
	FreightPerKg *money.Money
	FreightErr   error
	MassKg       *decimal.Decimal
	Packaging    *money.Money
	PackagingErr error
	Convert      func(money.Money) (money.Money, error)
}

// Component : une composante convertie, ou la raison de son absence.
type Component struct {
	Amount *money.Money
	Err    error
}

// Landed : détail du coût rendu d'une unité ; Total n'existe que si toutes les composantes existent.
type Landed struct {
	Purchase, Customs, Freight, Packaging Component
	Total                                 *money.Money
	Err                                   error // toutes les composantes manquantes, jointes
}

var hundred = decimal.NewFromInt(100)

// LandedCost calcule chaque composante (pur : ni base, ni réseau, ni horloge). Douane = achat converti × % ;
// transport = tarif au kg converti × masse ; emballage converti. Aucune composante manquante n'est remplacée
// par zéro.
func LandedCost(in LandedInput) Landed {
	var out Landed
	convert := func(m *money.Money, err error) Component {
		if err != nil {
			return Component{Err: err}
		}
		c, err := in.Convert(*m)
		if err != nil {
			return Component{Err: err}
		}
		return Component{Amount: &c}
	}
	out.Purchase = convert(in.Purchase, in.PurchaseErr)
	switch {
	case out.Purchase.Err != nil:
		out.Customs = Component{Err: fmt.Errorf("customs need the purchase price: %w", out.Purchase.Err)}
	case in.DutyErr != nil:
		out.Customs = Component{Err: in.DutyErr}
	default:
		d := out.Purchase.Amount.Amount().Mul(*in.Duty).Div(hundred) // ÷ 100 : exact en décimal
		c, err := money.New(d, string(out.Purchase.Amount.Currency()))
		out.Customs = Component{Amount: &c, Err: err}
	}
	switch {
	case in.FreightErr != nil:
		out.Freight = Component{Err: in.FreightErr}
	case in.MassKg == nil:
		out.Freight = Component{Err: ErrNoMass}
	default:
		f := in.FreightPerKg.Mul(*in.MassKg)
		out.Freight = convert(&f, nil)
	}
	out.Packaging = convert(in.Packaging, in.PackagingErr)

	var errs []error
	total := (*money.Money)(nil)
	for i, c := range []Component{out.Purchase, out.Customs, out.Freight, out.Packaging} {
		if c.Err != nil {
			if i != 1 || out.Purchase.Err == nil { // la douane dérivée d'un achat manquant ne répète pas l'erreur
				errs = append(errs, c.Err)
			}
			continue
		}
		if total == nil {
			t := *c.Amount
			total = &t
			continue
		}
		t, err := total.Add(*c.Amount)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		total = &t
	}
	if out.Err = errors.Join(errs...); out.Err == nil {
		out.Total = total
	}
	return out
}
