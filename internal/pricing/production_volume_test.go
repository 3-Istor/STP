package pricing

import (
	"bomstudio/internal/money"
	"bomstudio/internal/supplier"
	"errors"
	"github.com/shopspring/decimal"
	"testing"
	"time"
)

func productionBasis() CostBasis {
	return CostBasis{BOMVersion: "draft:1@2", ReferenceVolume: 100, Context: PriceContext{Mode: "live", AsOf: time.Date(2026, 11, 5, 0, 0, 0, 0, time.UTC)}}
}
func TestProductionVolumeExactFloorAndLimits(t *testing.T) {
	tests := []struct {
		name, budget, cost string
		want               int64
		wantErr            error
	}{
		{"66 cars", "1000000", "15000", 66, nil},
		{"unrounded cost before floor", "1000000", "15151.515152", 65, nil},
		{"not affordable", "1", "15000", 0, nil}, {"zero budget", "0", "15000", 0, nil},
		{"zero cost", "100", "0", 0, ErrZeroUnitCost}, {"missing cost", "100", "missing", 0, ErrUnitCostUnavailable},
		{"overflow", "2147483648", "1", 0, ErrProductionVolumeOverflow},
		{"large exact input", "9007199254740993", "9007199254740993", 1, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var cost *money.Exact
			if tt.cost != "missing" {
				cost = budgetExact(t, tt.cost, "EUR")
			}
			result, err := EstimateProduction(*budgetMoney(t, tt.budget, "EUR"), cost, productionBasis())
			if !errors.Is(err, tt.wantErr) {
				t.Fatal(err)
			}
			if err != nil {
				return
			}
			if result.Volume != tt.want || result.Affordable != (tt.want > 0) || result.Basis.ReferenceVolume != 100 {
				t.Fatal(result)
			}
			_, err = result.PublishableVolume()
			if tt.want == 0 && !errors.Is(err, ErrNoAffordableVolume) {
				t.Fatal("zero must not publish", err)
			}
			if tt.want > 0 && err != nil {
				t.Fatal(err)
			}
		})
	}
}
func TestProductionRetainsReferenceDiscountAndChangesWithPrice(t *testing.T) {
	// Pure supplier calculator is the real existing implementation; no second BOM engine.
	basis := productionBasis()
	contract := supplier.Contract{ID: 1, SupplierID: 2, StartsOn: "2026-01-01", EndsOn: "2026-12-31", OfferIDs: []int64{3}, Tiers: []supplier.Tier{{MinQuantity: decimal.RequireFromString("100"), DiscountPercent: decimal.RequireFromString("10")}}}
	calculate := func(price string) ProductionVolume {
		t.Helper()
		unit := *budgetMoney(t, price, "EUR")
		evaluation, err := supplier.EvaluateContracts([]supplier.Contract{contract}, supplier.EvaluationInput{OfferID: 3, Quantity: "100", UnitPrice: unit, AsOf: basis.Context.AsOf})
		if err != nil {
			t.Fatal(err)
		}
		total, err := money.NewExact(evaluation.Total)
		if err != nil {
			t.Fatal(err)
		}
		// Reference quantity is exactly 100: decimal shift is exact, not a division rounded to precision.
		exact, err := total.Mul(decimal.RequireFromString("0.01"))
		if err != nil {
			t.Fatal(err)
		}
		local := basis
		local.Discounts = []DiscountFact{{OfferID: 3, ContractID: evaluation.AppliedContractID, Quantity: evaluation.Quantity, MinQuantity: evaluation.Tier.MinQuantity.String(), DiscountPercent: evaluation.DiscountPercent, Reason: evaluation.Reason}}
		result, err := EstimateProduction(*budgetMoney(t, "1000000", "EUR"), &exact, local)
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	before := calculate("15000")
	after := calculate("20000")
	if before.Volume != 74 || after.Volume != 55 || before.Basis.ReferenceVolume != 100 || after.Basis.Discounts[0].Quantity != "100" {
		t.Fatal(before, after)
	}
	if after.Basis.Discounts[0].DiscountPercent != "10" || *after.Basis.Discounts[0].ContractID != 1 {
		t.Fatal("discount evidence lost")
	}
	basis.ReferenceVolume = 0
	if _, err := EstimateProduction(*budgetMoney(t, "10", "EUR"), budgetExact(t, "1", "EUR"), basis); !errors.Is(err, ErrInvalidBudget) {
		t.Fatal(err)
	}
}
func TestProductionInverseBudgetAndCopiedEvidence(t *testing.T) {
	basis := productionBasis()
	id := int64(7)
	basis.Context.SnapshotID = &id
	basis.Context.Mode = "snapshot"
	basis.Rates = []FXQuote{{ID: 4, BaseCurrency: "USD", QuoteCurrency: "EUR", Rate: "0.3", Source: "test", QuotedAt: basis.Context.AsOf}}
	contractID := int64(9)
	basis.Discounts = []DiscountFact{{OfferID: 1, ContractID: &contractID, Quantity: "100", DiscountPercent: "0"}}
	result, err := EstimateProduction(*budgetMoney(t, "3", "EUR"), budgetExact(t, "1", "USD"), basis)
	if err != nil || result.Volume != 10 || result.BudgetConversion.Direction != "inverse" {
		t.Fatal(result, err)
	}
	id = 99
	contractID = 99
	basis.Rates[0].Rate = "100"
	if *result.Basis.Context.SnapshotID != 7 || *result.Basis.Discounts[0].ContractID != 9 || result.Basis.Rates[0].Rate != "0.3" {
		t.Fatal("evidence aliases mutable caller input")
	}
}
