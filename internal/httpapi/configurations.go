package httpapi

import (
	"context"
	"errors"
	"net/http"
	"time"

	"bomstudio/internal/bom"
	"bomstudio/internal/money"
)

// ConfigurationService est un calcul en lecture seule, option 1 autorisée de l'ADR 0050.
type ConfigurationService interface {
	SimulateConfigurations(context.Context, int64, bom.ConfigurationRequest) (bom.ConfigurationComparison, error)
}

var _ ConfigurationService = (*bom.Service)(nil)

type configurationRequestJSON struct {
	RootID int64 `json:"root_id"`
	Guards []struct {
		ID       int64  `json:"id"`
		ParentID *int64 `json:"parent_id"`
		PartID   *int64 `json:"part_id"`
	} `json:"guards"`
	Scenarios []struct {
		Key           string  `json:"key"`
		ExcludedRoots []int64 `json:"excluded_roots"`
	} `json:"scenarios"`
}
type configurationJSON struct {
	Key          string       `json:"key"`
	ActiveIDs    []int64      `json:"active_ids"`
	ExcludedIDs  []int64      `json:"excluded_ids"`
	Delta        *money.Money `json:"delta"`
	DeltaPercent *string      `json:"delta_percent"`
	Tree         treeJSON     `json:"tree"`
}
type configurationComparisonJSON struct {
	RootID     int64               `json:"root_id"`
	PriceAsOf  *time.Time          `json:"price_as_of"`
	Persistent bool                `json:"persistent"`
	Scenarios  []configurationJSON `json:"scenarios"`
}

func mountConfigurations(mux *http.ServeMux, service BOMService) {
	svc, ok := service.(ConfigurationService)
	if !ok {
		return
	}
	mux.HandleFunc("POST /api/v1/boms/{id}/configuration-simulation", func(w http.ResponseWriter, r *http.Request) {
		id, err := bomPathID(r, "id")
		if err != nil {
			writeError(w, r, err)
			return
		}
		var body configurationRequestJSON
		if err = decode(r, &body); err != nil {
			writeError(w, r, err)
			return
		}
		req := bom.ConfigurationRequest{RootID: body.RootID}
		for _, g := range body.Guards {
			req.Guards = append(req.Guards, bom.OccurrenceGuard{ID: g.ID, ParentID: g.ParentID, PartID: g.PartID})
		}
		for _, s := range body.Scenarios {
			req.Scenarios = append(req.Scenarios, bom.ConfigurationDefinition{Key: s.Key, ExcludedRoots: s.ExcludedRoots})
		}
		result, err := svc.SimulateConfigurations(r.Context(), id, req)
		if err != nil {
			if errors.Is(err, bom.ErrInvalidInput) {
				writeErrorBody(w, http.StatusUnprocessableEntity, err.Error(), errorCode(err))
				return
			}
			writeError(w, r, err)
			return
		}
		out := configurationComparisonJSON{RootID: result.RootID, PriceAsOf: result.PriceAsOf, Scenarios: make([]configurationJSON, 0, len(result.Scenarios))}
		for _, s := range result.Scenarios {
			nodes := make([]treeNodeJSON, len(s.Tree.Nodes))
			for i, n := range s.Tree.Nodes {
				nodes[i] = treeNodeJSON{nodeJSON: toNodeJSON(n), Totals: nodeTotals(s.Tree.Totals, n.ID)}
			}
			out.Scenarios = append(out.Scenarios, configurationJSON{Key: s.Key, ActiveIDs: s.ActiveIDs, ExcludedIDs: s.ExcludedIDs, Delta: s.Delta, DeltaPercent: pctPtr(s.DeltaPercent),
				Tree: treeJSON{BOM: toBOMJSON(s.Tree.BOM), Nodes: nodes, Explanation: toExplanationJSON(s.Tree.Totals.Explanation), Stock: toStockJSON(s.Tree.Stock), Planning: toPlanningJSON(s.Tree.Planning), Budgets: toBudgetsJSON(s.Tree.Budgets)}})
		}
		writeJSON(w, http.StatusOK, out)
	})
}
