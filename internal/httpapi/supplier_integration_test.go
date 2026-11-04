package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"bomstudio/internal/catalog"
	"bomstudio/internal/catalog/catalogstore"
	"bomstudio/internal/supplier"
	"bomstudio/internal/supplier/supplierstore"
)

func TestSupplierHTTPContracts(t *testing.T) {
	_, tx := catalogAPI(t)
	h := NewRouter(Services{
		Catalog:  catalog.NewService(catalogstore.New(tx), time.Now),
		Supplier: supplier.NewService(supplierstore.New(tx)),
	})
	rec := apiCall(t, h, http.MethodPost, "/api/v1/suppliers",
		`{"name":"Fournisseur API","quality_score":"4.125","trust_score":"3.75"}`, http.StatusCreated)
	var vendor supplier.Supplier
	if err := json.Unmarshal(rec.Body.Bytes(), &vendor); err != nil {
		t.Fatal(err)
	}
	part := createAPIPart(t, h, completePartJSON)
	offerJSON := fmt.Sprintf(`{"supplier_id":%d,"currency":"EUR","price_min":"1","price_max":"20","lead_time_days":3,"quality_score":"4.25","origin_country":"FR"}`, vendor.ID)
	rec = apiCall(t, h, http.MethodPost, fmt.Sprintf("/api/v1/parts/%d/offers", part.ID), offerJSON, http.StatusCreated)
	var offer catalog.Offer
	if err := json.Unmarshal(rec.Body.Bytes(), &offer); err != nil {
		t.Fatal(err)
	}
	contractJSON := fmt.Sprintf(`{"starts_on":"2026-01-01","ends_on":"2026-12-31","conditions":"Livraison groupée","offer_ids":[%d],"tiers":[{"min_quantity":"500","discount_percent":"5.125"}]}`, offer.ID)
	rec = apiCall(t, h, http.MethodPost, fmt.Sprintf("/api/v1/suppliers/%d/contracts", vendor.ID), contractJSON, http.StatusCreated)
	var contract supplier.Contract
	if err := json.Unmarshal(rec.Body.Bytes(), &contract); err != nil {
		t.Fatal(err)
	}
	for _, method := range []string{http.MethodGet, http.MethodPatch} {
		t.Run(method, func(t *testing.T) {
			body := ""
			if method == http.MethodPatch {
				body = `{"conditions":"Retrait en usine"}`
			}
			rec := apiCall(t, h, method, fmt.Sprintf("/api/v1/contracts/%d", contract.ID), body, http.StatusOK)
			var got supplier.Contract
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			if got.StartsOn != "2026-01-01" || got.EndsOn != "2026-12-31" || len(got.OfferIDs) != 1 || got.OfferIDs[0] != offer.ID || len(got.Tiers) != 1 || got.Tiers[0].DiscountPercent.String() != "5.125" {
				t.Fatalf("contrat modifié ou arrondi : %+v", got)
			}
		})
	}
	for _, tc := range []struct {
		name, body string
	}{
		{"date impossible", `{"ends_on":"2026-02-30"}`},
		{"remise hors borne", `{"tiers":[{"min_quantity":"500","discount_percent":"101"}]}`},
		{"remise numérique refusée", `{"tiers":[{"min_quantity":"500","discount_percent":5}]}`},
		{"null explicite", `{"offer_ids":null}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			apiCall(t, h, http.MethodPatch, fmt.Sprintf("/api/v1/contracts/%d", contract.ID), tc.body, http.StatusBadRequest)
		})
	}
	otherOffer := createAPIOffer(t, h, tx, part.ID)
	apiCall(t, h, http.MethodPatch, fmt.Sprintf("/api/v1/contracts/%d", contract.ID),
		fmt.Sprintf(`{"offer_ids":[%d]}`, otherOffer.ID), http.StatusBadRequest)
	apiCall(t, h, http.MethodDelete, fmt.Sprintf("/api/v1/offers/%d", offer.ID), "", http.StatusOK)
	apiCall(t, h, http.MethodPost, fmt.Sprintf("/api/v1/suppliers/%d/contracts", vendor.ID), contractJSON, http.StatusConflict)
	rec = apiCall(t, h, http.MethodGet, fmt.Sprintf("/api/v1/suppliers/%d/contracts", vendor.ID), "", http.StatusOK)
	var contracts []supplier.Contract
	if err := json.Unmarshal(rec.Body.Bytes(), &contracts); err != nil || len(contracts) != 1 {
		t.Fatalf("contrats perdus ou création rejetée non atomique : %s (%v)", rec.Body.String(), err)
	}
	apiCall(t, h, http.MethodPatch, fmt.Sprintf("/api/v1/suppliers/%d", vendor.ID), `{"trust_score":"5.01"}`, http.StatusBadRequest)
	rec = apiCall(t, h, http.MethodGet, fmt.Sprintf("/api/v1/suppliers/%d/contracts?as_of=2027-01-01T00:00:00Z", vendor.ID), "", http.StatusOK)
	var dated []struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &dated); err != nil || len(dated) != 1 || dated[0].Status != "expired" {
		t.Fatalf("expiration non indiquée : %s (%v)", rec.Body.String(), err)
	}
	apiCall(t, h, http.MethodGet, "/api/v1/suppliers/9223372036854775807/contracts", "", http.StatusNotFound)
}
