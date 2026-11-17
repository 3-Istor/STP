package httpapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sort"
	"strconv"

	"bomstudio/internal/catalog"
	"bomstudio/internal/money"
)

type catalogHandler struct{ service *catalog.Service }

func (h catalogHandler) register(mux *http.ServeMux) {
	for _, route := range []struct{ path, allow string }{
		{"/api/v1/parts", "GET, HEAD, POST"},
		{"/api/v1/parts/{id}", "GET, HEAD, PATCH, DELETE"},
		{"/api/v1/parts/{id}/offers", "POST"},
		{"/api/v1/offers/{id}", "GET, HEAD, PATCH, DELETE"},
		{"/api/v1/parts/{id}/alternatives", "GET, HEAD"},
		{"/api/v1/parts/{id}/alternatives/{other_id}", "PUT"},
	} {
		mux.HandleFunc(route.path, func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Allow", route.allow)
			writeErrorBody(w, http.StatusMethodNotAllowed, "méthode HTTP non autorisée", "method_not_allowed")
		})
	}
	mux.HandleFunc("POST /api/v1/parts", h.createPart)
	mux.HandleFunc("GET /api/v1/parts", h.listParts)
	mux.HandleFunc("GET /api/v1/parts/{id}", h.part)
	mux.HandleFunc("PATCH /api/v1/parts/{id}", h.patchPart)
	mux.HandleFunc("DELETE /api/v1/parts/{id}", h.deletePart)
	mux.HandleFunc("POST /api/v1/parts/{id}/offers", h.createOffer)
	mux.HandleFunc("GET /api/v1/offers/{id}", h.offer)
	mux.HandleFunc("PATCH /api/v1/offers/{id}", h.patchOffer)
	mux.HandleFunc("DELETE /api/v1/offers/{id}", h.deleteOffer)
	mux.HandleFunc("GET /api/v1/parts/{id}/alternatives", h.alternatives)
	mux.HandleFunc("PUT /api/v1/parts/{id}/alternatives/{other_id}", h.addAlternative)
}

// readBody ne porte que le contrat JSON : objet unique, champs connus et taille bornée.
// Les bornes et autres règles métier restent dans le service du domaine.
func readBody(w http.ResponseWriter, r *http.Request, target any, patch bool) error {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("lire le corps JSON : %w", err)
	}
	body = bytes.TrimSpace(body)
	if len(body) == 0 || body[0] != '{' {
		return errors.New("le corps doit être un objet JSON")
	}
	if patch {
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(body, &fields); err != nil {
			return fmt.Errorf("objet JSON invalide : %w", err)
		}
		names := make([]string, 0, len(fields))
		for name := range fields {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			if bytes.Equal(bytes.TrimSpace(fields[name]), []byte("null")) {
				return fmt.Errorf("le champ %s ne peut pas être null", name)
			}
		}
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("requête JSON invalide : %w", err)
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return errors.New("le corps doit contenir un seul objet JSON")
	}
	return nil
}

func pathID(r *http.Request, name string) (int64, error) {
	id, err := strconv.ParseInt(r.PathValue(name), 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("identifiant %s invalide", name)
	}
	return id, nil
}

func badRequest(w http.ResponseWriter, err error) {
	writeErrorBody(w, http.StatusBadRequest, err.Error(), codeOrStatus(err, http.StatusBadRequest))
}

func catalogError(w http.ResponseWriter, r *http.Request, err error) {
	status := http.StatusInternalServerError
	message := "le catalogue est indisponible"
	switch {
	case errors.Is(err, catalog.ErrPartNotFound), errors.Is(err, catalog.ErrOfferNotFound),
		errors.Is(err, catalog.ErrSupplierNotFound):
		status, message = http.StatusNotFound, err.Error()
	case errors.Is(err, catalog.ErrInvalidInput), errors.Is(err, money.ErrInvalidAmount),
		errors.Is(err, money.ErrUnknownCurrency), errors.Is(err, money.ErrCurrencyMismatch):
		status, message = http.StatusBadRequest, err.Error()
	case errors.Is(err, catalog.ErrArchived), errors.Is(err, catalog.ErrReferenceConflict),
		errors.Is(err, catalog.ErrQuotedOfferIdentityChange):
		status, message = http.StatusConflict, err.Error()
		if errors.Is(err, catalog.ErrQuotedOfferIdentityChange) {
			message = "offre déjà cotée : créez une nouvelle offre et archivez l'ancienne pour changer de devise ou de fournisseur"
		}
	default:
		slog.ErrorContext(r.Context(), "catalog request failed", "err", err)
	}
	writeErrorBody(w, status, message, codeOrStatus(err, status))
}

