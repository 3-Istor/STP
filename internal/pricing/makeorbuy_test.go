package pricing

import (
	"errors"
	"testing"

	"github.com/shopspring/decimal"
)

func line(t *testing.T, label, amount string) CostLine {
	return CostLine{Label: label, Component: Component{Amount: m(t, amount, "EUR")}}
}

// TestEvaluateMakeOrBuy : QUAND je lance la simulation sur un sous-ensemble ALORS les deux coûts, l'écart et le
// volume d'équilibre sont donnés (make-or-buy, ADR 0026).
func TestEvaluateMakeOrBuy(t *testing.T) {
	tests := []struct {
		name                       string
		volume                     int32
		fixed                      string
		make, buy                  []string
		wantMake, wantBuy, wantDif string
		wantBreakEven              int64 // 0 : pas de volume d'équilibre attendu
		wantNever                  bool
	}{
		{
			// fixes 100 000 € / 1 000 voitures = 100 ; variable 60 → fabriquer 160 ; acheter 110 ; écart +50 ;
			// équilibre ⌈100 000 ÷ (110 − 60)⌉ = 2 000 voitures.
			name: "à 1 000 voitures, acheter est moins cher ; équilibre à 2 000", volume: 1000, fixed: "100000",
			make: []string{"40", "15", "5"}, buy: []string{"100", "10"},
			wantMake: "160", wantBuy: "110", wantDif: "50", wantBreakEven: 2000,
		},
		{
			name: "équilibre non entier : arrondi au volume supérieur", volume: 10, fixed: "1000",
			make: []string{"10"}, buy: []string{"13"}, // 1000 ÷ 3 = 333,33… → 334
			wantMake: "110", wantBuy: "13", wantDif: "97", wantBreakEven: 334,
		},
		{
			name: "sans coûts fixes et à coût égal : fabriquer coûte autant dès 1 voiture", volume: 100, fixed: "0",
			make: []string{"30"}, buy: []string{"30"},
			wantMake: "30", wantBuy: "30", wantDif: "0", wantBreakEven: 1,
		},
		{
			name: "acheter ≤ coût variable : jamais rentable, sans division", volume: 100, fixed: "500",
			make: []string{"30"}, buy: []string{"30"},
			wantMake: "35", wantBuy: "30", wantDif: "5", wantNever: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := MakeOrBuyInput{Volume: tt.volume, FixedCosts: *m(t, tt.fixed, "EUR")}
			for _, a := range tt.make {
				in.MakeVariable = append(in.MakeVariable, line(t, "make", a))
			}
			for _, a := range tt.buy {
				in.Buy = append(in.Buy, line(t, "buy", a))
			}
			got := EvaluateMakeOrBuy(in)
			if got.Err != nil {
				t.Fatal(got.Err)
			}
			eq := func(label string, v interface{ Amount() decimal.Decimal }, want string) {
				if !v.Amount().Equal(decimal.RequireFromString(want)) {
					t.Errorf("%s %s, want %s", label, v.Amount(), want)
				}
			}
			eq("make", got.MakePerCar, tt.wantMake)
			eq("buy", got.BuyPerCar, tt.wantBuy)
			eq("difference", got.Difference, tt.wantDif)
			if got.NeverProfitable != tt.wantNever {
				t.Errorf("never profitable %v, want %v", got.NeverProfitable, tt.wantNever)
			}
			switch {
			case tt.wantBreakEven == 0 && got.BreakEvenVolume != nil:
				t.Errorf("break-even %d, want none", *got.BreakEvenVolume)
			case tt.wantBreakEven != 0 && (got.BreakEvenVolume == nil || *got.BreakEvenVolume != tt.wantBreakEven):
				t.Errorf("break-even %v, want %d", got.BreakEvenVolume, tt.wantBreakEven)
			}
		})
	}
}

func TestEvaluateMakeOrBuyMissing(t *testing.T) {
	in := MakeOrBuyInput{Volume: 10, FixedCosts: *m(t, "100", "EUR"),
		MakeVariable: []CostLine{line(t, "pièces", "5"), {Label: "main-d'œuvre", Component: Component{Err: ErrNoRate}}},
		Buy:          []CostLine{line(t, "Tier 1", "50"), {Label: "douane", Component: Component{Err: ErrNoCustomsDuty}}},
	}
	got := EvaluateMakeOrBuy(in)
	if got.MakePerCar != nil || got.BuyPerCar != nil || got.Difference != nil || got.BreakEvenVolume != nil {
		t.Fatalf("totals with missing components: %+v", got)
	}
	if !errors.Is(got.Err, ErrNoRate) || !errors.Is(got.Err, ErrNoCustomsDuty) {
		t.Fatalf("err %v, want both missing components named", got.Err)
	}
	if got := EvaluateMakeOrBuy(MakeOrBuyInput{FixedCosts: *m(t, "1", "EUR")}); !errors.Is(got.Err, ErrInvalidVolume) {
		t.Fatalf("zero volume: err %v", got.Err)
	}
	huge := EvaluateMakeOrBuy(MakeOrBuyInput{Volume: 1, FixedCosts: *m(t, "100000000000", "EUR"),
		MakeVariable: []CostLine{line(t, "v", "1")}, Buy: []CostLine{line(t, "b", "1.000000000001")}})
	if !errors.Is(huge.Err, ErrBreakEvenOutOfRange) || huge.BreakEvenVolume != nil {
		t.Fatalf("tiny margin: err %v break-even %v, want ErrBreakEvenOutOfRange", huge.Err, huge.BreakEvenVolume)
	}
	fx := EvaluateMakeOrBuy(MakeOrBuyInput{Volume: 10, FixedCosts: *m(t, "0", "EUR"), FixedErr: ErrNoRate,
		MakeVariable: []CostLine{line(t, "v", "5")}, Buy: []CostLine{line(t, "b", "9")}})
	if fx.MakePerCar != nil || fx.BuyPerCar == nil || fx.MakeVariable == nil || !errors.Is(fx.Err, ErrNoRate) {
		t.Fatalf("fixed costs without rate: %+v, want buy and variable kept, make unavailable", fx)
	}
}
