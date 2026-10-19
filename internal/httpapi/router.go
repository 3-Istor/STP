// Package httpapi expose l'API REST JSON. Les handlers décodent la requête,
// appellent un service métier et encodent la réponse ; aucune règle métier ici.
package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

// NewRouter construit le routeur de l'application.
func NewRouter() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", healthz)
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
