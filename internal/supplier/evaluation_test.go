package supplier

import (
	"errors"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"bomstudio/internal/money"
)

func evaluationPrice(t *testing.T, amount, currency string) money.Money {
	t.Helper()
	p, err := money.Parse(amount, currency)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func evaluationDate(t *testing.T, value string) time.Time {
	t.Helper()
	d, err := time.Parse(time.RFC3339, value)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func evaluationContract(id int64) Contract {
	return Contract{
		ID: id, SupplierID: 7, StartsOn: "2026-01-01", EndsOn: "2026-12-31", OfferIDs: []int64{101},
		Tiers: []Tier{{MinQuantity: decimal.NewFromInt(500), DiscountPercent: decimal.NewFromInt(5)}},
	}
}

func evaluationID(id int64) *int64 { return &id }

func TestEvaluateContractsAmounts(t *testing.T) {
	tests := []struct {
		name, quantity, price, currency string
		tiers                           []Tier
		wantTotal, wantDiscount         string
		wantThreshold, wantReason       string
	}{
		{"below threshold", "499", "10", "EUR", nil, "4990", "0", "", "below_first_tier"},
		{"at threshold", "500", "10", "EUR", nil, "4750", "5", "500", "discount_applied"},
		{"whole quantity discounted", "1000", "10", "EUR", nil, "9500", "5", "500", "discount_applied"},
		{"fractional quantity", "500.5", "10", "EUR", nil, "4754.75", "5", "500", "discount_applied"},
		{"ninety five percent", "1000", "10", "EUR", []Tier{{decimal.NewFromInt(500), decimal.NewFromInt(95)}}, "500", "95", "500", "discount_applied"},
		{"full discount", "1000", "10", "EUR", []Tier{{decimal.NewFromInt(500), decimal.NewFromInt(100)}}, "0", "100", "500", "discount_applied"},
		{"no intermediate rounding", "1000", "0.005", "EUR", nil, "4.75", "5", "500", "discount_applied"},
		{"currency preserved", "1000", "10", "USD", nil, "9500", "5", "500", "discount_applied"},
		{"decimal rate beyond division precision", "1", "10", "EUR", []Tier{{decimal.NewFromInt(1), decimal.RequireFromString("33.33333333333333333333")}}, "6.666666666666666666667", "33.33333333333333333333", "1", "discount_applied"},
		{"amount beyond safe integer", "1000", "9007199254740993.123456789", "USD", nil, "8556839292003943467.28394955", "5", "500", "discount_applied"},
		{"highest threshold rather than highest discount", "1000", "10", "EUR", []Tier{
			{decimal.NewFromInt(1000), decimal.NewFromInt(1)},
			{decimal.NewFromInt(100), decimal.NewFromInt(20)},
			{decimal.NewFromInt(500), decimal.NewFromInt(5)},
		}, "9900", "1", "1000", "discount_applied"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			contract := evaluationContract(11)
			if tt.tiers != nil {
				contract.Tiers = tt.tiers
			}
			in := EvaluationInput{OfferID: 101, AsOf: evaluationDate(t, "2026-06-15T12:00:00Z"),
				Quantity: tt.quantity, UnitPrice: evaluationPrice(t, tt.price, tt.currency)}
			got, err := EvaluateContracts([]Contract{contract}, in)
			if err != nil {
				t.Fatal(err)
			}
			if !got.Total.Amount().Equal(decimal.RequireFromString(tt.wantTotal)) || string(got.Total.Currency()) != tt.currency {
				t.Fatalf("total = %s, want %s %s", got.Total, tt.wantTotal, tt.currency)
			}
			if got.DiscountPercent != tt.wantDiscount || got.Reason != tt.wantReason {
				t.Fatalf("discount/reason = %s/%s, want %s/%s", got.DiscountPercent, got.Reason, tt.wantDiscount, tt.wantReason)
			}
			if got.AppliedContractID == nil || *got.AppliedContractID != contract.ID || got.SelectedContractID != nil {
				t.Fatalf("contract provenance lost: %+v", got)
			}
			if got.OfferID != in.OfferID || got.AsOfDate != "2026-06-15" ||
				!decimal.RequireFromString(got.Quantity).Equal(decimal.RequireFromString(in.Quantity)) ||
				!got.UnitPrice.Amount().Equal(in.UnitPrice.Amount()) || got.UnitPrice.Currency() != in.UnitPrice.Currency() {
				t.Fatalf("calculation inputs changed: %+v", got)
			}
			if tt.wantThreshold == "" {
				if got.Tier != nil {
					t.Fatalf("unreached tier was applied: %+v", got.Tier)
				}
			} else if got.Tier == nil || !got.Tier.MinQuantity.Equal(decimal.RequireFromString(tt.wantThreshold)) ||
				!got.Tier.DiscountPercent.Equal(decimal.RequireFromString(tt.wantDiscount)) {
				t.Fatalf("tier = %+v, want threshold %s at %s percent", got.Tier, tt.wantThreshold, tt.wantDiscount)
			}
		})
	}
}

func TestEvaluateContractsInclusiveUTCDates(t *testing.T) {
	tests := []struct {
		name, asOf, wantDate string
		selected             bool
		wantTotal, reason    string
	}{
		{"before start", "2025-12-31T23:59:59Z", "2025-12-31", false, "10000", "no_active_contract"},
		{"inclusive first instant", "2026-01-01T00:00:00Z", "2026-01-01", false, "9500", "discount_applied"},
		{"inclusive final instant", "2026-12-31T23:59:59Z", "2026-12-31", false, "9500", "discount_applied"},
		{"after end", "2027-01-01T00:00:00Z", "2027-01-01", false, "10000", "no_active_contract"},
		{"selected before start", "2025-12-31T23:59:59Z", "2025-12-31", true, "10000", "contract_not_started"},
		{"selected after end", "2027-01-01T00:00:00Z", "2027-01-01", true, "10000", "contract_expired"},
		{"local first day still previous UTC day", "2026-01-01T00:30:00+01:00", "2025-12-31", true, "10000", "contract_not_started"},
		{"local next year still final UTC day", "2027-01-01T00:30:00+01:00", "2026-12-31", true, "9500", "discount_applied"},
		{"local final day already next UTC day", "2026-12-31T23:30:00-01:00", "2027-01-01", true, "10000", "contract_expired"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := EvaluationInput{OfferID: 101, AsOf: evaluationDate(t, tt.asOf), Quantity: "1000", UnitPrice: evaluationPrice(t, "10", "EUR")}
			if tt.selected {
				in.SelectedContractID = evaluationID(11)
			}
			got, err := EvaluateContracts([]Contract{evaluationContract(11)}, in)
			if err != nil {
				t.Fatal(err)
			}
			if got.AsOfDate != tt.wantDate || got.Reason != tt.reason || !got.Total.Amount().Equal(decimal.RequireFromString(tt.wantTotal)) {
				t.Fatalf("date/reason/total = %s/%s/%s, want %s/%s/%s", got.AsOfDate, got.Reason, got.Total, tt.wantDate, tt.reason, tt.wantTotal)
			}
			if (got.AppliedContractID != nil) != (tt.reason == "discount_applied") {
				t.Fatalf("incorrect applied contract: %+v", got)
			}
		})
	}
}

