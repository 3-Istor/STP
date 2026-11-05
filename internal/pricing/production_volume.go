package pricing

import (
	"errors"
	"math"

	"bomstudio/internal/money"
)

var (
	ErrUnitCostUnavailable      = errors.New("unit cost unavailable")
	ErrZeroUnitCost             = errors.New("unit cost is zero")
	ErrProductionVolumeOverflow = errors.New("production volume exceeds BOM integer capacity")
	ErrNoAffordableVolume       = errors.New("no affordable car at reference unit cost")
)

// DiscountFact is copied evidence supplied by the BOM cost provider, not a second
// implementation of supplier contract selection or the BOM total calculation.
type DiscountFact struct {
	OfferID         int64  `json:"offer_id"`
	ContractID      *int64 `json:"contract_id,omitempty"`
	Quantity        string `json:"quantity"`
	MinQuantity     string `json:"min_quantity,omitempty"`
	DiscountPercent string `json:"discount_percent"`
	Reason          string `json:"reason"`
}
type CostBasis struct {
	BOMVersion      string            `json:"bom_version"`
	ReferenceVolume int64             `json:"reference_volume"`
	Context         PriceContext      `json:"context"`
	Prices          []HistoricalQuote `json:"prices"`
	Rates           []FXQuote         `json:"rates"`
	Discounts       []DiscountFact    `json:"discounts"`
}
type ProductionVolume struct {
	Volume           int64       `json:"volume"`
	Affordable       bool        `json:"affordable"`
	Budget           money.Money `json:"budget"`
	BudgetConversion Conversion  `json:"budget_conversion"`
	UnitCost         money.Exact `json:"-"`
	Basis            CostBasis   `json:"basis"`
}

// EstimateProduction evaluates once at the explicitly supplied reference volume.
// It never feeds the quotient back into contract discounts or boms.volume.
func EstimateProduction(budget money.Money, unitCost *money.Exact, basis CostBasis) (ProductionVolume, error) {
	if unitCost == nil || !unitCost.Valid() {
		return ProductionVolume{}, ErrUnitCostUnavailable
	}
	if unitCost.Sign() == 0 {
		return ProductionVolume{}, ErrZeroUnitCost
	}
	if unitCost.Sign() < 0 || budget.Amount().IsNegative() || basis.BOMVersion == "" || basis.ReferenceVolume <= 0 || basis.ReferenceVolume > math.MaxInt32 || basis.Context.AsOf.IsZero() {
		return ProductionVolume{}, ErrInvalidBudget
	}
	switch basis.Context.Mode {
	case "live", "snapshot", "snapshot_fallback":
	default:
		return ProductionVolume{}, ErrInvalidBudget
	}
	converted, err := ConvertMoney(budget, unitCost.Currency(), basis.Rates, basis.Context)
	if err != nil {
		return ProductionVolume{}, err
	}
	count, err := converted.Exact.FloorRatio(*unitCost)
	if err != nil {
		return ProductionVolume{}, err
	}
	if !count.IsInt64() || count.Int64() > math.MaxInt32 {
		return ProductionVolume{}, ErrProductionVolumeOverflow
	}
	volume := count.Int64()
	basis = copyCostBasis(basis)
	converted.Context = basis.Context
	return ProductionVolume{Volume: volume, Affordable: volume > 0, Budget: budget, BudgetConversion: converted, UnitCost: *unitCost, Basis: basis}, nil
}

// PublishableVolume is for the future publication adapter: zero is a valid estimate,
// but cannot become the strictly positive effective volume of a published BOM.
func (p ProductionVolume) PublishableVolume() (int32, error) {
	if p.Volume <= 0 {
		return 0, ErrNoAffordableVolume
	}
	if p.Volume > math.MaxInt32 {
		return 0, ErrProductionVolumeOverflow
	}
	return int32(p.Volume), nil
}
func copyCostBasis(b CostBasis) CostBasis {
	b.Prices = append([]HistoricalQuote{}, b.Prices...)
	b.Rates = append([]FXQuote{}, b.Rates...)
	b.Discounts = append([]DiscountFact{}, b.Discounts...)
	for i, d := range b.Discounts {
		if d.ContractID != nil {
			id := *d.ContractID
			b.Discounts[i].ContractID = &id
		}
	}
	if b.Context.Tick != nil {
		tick := *b.Context.Tick
		b.Context.Tick = &tick
	}
	if b.Context.SnapshotID != nil {
		id := *b.Context.SnapshotID
		b.Context.SnapshotID = &id
	}
	return b
}
