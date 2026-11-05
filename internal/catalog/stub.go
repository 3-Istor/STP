package catalog

import (
	"context"
	"fmt"
)

// Stub est un catalogue en mémoire, déterministe, pour les tests et le développement des autres domaines.
// Il n'est pas sûr en écriture concurrente : son contenu est fixé à la construction.
type Stub struct {
	parts  map[int64]Part
	offers map[int64]Offer
}

var _ Catalog = (*Stub)(nil)

// NewStub construit un catalogue en mémoire. Un identifiant en double est une erreur de jeu de données.
func NewStub(parts []Part, offers []Offer) (*Stub, error) {
	s := &Stub{parts: make(map[int64]Part, len(parts)), offers: make(map[int64]Offer, len(offers))}
	for _, p := range parts {
		if _, dup := s.parts[p.ID]; dup {
			return nil, fmt.Errorf("catalog stub: duplicate part %d", p.ID)
		}
		if (p.CO2KGPerUnit != nil && p.CO2KGPerUnit.IsNegative()) || (p.MassKG != nil && p.MassKG.IsNegative()) ||
			(p.ProductionDays != nil && *p.ProductionDays < 0) {
			return nil, fmt.Errorf("catalog stub: part %d: negative CO2, mass or production days", p.ID)
		}
		s.parts[p.ID] = p
	}
	for _, o := range offers {
		if _, dup := s.offers[o.ID]; dup {
			return nil, fmt.Errorf("catalog stub: duplicate offer %d", o.ID)
		}
		if _, ok := s.parts[o.PartID]; !ok {
			return nil, fmt.Errorf("catalog stub: offer %d: part %d: %w", o.ID, o.PartID, ErrPartNotFound)
		}
		s.offers[o.ID] = o
	}
	return s, nil
}

// Part implémente Catalog.
func (s *Stub) Part(_ context.Context, id int64) (Part, error) {
	p, ok := s.parts[id]
	if !ok {
		return Part{}, fmt.Errorf("part %d: %w", id, ErrPartNotFound)
	}
	return p, nil
}

// Offer implémente Catalog.
func (s *Stub) Offer(_ context.Context, id int64) (Offer, error) {
	o, ok := s.offers[id]
	if !ok {
		return Offer{}, fmt.Errorf("offer %d: %w", id, ErrOfferNotFound)
	}
	return o, nil
}
