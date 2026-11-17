package httpapi

import (
	"context"
	"net/http"
	"time"

	"bomstudio/internal/bom"
	"bomstudio/internal/money"
	"bomstudio/internal/snapshot"
)

// Méthode de calcul du coût et ventilation (ADR 0031) : handlers fins, le calcul est dans bom.

// CostService est ce que l'API attend du domaine pour la méthode de coût ; *bom.Service la satisfait.
type CostService interface {
	GetBOM(ctx context.Context, id int64) (bom.BOM, error)
	SetCostMethod(ctx context.Context, id int64, m bom.CostMethod) (bom.BOM, error)
	CostBreakdown(ctx context.Context, bomID, nodeID int64) (bom.Breakdown, error)
	TargetBudgets(ctx context.Context, bomID int64) ([]bom.BudgetStatus, error)
}

// SnapshotCostService : ventilation d'un nœud d'un snapshot ; *snapshot.Service la satisfait.
type SnapshotCostService interface {
	CostBreakdown(ctx context.Context, id, nodeKey int64) (bom.Breakdown, error)
}

var (
	_ CostService         = (*bom.Service)(nil)
	_ SnapshotCostService = (*snapshot.Service)(nil)
)

// costMethodJSON : {"basis":"purchase|landed|full","fx":"budget|latest","program_volume":60000|null}.
type costMethodJSON struct {
	Basis         string `json:"basis"`
	FX            string `json:"fx"`
	ProgramVolume *int32 `json:"program_volume"`
}

func toCostMethodJSON(m bom.CostMethod) costMethodJSON {
	m = m.OrDefault()
	return costMethodJSON{Basis: string(m.Basis), FX: string(m.FX), ProgramVolume: m.ProgramVolume}
}

type breakdownLineJSON struct {
	Kind   string      `json:"kind"`
	Label  string      `json:"label"`
	Amount money.Money `json:"amount"`
	Source string      `json:"source"`
	AsOf   *time.Time  `json:"as_of"`
}

// breakdownJSON : lignes additives dans la devise de la BOM ; Σ amount = total (montants exacts, arrondi à
// l'affichage, ADR 0004).
type breakdownJSON struct {
	NodeID   int64               `json:"node_id"`
	Currency string              `json:"currency"`
	Method   costMethodJSON      `json:"method"`
	CostAsOf *time.Time          `json:"cost_as_of"`
	Lines    []breakdownLineJSON `json:"lines"`
	Total    money.Money         `json:"total"`
}

func toBreakdownJSON(b bom.Breakdown) breakdownJSON {
	out := breakdownJSON{NodeID: b.NodeID, Currency: string(b.Currency), Method: toCostMethodJSON(b.Method), CostAsOf: b.AsOf,
		Lines: make([]breakdownLineJSON, len(b.Lines)), Total: b.Total}
	for i, l := range b.Lines {
		out.Lines[i] = breakdownLineJSON{Kind: string(l.Kind), Label: l.Label(), Amount: l.Amount, Source: l.Source(), AsOf: l.AsOf}
	}
	return out
}

type budgetJSON struct {
	NodeID int64        `json:"node_id"`
	Target money.Money  `json:"target"`
	Cost   *money.Money `json:"cost"`
	Delta  *money.Money `json:"delta"`
	// Percent : coût ÷ budget × 100, chaîne décimale à 1 décimale (half-even) ; null si indisponible.
	Percent *string `json:"percent"`
	State   string  `json:"state"` // ok | warn | over | unavailable
	Error   *string `json:"error"`
}

func toBudgetsJSON(st []bom.BudgetStatus) []budgetJSON {
	out := make([]budgetJSON, len(st))
	for i, s := range st {
		out[i] = budgetJSON{NodeID: s.NodeID, Target: s.Target, Cost: s.Cost, Delta: s.Delta, State: s.State}
		if s.Percent != nil {
			p := s.Percent.StringFixedBank(1)
			out[i].Percent = &p
		}
		if s.Error != "" {
			msg := s.Error
			out[i].Error = &msg
		}
	}
	return out
}

func (a *api) getCostMethod(w http.ResponseWriter, r *http.Request) {
	id, err := bomPathID(r, "id")
	var b bom.BOM
	if err == nil {
		b, err = a.costs.GetBOM(r.Context(), id)
	}
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toCostMethodJSON(b.CostMethod))
}

func (a *api) putCostMethod(w http.ResponseWriter, r *http.Request) {
	id, err := bomPathID(r, "id")
	var req costMethodJSON
	if err == nil {
		err = decode(r, &req)
	}
	var b bom.BOM
	if err == nil {
		b, err = a.costs.SetCostMethod(r.Context(), id, bom.CostMethod{
			Basis: bom.CostBasis(req.Basis), FX: bom.FXPolicy(req.FX), ProgramVolume: req.ProgramVolume,
		})
	}
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toCostMethodJSON(b.CostMethod))
}

func (a *api) costBreakdown(w http.ResponseWriter, r *http.Request) {
	id, err := bomPathID(r, "id")
	var nodeID int64
	if err == nil {
		nodeID, err = bomPathID(r, "nodeID")
	}
	var b bom.Breakdown
	if err == nil {
		b, err = a.costs.CostBreakdown(r.Context(), id, nodeID)
	}
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toBreakdownJSON(b))
}

func (a *api) snapshotCostBreakdown(w http.ResponseWriter, r *http.Request) {
	id, err := bomPathID(r, "id")
	var nodeID int64
	if err == nil {
		nodeID, err = bomPathID(r, "nodeID")
	}
	var b bom.Breakdown
	if err == nil {
		b, err = a.snapCosts.CostBreakdown(r.Context(), id, nodeID)
	}
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toBreakdownJSON(b))
}

func (a *api) budgets(w http.ResponseWriter, r *http.Request) {
	id, err := bomPathID(r, "id")
	var st []bom.BudgetStatus
	if err == nil {
		st, err = a.costs.TargetBudgets(r.Context(), id)
	}
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toBudgetsJSON(st))
}
