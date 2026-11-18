package httpapi

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"bomstudio/internal/bom"
)

type fakeConfigurations struct {
	fakeBOMs
	request bom.ConfigurationRequest
	result  bom.ConfigurationComparison
}

func (f *fakeConfigurations) SimulateConfigurations(_ context.Context, id int64, r bom.ConfigurationRequest) (bom.ConfigurationComparison, error) {
	f.request = r
	if err := f.record("SimulateConfigurations %d", id); err != nil {
		return bom.ConfigurationComparison{}, err
	}
	return f.result, nil
}
func TestConfigurationAPI(t *testing.T) {
	const body = `{"root_id":1,"guards":[{"id":1,"parent_id":null,"part_id":null}],"scenarios":[{"key":"hybrid","excluded_roots":[]},{"key":"thermal","excluded_roots":[]},{"key":"electric","excluded_roots":[]}]}`
	for _, tt := range []struct {
		name, body  string
		domainError error
		status      int
	}{
		{"success", body, nil, 200}, {"bad json", "{", nil, 400}, {"unknown field", `{"unexpected":true}`, nil, 400},
		{"invalid definition", body, bom.ErrInvalidInput, 422}, {"absent bom", body, bom.ErrBOMNotFound, 404},
	} {
		t.Run(tt.name, func(t *testing.T) {
			tree := sampleTree(t) // named missing price survives serialization, never a zero total
			f := &fakeConfigurations{fakeBOMs: fakeBOMs{err: tt.domainError}, result: bom.ConfigurationComparison{RootID: 1, Scenarios: []bom.ConfigurationResult{{Key: "hybrid", Tree: tree, Totals: tree.Totals, ActiveIDs: []int64{1, 2, 3, 4, 5}, ExcludedIDs: []int64{}}}}}
			h := NewRouter(Services{BOMs: f, Snapshots: &fakeSnapshots{}})
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest("POST", "/api/v1/boms/7/configuration-simulation", strings.NewReader(tt.body)))
			if rec.Code != tt.status {
				t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
			}
			if tt.status == 200 {
				var out struct {
					RootID    int64 `json:"root_id"`
					Scenarios []struct {
						Key  string `json:"key"`
						Tree struct {
							Nodes []struct {
								Totals struct {
									Cost  any     `json:"cost"`
									Error *string `json:"cost_error_code"`
								} `json:"totals"`
							} `json:"nodes"`
						} `json:"tree"`
						Delta any `json:"delta"`
					} `json:"scenarios"`
				}
				if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
					t.Fatal(err)
				}
				if out.RootID != 1 || len(out.Scenarios) != 1 || out.Scenarios[0].Key != "hybrid" || len(out.Scenarios[0].Tree.Nodes) != 5 {
					t.Fatal("invalid response", rec.Body.String())
				}
				root := out.Scenarios[0].Tree.Nodes[0].Totals
				if root.Cost != nil || root.Error == nil || *root.Error != "missing_cost" || out.Scenarios[0].Delta != nil {
					t.Fatal("incomplete price hidden")
				}
				if f.request.RootID != 1 || len(f.request.Guards) != 1 || len(f.request.Scenarios) != 3 {
					t.Fatal("request not decoded")
				}
			} else {
				var e map[string]string
				if err := json.Unmarshal(rec.Body.Bytes(), &e); err != nil || e["error"] == "" || e["code"] == "" {
					t.Fatal("missing JSON error or stable error code")
				}
			}
		})
	}
}
func TestConfigurationAPIBounds(t *testing.T) {
	f := &fakeConfigurations{}
	h := NewRouter(Services{BOMs: f, Snapshots: &fakeSnapshots{}})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("POST", "/api/v1/boms/7/configuration-simulation", strings.NewReader(`{"padding":"`+strings.Repeat("x", maxBodyBytes)+`"}`)))
	if rec.Code < 400 || rec.Code >= 500 || len(f.calls) != 0 {
		t.Fatal("oversized request reached service", rec.Code)
	}
}
