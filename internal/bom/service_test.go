package bom

import (
	"errors"
	"os/exec"
	"strings"
	"testing"
	"time"

	"bomstudio/internal/catalog"
	"bomstudio/internal/money"
	"bomstudio/internal/pricing"
)

func newTestService(t *testing.T) *Service {
	t.Helper()
	price, err := money.Parse("100.00", "USD")
	if err != nil {
		t.Fatal(err)
	}
	cat, err := catalog.NewStub(
		[]catalog.Part{{ID: 1, Name: "Roue"}},
		[]catalog.Offer{{ID: 10, PartID: 1, Currency: "USD"}, {ID: 11, PartID: 1, Currency: "USD"}},
	)
	if err != nil {
		t.Fatal(err)
	}
	prices := pricing.NewStub([]pricing.Quote{
		{OfferID: 10, Price: price, Source: "sim", QuotedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)},
	}, nil) // l'offre 11 n'a aucune cotation
	return NewService(cat, prices)
}

func TestPricedOffer(t *testing.T) {
	svc := newTestService(t)
	tests := []struct {
		name      string
		offerID   int64
		wantPrice string
		wantErr   error
	}{
		{"offre cotée", 10, "100.00 USD", nil},
		{"SI l'API ne renvoie pas de prix ALORS erreur nommée, pas de prix à zéro", 11, "", pricing.ErrNoQuote},
		{"offre inconnue", 99, "", catalog.ErrOfferNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := svc.PricedOffer(t.Context(), tt.offerID)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if err == nil && got.Quote.Price.String() != tt.wantPrice {
				t.Fatalf("price = %s, want %s", got.Quote.Price, tt.wantPrice)
			}
		})
	}
}

// La cotation doit rester explicable et dans sa devise d'origine avant tout calcul de BOM.
func TestPricedOfferPreservesQuoteProvenance(t *testing.T) {
	tests := []struct {
		name     string
		amount   string
		currency string
		source   string
		quotedAt time.Time
	}{
		{"USD exact à forte précision", "9007199254740993.123456789", "USD", "sim/reference-a", time.Date(2026, 2, 3, 4, 5, 6, 123456789, time.UTC)},
		{"JPY fractionnaire non arrondi", "100.5", "JPY", "sim/reference-b", time.Date(2026, 7, 8, 9, 10, 11, 987654321, time.UTC)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			price, err := money.Parse(tt.amount, tt.currency)
			if err != nil {
				t.Fatal(err)
			}
			cat, err := catalog.NewStub(
				[]catalog.Part{{ID: 7, Name: "Pièce test"}},
				[]catalog.Offer{{ID: 23, PartID: 7, SupplierID: 41, Currency: money.Currency(tt.currency)}},
			)
			if err != nil {
				t.Fatal(err)
			}
			quote := pricing.Quote{OfferID: 23, Price: price, Source: tt.source, QuotedAt: tt.quotedAt}
			svc := NewService(cat, pricing.NewStub([]pricing.Quote{quote}, nil))
			got, err := svc.PricedOffer(t.Context(), 23)
			if err != nil {
				t.Fatal(err)
			}
			if got.Offer.ID != 23 || got.Offer.PartID != 7 || got.Offer.SupplierID != 41 || got.Quote.OfferID != 23 {
				t.Errorf("offre/cotation dissociées : offer = %+v, quoted offer = %d", got.Offer, got.Quote.OfferID)
			}
			if got.Offer.Currency != money.Currency(tt.currency) || got.Quote.Price.Currency() != money.Currency(tt.currency) {
				t.Errorf("devises = %s / %s, want %s", got.Offer.Currency, got.Quote.Price.Currency(), tt.currency)
			}
			if !got.Quote.Price.Amount().Equal(price.Amount()) {
				t.Errorf("prix = %s, want %s sans arrondi ni conversion", got.Quote.Price.Amount(), tt.amount)
			}
			if got.Quote.Source != tt.source || !got.Quote.QuotedAt.Equal(tt.quotedAt) {
				t.Errorf("provenance = %q à %s, want %q à %s", got.Quote.Source, got.Quote.QuotedAt, tt.source, tt.quotedAt)
			}
		})
	}
}

// Le domaine ne dépend ni du réseau ni de la base (AGENTS §2) : ni bom, ni ce qu'il importe
// (catalog, pricing, money…) ne doit tirer net/http ou pgx.
func TestDomainHasNoForbiddenDependency(t *testing.T) {
	out, err := exec.CommandContext(t.Context(), "go", "list", "-deps", ".").Output()
	if err != nil {
		t.Fatalf("go list -deps: %v", err)
	}
	forbidden := []string{"net/http", "github.com/jackc/pgx"}
	for _, dep := range strings.Fields(string(out)) {
		for _, f := range forbidden {
			if dep == f || strings.HasPrefix(dep, f+"/") {
				t.Errorf("internal/bom depends on %s", dep)
			}
		}
	}
}
