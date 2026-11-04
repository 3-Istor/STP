// Package httpapi expose l'API REST JSON. Les handlers décodent la requête,
// appellent un service métier et encodent la réponse ; aucune règle métier ici.
package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"bomstudio/internal/catalog"
	"bomstudio/internal/supplier"
)

// Services reçoit les services déjà câblés ; aucun accès base n'est construit dans les handlers.
type Services struct {
	Catalog  *catalog.Service
	Supplier *supplier.Service
}

// NewRouter construit le routeur de l'application.
func NewRouter(services ...Services) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", healthz)
	mux.HandleFunc("/api/v1/", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "route API introuvable"})
	})
	if len(services) > 0 && services[0].Catalog != nil {
		catalogHandler{service: services[0].Catalog}.register(mux)
	}
	if len(services) > 0 && services[0].Supplier != nil {
		supplierHandler{service: services[0].Supplier}.register(mux)
	}
	return mux
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
