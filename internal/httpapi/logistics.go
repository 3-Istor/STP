package httpapi

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/shopspring/decimal"

	"bomstudio/internal/money"
	"bomstudio/internal/pricing"
)

// logisticsHandler : tables paramétrables du coût total rendu (ADR 0025). Fin : décoder → service → encoder.
type logisticsHandler struct{ service *pricing.LogisticsService }

func (h logisticsHandler) register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/customs-duties", h.listDuties)
	mux.HandleFunc("PUT /api/v1/customs-duties/{origin}/{destination}", h.putDuty)
	mux.HandleFunc("GET /api/v1/freight-rates", h.listFreight)
	mux.HandleFunc("PUT /api/v1/freight-rates/{origin}/{destination}", h.putFreight)
	mux.HandleFunc("PUT /api/v1/offers/{id}/packaging", h.putPackaging)
	mux.HandleFunc("DELETE /api/v1/offers/{id}/packaging", h.deletePackaging)
	mux.HandleFunc("DELETE /api/v1/customs-duties/{origin}/{destination}", h.deleteDuty)
	mux.HandleFunc("DELETE /api/v1/freight-rates/{origin}/{destination}", h.deleteFreight)
}

func (h logisticsHandler) deletePackaging(w http.ResponseWriter, r *http.Request) {
	id, err := bomPathID(r, "id")
	if err == nil {
		err = h.service.ClearOfferPackaging(r.Context(), id)
	}
	if err != nil {
		logisticsError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h logisticsHandler) deleteDuty(w http.ResponseWriter, r *http.Request) {
	if err := h.service.DeleteCustomsDuty(r.Context(), r.PathValue("origin"), r.PathValue("destination")); err != nil {
		logisticsError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h logisticsHandler) deleteFreight(w http.ResponseWriter, r *http.Request) {
	if err := h.service.DeleteFreightRate(r.Context(), r.PathValue("origin"), r.PathValue("destination")); err != nil {
		logisticsError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type customsDutyJSON struct {
	Origin      string `json:"origin_country"`
	Destination string `json:"destination_country"`
	DutyPercent string `json:"duty_percent"`
}

type freightRateJSON struct {
	Origin      string      `json:"origin_country"`
	Destination string      `json:"destination_country"`
	PerKg       money.Money `json:"per_kg"`
}

func toDutyJSON(d pricing.CustomsDuty) customsDutyJSON {
	return customsDutyJSON{Origin: d.Origin, Destination: d.Destination, DutyPercent: d.Percent.String()}
}

func toFreightJSON(f pricing.FreightRate) freightRateJSON {
	return freightRateJSON{Origin: f.Origin, Destination: f.Destination, PerKg: f.PerKg}
}

func (h logisticsHandler) listDuties(w http.ResponseWriter, r *http.Request) {
	ds, err := h.service.ListCustomsDuties(r.Context())
	if err != nil {
		logisticsError(w, r, err)
		return
	}
	out := make([]customsDutyJSON, len(ds))
	for i, d := range ds {
		out[i] = toDutyJSON(d)
	}
	writeJSON(w, http.StatusOK, out)
}

func (h logisticsHandler) putDuty(w http.ResponseWriter, r *http.Request) {
	var req struct {
		DutyPercent *decimal.Decimal `json:"duty_percent"`
	}
	if err := decode(r, &req); err != nil {
		logisticsError(w, r, err)
		return
	}
	if req.DutyPercent == nil {
		logisticsError(w, r, fmt.Errorf("duty_percent is required: %w", errBadRequest))
		return
	}
	d, err := h.service.SetCustomsDuty(r.Context(), pricing.CustomsDuty{
		Origin: r.PathValue("origin"), Destination: r.PathValue("destination"), Percent: *req.DutyPercent,
	})
	if err != nil {
		logisticsError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toDutyJSON(d))
}

func (h logisticsHandler) listFreight(w http.ResponseWriter, r *http.Request) {
	fs, err := h.service.ListFreightRates(r.Context())
	if err != nil {
		logisticsError(w, r, err)
		return
	}
	out := make([]freightRateJSON, len(fs))
	for i, f := range fs {
		out[i] = toFreightJSON(f)
	}
	writeJSON(w, http.StatusOK, out)
}

func (h logisticsHandler) putFreight(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PerKg *money.Money `json:"per_kg"`
	}
	if err := decode(r, &req); err != nil {
		logisticsError(w, r, err)
		return
	}
	if req.PerKg == nil {
		logisticsError(w, r, fmt.Errorf("per_kg is required: %w", errBadRequest))
		return
	}
	f, err := h.service.SetFreightRate(r.Context(), pricing.FreightRate{
		Origin: r.PathValue("origin"), Destination: r.PathValue("destination"), PerKg: *req.PerKg,
	})
	if err != nil {
		logisticsError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toFreightJSON(f))
}

func (h logisticsHandler) putPackaging(w http.ResponseWriter, r *http.Request) {
	id, err := bomPathID(r, "id")
	if err != nil {
		logisticsError(w, r, err)
		return
	}
	var req struct {
		Packaging *money.Money `json:"packaging"`
	}
	if err := decode(r, &req); err != nil {
		logisticsError(w, r, err)
		return
	}
	if req.Packaging == nil {
		logisticsError(w, r, fmt.Errorf("packaging is required: %w", errBadRequest))
		return
	}
	m, err := h.service.SetOfferPackaging(r.Context(), id, *req.Packaging)
	if err != nil {
		logisticsError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"offer_id": id, "packaging": m})
}

func logisticsError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, pricing.ErrUnknownOffer):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
	case errors.Is(err, pricing.ErrInvalidLogistics):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
	default:
		writeError(w, r, err)
	}
}
