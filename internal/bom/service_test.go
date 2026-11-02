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
