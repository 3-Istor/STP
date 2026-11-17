package httpapi

import (
	"errors"
	"log/slog"
	"net/http"
	"time"

	"bomstudio/internal/money"
	"bomstudio/internal/pricing"
)

type pricingHandler struct{ service *pricing.ReadService }

func pricingError(w http.ResponseWriter, r *http.Request, err error) {
	status := http.StatusInternalServerError
	switch {
	case errors.Is(err, pricing.ErrInvalidReadRequest), errors.Is(err, pricing.ErrInvalidHistoryRequest):
		status = http.StatusBadRequest
	case errors.Is(err, pricing.ErrPriceEntityNotFound), errors.Is(err, pricing.ErrPriceSnapshotNotFound):
		status = http.StatusNotFound
	case errors.Is(err, pricing.ErrRuntimeConfigConflict):
		status = http.StatusConflict
	case errors.Is(err, pricing.ErrSourceUnavailable), errors.Is(err, pricing.ErrRuntimeNotInitialized), errors.Is(err, pricing.ErrNoQuote), errors.Is(err, pricing.ErrNoRate):
		status = http.StatusServiceUnavailable
	}
	if status == http.StatusInternalServerError {
		slog.ErrorContext(r.Context(), "pricing read failed", "err", err)
		writeErrorBody(w, status, "lecture des prix indisponible", "internal")
		return
	}
	writeErrorBody(w, status, err.Error(), codeOrStatus(err, status))
}
func pricingTime(r *http.Request, key string) (*time.Time, error) {
	values, ok := r.URL.Query()[key]
	if !ok {
		return nil, nil
	}
	if len(values) != 1 {
		return nil, pricing.ErrInvalidReadRequest
	}
	at, err := time.Parse(time.RFC3339Nano, values[0])
	if err != nil {
		return nil, pricing.ErrInvalidReadRequest
	}
	at = at.UTC()
	return &at, nil
}
func historyTimes(r *http.Request) (time.Time, time.Time, error) {
	from, err := pricingTime(r, "from")
	if err != nil || from == nil {
		return time.Time{}, time.Time{}, pricing.ErrInvalidReadRequest
	}
	to, err := pricingTime(r, "to")
	if err != nil || to == nil {
		return time.Time{}, time.Time{}, pricing.ErrInvalidReadRequest
	}
	return *from, *to, nil
}
func pricingPair(r *http.Request) pricing.FXPair {
	return pricing.FXPair{Base: money.Currency(r.PathValue("base")), Quote: money.Currency(r.PathValue("quote"))}
}

func (h pricingHandler) register(mux *http.ServeMux) {
	h.registerConversion(mux)
	for path, kind := range map[string]string{"/quotes": "all", "/quotes/offers/{id}": "offer", "/quotes/suppliers/{id}": "supplier"} {
		mux.HandleFunc("GET /api/v1"+path, func(w http.ResponseWriter, r *http.Request) {
			scope := pricing.PriceScope{}
			if kind != "all" {
				id, err := pathID(r, "id")
				if err != nil {
					badRequest(w, err)
					return
				}
				if kind == "offer" {
					scope.OfferID = id
				} else {
					scope.SupplierID = id
				}
			}
			at, err := pricingTime(r, "as_of")
			if err != nil {
				pricingError(w, r, err)
				return
			}
			page, err := h.service.Prices(r.Context(), scope, at)
			if err != nil {
				pricingError(w, r, err)
				return
			}
			writeJSON(w, http.StatusOK, page)
		})
	}
	for _, path := range []string{"/fx", "/fx/{base}/{quote}"} {
		mux.HandleFunc("GET /api/v1"+path, func(w http.ResponseWriter, r *http.Request) {
			var pair *pricing.FXPair
			if r.PathValue("base") != "" {
				p := pricingPair(r)
				pair = &p
			}
			at, err := pricingTime(r, "as_of")
			if err != nil {
				pricingError(w, r, err)
				return
			}
			page, err := h.service.Rates(r.Context(), pair, at)
			if err != nil {
				pricingError(w, r, err)
				return
			}
			writeJSON(w, http.StatusOK, page)
		})
	}
	mux.HandleFunc("GET /api/v1/pricing/status", func(w http.ResponseWriter, r *http.Request) {
		state, err := h.service.Status(r.Context())
		if err != nil {
			pricingError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, state)
	})
	mux.HandleFunc("GET /api/v1/offers/{id}/quotes", func(w http.ResponseWriter, r *http.Request) {
		id, err := pathID(r, "id")
		if err != nil {
			badRequest(w, err)
			return
		}
		from, to, err := historyTimes(r)
		if err != nil {
			pricingError(w, r, err)
			return
		}
		rows, err := h.service.OfferHistory(r.Context(), id, from, to)
		if err != nil {
			pricingError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, rows)
	})
	mux.HandleFunc("GET /api/v1/fx/{base}/{quote}/history", func(w http.ResponseWriter, r *http.Request) {
		from, to, err := historyTimes(r)
		if err != nil {
			pricingError(w, r, err)
			return
		}
		rows, err := h.service.PairHistory(r.Context(), pricingPair(r), from, to)
		if err != nil {
			pricingError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, rows)
	})
}