func TestEvaluateContractsSelection(t *testing.T) {
	active := evaluationContract(11)
	other := evaluationContract(12)
	other.Tiers = []Tier{{decimal.NewFromInt(500), decimal.NewFromInt(50)}}
	expired := evaluationContract(13)
	expired.StartsOn, expired.EndsOn = "2025-01-01", "2025-12-31"
	future := evaluationContract(14)
	future.StartsOn, future.EndsOn = "2027-01-01", "2027-12-31"
	foreign := evaluationContract(15)
	foreign.OfferIDs = []int64{999}
	empty := evaluationContract(16)
	empty.Tiers = []Tier{}
	tests := []struct {
		name                  string
		contracts             []Contract
		selected              *int64
		wantErr               error
		wantTotal, wantReason string
		wantApplied           int64
	}{
		{"no contract", nil, nil, nil, "10000", "no_active_contract", 0},
		{"foreign offer ignored", []Contract{foreign}, nil, nil, "10000", "no_active_contract", 0},
		{"single active chosen", []Contract{expired, active, future}, nil, nil, "9500", "discount_applied", 11},
		{"multiple active require choice", []Contract{active, other}, nil, ErrSelectionRequired, "", "", 0},
		{"explicit choice among active", []Contract{other, active}, evaluationID(11), nil, "9500", "discount_applied", 11},
		{"expired selection not substituted", []Contract{active, expired}, evaluationID(13), nil, "10000", "contract_expired", 0},
		{"future selection not substituted", []Contract{future, active}, evaluationID(14), nil, "10000", "contract_not_started", 0},
		{"missing selection", []Contract{active}, evaluationID(404), ErrInvalidInput, "", "", 0},
		{"selection covers another offer", []Contract{active, foreign}, evaluationID(15), ErrInvalidInput, "", "", 0},
		{"no tier reached", []Contract{empty}, nil, nil, "10000", "below_first_tier", 16},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := EvaluationInput{OfferID: 101, SelectedContractID: tt.selected, AsOf: evaluationDate(t, "2026-06-15T12:00:00Z"), Quantity: "1000", UnitPrice: evaluationPrice(t, "10", "EUR")}
			got, err := EvaluateContracts(tt.contracts, in)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("err = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got.Reason != tt.wantReason || !got.Total.Amount().Equal(decimal.RequireFromString(tt.wantTotal)) {
				t.Fatalf("reason/total = %s/%s, want %s/%s", got.Reason, got.Total, tt.wantReason, tt.wantTotal)
			}
			if tt.wantApplied == 0 {
				if got.AppliedContractID != nil || got.Tier != nil || got.DiscountPercent != "0" {
					t.Fatalf("unexpected discount: %+v", got)
				}
			} else if got.AppliedContractID == nil || *got.AppliedContractID != tt.wantApplied {
				t.Fatalf("wrong applied contract: %+v", got)
			}
			if tt.selected != nil && (got.SelectedContractID == nil || *got.SelectedContractID != *tt.selected) {
				t.Fatalf("explicit selection lost: %+v", got)
			}
		})
	}
}

