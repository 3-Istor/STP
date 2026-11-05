package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"bomstudio/internal/catalog"
	"bomstudio/internal/catalog/catalogstore"
)

const completePartJSON = `{"name":"Vis moteur","category":"http-tests","zone":"moteur",
"mass_kg":"0.01234567890123456789","co2_kg_per_unit":"9007199254740993.123456789",
"recycled_percent":"32.123456789","recyclability_percent":"98.7654321",
"manufacturing_cost":{"amount":"0.005","currency":"USD"},"production_days":0,
"labor_hours":"0.125","hourly_rate":{"amount":"17.123456789","currency":"EUR"}}`

func catalogAPI(t *testing.T) (http.Handler, pgx.Tx) {
	t.Helper()
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		if os.Getenv("CI") != "" {
			t.Fatal("DATABASE_URL absente en CI")
		}
		t.Skip("DATABASE_URL absente : intégration catalogue non exécutée")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	conn, err := pgx.Connect(ctx, url)
	if err != nil {
		if os.Getenv("CI") != "" {
			t.Fatalf("connect test database: %v", err)
		}
		t.Skipf("base indisponible : %v", err)
	}
	t.Cleanup(func() { _ = conn.Close(context.Background()) })
	tx, err := conn.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tx.Rollback(context.Background()) })
	now := func() time.Time { return time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC) }
	return NewRouter(Services{Catalog: catalog.NewService(catalogstore.New(tx), now)}), tx
}

func apiCall(t *testing.T, h http.Handler, method, path, body string, want int) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(rec, r)
	if rec.Code != want {
		t.Fatalf("%s %s: status %d want %d; %s", method, path, rec.Code, want, rec.Body.String())
	}
	if want >= 400 {
		var payload struct {
			Error string `json:"error"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil || payload.Error == "" {
			t.Fatalf("erreur JSON explicite manquante : %s (%v)", rec.Body.String(), err)
		}
	}
	return rec
}

func createAPIPart(t *testing.T, h http.Handler, body string) catalog.Part {
	t.Helper()
	rec := apiCall(t, h, http.MethodPost, "/api/v1/parts", body, http.StatusCreated)
	var part catalog.Part
	if err := json.Unmarshal(rec.Body.Bytes(), &part); err != nil {
		t.Fatal(err)
	}
	return part
}

func createAPIOffer(t *testing.T, h http.Handler, tx pgx.Tx, partID int64) catalog.Offer {
	t.Helper()
	var supplierID int64
	if err := tx.QueryRow(t.Context(), "INSERT INTO suppliers(name) VALUES ('HTTP fournisseur') RETURNING id").Scan(&supplierID); err != nil {
		t.Fatal(err)
	}
	body := fmt.Sprintf(`{"supplier_id":%d,"currency":"USD","price_min":"0.005","price_max":"20.123456789","lead_time_days":3,"quality_score":"4.25","origin_country":"US"}`, supplierID)
	rec := apiCall(t, h, http.MethodPost, fmt.Sprintf("/api/v1/parts/%d/offers", partID), body, http.StatusCreated)
	var offer catalog.Offer
	if err := json.Unmarshal(rec.Body.Bytes(), &offer); err != nil {
		t.Fatal(err)
	}
	return offer
}

func TestCatalogHTTPRoundTripAndPatch(t *testing.T) {
	h, _ := catalogAPI(t)
	part := createAPIPart(t, h, completePartJSON)
	path := "/api/v1/parts/" + strconv.FormatInt(part.ID, 10)
	for _, method := range []string{http.MethodGet, http.MethodPatch} {
		t.Run(method, func(t *testing.T) {
			body := ""
			if method == http.MethodPatch {
				body = `{"name":"Vis renommée"}`
			}
			rec := apiCall(t, h, method, path, body, http.StatusOK)
			var fields map[string]json.RawMessage
			if err := json.Unmarshal(rec.Body.Bytes(), &fields); err != nil {
				t.Fatal(err)
			}
			checks := []struct{ field, want string }{
				{"mass_kg", `"0.01234567890123456789"`},
				{"co2_kg_per_unit", `"9007199254740993.123456789"`},
				{"recycled_percent", `"32.123456789"`},
				{"recyclability_percent", `"98.7654321"`},
				{"manufacturing_cost", `{"amount":"0.005","currency":"USD"}`},
			}
			for _, check := range checks {
				if !bytes.Equal(fields[check.field], []byte(check.want)) {
					t.Errorf("%s = %s, want %s", check.field, fields[check.field], check.want)
				}
			}
		})
	}
	for _, body := range []string{`{"co2_kg_per_unit":null}`, `{"recycled_percent":30}`, `{"unknown":true}`} {
		apiCall(t, h, http.MethodPatch, path, body, http.StatusBadRequest)
	}
}

func TestCatalogHTTPInvalidInput(t *testing.T) {
	h, _ := catalogAPI(t)
	tests := []struct{ name, body string }{
		{"nombre décimal JSON", strings.Replace(completePartJSON, `"32.123456789"`, `32.123456789`, 1)},
		{"pourcentage hors borne", strings.Replace(completePartJSON, `"98.7654321"`, `"100.01"`, 1)},
		{"devise inconnue", strings.Replace(completePartJSON, `"USD"`, `"ZZZ"`, 1)},
		{"deux objets", completePartJSON + "{}"},
		{"null", "null"},
		{"tableau", "[]"},
		{"objet incomplet", `{"name":"seulement un nom"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			apiCall(t, h, http.MethodPost, "/api/v1/parts", tt.body, http.StatusBadRequest)
		})
	}
	apiCall(t, h, http.MethodGet, "/api/v1/parts/abc", "", http.StatusBadRequest)
	apiCall(t, h, http.MethodGet, "/api/v1/parts/9223372036854775807", "", http.StatusNotFound)
	apiCall(t, h, http.MethodGet, "/api/v1/inconnu", "", http.StatusNotFound)
	apiCall(t, h, http.MethodPut, "/api/v1/parts", "{}", http.StatusMethodNotAllowed)
}

