// Package money porte le type Money : un montant décimal et sa devise ISO 4217.
//
// Règles (AGENTS §3) : jamais de float ; aucune opération n'arrondit ; additionner deux devises
// différentes est une erreur ; seul Round arrondit, aux unités mineures de la devise, en half-even
// (voir ADR 0004). Le package ne dépend d'aucun domaine.
package money

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/shopspring/decimal"
)

var (
	// ErrCurrencyMismatch : opération entre deux montants de devises différentes, sans conversion.
	ErrCurrencyMismatch = errors.New("currency mismatch")
	// ErrUnknownCurrency : code absent de la table des devises prises en charge.
	ErrUnknownCurrency = errors.New("unknown currency")
	// ErrInvalidAmount : montant illisible ou absent.
	ErrInvalidAmount = errors.New("invalid amount")
)

// Currency est un code devise ISO 4217 (trois lettres majuscules) pris en charge.
type Currency string

// minorUnits donne, pour chaque devise prise en charge, son nombre d'unités mineures (ISO 4217).
// Ajouter une devise = ajouter une ligne ici, avec son test.
var minorUnits = map[Currency]int32{
	"EUR": 2, "USD": 2, "GBP": 2, "CHF": 2, "CNY": 2, "CAD": 2, "AUD": 2,
	"SEK": 2, "NOK": 2, "DKK": 2, "PLN": 2, "CZK": 2, "HUF": 2, "RON": 2,
	"INR": 2, "MXN": 2, "BRL": 2, "TRY": 2, "ZAR": 2, "MAD": 2,
	"JPY": 0, "KRW": 0,
	"KWD": 3, "BHD": 3, "TND": 3,
}

// ParseCurrency valide un code ISO 4217 contre la table des devises prises en charge.
func ParseCurrency(code string) (Currency, error) {
	c := Currency(code)
	if _, ok := minorUnits[c]; !ok {
		return "", fmt.Errorf("parse currency %q: %w", code, ErrUnknownCurrency)
	}
	return c, nil
}

// MinorUnits renvoie le nombre de décimales de la devise (2 pour EUR, 0 pour JPY…).
func (c Currency) MinorUnits() int32 {
	return minorUnits[c]
}

// Money est un montant exact dans une devise. La valeur zéro (sans devise) n'est pas un montant valide.
type Money struct {
	amount   decimal.Decimal
	currency Currency
}

// New construit un montant ; la devise doit être prise en charge.
func New(amount decimal.Decimal, currency string) (Money, error) {
	c, err := ParseCurrency(currency)
	if err != nil {
		return Money{}, err
	}
	return Money{amount: amount, currency: c}, nil
}

// Parse construit un montant depuis sa forme texte (ex. "92.00", "EUR").
func Parse(amount, currency string) (Money, error) {
	d, err := decimal.NewFromString(amount)
	if err != nil {
		return Money{}, fmt.Errorf("parse amount %q: %w", amount, ErrInvalidAmount)
	}
	return New(d, currency)
}

// Amount renvoie la valeur exacte, non arrondie.
func (m Money) Amount() decimal.Decimal { return m.amount }

// Currency renvoie la devise.
func (m Money) Currency() Currency { return m.currency }

// Add additionne deux montants de même devise, sans arrondi.
func (m Money) Add(o Money) (Money, error) {
	if m.currency != o.currency {
		return Money{}, fmt.Errorf("add %s to %s: %w", o.currency, m.currency, ErrCurrencyMismatch)
	}
	return Money{amount: m.amount.Add(o.amount), currency: m.currency}, nil
}

// Mul multiplie le montant par une quantité, sans arrondi.
func (m Money) Mul(qty decimal.Decimal) Money {
	return Money{amount: m.amount.Mul(qty), currency: m.currency}
}

// Round arrondit aux unités mineures de la devise, en half-even (ADR 0004).
// À n'appeler qu'en sortie (affichage, publication) : c'est le seul endroit qui arrondit.
func (m Money) Round() Money {
	return Money{amount: m.amount.RoundBank(m.currency.MinorUnits()), currency: m.currency}
}

// String affiche le montant exact suivi de sa devise, ex. "92.00 EUR".
func (m Money) String() string {
	return m.amountString() + " " + string(m.currency)
}

// amountString écrit le montant avec au moins les décimales de la devise, sans jamais en retirer :
// 92 EUR → "92.00", 0.005 EUR → "0.005". Aucun arrondi.
func (m Money) amountString() string {
	places := m.currency.MinorUnits()
	if exp := -m.amount.Exponent(); exp > places {
		places = exp
	}
	return m.amount.StringFixed(places)
}

type moneyJSON struct {
	Amount   string `json:"amount"`
	Currency string `json:"currency"`
}

// MarshalJSON écrit {"amount":"92.00","currency":"EUR"} : le montant est une chaîne, jamais un nombre.
func (m Money) MarshalJSON() ([]byte, error) {
	return json.Marshal(moneyJSON{Amount: m.amountString(), Currency: string(m.currency)})
}

// UnmarshalJSON lit {"amount":"92.00","currency":"EUR"} ; un montant numérique JSON est refusé.
func (m *Money) UnmarshalJSON(data []byte) error {
	var raw moneyJSON
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("decode money: %w", err)
	}
	parsed, err := Parse(raw.Amount, raw.Currency)
	if err != nil {
		return fmt.Errorf("decode money: %w", err)
	}
	*m = parsed
	return nil
}
