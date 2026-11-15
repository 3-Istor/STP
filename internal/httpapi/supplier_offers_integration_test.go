package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"bomstudio/internal/catalog"
	"bomstudio/internal/catalog/catalogstore"
	"bomstudio/internal/pricing"
	"bomstudio/internal/pricing/pricingstore"
	"bomstudio/internal/supplier"
	"bomstudio/internal/supplier/supplierstore"
)

func TestSupplierOffersHTTPStatistics(t *testing.T) {
	_, tx := catalogAPI(t)
	h := NewRouter(Services{
		Catalog:  catalog.NewService(catalogstore.New(tx), time.Now),
		Supplier: supplier.NewService(supplierstore.New(tx)),
		History:  pricing.NewHistoryService(pricingstore.New(tx)),
	})
	part := createAPIPart(t, h, completePartJSON)
	offer := createAPIOffer(t, h, tx, part.ID)
	foreign := createAPIOffer(t, h, tx, part.ID)
	for _, id := range []int64{offer.ID, foreign.ID} {
		apiCall(t, h, http.MethodPatch, fmt.Sprintf("/api/v1/offers/%d", id), `{"price_max":"1000"}`, http.StatusOK)
	}
	var missing catalog.Offer
	rec := apiCall(t, h, http.MethodPost, fmt.Sprintf("/api/v1/parts/%d/offers", part.ID),
		fmt.Sprintf(`{"supplier_id":%d,"currency":"USD","price_min":"1","price_max":"20","lead_time_days":2,"quality_score":"4","origin_country":"US"}`, offer.SupplierID), http.StatusCreated)
	if err := json.Unmarshal(rec.Body.Bytes(), &missing); err != nil {
		t.Fatal(err)
	}
	for _, quote := range []struct{ amount, date string }{
		{"100", "2025-11-04T12:00:00Z"},
		{"105", "2026-05-01T12:00:00Z"},
		{"110", "2026-11-05T12:00:00Z"},
		{"999", "2026-11-06T12:00:00Z"},
	} {
		at, err := time.Parse(time.RFC3339, quote.date)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := tx.Exec(t.Context(), `INSERT INTO price_quotes(offer_id,amount,currency,source,quoted_at)
 VALUES ($1,$2,'USD','http-history',$3)`, offer.ID, quote.amount, at); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := tx.Exec(t.Context(), `INSERT INTO price_quotes(offer_id,amount,currency,source,quoted_at)
 VALUES ($1,500,'USD','foreign','2026-11-05T12:00:00Z')`, foreign.ID); err != nil {
		t.Fatal(err)
	}
	path := fmt.Sprintf("/api/v1/suppliers/%d/offers?as_of=2026-11-05T12:00:00Z", offer.SupplierID)
	rec = apiCall(t, h, http.MethodGet, path, "", http.StatusOK)
	var page supplierOffersPage
	if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if len(page.Offers) != 2 || page.AsOf == nil {
		t.Fatalf("offres propres au fournisseur attendues : %s", rec.Body.String())
	}
	for _, row := range page.Offers {
		if row.Offer.SupplierID != offer.SupplierID || row.Part.ID != part.ID {
			t.Fatalf("offre d'un autre fournisseur : %+v", row)
		}
		if row.Offer.ID == missing.ID {
			if row.Statistics != nil || row.Error == "" {
				t.Fatalf("cotation manquante cachée : %+v", row)
			}
			continue
		}
		stats := row.Statistics
		if stats == nil || stats.CurrentQuote == nil || stats.ReferenceQuote == nil || stats.VariationPercent == nil || *stats.VariationPercent != "10.0" || stats.Trend == nil || *stats.Trend != "up" {
			t.Fatalf("variation annuelle exacte manquante : %+v", stats)
		}
		if stats.CurrentQuote.Price.String() != "110.00 USD" || stats.CurrentQuote.Source != "http-history" || stats.ReferenceQuote.Price.String() != "100.00 USD" || stats.Minimum == nil || stats.Minimum.String() != "105.00 USD" || stats.Maximum == nil || stats.Maximum.String() != "110.00 USD" || len(stats.History) != 2 {
			t.Fatalf("source, référence, fenêtre ou futur incorrects : %+v", stats)
		}
	}
	apiCall(t, h, http.MethodGet, fmt.Sprintf("/api/v1/suppliers/%d/offers?as_of=invalide", offer.SupplierID), "", http.StatusBadRequest)
	// ADR 0030 : devise d'affichage validée ; sans source de taux, la conversion est refusée, pas ignorée.
	apiCall(t, h, http.MethodGet, fmt.Sprintf("/api/v1/suppliers/%d/offers?currency=euro", offer.SupplierID), "", http.StatusBadRequest)
	apiCall(t, h, http.MethodGet, fmt.Sprintf("/api/v1/suppliers/%d/offers?currency=EUR&currency=USD", offer.SupplierID), "", http.StatusBadRequest)
	apiCall(t, h, http.MethodGet, fmt.Sprintf("/api/v1/suppliers/%d/offers?currency=EUR", offer.SupplierID), "", http.StatusBadRequest)
	// Sans le paramètre, le contrat reste celui d'avant : aucune clé converted.
	if strings.Contains(rec.Body.String(), `"converted"`) {
		t.Fatalf("converted présent sans ?currency : %s", rec.Body.String())
	}
	apiCall(t, h, http.MethodGet, "/api/v1/suppliers/9223372036854775807/offers", "", http.StatusNotFound)
}
