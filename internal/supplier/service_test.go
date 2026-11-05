package supplier

import (
	"errors"
	"testing"

	"github.com/shopspring/decimal"
)

func TestSupplierScores(t *testing.T) {
	tests := []struct {
		name, value string
		valid       bool
	}{
		{"zero", "0", true}, {"five", "5", true}, {"fraction exact", "4.1234567890123456789", true},
		{"missing", "", false}, {"negative", "-0.001", false}, {"above five", "5.00000000000001", false}, {"nan", "NaN", false}, {"infinity", "Infinity", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := patchSupplier(Supplier{}, SupplierPatch{QualityScore: &tt.value, TrustScore: &tt.value})
			if tt.valid {
				if err != nil {
					t.Fatal(err)
				}
				want := decimal.RequireFromString(tt.value)
				if p.QualityScore == nil || p.TrustScore == nil || !p.QualityScore.Equal(want) || !p.TrustScore.Equal(want) {
					t.Fatalf("scores lost precision: %+v", p)
				}
			} else if !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("err=%v, want ErrInvalidInput", err)
			}
		})
	}
}

func TestContractValidation(t *testing.T) {
	tests := []struct {
		name   string
		change func(*ContractInput)
		valid  bool
	}{
		{"valid", func(*ContractInput) {}, true},
		{"same start and end", func(c *ContractInput) { c.EndsOn = c.StartsOn }, true},
		{"bad leap day", func(c *ContractInput) { c.StartsOn = "2025-02-29" }, false},
		{"year zero", func(c *ContractInput) { c.StartsOn = "0000-01-01" }, false},
		{"infinite date", func(c *ContractInput) { c.EndsOn = "infinity" }, false},
		{"reverse dates", func(c *ContractInput) { c.EndsOn = "2025-12-31" }, false},
		{"missing date", func(c *ContractInput) { c.StartsOn = "" }, false},
		{"negative offer", func(c *ContractInput) { c.OfferIDs = []int64{-1} }, false},
		{"duplicate offer", func(c *ContractInput) { c.OfferIDs = []int64{10, 10} }, false},
		{"zero threshold", func(c *ContractInput) { c.Tiers[0].MinQuantity = "0" }, false},
		{"negative threshold", func(c *ContractInput) { c.Tiers[0].MinQuantity = "-1" }, false},
		{"nan threshold", func(c *ContractInput) { c.Tiers[0].MinQuantity = "NaN" }, false},
		{"negative discount", func(c *ContractInput) { c.Tiers[0].DiscountPercent = "-0.001" }, false},
		{"discount above hundred", func(c *ContractInput) { c.Tiers[0].DiscountPercent = "100.0001" }, false},
		{"same threshold different scale", func(c *ContractInput) {
			c.Tiers = append(c.Tiers, TierInput{MinQuantity: "500.0", DiscountPercent: "1"})
		}, false},
		{"decreasing discount allowed", func(c *ContractInput) {
			c.Tiers = append(c.Tiers, TierInput{MinQuantity: "1000", DiscountPercent: "1"})
		}, true},
		{"empty optional relations", func(c *ContractInput) { c.OfferIDs = nil; c.Tiers = nil; c.Conditions = "" }, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := ContractInput{StartsOn: "2026-01-01", EndsOn: "2026-12-31", Conditions: "Volume", OfferIDs: []int64{10}, Tiers: []TierInput{{MinQuantity: "500", DiscountPercent: "5"}}}
			tt.change(&in)
			got, err := contractFromInput(7, in)
			if !tt.valid {
				if !errors.Is(err, ErrInvalidInput) {
					t.Fatalf("err=%v, want ErrInvalidInput", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got.OfferIDs == nil || got.Tiers == nil || got.SupplierID != 7 {
				t.Fatalf("contract incomplete: %+v", got)
			}
		})
	}
}
