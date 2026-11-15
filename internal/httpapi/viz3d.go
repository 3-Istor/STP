package httpapi

import (
	"context"
	"net/http"

	"bomstudio/internal/viz3d"
)

// AssemblyService est ce que l'API attend du domaine viz3d ; *viz3d.Service la satisfait (ADR 0031).
type AssemblyService interface {
	Assembly(ctx context.Context, bomID int64) (viz3d.Assembly, error)
}

var _ AssemblyService = (*viz3d.Service)(nil)

type viz3dHandler struct {
	service AssemblyService
}

func (h viz3dHandler) register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/boms/{id}/3d", h.assembly)
}

// assembly renvoie la scène 3D d'un brouillon : décoder l'id, appeler le domaine, encoder.
func (h viz3dHandler) assembly(w http.ResponseWriter, r *http.Request) {
	id, err := bomPathID(r, "id")
	if err != nil {
		writeError(w, r, err)
		return
	}
	a, err := h.service.Assembly(r.Context(), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, a)
}