func TestCatalogHTTPOffersAndFilters(t *testing.T) {
	h, tx := catalogAPI(t)
	part := createAPIPart(t, h, completePartJSON)
	first := createAPIOffer(t, h, tx, part.ID)
	second := createAPIOffer(t, h, tx, part.ID)
	createAPIPart(t, h, strings.ReplaceAll(completePartJSON, "moteur", "chassis"))
	tests := []struct {
		name, query string
		want        int
	}{
		{"zone", "category=http-tests&zone=moteur", 1},
		{"nom sans casse", "category=http-tests&name=MOTEUR", 1},
		{"fournisseur", fmt.Sprintf("supplier_id=%d", first.SupplierID), 1},
		{"filtres combinés", fmt.Sprintf("supplier_id=%d&zone=chassis", second.SupplierID), 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := apiCall(t, h, http.MethodGet, "/api/v1/parts?"+tt.query, "", http.StatusOK)
			var parts []catalog.PartWithOffers
			if err := json.Unmarshal(rec.Body.Bytes(), &parts); err != nil {
				t.Fatal(err)
			}
			if len(parts) != tt.want || (tt.want == 1 && (parts[0].ID != part.ID || len(parts[0].Offers) != 2)) {
				t.Fatalf("pièce unique et deux offres attendues : %+v", parts)
			}
		})
	}
	invalid := fmt.Sprintf(`{"supplier_id":%d,"currency":"USD","price_min":"21","price_max":"20","lead_time_days":0,"quality_score":"4","origin_country":"US"}`, first.SupplierID)
	rec := apiCall(t, h, http.MethodPost, fmt.Sprintf("/api/v1/parts/%d/offers", part.ID), invalid, http.StatusBadRequest)
	if !strings.Contains(rec.Body.String(), "price_min") {
		t.Fatalf("borne invalide non expliquée : %s", rec.Body.String())
	}
}

