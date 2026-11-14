package httpapi

import (
	"context"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"bomstudio/internal/money"
	"bomstudio/internal/pricing"
)

// memLogistics : tables en mémoire ; l'offre 404 n'existe pas.
type memLogistics struct {
	pricing.LogisticsStore
	calls []string
}

func (m *memLogistics) SetCustomsDuty(_ context.Context, d pricing.CustomsDuty) (pricing.CustomsDuty, error) {
	m.calls = append(m.calls, fmt.Sprintf("duty %s %s %s", d.Origin, d.Destination, d.Percent))
	return d, nil
}

func (m *memLogistics) SetFreightRate(_ context.Context, f pricing.FreightRate) (pricing.FreightRate, error) {
	m.calls = append(m.calls, fmt.Sprintf("freight %s %s %s", f.Origin, f.Destination, f.PerKg))
	return f, nil
}

func (m *memLogistics) SetOfferPackaging(_ context.Context, id int64, p money.Money) (money.Money, error) {
	if id == 404 {
		return money.Money{}, pricing.ErrUnknownOffer
	}
	m.calls = append(m.calls, fmt.Sprintf("packaging %d %s", id, p))
	return p, nil
}

func (m *memLogistics) ClearOfferPackaging(_ context.Context, id int64) error {
	m.calls = append(m.calls, fmt.Sprintf("clear packaging %d", id))
	return nil
}

func (m *memLogistics) DeleteCustomsDuty(_ context.Context, o, d string) error {
	m.calls = append(m.calls, fmt.Sprintf("delete duty %s %s", o, d))
	return nil
}

// TestLogisticsRoutes : tables du coût rendu (ADR 0025) — décodage, validation par le service, codes HTTP.
func TestLogisticsRoutes(t *testing.T) {
	tests := []struct {
		name, method, path, body string
		wantStatus               int
		wantCall                 string
	}{
		{"set duty", "PUT", "/api/v1/customs-duties/CN/FR", `{"duty_percent":"4.5"}`, 200, "duty CN FR 4.5"},
		{"duty with lowercase country", "PUT", "/api/v1/customs-duties/cn/FR", `{"duty_percent":"4.5"}`, 400, ""},
		{"duty without value", "PUT", "/api/v1/customs-duties/CN/FR", `{}`, 400, ""},
		{"negative duty", "PUT", "/api/v1/customs-duties/CN/FR", `{"duty_percent":"-1"}`, 400, ""},
		{"set freight", "PUT", "/api/v1/freight-rates/CN/FR", `{"per_kg":{"amount":"0.8","currency":"EUR"}}`, 200, "freight CN FR 0.80 EUR"},
		{"set packaging", "PUT", "/api/v1/offers/7/packaging", `{"packaging":{"amount":"1.25","currency":"USD"}}`, 200, "packaging 7 1.25 USD"},
		{"packaging of unknown offer", "PUT", "/api/v1/offers/404/packaging", `{"packaging":{"amount":"1","currency":"EUR"}}`, 404, ""},
		{"clear packaging", "DELETE", "/api/v1/offers/7/packaging", "", 204, "clear packaging 7"},
		{"delete duty", "DELETE", "/api/v1/customs-duties/CN/FR", "", 204, "delete duty CN FR"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &memLogistics{}
			h := NewRouter(Services{Logistics: pricing.NewLogisticsService(store)})
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body)))
			if rec.Code != tt.wantStatus {
				t.Fatalf("status %d, want %d, body %s", rec.Code, tt.wantStatus, rec.Body)
			}
			got := strings.Join(store.calls, ";")
			if got != tt.wantCall {
				t.Errorf("calls %q, want %q", got, tt.wantCall)
			}
		})
	}
}