func (h catalogHandler) createPart(w http.ResponseWriter, r *http.Request) {
	var input catalog.PartInput
	if err := readBody(w, r, &input, false); err != nil {
		badRequest(w, err)
		return
	}
	part, err := h.service.CreatePart(r.Context(), input)
	if err != nil {
		catalogError(w, r, err)
		return
	}
	w.Header().Set("Location", "/api/v1/parts/"+strconv.FormatInt(part.ID, 10))
	writeJSON(w, http.StatusCreated, part)
}

func (h catalogHandler) listParts(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	filters := catalog.Filters{Name: query.Get("name"), Category: query.Get("category"), Zone: query.Get("zone")}
	if raw, present := query["supplier_id"]; present {
		if len(raw) != 1 {
			badRequest(w, errors.New("supplier_id doit apparaître une seule fois"))
			return
		}
		id, err := strconv.ParseInt(raw[0], 10, 64)
		if err != nil || id <= 0 {
			badRequest(w, errors.New("supplier_id invalide"))
			return
		}
		filters.SupplierID = &id
	}
	parts, err := h.service.ListParts(r.Context(), filters)
	if err != nil {
		catalogError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, parts)
}

func (h catalogHandler) part(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		badRequest(w, err)
		return
	}
	part, err := h.service.Part(r.Context(), id)
	if err != nil {
		catalogError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, part)
}

func (h catalogHandler) patchPart(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		badRequest(w, err)
		return
	}
	var patch catalog.PartPatch
	if err := readBody(w, r, &patch, true); err != nil {
		badRequest(w, err)
		return
	}
	part, err := h.service.UpdatePart(r.Context(), id, patch)
	if err != nil {
		catalogError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, part)
}

func (h catalogHandler) deletePart(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		badRequest(w, err)
		return
	}
	result, err := h.service.DeletePart(r.Context(), id)
	if err != nil {
		catalogError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h catalogHandler) createOffer(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		badRequest(w, err)
		return
	}
	var input catalog.OfferInput
	if err := readBody(w, r, &input, false); err != nil {
		badRequest(w, err)
		return
	}
	offer, err := h.service.CreateOffer(r.Context(), id, input)
	if err != nil {
		catalogError(w, r, err)
		return
	}
	w.Header().Set("Location", "/api/v1/offers/"+strconv.FormatInt(offer.ID, 10))
	writeJSON(w, http.StatusCreated, offer)
}

func (h catalogHandler) offer(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		badRequest(w, err)
		return
	}
	offer, err := h.service.Offer(r.Context(), id)
	if err != nil {
		catalogError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, offer)
}

func (h catalogHandler) patchOffer(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		badRequest(w, err)
		return
	}
	var patch catalog.OfferPatch
	if err := readBody(w, r, &patch, true); err != nil {
		badRequest(w, err)
		return
	}
	offer, err := h.service.UpdateOffer(r.Context(), id, patch)
	if err != nil {
		catalogError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, offer)
}

func (h catalogHandler) deleteOffer(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		badRequest(w, err)
		return
	}
	result, err := h.service.DeleteOffer(r.Context(), id)
	if err != nil {
		catalogError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h catalogHandler) alternatives(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		badRequest(w, err)
		return
	}
	parts, err := h.service.Alternatives(r.Context(), id)
	if err != nil {
		catalogError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, parts)
}

func (h catalogHandler) addAlternative(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		badRequest(w, err)
		return
	}
	otherID, err := pathID(r, "other_id")
	if err != nil {
		badRequest(w, err)
		return
	}
	if err := h.service.AddAlternative(r.Context(), id, otherID); err != nil {
		catalogError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
