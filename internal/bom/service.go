package bom

import (
	"context"
	"fmt"

	"bomstudio/internal/catalog"
	"bomstudio/internal/pricing"
)

// Service porte les règles métier de la BOM. Il ne connaît le catalogue et les prix qu'à travers
// leurs interfaces : ni base, ni réseau, ni fournisseur de prix (AGENTS §2).
type Service struct {
	catalog catalog.Catalog
	pricing pricing.Pricing
}

// NewService câble le service sur un catalogue et une source de prix.
func NewService(c catalog.Catalog, p pricing.Pricing) *Service {
	return &Service{catalog: c, pricing: p}
}

// PricedOffer est une offre accompagnée de la cotation qui donne son prix.
type PricedOffer struct {
	Offer catalog.Offer
	Quote pricing.Quote
}

// PricedOffer renvoie l'offre et sa dernière cotation. Une offre sans cotation est une erreur
// (pricing.ErrNoQuote), jamais un prix à zéro.
func (s *Service) PricedOffer(ctx context.Context, offerID int64) (PricedOffer, error) {
	o, err := s.catalog.Offer(ctx, offerID)
	if err != nil {
		return PricedOffer{}, fmt.Errorf("priced offer %d: %w", offerID, err)
	}
	q, err := s.pricing.OfferPrice(ctx, offerID)
	if err != nil {
		return PricedOffer{}, fmt.Errorf("priced offer %d: %w", offerID, err)
	}
	return PricedOffer{Offer: o, Quote: q}, nil
}
