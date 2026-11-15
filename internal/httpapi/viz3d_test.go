package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"bomstudio/internal/bom"
	"bomstudio/internal/viz3d"
)

type fakeAssemblies struct {
	err    error
	gotBOM int64
}

func (f *fakeAssemblies) Assembly(_ context.Context, bomID int64) (viz3d.Assembly, error) {
	f.gotBOM = bomID
	return viz3d.Assembly{BOMID: bomID, Source: viz3d.SourceGenerated, Zones: []viz3d.ZoneSummary{},
		Placements: []viz3d.Placement{{NodeID: 3, PartID: 10, Match: viz3d.MatchGenerated, Confidence: "0.20", Estimated: true,
			Notes: []string{}, Instances: []viz3d.Instance{{}}}},
		Missing: []viz3d.MissingModel{}}, f.err
}

func TestAssemblyRoute(t *testing.T) {
	tests := []struct {
		name       string
		path       string
		err        error
		wantStatus int
	}{
		{"QUAND la BOM existe ALORS 200 avec la scène", "/api/v1/boms/7/3d", nil, http.StatusOK},
		{"SI la BOM n'existe pas ALORS 404", "/api/v1/boms/7/3d", bom.ErrBOMNotFound, http.StatusNotFound},
		{"SI l'id n'est pas un entier ALORS 400", "/api/v1/boms/abc/3d", nil, http.StatusBadRequest},
		{"SI le domaine échoue ALORS 500 sans détail", "/api/v1/boms/7/3d", errors.New("boom"), http.StatusInternalServerError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeAssemblies{err: tt.err}
			h := NewRouter(Services{Assemblies: f})
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tt.path, nil))
			if rec.Code != tt.wantStatus {
				t.Fatalf("status %d, attendu %d : %s", rec.Code, tt.wantStatus, rec.Body)
			}
			if tt.wantStatus != http.StatusOK {
				var body map[string]string
				if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || body["error"] == "" {
					t.Fatalf("corps d'erreur attendu : %s", rec.Body)
				}
				if tt.wantStatus == http.StatusInternalServerError && body["error"] != "internal error" {
					t.Fatalf("le détail d'une erreur serveur ne sort pas : %q", body["error"])
				}
				return
			}
			var got struct {
				BOMID      int64 `json:"bom_id"`
				Source     string
				Placements []struct {
					NodeID    int64  `json:"node_id"`
					MatchType string `json:"match_type"`
					Estimated bool   `json:"estimated"`
				}
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			if f.gotBOM != 7 || got.BOMID != 7 || got.Source != "generated" || len(got.Placements) != 1 ||
				got.Placements[0].MatchType != "generated" || !got.Placements[0].Estimated {
				t.Fatalf("réponse inattendue : %s", rec.Body)
			}
		})
	}
}
