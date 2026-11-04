package httpapi

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"bomstudio/internal/catalog"
	"bomstudio/internal/pricing"
	"bomstudio/internal/supplier"
)

type offerView struct {
	Offer      catalog.Offer       `json:"offer"`
	Part       catalog.Part        `json:"part"`
	Statistics *pricing.Statistics `json:"statistics"`
	Error      string              `json:"error,omitempty"`
}

type supplierOffersPage struct {
	AsOf   *time.Time  `json:"as_of"`
	Offers []offerView `json:"offers"`
}

// supplierOffersReader compose les lectures ; les calculs et règles de prix restent dans pricing.
// Il conserve les données partielles et leurs erreurs plutôt que de remplacer un prix par zéro.
type supplierOffersReader struct {
	catalog   *catalog.Service
	suppliers *supplier.Service
	history   *pricing.HistoryService
}

func (s supplierOffersReader) page(ctx context.Context, supplierID int64, requested *time.Time) (supplierOffersPage, error) {
	page := supplierOffersPage{Offers: []offerView{}}
	if _, err := s.suppliers.Supplier(ctx, supplierID); err != nil {
		return page, err
	}
	offers, err := s.catalog.ListSupplierOffers(ctx, supplierID)
	if err != nil {
		return page, err
	}
	page.AsOf = requested
	if requested == nil {
		asOf, err := s.history.CurrentAsOf(ctx)
		if err != nil && !errors.Is(err, pricing.ErrNoQuote) {
			return page, err
		}
		if err == nil {
			page.AsOf = &asOf
		}
	}
	for _, offer := range offers {
		part, err := s.catalog.Part(ctx, offer.PartID)
		if err != nil {
			return page, err
		}
		view := offerView{Offer: offer, Part: part}
		if page.AsOf == nil {
			view.Error = "Aucune cotation disponible"
		} else {
			stats, err := s.history.Statistics(ctx, offer.ID, *page.AsOf)
			switch {
			case err == nil:
				view.Statistics = &stats
			case errors.Is(err, pricing.ErrNoReference):
				view.Statistics, view.Error = &stats, "Référence annuelle indisponible"
			case errors.Is(err, pricing.ErrNoWindow):
				view.Statistics, view.Error = &stats, "Aucune cotation dans la période"
			case errors.Is(err, pricing.ErrNoQuote):
				view.Error = "Aucune cotation disponible"
			case errors.Is(err, pricing.ErrInvalidHistoryData):
				view.Error = "Historique incohérent : " + err.Error()
			default:
				return page, fmt.Errorf("supplier offer %d statistics: %w", offer.ID, err)
			}
		}
		page.Offers = append(page.Offers, view)
	}
	return page, nil
}

func (s supplierOffersReader) register(mux *http.ServeMux) {
	mux.HandleFunc("/api/v1/suppliers/{id}/offers", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Allow", "GET, HEAD")
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "méthode HTTP non autorisée"})
	})
	mux.HandleFunc("GET /api/v1/suppliers/{id}/offers", func(w http.ResponseWriter, r *http.Request) {
		id, err := pathID(r, "id")
		if err != nil {
			badRequest(w, err)
			return
		}
		var requested *time.Time
		if raw, present := r.URL.Query()["as_of"]; present {
			if len(raw) != 1 {
				badRequest(w, errors.New("as_of doit apparaître une seule fois"))
				return
			}
			instant, err := time.Parse(time.RFC3339Nano, raw[0])
			if err != nil {
				badRequest(w, errors.New("as_of doit être un instant RFC3339 avec fuseau"))
				return
			}
			instant = instant.UTC()
			requested = &instant
		}
		page, err := s.page(r.Context(), id, requested)
		if err != nil {
			switch {
			case errors.Is(err, supplier.ErrSupplierNotFound):
				supplierError(w, r, err)
			case errors.Is(err, pricing.ErrInvalidHistoryRequest):
				badRequest(w, err)
			default:
				slog.ErrorContext(r.Context(), "supplier offers read failed", "err", err)
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "lecture des offres indisponible"})
			}
			return
		}
		writeJSON(w, http.StatusOK, page)
	})
}
