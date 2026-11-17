package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"bomstudio/internal/bom"
	"bomstudio/internal/money"
	"bomstudio/internal/pricing"
	"bomstudio/internal/snapshot"
	"bomstudio/internal/supplier"
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
		{"donnée de prix invalide dans le contrôle de budget", fmt.Errorf("budget rates of bom 1: %w", pricing.ErrInvalidHistoryData), "invalid_history_data"},
		{"budgets modifiés pendant l'écriture", bom.ErrBudgetsChanged, "budgets_changed"},
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

// Le champ cost_error_code de l'arbre et le code d'erreur HTTP disent la même chose pour la même sentinelle.
func TestCostErrorCodesAgreeWithErrorTable(t *testing.T) {
	for _, e := range []error{
		bom.ErrNoBudgetRate, bom.ErrNoTooling, bom.ErrNoProgramVolume, bom.ErrNoCostDate, bom.ErrNoCostFacts,
		pricing.ErrNoQuote, pricing.ErrNoRate, supplier.ErrSelectionRequired, pricing.ErrNoDestination,
		pricing.ErrNoCustomsDuty, pricing.ErrNoFreightRate, pricing.ErrNoPackaging, bom.ErrMissingCost,
	} {
		if a, b := costErrorCode(e), errorCode(e); a != b {
			t.Errorf("%v: cost_error_code %q, error code %q", e, a, b)
		}
	}
}

// Quand plusieurs sentinelles sont jointes, la première de la table l'emporte (comme l'ancien switch).
func TestErrorTableOrderDecides(t *testing.T) {
	err := errors.Join(bom.ErrInvalidInput, bom.ErrNodeNotFound)
	if got := errorCode(err); got != "node_not_found" || statusOf(err) != http.StatusNotFound {
		t.Fatalf("got %s %d, want node_not_found 404", got, statusOf(err))
	}
}

// Aucun corps d'erreur écrit à la main : tout passe par writeErrorBody, donc porte un code.
func TestNoHandWrittenErrorBody(t *testing.T) {
	files, _ := filepath.Glob("*.go")
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") || f == "errorcode.go" {
			continue
		}
		b, err := os.ReadFile(filepath.Clean(f))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(b), `map[string]string{"error"`) {
			t.Errorf("%s écrit un corps d'erreur sans code : utilisez writeErrorBody", f)
		}
	}
}

// Une donnée de prix invalide dans le contrôle de budget est un conflit (409), pas une panne serveur (500).
func TestInvalidHistoryDataIsAConflict(t *testing.T) {
	err := fmt.Errorf("budget rates of bom 1: %w", pricing.ErrInvalidHistoryData)
	if got := statusOf(err); got != http.StatusConflict {
		t.Fatalf("status %d, want 409", got)
	}
}
