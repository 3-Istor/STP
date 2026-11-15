package httpapi

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/shopspring/decimal"

	"bomstudio/internal/bom"
	"bomstudio/internal/catalog"
	"bomstudio/internal/viz3d"
)

// fakeAssemblies : le domaine viz3d vu de l'API, piloté par des erreurs à renvoyer et ce qu'il a reçu.
type fakeAssemblies struct {
	err    error
	gotBOM int64
	// gotUpload : ce que l'API a transmis au domaine lors d'un téléversement.
	gotUpload *viz3d.Upload
	gotData   []byte
	gotAssign *viz3d.Association
	file      []byte
}

func (f *fakeAssemblies) Assembly(_ context.Context, bomID int64) (viz3d.Assembly, error) {
	f.gotBOM = bomID
	return viz3d.Assembly{BOMID: bomID, Source: viz3d.SourceGenerated, Zones: []viz3d.ZoneSummary{},
		Placements: []viz3d.Placement{{NodeID: 3, PartID: 10, Match: viz3d.MatchGenerated, Confidence: "0.20", Estimated: true,
			Notes: []string{}, Instances: []viz3d.Instance{{}}}},
		Missing: []viz3d.MissingModel{}}, f.err
}

func (f *fakeAssemblies) GenerationStatus(_ context.Context, bomID int64) (viz3d.GenerationView, error) {
	f.gotBOM = bomID
	return viz3d.GenerationView{BOMID: bomID, Status: viz3d.StatusStale, Done: 3, Total: 3}, f.err
}

func (f *fakeAssemblies) StartGeneration(_ context.Context, bomID int64) (viz3d.GenerationView, error) {
	f.gotBOM = bomID
	return viz3d.GenerationView{BOMID: bomID, Status: viz3d.StatusQueued, Total: 3}, f.err
}

func (f *fakeAssemblies) PartModel(_ context.Context, partID int64) (viz3d.PartModelView, error) {
	return viz3d.PartModelView{PartID: partID}, f.err
}

func (f *fakeAssemblies) AssignModel(_ context.Context, partID int64, a viz3d.Association) (viz3d.PartModelView, error) {
	f.gotAssign = &a
	return viz3d.PartModelView{PartID: partID, Match: a.Match, Model: &viz3d.ModelView{ID: a.ModelID}}, f.err
}

func (f *fakeAssemblies) UploadModel(_ context.Context, partID int64, u viz3d.Upload) (viz3d.PartModelView, error) {
	data, err := io.ReadAll(u.Data)
	if err != nil {
		return viz3d.PartModelView{}, err
	}
	f.gotUpload, f.gotData = &u, data
	return viz3d.PartModelView{PartID: partID, Match: u.Match, Model: &viz3d.ModelView{ID: 7, Format: viz3d.FormatGLB, URL: ModelFileURL(7)}}, f.err
}

