package pricing

import (
	"errors"
	"testing"

	"github.com/shopspring/decimal"

	"bomstudio/internal/money"
)

func m(t *testing.T, amount, cur string) *money.Money {
	t.Helper()
	v, err := money.Parse(amount, cur)
	if err != nil {
		t.Fatal(err)
	}
	return &v
}

// usdToEUR : convertisseur de test (1 USD = 0,9 EUR) ; une autre devise n'a pas de taux.
func usdToEUR(x money.Money) (money.Money, error) {
	switch x.Currency() {
	case "EUR":
		return x, nil
	case "USD":
		return money.New(x.Amount().Mul(decimal.RequireFromString("0.9")), "EUR")
	}
	return money.Money{}, ErrNoRate
}

// TestLandedCost : achat + douane + transport + emballage, chaque composante détaillée (ADR 0025).
func TestLandedCost(t *testing.T) {
	duty := decimal.RequireFromString("4.5")
	mass := decimal.RequireFromString("2.5")
	tests := []struct {
		name                           string
		in                             func(t *testing.T) LandedInput
		purchase, customs, freight, pk string // "" : composante absente
		total                          string
		wantErr                        []error
	}{
		{
			name: "QUAND je compare deux origines ALORS chaque composante du coût rendu est détaillée",
			in: func(t *testing.T) LandedInput {
				return LandedInput{Purchase: m(t, "100", "USD"), Duty: &duty, FreightPerKg: m(t, "2", "EUR"), MassKg: &mass,
					Packaging: m(t, "1.5", "USD"), Convert: usdToEUR}
			},
			// 100 USD → 90 € ; douane 4,5 % de 90 = 4,05 € ; transport 2 × 2,5 = 5 € ; emballage 1,5 USD → 1,35 €
			purchase: "90", customs: "4.05", freight: "5", pk: "1.35", total: "100.4",
		},
		{
			name: "SANS ligne de douane ALORS pas de douane à 0 ni de total, l'erreur est nommée",
			in: func(t *testing.T) LandedInput {
				return LandedInput{Purchase: m(t, "100", "EUR"), DutyErr: ErrNoCustomsDuty, FreightPerKg: m(t, "2", "EUR"), MassKg: &mass,
					Packaging: m(t, "1", "EUR"), Convert: usdToEUR}
			},
			purchase: "100", freight: "5", pk: "1", wantErr: []error{ErrNoCustomsDuty},
		},
		{
			name: "sans masse ni emballage ni prix : chaque composante dit pourquoi",
			in: func(t *testing.T) LandedInput {
				return LandedInput{PurchaseErr: ErrNoQuote, Duty: &duty, FreightPerKg: m(t, "2", "EUR"),
					PackagingErr: ErrNoPackaging, Convert: usdToEUR}
			},
			wantErr: []error{ErrNoQuote, ErrNoMass, ErrNoPackaging},
		},
		{
			name: "taux manquant pour le transport : ErrNoRate sur cette composante",
			in: func(t *testing.T) LandedInput {
				return LandedInput{Purchase: m(t, "10", "EUR"), Duty: &duty, FreightPerKg: m(t, "2", "GBP"), MassKg: &mass,
					Packaging: m(t, "1", "EUR"), Convert: usdToEUR}
			},
			purchase: "10", customs: "0.45", pk: "1", wantErr: []error{ErrNoRate},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := LandedCost(tt.in(t))
			check := func(label string, c Component, want string) {
				if want == "" {
					if c.Amount != nil || c.Err == nil {
						t.Errorf("%s: %v (err %v), want none with an error", label, c.Amount, c.Err)
					}
					return
				}
				if c.Amount == nil || !c.Amount.Amount().Equal(decimal.RequireFromString(want)) || c.Amount.Currency() != "EUR" {
					t.Errorf("%s: %v (err %v), want %s EUR", label, c.Amount, c.Err, want)
				}
			}
			check("purchase", got.Purchase, tt.purchase)
			check("customs", got.Customs, tt.customs)
			check("freight", got.Freight, tt.freight)
			check("packaging", got.Packaging, tt.pk)
			switch {
			case tt.total == "" && got.Total != nil:
				t.Errorf("total %v, want none", got.Total)
			case tt.total != "" && (got.Total == nil || !got.Total.Amount().Equal(decimal.RequireFromString(tt.total))):
				t.Errorf("total %v (err %v), want %s", got.Total, got.Err, tt.total)
			}
			for _, e := range tt.wantErr {
				if !errors.Is(got.Err, e) {
					t.Errorf("err %v, want it to wrap %v", got.Err, e)
				}
			}
		})
	}
}

func TestLogisticsValidation(t *testing.T) {
	svc := NewLogisticsService(nil) // les saisies invalides sont refusées avant le store
	tests := []struct {
		name string
		err  error
	}{
		{"pays en minuscules", func() error {
			_, err := svc.SetCustomsDuty(t.Context(), CustomsDuty{Origin: "cn", Destination: "FR", Percent: decimal.NewFromInt(1)})
			return err
		}()},
		{"douane négative", func() error {
			_, err := svc.SetCustomsDuty(t.Context(), CustomsDuty{Origin: "CN", Destination: "FR", Percent: decimal.NewFromInt(-1)})
			return err
		}()},
		{"transport négatif", func() error {
			_, err := svc.SetFreightRate(t.Context(), FreightRate{Origin: "CN", Destination: "FR", PerKg: *m(t, "-1", "EUR")})
			return err
		}()},
		{"emballage démesuré", func() error {
			_, err := svc.SetOfferPackaging(t.Context(), 1, *m(t, "1e20", "EUR"))
			return err
		}()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !errors.Is(tt.err, ErrInvalidLogistics) {
				t.Errorf("err %v, want ErrInvalidLogistics", tt.err)
			}
		})
	}
}
