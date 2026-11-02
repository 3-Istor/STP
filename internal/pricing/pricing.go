package pricing

import (
	"context"
	"errors"
	"time"

	"github.com/shopspring/decimal"

	"bomstudio/internal/money"
)

var (
	// ErrNoQuote : aucune cotation pour cette offre. Jamais remplacée par un prix à zéro (AGENTS §3).
	ErrNoQuote = errors.New("no price quote")
	// ErrNoRate : aucune cotation de taux pour cette paire. Jamais remplacée par un taux implicite.
	ErrNoRate = errors.New("no exchange rate")
	// ErrPriceSnapshotNotFound : aucun snapshot de prix avec cet identifiant.
	ErrPriceSnapshotNotFound = errors.New("price snapshot not found")
)

// Quote est une cotation de prix : le prix d'une offre, dans la devise de l'offre, avec sa provenance.
type Quote struct {
	OfferID  int64
	Price    money.Money
	Source   string
	QuotedAt time.Time // horodatage de la source (temps simulé pour le simulateur)
}

// Rate est une cotation de taux : 1 From = Rate To.
type Rate struct {
	From     money.Currency
	To       money.Currency
	Rate     decimal.Decimal
	Source   string
	QuotedAt time.Time
}

// Pricing fournit les prix et les taux en vigueur (dernière cotation). La source exacte (simulateur,
// base, snapshot de prix) est un détail d'implémentation ; chaque valeur porte sa source et sa date.
type Pricing interface {
	// OfferPrice renvoie la dernière cotation de l'offre, ou ErrNoQuote.
	OfferPrice(ctx context.Context, offerID int64) (Quote, error)
	// Rate renvoie la dernière cotation de la paire exacte from → to, ou ErrNoRate.
	// L'inversion d'un taux relève de multi-currency-costing, pas de cette interface.
	Rate(ctx context.Context, from, to money.Currency) (Rate, error)

	// FreezePrices crée un snapshot de prix immuable (ADR 0002) et renvoie son identifiant. Il fige la
	// dernière cotation de chaque offre de offerIDs et, pour chaque devise de currencies, le dernier taux
	// de chaque devise d'offre vers cette devise (paire exacte). at est l'instant du snapshot, fourni par
	// l'appelant (pas d'horloge dans le domaine). S'il manque un prix ou un taux, rien n'est créé : l'erreur
	// liste tout ce qui manque et enveloppe ErrNoQuote et/ou ErrNoRate.
	FreezePrices(ctx context.Context, offerIDs []int64, currencies []money.Currency, at time.Time) (int64, error)
	// SnapshotOfferPrice renvoie la cotation figée de l'offre dans le snapshot, ErrNoQuote si le snapshot
	// ne la couvre pas, ou ErrPriceSnapshotNotFound.
	SnapshotOfferPrice(ctx context.Context, snapshotID, offerID int64) (Quote, error)
	// SnapshotRate renvoie le taux figé de la paire exacte dans le snapshot, ErrNoRate s'il ne la couvre
	// pas, ou ErrPriceSnapshotNotFound.
	SnapshotRate(ctx context.Context, snapshotID int64, from, to money.Currency) (Rate, error)
}
