package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"bomstudio/internal/bom"
	"bomstudio/internal/money"
)

// fakeAnalysis : BOM qui sait aussi les analyses et les profils (routes du lot E montées dans ce cas). Les
// profils passent par la vraie validation du domaine (bom.NewVehicleProfile).
type fakeAnalysis struct {
	fakeBOMs
	profile *bom.VehicleProfile
}

func (f *fakeAnalysis) FXSensitivity(_ context.Context, id int64, shock decimal.Decimal) (bom.FXSensitivity, error) {
	if err := f.record("FXSensitivity %d %s", id, shock); err != nil {
		return bom.FXSensitivity{}, err
	}
	m := func(s string) money.Money { v, _ := money.Parse(s, "EUR"); return v }
	share, after := decimal.RequireFromString("11.8049"), decimal.RequireFromString("97.995")
	return bom.FXSensitivity{ShockPercent: shock, Total: m("1000"), Currencies: []bom.FXExposure{
		{Currency: "JPY", SharePercent: share, Exposed: m("118.049"), Impact: m("5.90245"), CostAfter: m("1005.90245"), BudgetPercentAfter: &after},
	}}, nil
}

func (f *fakeAnalysis) PriceDrivers(_ context.Context, id int64, limit int) (bom.PriceDrivers, error) {
	if err := f.record("PriceDrivers %d %d", id, limit); err != nil {
		return bom.PriceDrivers{}, err
	}
	a, _ := money.Parse("192.54", "EUR")
	return bom.PriceDrivers{Drivers: []bom.PriceDriver{{PartID: 7, Name: "Armature", Amplitude: &a}},
		Excluded: []bom.PriceDriver{{PartID: 9, Name: "Vis", Err: fmt.Errorf("offer 90 quotes: no window")}}}, nil
}

func (f *fakeAnalysis) Sourcing(_ context.Context, id int64) (bom.Sourcing, error) {
	if err := f.record("Sourcing %d", id); err != nil {
		return bom.Sourcing{}, err
	}
	return bom.Sourcing{Currency: "EUR", TopSharePercent: decimal.RequireFromString("44.333")}, nil
}

func (f *fakeAnalysis) PartAnomalies(_ context.Context, id int64, m bom.AnomalyMethod, th *decimal.Decimal) (bom.PartAnomalies, error) {
	if err := f.record("PartAnomalies %d %s %v", id, m, th); err != nil {
		return bom.PartAnomalies{}, err
	}
	return bom.PartAnomalies{Method: m, Currency: "EUR"}, nil
}

func (f *fakeAnalysis) VehicleProfile(_ context.Context, id int64) (*bom.VehicleProfile, error) {
	if err := f.record("VehicleProfile %d", id); err != nil {
		return nil, err
	}
	return f.profile, nil
}

func (f *fakeAnalysis) SetVehicleProfile(_ context.Context, id int64, preset string, w *bom.Weights) (bom.VehicleProfile, error) {
	if err := f.record("SetVehicleProfile %d %s", id, preset); err != nil {
		return bom.VehicleProfile{}, err
	}
	return bom.NewVehicleProfile(preset, w)
}

