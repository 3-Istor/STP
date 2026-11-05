package priceapi

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"bomstudio/internal/pricesim"
	"bomstudio/internal/pricing"
)

// NewRouter construit le routeur de l'API de prix.
func NewRouter(sources ...pricing.Source) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", healthz)
	var source pricing.Source = pricesim.Simulator{}
	if len(sources) > 0 && sources[0] != nil {
		source = sources[0]
	}
	sourceRoutes(mux, source)
	return mux
}

func healthz(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(map[string]string{"status": "ok"}); err != nil {
		slog.Error("encode healthz", "err", err)
	}
}