func (f *fakeAssemblies) ModelFile(_ context.Context, modelID int64) (viz3d.Model, io.ReadCloser, int64, error) {
	if f.err != nil {
		return viz3d.Model{}, nil, 0, f.err
	}
	return viz3d.Model{ID: modelID, Format: viz3d.FormatGLB}, io.NopCloser(bytes.NewReader(f.file)), int64(len(f.file)), nil
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

// Génération et statut (tranche 2) : 202 au lancement, 409 si déjà en cours, 503 sans stockage.
func TestGenerationRoutes(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		path       string
		err        error
		wantStatus int
		wantState  string
	}{
		{"POST generate → 202 queued", http.MethodPost, "/api/v1/boms/7/3d/generate", nil, http.StatusAccepted, "queued"},
		{"POST generate déjà en cours → 409", http.MethodPost, "/api/v1/boms/7/3d/generate", viz3d.ErrGenerationRunning, http.StatusConflict, ""},
		{"POST generate BOM inconnue → 404", http.MethodPost, "/api/v1/boms/7/3d/generate", bom.ErrBOMNotFound, http.StatusNotFound, ""},
		{"POST generate sans stockage → 503", http.MethodPost, "/api/v1/boms/7/3d/generate", viz3d.ErrModelsUnavailable, http.StatusServiceUnavailable, ""},
		{"GET status → 200 avec le statut (stale)", http.MethodGet, "/api/v1/boms/7/3d/status", nil, http.StatusOK, "stale"},
		{"GET status BOM inconnue → 404", http.MethodGet, "/api/v1/boms/7/3d/status", bom.ErrBOMNotFound, http.StatusNotFound, ""},
		{"GET status id invalide → 400", http.MethodGet, "/api/v1/boms/x/3d/status", nil, http.StatusBadRequest, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeAssemblies{err: tt.err}
			rec := httptest.NewRecorder()
			NewRouter(Services{Assemblies: f}).ServeHTTP(rec, httptest.NewRequest(tt.method, tt.path, nil))
			if rec.Code != tt.wantStatus {
				t.Fatalf("status %d, attendu %d : %s", rec.Code, tt.wantStatus, rec.Body)
			}
			if tt.wantState == "" {
				return
			}
			var got struct {
				BOMID  int64  `json:"bom_id"`
				Status string `json:"status"`
				Done   int    `json:"done"`
				Total  int    `json:"total"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			if got.BOMID != 7 || got.Status != tt.wantState || got.Total != 3 {
				t.Fatalf("réponse inattendue : %s", rec.Body)
			}
		})
	}
}

// glb fabrique un GLB minimal valide : en-tête (magic, version 2, longueur) + charge.
func glb(payload string) []byte {
	b := make([]byte, 12, 12+len(payload))
	binary.LittleEndian.PutUint32(b[0:4], 0x46546C67)
	binary.LittleEndian.PutUint32(b[4:8], 2)
	binary.LittleEndian.PutUint32(b[8:12], uint32(12+len(payload))) //nolint:gosec // petite charge de test
	return append(b, payload...)
}

func multipartBody(t *testing.T, filename string, data []byte, fields map[string]string) (*bytes.Buffer, string) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for k, v := range fields {
		if err := mw.WriteField(k, v); err != nil {
			t.Fatal(err)
		}
	}
	if filename != "" {
		fw, err := mw.CreateFormFile("file", filename)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := fw.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}
	return &buf, mw.FormDataContentType()
}

// PUT /parts/{id}/3d : association JSON ou téléversement multipart. La validation du GLB est au domaine ;
// l'API transmet fichier et champs tels quels, et traduit ses refus.
func TestPutPartModelRoute(t *testing.T) {
	data := glb(`{"asset":{"version":"2.0"}}`)
	tests := []struct {
		name       string
		body       func(t *testing.T) (io.Reader, string)
		err        error
		wantStatus int
		check      func(t *testing.T, f *fakeAssemblies)
	}{
		{"association JSON transmise au domaine", func(*testing.T) (io.Reader, string) {
			return strings.NewReader(`{"model_id": 12, "match_type": "similar", "confidence": "0.7"}`), "application/json"
		}, nil, http.StatusOK, func(t *testing.T, f *fakeAssemblies) {
			if f.gotAssign == nil || f.gotAssign.ModelID != 12 || f.gotAssign.Match != viz3d.MatchSimilar ||
				f.gotAssign.Confidence == nil || !f.gotAssign.Confidence.Equal(decimal.RequireFromString("0.7")) {
				t.Fatalf("association reçue : %+v", f.gotAssign)
			}
		}},
		{"JSON avec champ inconnu → 400", func(*testing.T) (io.Reader, string) {
			return strings.NewReader(`{"model_id": 12, "position": [1,2,3]}`), "application/json"
		}, nil, http.StatusBadRequest, nil},
		{"confiance non décimale → 400", func(*testing.T) (io.Reader, string) {
			return strings.NewReader(`{"model_id": 12, "match_type": "exact", "confidence": "high"}`), "application/json"
		}, nil, http.StatusBadRequest, nil},
		{"modèle inconnu → 404", func(*testing.T) (io.Reader, string) {
			return strings.NewReader(`{"model_id": 99, "match_type": "exact"}`), "application/json"
		}, viz3d.ErrModelNotFound, http.StatusNotFound, nil},
		{"téléversement GLB transmis avec ses champs", func(t *testing.T) (io.Reader, string) {
			return multipartBody(t, "jante.glb", data, map[string]string{"match_type": "similar", "confidence": "0.55", "license": "CC-BY"})
		}, nil, http.StatusOK, func(t *testing.T, f *fakeAssemblies) {
			if f.gotUpload == nil || f.gotUpload.Filename != "jante.glb" || f.gotUpload.Match != viz3d.MatchSimilar ||
				f.gotUpload.License != "CC-BY" || f.gotUpload.Confidence == nil || !bytes.Equal(f.gotData, data) {
				t.Fatalf("téléversement reçu : %+v", f.gotUpload)
			}
		}},
		{"téléversement sans match_type → exact par défaut", func(t *testing.T) (io.Reader, string) {
			return multipartBody(t, "jante.glb", data, nil)
		}, nil, http.StatusOK, func(t *testing.T, f *fakeAssemblies) {
			if f.gotUpload == nil || f.gotUpload.Match != viz3d.MatchExact || f.gotUpload.Confidence != nil {
				t.Fatalf("téléversement reçu : %+v", f.gotUpload)
			}
		}},
		{"multipart sans champ file → 400", func(t *testing.T) (io.Reader, string) {
			return multipartBody(t, "", nil, map[string]string{"match_type": "exact"})
		}, nil, http.StatusBadRequest, nil},
		{"GLB refusé par le domaine → 400", func(t *testing.T) (io.Reader, string) {
			return multipartBody(t, "jante.zip", []byte("PK"), nil)
		}, viz3d.ErrInvalidModel, http.StatusBadRequest, nil},
		{"pièce inconnue → 400", func(*testing.T) (io.Reader, string) {
			return strings.NewReader(`{"model_id": 12, "match_type": "exact"}`), "application/json"
		}, catalog.ErrPartNotFound, http.StatusBadRequest, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeAssemblies{err: tt.err}
			body, ct := tt.body(t)
			req := httptest.NewRequest(http.MethodPut, "/api/v1/parts/5/3d", body)
			req.Header.Set("Content-Type", ct)
			rec := httptest.NewRecorder()
			NewRouter(Services{Assemblies: f}).ServeHTTP(rec, req)
			if rec.Code != tt.wantStatus {
				t.Fatalf("status %d, attendu %d : %s", rec.Code, tt.wantStatus, rec.Body)
			}
			if tt.check != nil {
				tt.check(t, f)
			}
		})
	}
}

// GET /models/{id}/file sert le GLB avec son type MIME ; 404 sans fichier.
func TestModelFileRoute(t *testing.T) {
	data := glb("x")
	tests := []struct {
		name       string
		path       string
		err        error
		wantStatus int
	}{
		{"fichier servi en model/gltf-binary", "/api/v1/models/7/file", nil, http.StatusOK},
		{"modèle inconnu → 404", "/api/v1/models/7/file", viz3d.ErrModelNotFound, http.StatusNotFound},
		{"primitive sans fichier → 404", "/api/v1/models/7/file", viz3d.ErrNoModel, http.StatusNotFound},
		{"fichier absent du stockage → 404", "/api/v1/models/7/file", viz3d.ErrBlobNotFound, http.StatusNotFound},
		{"id invalide → 400", "/api/v1/models/0/file", nil, http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeAssemblies{err: tt.err, file: data}
			rec := httptest.NewRecorder()
			NewRouter(Services{Assemblies: f}).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tt.path, nil))
			if rec.Code != tt.wantStatus {
				t.Fatalf("status %d, attendu %d : %s", rec.Code, tt.wantStatus, rec.Body)
			}
			if tt.wantStatus != http.StatusOK {
				return
			}
			if ct := rec.Header().Get("Content-Type"); ct != "model/gltf-binary" {
				t.Fatalf("Content-Type %q", ct)
			}
			if rec.Header().Get("Content-Length") != "13" || !bytes.Equal(rec.Body.Bytes(), data) {
				t.Fatalf("corps inattendu (%s octets)", rec.Header().Get("Content-Length"))
			}
		})
	}
}

// GET /parts/{id}/3d : 200 avec model null quand la pièce n'a pas de modèle ; 400 si la pièce n'existe pas.
func TestPartModelRoute(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
	}{
		{"pièce sans modèle → 200, model null", nil, http.StatusOK},
		{"pièce inconnue → 400", catalog.ErrPartNotFound, http.StatusBadRequest},
		{"sans stockage → 503", viz3d.ErrModelsUnavailable, http.StatusServiceUnavailable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			NewRouter(Services{Assemblies: &fakeAssemblies{err: tt.err}}).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/parts/5/3d", nil))
			if rec.Code != tt.wantStatus {
				t.Fatalf("status %d, attendu %d : %s", rec.Code, tt.wantStatus, rec.Body)
			}
			if tt.wantStatus == http.StatusOK && !strings.Contains(rec.Body.String(), `"model":null`) {
				t.Fatalf("model null attendu : %s", rec.Body)
			}
		})
	}
}
