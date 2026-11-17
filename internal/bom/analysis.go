package bom

import (
	"context"
	"time"

	"bomstudio/internal/pricing"
	"bomstudio/internal/supplier"
)

// Analyses d'un brouillon (vision V2 étapes Chiffrer et Sourcer) : sensibilité au change, pièces influentes,
// sourcing, anomalies €/kg. Toutes chiffrent avec le moteur des totaux (ComputeTotals, ADR 0031) sur l'entrée
// du brouillon (DraftCost) : mêmes prix, mêmes taux, même méthode, donc cohérentes au centime avec les totaux.

// QuoteHistory : historique immuable des cotations d'une offre (ADR 0011). pricingstore.Store la satisfait.
type QuoteHistory interface {
	ReadHistory(ctx context.Context, offerID int64, from, to time.Time) ([]pricing.HistoricalQuote, error)
}

// SupplierDirectory : les fournisseurs (nom, notes). *supplier.Service la satisfait.
type SupplierDirectory interface {
	ListSuppliers(ctx context.Context) ([]supplier.Supplier, error)
}

// WithAnalysisSources branche l'historique des cotations (pièces influentes) et l'annuaire des fournisseurs
// (sourcing).
func (s *Service) WithAnalysisSources(history QuoteHistory, suppliers SupplierDirectory) *Service {
	s.history, s.suppliers = history, suppliers
	return s
}
