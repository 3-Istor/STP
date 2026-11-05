package priceapi

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"bomstudio/internal/pricing"
)

func sourceRoutes(mux *http.ServeMux, source pricing.Source) {
	operations := map[string]pricing.SourceOperation{"/quotes/offer": pricing.SourceOffer, "/quotes/supplier": pricing.SourceSupplier, "/quotes/catalog": pricing.SourceCatalog, "/fx/pair": pricing.SourcePair, "/fx": pricing.SourceFX}
	for path, op := range operations {
		mux.HandleFunc("POST /api/v1"+path, func(w http.ResponseWriter, r *http.Request) {
			var input pricing.SourceInput
			decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<20))
			decoder.DisallowUnknownFields()
			if err := decoder.Decode(&input); err != nil {
				sourceJSON(w, http.StatusBadRequest, map[string]string{"error": "JSON source invalide"})
				return
			}
			var extra any
			if err := decoder.Decode(&extra); err != io.EOF {
				sourceJSON(w, http.StatusBadRequest, map[string]string{"error": "un seul objet JSON est attendu"})
				return
			}
			if err := input.Validate(op); err != nil {
				sourceJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
				return
			}
			out, err := source.Fetch(r.Context(), op, input)
			if err != nil {
				status := http.StatusInternalServerError
				if errors.Is(err, pricing.ErrInvalidSourceRequest) {
					status = http.StatusBadRequest
				} else if errors.Is(err, pricing.ErrSourceUnavailable) {
					status = http.StatusServiceUnavailable
				}
				slog.ErrorContext(r.Context(), "source generation failed", "err", err)
				sourceJSON(w, status, map[string]string{"error": "génération des cotations indisponible"})
				return
			}
			if err := pricing.ValidateSourceOutput(op, input, out); err != nil {
				sourceJSON(w, http.StatusBadGateway, map[string]string{"error": "cotations produites incohérentes"})
				return
			}
			sourceJSON(w, http.StatusOK, out)
		})
	}
}

func sourceJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		slog.Error("encode source response", "err", err)
	}
}