func TestAnalysisRoutes(t *testing.T) {
	tests := []struct {
		name     string
		method   string
		path     string
		body     string
		err      error
		status   int
		wantBody string
		wantCall string
	}{
		{"sensibilité au change, choc par défaut 5 %", "GET", "/api/v1/boms/3/analysis/fx-sensitivity", "", nil, 200,
			`"share_percent":"11.80","impact":{"amount":"5.90245","currency":"EUR"},"budget_percent_after":"98.00"`, "FXSensitivity 3 5"},
		{"choc explicite", "GET", "/api/v1/boms/3/analysis/fx-sensitivity?shock_percent=-10", "", nil, 200, `"currency":"JPY"`, "FXSensitivity 3 -10"},
		{"choc illisible", "GET", "/api/v1/boms/3/analysis/fx-sensitivity?shock_percent=abc", "", nil, 400, `"error"`, ""},
		{"choc refusé par le domaine", "GET", "/api/v1/boms/3/analysis/fx-sensitivity", "", fmt.Errorf("x: %w", bom.ErrInvalidInput), 400, `"error"`, "FXSensitivity 3 5"},
		{"BOM inconnue", "GET", "/api/v1/boms/3/analysis/fx-sensitivity", "", bom.ErrBOMNotFound, 404, `"error"`, "FXSensitivity 3 5"},
		{"coût incomplet", "GET", "/api/v1/boms/3/analysis/sourcing", "", fmt.Errorf("x: %w", bom.ErrMissingCost), 409, `"error"`, "Sourcing 3"},
		{"pièces influentes : classées puis exclues", "GET", "/api/v1/boms/3/analysis/price-drivers", "", nil, 200,
			`"part_id":7,"name":"Armature","amplitude":{"amount":"192.54","currency":"EUR"},"rank":1`, "PriceDrivers 3 10"},
		{"exclue avec sa raison", "GET", "/api/v1/boms/3/analysis/price-drivers?limit=5", "", nil, 200,
			`"part_id":9,"name":"Vis","amplitude":null,"rank":null,"error":"offer 90 quotes: no window"`, "PriceDrivers 3 5"},
		{"limite illisible", "GET", "/api/v1/boms/3/analysis/price-drivers?limit=x", "", nil, 400, `"error"`, ""},
		{"historique non câblé : 503", "GET", "/api/v1/boms/3/analysis/price-drivers", "", bom.ErrAnalysisUnavailable, 503, `"error"`, "PriceDrivers 3 10"},
		{"sourcing", "GET", "/api/v1/boms/3/analysis/sourcing", "", nil, 200, `"top5_share_percent":"44.33"`, "Sourcing 3"},
		{"anomalies : €/kg par défaut", "GET", "/api/v1/boms/3/analysis/anomalies", "", nil, 200, `"method":"per_kg"`, "PartAnomalies 3 per_kg <nil>"},
		{"anomalies : seuil illisible", "GET", "/api/v1/boms/3/analysis/anomalies?threshold=x", "", nil, 400, `"error"`, ""},
		{"profils proposés", "GET", "/api/v1/vehicle-profiles", "", nil, 200,
			`{"preset":"family","label":"Familiale","hint":"Équilibre coût, délai et CO₂.","weights":{"cost":40,"co2":25,"lead_time":20,"mass":5,"quality":10},"custom":false}`, ""},
		{"sans profil : null", "GET", "/api/v1/boms/3/profile", "", nil, 200, `{"preset":null,"weights":null}`, "VehicleProfile 3"},
		{"profil familiale", "PUT", "/api/v1/boms/3/profile", `{"preset":"family"}`, nil, 200,
			`{"preset":"family","weights":{"cost":40,"co2":25,"lead_time":20,"mass":5,"quality":10}}`, "SetVehicleProfile 3 family"},
		{"profil personnalisé", "PUT", "/api/v1/boms/3/profile",
			`{"preset":"custom","weights":{"cost":20,"co2":20,"lead_time":20,"mass":20,"quality":20}}`, nil, 200, `"mass":20`, "SetVehicleProfile 3 custom"},
		{"somme 99 : 400", "PUT", "/api/v1/boms/3/profile",
			`{"preset":"custom","weights":{"cost":19,"co2":20,"lead_time":20,"mass":20,"quality":20}}`, nil, 400, `sum to 99`, "SetVehicleProfile 3 custom"},
		{"poids décimal : 400", "PUT", "/api/v1/boms/3/profile",
			`{"preset":"custom","weights":{"cost":19.5,"co2":20,"lead_time":20,"mass":20,"quality":20.5}}`, nil, 400, `"error"`, ""},
		{"champ inconnu : 400", "PUT", "/api/v1/boms/3/profile", `{"preset":"family","weight":{}}`, nil, 400, `"error"`, ""},
		{"sans préréglage : 400", "PUT", "/api/v1/boms/3/profile", `{}`, nil, 400, `preset is required`, ""},
		{"BOM inconnue : 404", "PUT", "/api/v1/boms/3/profile", `{"preset":"family"}`, bom.ErrBOMNotFound, 404, `"error"`, "SetVehicleProfile 3 family"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeAnalysis{fakeBOMs: fakeBOMs{err: tt.err}}
			h := NewRouter(Services{BOMs: f, Snapshots: &fakeSnapshots{}, Now: func() time.Time { return testNow }})
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body)))
			if rec.Code != tt.status || !strings.Contains(rec.Body.String(), tt.wantBody) {
				t.Errorf("status %d body %s\nwant %d with %s", rec.Code, rec.Body.String(), tt.status, tt.wantBody)
			}
			if got := strings.Join(f.calls, ";"); got != tt.wantCall {
				t.Errorf("calls %q, want %q", got, tt.wantCall)
			}
			if rec.Code == 200 && !json.Valid(rec.Body.Bytes()) {
				t.Errorf("invalid JSON %s", rec.Body.String())
			}
		})
	}
}

// Sans service d'analyse (BOM qui ne le satisfait pas), les routes ne sont pas montées.
func TestAnalysisRoutesNeedTheService(t *testing.T) {
	h := NewRouter(Services{BOMs: &fakeBOMs{}, Snapshots: &fakeSnapshots{}, Now: func() time.Time { return testNow }})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/api/v1/boms/3/analysis/sourcing", nil))
	if rec.Code != 404 {
		t.Fatalf("status %d, want 404", rec.Code)
	}
}

// Un pourcentage n'est arrondi qu'une fois, en sortie, half-even (AGENTS §3, ADR 0004, v2-lot-g T5) : 99,94996 %
// ne devient pas « 100,0 % » par un double arrondi, une égalité va au pair.
func TestPercentOut(t *testing.T) {
	tests := []struct {
		in     string
		places int32
		want   string
	}{
		{"99.94996", 1, "99.9"},
		{"99.95", 1, "100.0"},
		{"11.25", 1, "11.2"},
		{"11.35", 1, "11.4"},
		{"0.125", 2, "0.12"},
		{"21.1249999", 2, "21.12"},
		{"-3.835", 2, "-3.84"},
		{"96", 1, "96.0"},
	}
	for _, tt := range tests {
		if got := percentOut(decimal.RequireFromString(tt.in), tt.places); got != tt.want {
			t.Errorf("percentOut(%s, %d) = %s, want %s", tt.in, tt.places, got, tt.want)
		}
	}
}