func TestEvaluateContractsRejectsInvalidInputs(t *testing.T) {
	tests := []struct {
		name   string
		change func(*EvaluationInput, *Contract)
	}{
		{"empty quantity", func(in *EvaluationInput, _ *Contract) { in.Quantity = "" }},
		{"zero quantity", func(in *EvaluationInput, _ *Contract) { in.Quantity = "0" }},
		{"negative quantity", func(in *EvaluationInput, _ *Contract) { in.Quantity = "-1" }},
		{"nonnumeric quantity", func(in *EvaluationInput, _ *Contract) { in.Quantity = "abc" }},
		{"NaN quantity", func(in *EvaluationInput, _ *Contract) { in.Quantity = "NaN" }},
		{"infinite quantity", func(in *EvaluationInput, _ *Contract) { in.Quantity = "Infinity" }},
		{"missing price", func(in *EvaluationInput, _ *Contract) { in.UnitPrice = money.Money{} }},
		{"zero price", func(in *EvaluationInput, _ *Contract) { in.UnitPrice = evaluationPrice(t, "0", "EUR") }},
		{"negative price", func(in *EvaluationInput, _ *Contract) { in.UnitPrice = evaluationPrice(t, "-1", "EUR") }},
		{"missing offer", func(in *EvaluationInput, _ *Contract) { in.OfferID = 0 }},
		{"missing date", func(in *EvaluationInput, _ *Contract) { in.AsOf = time.Time{} }},
		{"date outside supported range", func(in *EvaluationInput, _ *Contract) { in.AsOf = time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC) }},
		{"invalid stored date", func(_ *EvaluationInput, c *Contract) { c.StartsOn = "2026-02-30" }},
		{"stored dates reversed", func(_ *EvaluationInput, c *Contract) { c.StartsOn, c.EndsOn = "2026-12-31", "2026-01-01" }},
		{"zero stored tier quantity", func(_ *EvaluationInput, c *Contract) { c.Tiers[0].MinQuantity = decimal.Zero }},
		{"negative stored tier quantity", func(_ *EvaluationInput, c *Contract) { c.Tiers[0].MinQuantity = decimal.NewFromInt(-1) }},
		{"negative stored discount", func(_ *EvaluationInput, c *Contract) { c.Tiers[0].DiscountPercent = decimal.NewFromInt(-1) }},
		{"stored discount above hundred", func(_ *EvaluationInput, c *Contract) {
			c.Tiers[0].DiscountPercent = decimal.RequireFromString("100.001")
		}},
		{"duplicate numeric thresholds", func(_ *EvaluationInput, c *Contract) {
			c.Tiers = append(c.Tiers, Tier{decimal.RequireFromString("500.00"), decimal.NewFromInt(10)})
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := EvaluationInput{OfferID: 101, AsOf: evaluationDate(t, "2026-06-15T12:00:00Z"), Quantity: "1000", UnitPrice: evaluationPrice(t, "10", "EUR")}
			c := evaluationContract(11)
			tt.change(&in, &c)
			if _, err := EvaluateContracts([]Contract{c}, in); !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("err = %v, want ErrInvalidInput", err)
			}
		})
	}
}
