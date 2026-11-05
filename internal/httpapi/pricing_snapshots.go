package httpapi

import (
	"errors"
	"fmt"
	"net/http"

	"bomstudio/internal/pricing"
)

type pricingSnapshotsHandler struct{ service *pricing.SnapshotService }

func (h pricingSnapshotsHandler) register(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/price-snapshots", func(w http.ResponseWriter, r *http.Request) {
		var req pricing.SnapshotRequest
		if err := readBody(w, r, &req, false); err != nil {
			badRequest(w, err)
			return
		}
		result, err := h.service.Create(r.Context(), req)
		if err != nil {
			var coverage *pricing.CoverageError
			if errors.As(err, &coverage) {
				writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": err.Error(), "missing": coverage.Missing})
				return
			}
			pricingError(w, r, err)
			return
		}
		w.Header().Set("Location", fmt.Sprintf("/api/v1/price-snapshots/%d", result.ID))
		writeJSON(w, http.StatusCreated, result)
	})
	mux.HandleFunc("GET /api/v1/price-snapshots/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, err := pathID(r, "id")
		if err != nil {
			badRequest(w, err)
			return
		}
		result, err := h.service.Get(r.Context(), id)
		if err != nil {
			pricingError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, result)
	})
}
