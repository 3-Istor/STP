package pricing

import (
	"context"

	"github.com/shopspring/decimal"

	"bomstudio/internal/money"
)

// Coût complet (ADR 0031) : paramètres de coût d'une offre et taux budget annuels.

// OfferCostParams : origine, emballage et outillage d'une offre ; nil = non renseigné (jamais 0 implicite).
type OfferCostParams struct {
	OfferID   int64
	Origin    string
	Packaging *money.Money
	Tooling   *money.Money
}

// CostParamsStore lit en une fois les paramètres de coût d'un lot d'offres et les taux budget d'une année.
// L'implémentation Postgres est pricingstore.Logistics.
type CostParamsStore interface {
	// OfferCostParams : une entrée par offre existante de offerIDs (une offre inconnue est absente).
	OfferCostParams(ctx context.Context, offerIDs []int64) (map[int64]OfferCostParams, error)
	// BudgetRates : taux budget de l'année vers to, par devise d'origine (paires directes seulement).
	BudgetRates(ctx context.Context, year int, to money.Currency) (map[money.Currency]decimal.Decimal, error)
	ListCustomsDuties(ctx context.Context) ([]CustomsDuty, error)
	ListFreightRates(ctx context.Context) ([]FreightRate, error)
}
