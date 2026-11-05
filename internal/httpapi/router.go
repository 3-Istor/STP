// Package httpapi expose l'API REST JSON. Les handlers décodent la requête,
// appellent un service métier et encodent la réponse ; aucune règle métier ici.
package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"
)

// Deps sont les services métier dont l'API a besoin.
type Deps struct {
	BOMs      BOMService
	Snapshots SnapshotService
	// Now donne l'instant d'un gel de prix ou d'une publication (le domaine n'a pas d'horloge) ;
	// time.Now si nil.
	Now func() time.Time
}

// NewRouter construit le routeur de l'application. Les routes /api/v1 ne sont montées que si leurs
// services sont fournis.
func NewRouter(d Deps) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", healthz)
	if d.BOMs != nil && d.Snapshots != nil {
		now := d.Now
		if now == nil {
			now = func() time.Time { return time.Now().UTC() }
		}
		(&api{boms: d.BOMs, snapshots: d.Snapshots, now: now}).routes(mux)
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