func TestCatalogHTTPAlternativeAndArchive(t *testing.T) {
	h, tx := catalogAPI(t)
	first := createAPIPart(t, h, completePartJSON)
	second := createAPIPart(t, h, strings.Replace(completePartJSON, "Vis moteur", "Autre vis", 1))
	path := fmt.Sprintf("/api/v1/parts/%d/alternatives/%d", first.ID, second.ID)
	apiCall(t, h, http.MethodPut, path, "", http.StatusNoContent)
	apiCall(t, h, http.MethodPut, path, "", http.StatusNoContent)
	rec := apiCall(t, h, http.MethodGet, fmt.Sprintf("/api/v1/parts/%d/alternatives", second.ID), "", http.StatusOK)
	var alternatives []catalog.Part
	if err := json.Unmarshal(rec.Body.Bytes(), &alternatives); err != nil || len(alternatives) != 1 || alternatives[0].ID != first.ID {
		t.Fatalf("alternative réciproque manquante : %s (%v)", rec.Body.String(), err)
	}
	offer := createAPIOffer(t, h, tx, first.ID)
	var bomID int64
	if err := tx.QueryRow(t.Context(), "INSERT INTO boms(name,currency,volume) VALUES ('BOM HTTP','EUR',1) RETURNING id").Scan(&bomID); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(t.Context(), "INSERT INTO bom_nodes(bom_id,part_id,offer_id,quantity) VALUES ($1,$2,$3,1)", bomID, first.ID, offer.ID); err != nil {
		t.Fatal(err)
	}
	for _, item := range []struct {
		path string
		id   int64
	}{{"offers", offer.ID}, {"parts", first.ID}} {
		t.Run(item.path, func(t *testing.T) {
			resource := fmt.Sprintf("/api/v1/%s/%d", item.path, item.id)
			rec := apiCall(t, h, http.MethodDelete, resource, "", http.StatusOK)
			var result catalog.DeleteResult
			if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil || !result.Archived {
				t.Fatalf("archivage attendu : %s (%v)", rec.Body.String(), err)
			}
			apiCall(t, h, http.MethodGet, resource, "", http.StatusOK)
		})
	}
	var readable bool
	if err := tx.QueryRow(t.Context(), `SELECT p.archived_at IS NOT NULL AND o.archived_at IS NOT NULL
 FROM bom_nodes n JOIN parts p ON p.id=n.part_id JOIN offers o ON o.id=n.offer_id
 WHERE n.bom_id=$1`, bomID).Scan(&readable); err != nil || !readable {
		t.Fatalf("BOM ne relit plus ses archives : %v", err)
	}
}

func TestCatalogHTTPQuotedOfferIdentity(t *testing.T) {
	h, tx := catalogAPI(t)
	part := createAPIPart(t, h, completePartJSON)
	offer := createAPIOffer(t, h, tx, part.ID)
	if _, err := tx.Exec(t.Context(), `INSERT INTO price_quotes(offer_id,amount,currency,source,quoted_at)
 VALUES ($1,10,'USD','http-fixture','2026-11-04T12:00:00Z')`, offer.ID); err != nil {
		t.Fatal(err)
	}
	var supplierID int64
	if err := tx.QueryRow(t.Context(), "INSERT INTO suppliers(name) VALUES ('Autre fournisseur') RETURNING id").Scan(&supplierID); err != nil {
		t.Fatal(err)
	}
	for _, patch := range []string{`{"currency":"EUR"}`, fmt.Sprintf(`{"supplier_id":%d}`, supplierID)} {
		apiCall(t, h, http.MethodPatch, fmt.Sprintf("/api/v1/offers/%d", offer.ID), patch, http.StatusConflict)
	}
	rec := apiCall(t, h, http.MethodGet, fmt.Sprintf("/api/v1/offers/%d", offer.ID), "", http.StatusOK)
	var unchanged catalog.Offer
	if err := json.Unmarshal(rec.Body.Bytes(), &unchanged); err != nil || unchanged.Currency != "USD" || unchanged.SupplierID != offer.SupplierID {
		t.Fatalf("identité cotée modifiée : %s (%v)", rec.Body.String(), err)
	}
}
