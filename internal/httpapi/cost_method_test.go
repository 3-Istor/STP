package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"bomstudio/internal/bom"
	"bomstudio/internal/money"
	"bomstudio/internal/pricing"
	"bomstudio/internal/snapshot"
	"bomstudio/internal/supplier"
)

// fakeSnapshotCosts : snapshot qui sait ventiler un nœud (route montée seulement dans ce cas).
type fakeSnapshotCosts struct {
	fakeSnapshots
	err error
}

func (f *fakeSnapshotCosts) CostBreakdown(_ context.Context, id, node int64) (bom.Breakdown, error) {
	if err := f.record("CostBreakdown %d %d", id, node); err != nil {
		return bom.Breakdown{}, err
	}
	if f.err != nil {
		return bom.Breakdown{}, f.err
	}
	total, _ := money.Parse("12.5", "EUR")
	return bom.Breakdown{NodeID: node, Currency: "EUR", Method: bom.DefaultCostMethod(), Total: total,
		Lines: []bom.CostLine{{Kind: bom.KindPrice, Amount: total}}}, nil
}

func TestSnapshotCostBreakdownRoute(t *testing.T) {
	tests := []struct {
		name   string
		path   string
		err    error
		status int
		body   string
	}{
		{"ventilation figée", "/api/v1/snapshots/4/nodes/9/cost-breakdown", nil, 200, `"kind":"price"`},
		{"id illisible", "/api/v1/snapshots/4/nodes/x/cost-breakdown", nil, 400, `"error"`},
		{"snapshot inconnu", "/api/v1/snapshots/4/nodes/9/cost-breakdown", snapshot.ErrSnapshotNotFound, 404, `"error"`},
		{"coût incomplet", "/api/v1/snapshots/4/nodes/9/cost-breakdown", fmt.Errorf("x: %w", bom.ErrNoTooling), 409, `"error"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewRouter(Services{BOMs: &fakeBOMs{}, Snapshots: &fakeSnapshotCosts{err: tt.err}, Now: func() time.Time { return testNow }})
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest("GET", tt.path, strings.NewReader("")))
			if rec.Code != tt.status || !strings.Contains(rec.Body.String(), tt.body) {
				t.Errorf("status %d body %s, want %d with %s", rec.Code, rec.Body.String(), tt.status, tt.body)
			}
		})
	}
}

// Les erreurs nommées du coût complet sont des conflits de données (409), une source non câblée est 503.
func TestCostErrorsStatus(t *testing.T) {
	tests := []struct {
		err    error
		status int
		code   string
	}{
		{bom.ErrNoBudgetRate, 409, "no_budget_rate"},
		{bom.ErrNoTooling, 409, "no_tooling"},
		{bom.ErrNoProgramVolume, 409, "no_program_volume"},
		{pricing.ErrNoCustomsDuty, 409, "no_customs_duty"},
		{supplier.ErrSelectionRequired, 409, "contract_selection_required"},
		{fmt.Errorf("node 1: %w", errors.Join(bom.ErrMissingCost, bom.ErrNoTooling)), 409, "missing_cost"},
		{snapshot.ErrIncompleteCost, 409, "missing_cost"},
		{bom.ErrCostSourcesUnavailable, 503, "missing_cost"},
	}
	for _, tt := range tests {
		t.Run(tt.err.Error(), func(t *testing.T) {
			if got := statusOf(tt.err); got != tt.status {
				t.Errorf("statusOf = %d, want %d", got, tt.status)
			}
			if got := costErrorCode(tt.err); got != tt.code {
				t.Errorf("costErrorCode = %s, want %s", got, tt.code)
			}
		})
	}
}
