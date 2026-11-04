package catalog

import "bomstudio/internal/money"

// PartInput impose les décimaux textuels et la présence du délai, y compris lorsqu'il vaut zéro.
type PartInput struct {
	Name                 string      `json:"name"`
	Category             string      `json:"category"`
	Zone                 string      `json:"zone"`
	MassKG               string      `json:"mass_kg"`
	CO2KGPerUnit         string      `json:"co2_kg_per_unit"`
	RecycledPercent      string      `json:"recycled_percent"`
	RecyclabilityPercent string      `json:"recyclability_percent"`
	ManufacturingCost    money.Money `json:"manufacturing_cost"`
	ProductionDays       *int        `json:"production_days"`
	LaborHours           string      `json:"labor_hours"`
	HourlyRate           money.Money `json:"hourly_rate"`
}

// PartPatch ne modifie que les champs présents. L'adapter HTTP refuse un null explicite.
type PartPatch struct {
	Name                 *string      `json:"name"`
	Category             *string      `json:"category"`
	Zone                 *string      `json:"zone"`
	MassKG               *string      `json:"mass_kg"`
	CO2KGPerUnit         *string      `json:"co2_kg_per_unit"`
	RecycledPercent      *string      `json:"recycled_percent"`
	RecyclabilityPercent *string      `json:"recyclability_percent"`
	ManufacturingCost    *money.Money `json:"manufacturing_cost"`
	ProductionDays       *int         `json:"production_days"`
	LaborHours           *string      `json:"labor_hours"`
	HourlyRate           *money.Money `json:"hourly_rate"`
}

// OfferInput contient les bornes de référence, jamais un prix d'achat courant.
type OfferInput struct {
	SupplierID    int64  `json:"supplier_id"`
	Currency      string `json:"currency"`
	PriceMin      string `json:"price_min"`
	PriceMax      string `json:"price_max"`
	LeadTimeDays  *int   `json:"lead_time_days"`
	QualityScore  string `json:"quality_score"`
	OriginCountry string `json:"origin_country"`
}

// OfferPatch ne change jamais la pièce d'origine de l'offre.
type OfferPatch struct {
	SupplierID    *int64  `json:"supplier_id"`
	Currency      *string `json:"currency"`
	PriceMin      *string `json:"price_min"`
	PriceMax      *string `json:"price_max"`
	LeadTimeDays  *int    `json:"lead_time_days"`
	QualityScore  *string `json:"quality_score"`
	OriginCountry *string `json:"origin_country"`
}
