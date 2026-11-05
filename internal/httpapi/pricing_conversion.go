package httpapi

import (
	"bomstudio/internal/money"
	"bomstudio/internal/pricing"
	"net/http"
)

func (h pricingHandler) registerConversion(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/convert", func(w http.ResponseWriter, r *http.Request) {
		var req pricing.ConversionRequest
		if err := readBody(w, r, &req, false); err != nil {
			badRequest(w, err)
			return
		}
		result, err := h.service.Convert(r.Context(), req)
		if err != nil {
			pricingError(w, r, err)
			return
		}
		// Output boundary only; BOM consumers use result.Exact until their aggregate output.
		rounded, err := result.Exact.Round()
		if err != nil {
			pricingError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, struct {
			pricing.Conversion
			Converted money.Money `json:"converted"`
		}{Conversion: result, Converted: rounded})
	})
}
