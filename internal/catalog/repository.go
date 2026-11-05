package catalog

import (
	"context"
	"time"
)

// Filters combine les critères par AND ; seul le nom est une sous-chaîne insensible à la casse.
type Filters struct {
	Name       string
	Category   string
	Zone       string
	SupplierID *int64
}

// Repository appartient au domaine. Le sous-package catalogstore l'implémente sans être importé ici.
type Repository interface {
	Catalog
	ListParts(context.Context, Filters) ([]PartWithOffers, error)
	ListSupplierOffers(context.Context, int64) ([]Offer, error)
	Alternatives(context.Context, int64) ([]Part, error)
	WithinTx(context.Context, func(TxRepository) error) error
}

// TxRepository garantit l'atomicité et les verrous de la transaction (voir ADR 0009).
// Acquisition des verrous : pièce d'abord, puis offres par identifiant croissant.
type TxRepository interface {
	Catalog
	PartForUpdate(context.Context, int64) (Part, error)
	OfferForUpdate(context.Context, int64) (Offer, error)
	OffersForPartForUpdate(context.Context, int64) ([]Offer, error)
	InsertPart(context.Context, Part) (Part, error)
	ReplacePart(context.Context, Part) (Part, error)
	InsertOffer(context.Context, Offer) (Offer, error)
	ReplaceOffer(context.Context, Offer) (Offer, error)
	InsertAlternative(context.Context, int64, int64) error
	PartHasReferences(context.Context, int64) (bool, error)
	OfferHasReferences(context.Context, int64) (bool, error)
	OfferHasQuotes(context.Context, int64) (bool, error)
	ArchivePart(context.Context, int64, time.Time) (Part, error)
	ArchiveOffer(context.Context, int64, time.Time) (Offer, error)
	DeletePart(context.Context, int64) error
	DeleteOffer(context.Context, int64) error
}

// DeleteResult rend visible l'archivage à la place d'une suppression.
type DeleteResult struct {
	ID       int64 `json:"id"`
	Archived bool  `json:"archived"`
}
