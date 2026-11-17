package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"bomstudio/internal/bom"
	"bomstudio/internal/money"
	"bomstudio/internal/pricing"
	"bomstudio/internal/snapshot"
)

func TestErrorCodeIsStableAndKeepsTheMessage(t *testing.T) {
	tests := []struct {
		name string
		err  error
		code string
	}{
		{"taux budget absent", bom.ErrNoBudgetRate, "no_budget_rate"},
		{"cotation absente, enveloppée", fmt.Errorf("part 3: %w", pricing.ErrNoQuote), "no_quote"},
		{"taux absent", pricing.ErrNoRate, "no_rate"},
		{"devises différentes", money.ErrCurrencyMismatch, "currency_mismatch"},
		{"coût incomplet à la publication", snapshot.ErrIncompleteCost, "incomplete_cost"},
		{"requête illisible", errBadRequest, "bad_request"},
		{"erreur inconnue", errors.New("boom"), "internal"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			writeError(w, httptest.NewRequest(http.MethodGet, "/api/v1/x", nil), tt.err)
			var got map[string]string
			if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			if got["code"] != tt.code || got["error"] == "" {
				t.Fatalf("body %v, want code %q and a non-empty error", got, tt.code)
			}
		})
	}
}

// Les codes sont uniques et non vides : l'interface s'appuie dessus pour choisir sa phrase.
func TestErrorTableCodesAreUnique(t *testing.T) {
	seen := map[string]bool{}
	for _, e := range errorTable {
		if e.code == "" || e.code == "internal" || seen[e.code] {
			t.Errorf("code %q of %v is empty, reserved or duplicated", e.code, e.err)
		}
		seen[e.code] = true
	}
}
