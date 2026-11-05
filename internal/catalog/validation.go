package catalog

import (
	"github.com/shopspring/decimal"

	"bomstudio/internal/money"
)

func partFromInput(in PartInput) (Part, error) {
	p := Part{}
	patch := PartPatch{
		Name: &in.Name, Category: &in.Category, Zone: &in.Zone, MassKG: &in.MassKG,
		CO2KGPerUnit: &in.CO2KGPerUnit, RecycledPercent: &in.RecycledPercent,
		RecyclabilityPercent: &in.RecyclabilityPercent, ManufacturingCost: &in.ManufacturingCost,
		ProductionDays: in.ProductionDays, LaborHours: &in.LaborHours, HourlyRate: &in.HourlyRate,
	}
	if in.ProductionDays == nil {
		return p, invalid("production_days is required")
	}
	return patchPart(p, patch)
}

func patchPart(p Part, patch PartPatch) (Part, error) {
	texts := []struct {
		name   string
		input  *string
		target *string
	}{
		{"name", patch.Name, &p.Name}, {"category", patch.Category, &p.Category}, {"zone", patch.Zone, &p.Zone},
	}
	for _, field := range texts {
		if field.input != nil {
			if err := validText(field.name, *field.input); err != nil {
				return Part{}, err
			}
			*field.target = *field.input
		}
	}
	hundred := decimal.NewFromInt(100)
	decimals := []struct {
		name    string
		input   *string
		target  **decimal.Decimal
		maximum *decimal.Decimal
	}{
		{"mass_kg", patch.MassKG, &p.MassKG, nil},
		{"co2_kg_per_unit", patch.CO2KGPerUnit, &p.CO2KGPerUnit, nil},
		{"recycled_percent", patch.RecycledPercent, &p.RecycledPercent, &hundred},
		{"recyclability_percent", patch.RecyclabilityPercent, &p.RecyclabilityPercent, &hundred},
		{"labor_hours", patch.LaborHours, &p.LaborHours, nil},
	}
	for _, field := range decimals {
		if field.input != nil {
			d, err := parseNonnegative(field.name, *field.input, field.maximum)
			if err != nil {
				return Part{}, err
			}
			*field.target = d
		}
	}
	moneys := []struct {
		name   string
		input  *money.Money
		target **money.Money
	}{
		{"manufacturing_cost", patch.ManufacturingCost, &p.ManufacturingCost},
		{"hourly_rate", patch.HourlyRate, &p.HourlyRate},
	}
	for _, field := range moneys {
		if field.input != nil {
			if err := validMoney(field.name, *field.input); err != nil {
				return Part{}, err
			}
			copyValue := *field.input
			*field.target = &copyValue
		}
	}
	if patch.ProductionDays != nil {
		if *patch.ProductionDays < 0 {
			return Part{}, invalid("production_days must be nonnegative")
		}
		copyDays := *patch.ProductionDays
		p.ProductionDays = &copyDays
	}
	return p, nil
}

func offerFromInput(partID int64, in OfferInput) (Offer, error) {
	if in.LeadTimeDays == nil {
		return Offer{}, invalid("lead_time_days is required")
	}
	return patchOffer(Offer{PartID: partID}, OfferPatch{
		SupplierID: &in.SupplierID, Currency: &in.Currency, PriceMin: &in.PriceMin, PriceMax: &in.PriceMax,
		LeadTimeDays: in.LeadTimeDays, QualityScore: &in.QualityScore, OriginCountry: &in.OriginCountry,
	})
}

func patchOffer(o Offer, patch OfferPatch) (Offer, error) {
	if patch.SupplierID != nil {
		if *patch.SupplierID <= 0 {
			return Offer{}, invalid("supplier_id must be positive")
		}
		o.SupplierID = *patch.SupplierID
	}
	if patch.Currency != nil {
		c, err := money.ParseCurrency(*patch.Currency)
		if err != nil {
			return Offer{}, invalid("currency must be a supported ISO 4217 code")
		}
		o.Currency = c
	}
	min, max := o.PriceMin.Amount().String(), o.PriceMax.Amount().String()
	if patch.PriceMin != nil {
		min = *patch.PriceMin
	}
	if patch.PriceMax != nil {
		max = *patch.PriceMax
	}
	var err error
	o.PriceMin, err = money.Parse(min, string(o.Currency))
	if err != nil || !o.PriceMin.Amount().IsPositive() {
		return Offer{}, invalid("price_min must be a positive decimal string in the offer currency")
	}
	o.PriceMax, err = money.Parse(max, string(o.Currency))
	if err != nil || !o.PriceMax.Amount().IsPositive() {
		return Offer{}, invalid("price_max must be a positive decimal string in the offer currency")
	}
	if o.PriceMin.Amount().GreaterThan(o.PriceMax.Amount()) {
		return Offer{}, invalid("price_min must not exceed price_max")
	}
	if patch.LeadTimeDays != nil {
		if *patch.LeadTimeDays < 0 {
			return Offer{}, invalid("lead_time_days must be nonnegative")
		}
		o.LeadTimeDays = *patch.LeadTimeDays
	}
	if patch.OriginCountry != nil {
		if !validCountry(*patch.OriginCountry) {
			return Offer{}, invalid("origin_country must be an ISO 3166-1 alpha-2 code")
		}
		o.OriginCountry = *patch.OriginCountry
	}
	if patch.QualityScore != nil {
		five := decimal.NewFromInt(5)
		o.QualityScore, err = parseNonnegative("quality_score", *patch.QualityScore, &five)
		if err != nil {
			return Offer{}, err
		}
	}
	return o, nil
}
