package catalog

import (
	"context"
	"errors"

	"bomstudio/internal/money"
)

var (
	// ErrPartNotFound : aucune pièce avec cet identifiant.
	ErrPartNotFound = errors.New("part not found")
	// ErrOfferNotFound : aucune offre avec cet identifiant.
	ErrOfferNotFound = errors.New("offer not found")
)

// Part est une pièce du catalogue, décrite indépendamment de qui la vend.
// Les attributs détaillés (masse, CO2, coûts…) arrivent avec la feature part-catalog.
type Part struct {
	ID       int64
	Name     string
	Category string
	Zone     string
}

// Offer est une pièce vendue par un fournisseur. PriceMin et PriceMax sont les bornes de référence
// des cotations, dans la devise de l'offre ; ce ne sont pas des prix d'achat (le prix est la dernière
// cotation, fournie par pricing).
type Offer struct {
	ID            int64
	PartID        int64
	SupplierID    int64
	Currency      money.Currency
	PriceMin      money.Money
	PriceMax      money.Money
	LeadTimeDays  int
	OriginCountry string // ISO 3166-1 alpha-2
}

// Catalog donne accès aux pièces et aux offres. Implémentations : Stub (en mémoire), puis la base.
type Catalog interface {
	// Part renvoie la pièce, ou ErrPartNotFound.
	Part(ctx context.Context, id int64) (Part, error)
	// Offer renvoie l'offre, ou ErrOfferNotFound.
	Offer(ctx context.Context, id int64) (Offer, error)
}
