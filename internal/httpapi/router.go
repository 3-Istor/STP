// Package httpapi expose l'API REST JSON. Les handlers décodent la requête,
// appellent un service métier et encodent la réponse ; aucune règle métier ici.
package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"bomstudio/internal/catalog"
	"bomstudio/internal/pricing"
	"bomstudio/internal/supplier"
)

// Services reçoit les services déjà câblés ; aucun accès base n'est construit dans les handlers.
// Chaque groupe de routes /api/v1 n'est monté que si ses services sont fournis.
type Services struct {
	Catalog        *catalog.Service
	Supplier       *supplier.Service
	History        *pricing.HistoryService
	Pricing        *pricing.ReadService
	PriceSnapshots *pricing.SnapshotService
	PricingSignals *PricingHub
	// BOMs et Snapshots (snapshots de BOM) : routes /api/v1/boms et /api/v1/snapshots.
	BOMs      BOMService
	Snapshots SnapshotService
	// Logistics : tables de douane, de transport et emballage (coût total rendu, ADR 0025).
	Logistics *pricing.LogisticsService
	// Now donne l'instant d'un gel de prix ou d'une publication (le domaine n'a pas d'horloge) ;
	// time.Now si nil.
	Now func() time.Time
}

// NewRouter construit le routeur de l'application.
func NewRouter(services ...Services) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", healthz)
	if len(services) == 0 {
		return jsonNotFound(mux)
	}
	s := services[0]
	if s.Catalog != nil {
		catalogHandler{service: s.Catalog}.register(mux)
	}
	if s.Supplier != nil {
		supplierHandler{service: s.Supplier}.register(mux)
	}
	if s.Pricing != nil {
		pricingHandler{service: s.Pricing}.register(mux)
		if s.PricingSignals != nil {
			mux.Handle("GET /api/v1/quotes/stream", pricingStream{status: s.Pricing, hub: s.PricingSignals, writeTimeout: 5 * time.Second, heartbeat: 15 * time.Second})
		}
	}
	if s.PriceSnapshots != nil {
		pricingSnapshotsHandler{service: s.PriceSnapshots}.register(mux)
	}
	if s.Catalog != nil && s.Supplier != nil && s.History != nil {
		supplierOffersReader{catalog: s.Catalog, suppliers: s.Supplier, history: s.History, prices: s.Pricing}.register(mux)
	}
	if s.Logistics != nil {
		logisticsHandler{service: s.Logistics}.register(mux)
	}
	if s.BOMs != nil && s.Snapshots != nil {
		now := s.Now
		if now == nil {
			now = func() time.Time { return time.Now().UTC() }
		}
		a := &api{boms: s.BOMs, snapshots: s.Snapshots, now: now}
		a.costs, _ = s.BOMs.(CostService)
		a.snapCosts, _ = s.Snapshots.(SnapshotCostService)
		a.routes(mux)
	}
	return jsonNotFound(mux)
}

// jsonNotFound répond {"error": ...} pour une route /api/v1 inconnue. Un motif fourre-tout "/api/v1/"
// ne convient pas : il prendrait aussi les mauvaises méthodes, qui doivent rester en 405 (avec Allow).
func jsonNotFound(mux *http.ServeMux) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h, pattern := mux.Handler(r)
		if pattern != "" || !strings.HasPrefix(r.URL.Path, "/api/v1/") {
			mux.ServeHTTP(w, r)
			return
		}
		// Aucun motif : h est la page 404 ou 405 du mux. On garde la 405, on remplace la 404.
		c := &notFoundCatcher{ResponseWriter: w}
		h.ServeHTTP(c, r)
		if c.notFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "route API introuvable"})
		}
	})
}

type notFoundCatcher struct {
	http.ResponseWriter
	notFound bool
}

func (c *notFoundCatcher) WriteHeader(code int) {
	if code == http.StatusNotFound {
		c.notFound = true
		return
	}
	c.ResponseWriter.WriteHeader(code)
}

func (c *notFoundCatcher) Write(b []byte) (int, error) {
	if c.notFound {
		return len(b), nil
	}
	return c.ResponseWriter.Write(b)
}

func healthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("encode response", "err", err)
	}
}
