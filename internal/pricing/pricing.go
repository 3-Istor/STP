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
}
