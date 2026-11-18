package money

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/shopspring/decimal"
)

// FuzzParse explore les montants qu'un client peut envoyer (JSON /api/v1) et vérifie les invariants
// d'AGENTS §3 : jamais de panique, aller-retour JSON exact, arrondi idempotent et borné à une
// demi-unité mineure, aucune addition silencieuse entre devises. Lancé chaque nuit
// (`make verify LEVEL=nightly`, ADR 0039) ; en `go test` ordinaire, seul le corpus ci-dessous tourne.
func FuzzParse(f *testing.F) {
	for _, seed := range []struct{ amount, currency string }{
		{"92.00", "EUR"}, {"0.005", "EUR"}, {"-12.345", "USD"}, {"1500", "JPY"}, {"0.0005", "KWD"},
		{"1e3", "EUR"}, {"", "EUR"}, {"12,5", "EUR"}, {"NaN", "EUR"}, {"1.0", "XXX"}, {"9999999999999999999.999999", "GBP"},
	} {
		f.Add(seed.amount, seed.currency)
	}
	f.Fuzz(func(t *testing.T, amount, currency string) {
		if len(amount) > 64 {
			t.Skip() // un montant réel tient en 64 caractères ; au-delà on teste la mémoire, pas la règle
		}
		m, err := Parse(amount, currency)
		if err != nil {
			if !errors.Is(err, ErrInvalidAmount) && !errors.Is(err, ErrUnknownCurrency) {
				t.Fatalf("Parse(%q, %q) : erreur non nommée %v", amount, currency, err)
			}
			return
		}

		data, err := json.Marshal(m)
		if err != nil {
			t.Fatalf("Marshal(%v) : %v", m, err)
		}
		var back Money
		if err := json.Unmarshal(data, &back); err != nil {
			t.Fatalf("Unmarshal(%s) : %v", data, err)
		}
		if !back.Amount().Equal(m.Amount()) || back.Currency() != m.Currency() {
			t.Fatalf("aller-retour JSON inexact : %v → %s → %v", m, data, back)
		}

		r := m.Round()
		if !r.Round().Amount().Equal(r.Amount()) {
			t.Fatalf("Round non idempotent : %v → %v → %v", m, r, r.Round())
		}
		half := decimal.New(5, -m.Currency().MinorUnits()-1)
		if m.Amount().Sub(r.Amount()).Abs().GreaterThan(half) {
			t.Fatalf("Round(%v) = %v s'écarte de plus d'une demi-unité mineure", m, r)
		}

		other := Currency("EUR")
		if m.Currency() == other {
			other = "USD"
		}
		o, _ := New(decimal.NewFromInt(1), string(other))
		if _, err := m.Add(o); !errors.Is(err, ErrCurrencyMismatch) {
			t.Fatalf("Add %v + %v : attendu ErrCurrencyMismatch, obtenu %v", m, o, err)
		}
	})
}
