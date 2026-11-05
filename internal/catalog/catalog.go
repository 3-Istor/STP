package catalog

import (
	"context"
	"errors"

	"github.com/shopspring/decimal"

	"bomstudio/internal/money"
)

var (
	// ErrPartNotFound : aucune pièce avec cet identifiant.
	ErrPartNotFound = errors.New("part not found")
	// ErrOfferNotFound : aucune offre avec cet identifiant.
	ErrOfferNotFound = errors.New("offer not found")
)

// Part est une pièce du catalogue, décrite indépendamment de qui la vend.
// CO2Kg, MassKg et ProductionDays servent aux totaux de la BOM (ADR 0007) et sont copiés dans
// les snapshots de BOM (ADR 0006). Leur stockage en base arrive avec part-catalog (NLPF-67).
type Part struct {
	ID       int64
	Name     string
	Category string
	Zone     string
	// CO2Kg : émissions d'une unité de la pièce, en kg équivalent CO2 (décimal, jamais float).
	CO2Kg decimal.Decimal
	// MassKg : masse d'une unité, en kg.
	MassKg decimal.Decimal
	// ProductionDays : jours de production / préparation de la pièce, ajoutés au délai de livraison
	// de l'offre pour obtenir le délai d'une feuille de BOM (ADR 0007).
	ProductionDays int
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
