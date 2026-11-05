package httpapi

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"bomstudio/internal/supplier"
)

type supplierHandler struct{ service *supplier.Service }

func (h supplierHandler) register(mux *http.ServeMux) {
	for _, route := range []struct{ path, allow string }{
		{"/api/v1/suppliers", "GET, HEAD, POST"},
		{"/api/v1/suppliers/{id}", "GET, HEAD, PATCH"},
		{"/api/v1/suppliers/{id}/contracts", "GET, HEAD, POST"},
		{"/api/v1/contracts/{id}", "GET, HEAD, PATCH"},
	} {
		mux.HandleFunc(route.path, func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Allow", route.allow)
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "méthode HTTP non autorisée"})
		})
	}
	mux.HandleFunc("GET /api/v1/suppliers", h.list)
	mux.HandleFunc("POST /api/v1/suppliers", h.create)
	mux.HandleFunc("GET /api/v1/suppliers/{id}", h.get)
	mux.HandleFunc("PATCH /api/v1/suppliers/{id}", h.patch)
	mux.HandleFunc("GET /api/v1/suppliers/{id}/contracts", h.contracts)
	mux.HandleFunc("POST /api/v1/suppliers/{id}/contracts", h.createContract)
	mux.HandleFunc("GET /api/v1/contracts/{id}", h.contract)
	mux.HandleFunc("PATCH /api/v1/contracts/{id}", h.patchContract)
}

func supplierError(w http.ResponseWriter, r *http.Request, err error) {
	status, message := http.StatusInternalServerError, "le service fournisseurs est indisponible"
	switch {
	case errors.Is(err, supplier.ErrSupplierNotFound), errors.Is(err, supplier.ErrContractNotFound),
		errors.Is(err, supplier.ErrOfferNotFound):
		status, message = http.StatusNotFound, err.Error()
	case errors.Is(err, supplier.ErrInvalidInput), errors.Is(err, supplier.ErrSupplierMismatch):
		status, message = http.StatusBadRequest, err.Error()
	case errors.Is(err, supplier.ErrArchivedOffer), errors.Is(err, supplier.ErrReferenceConflict),
		errors.Is(err, supplier.ErrSelectionRequired):
		status, message = http.StatusConflict, err.Error()
	default:
		slog.ErrorContext(r.Context(), "supplier request failed", "err", err)
	}
	writeJSON(w, status, map[string]string{"error": message})
}

func (h supplierHandler) list(w http.ResponseWriter, r *http.Request) {
	items, err := h.service.ListSuppliers(r.Context())
	if err != nil {
		supplierError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (h supplierHandler) create(w http.ResponseWriter, r *http.Request) {
	var input supplier.SupplierInput
	if err := readBody(w, r, &input, false); err != nil {
		badRequest(w, err)
		return
	}
	item, err := h.service.CreateSupplier(r.Context(), input)
	if err != nil {
		supplierError(w, r, err)
		return
	}
	w.Header().Set("Location", "/api/v1/suppliers/"+strconv.FormatInt(item.ID, 10))
	writeJSON(w, http.StatusCreated, item)
}

func (h supplierHandler) get(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		badRequest(w, err)
		return
	}
	item, err := h.service.Supplier(r.Context(), id)
	if err != nil {
		supplierError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (h supplierHandler) patch(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		badRequest(w, err)
		return
	}
	var patch supplier.SupplierPatch
	if err := readBody(w, r, &patch, true); err != nil {
		badRequest(w, err)
		return
	}
	item, err := h.service.UpdateSupplier(r.Context(), id, patch)
	if err != nil {
		supplierError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (h supplierHandler) contracts(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		badRequest(w, err)
		return
	}
	items, err := h.service.ListContracts(r.Context(), id)
	if err != nil {
		supplierError(w, r, err)
		return
	}
	if raw := r.URL.Query().Get("as_of"); raw != "" {
		asOf, err := time.Parse(time.RFC3339Nano, raw)
		if err != nil {
			badRequest(w, errors.New("as_of doit être un instant RFC3339 avec fuseau"))
			return
		}
		type datedContract struct {
			supplier.Contract
			Status string `json:"status"`
		}
		dated := make([]datedContract, 0, len(items))
		for _, item := range items {
			status, err := supplier.ContractStatus(item, asOf)
			if err != nil {
				supplierError(w, r, err)
				return
			}
			dated = append(dated, datedContract{Contract: item, Status: status})
		}
		writeJSON(w, http.StatusOK, dated)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (h supplierHandler) createContract(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		badRequest(w, err)
		return
	}
	var input supplier.ContractInput
	if err := readBody(w, r, &input, false); err != nil {
		badRequest(w, err)
		return
	}
	item, err := h.service.CreateContract(r.Context(), id, input)
	if err != nil {
		supplierError(w, r, err)
		return
	}
	w.Header().Set("Location", "/api/v1/contracts/"+strconv.FormatInt(item.ID, 10))
	writeJSON(w, http.StatusCreated, item)
}

func (h supplierHandler) contract(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		badRequest(w, err)
		return
	}
	item, err := h.service.Contract(r.Context(), id)
	if err != nil {
		supplierError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (h supplierHandler) patchContract(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		badRequest(w, err)
		return
	}
	var patch supplier.ContractPatch
	if err := readBody(w, r, &patch, true); err != nil {
		badRequest(w, err)
		return
	}
	item, err := h.service.UpdateContract(r.Context(), id, patch)
	if err != nil {
		supplierError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}
