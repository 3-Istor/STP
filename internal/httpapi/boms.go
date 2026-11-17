package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"bomstudio/internal/bom"
	"bomstudio/internal/catalog"
	"bomstudio/internal/money"
	"bomstudio/internal/pricing"
	"bomstudio/internal/snapshot"
)

// BOMService est ce que l'API attend du domaine bom ; *bom.Service la satisfait.
type BOMService interface {
	CreateBOM(ctx context.Context, b bom.NewBOM) (bom.BOM, error)
	ListBOMs(ctx context.Context) ([]bom.BOM, error)
	RenameBOM(ctx context.Context, id int64, name string) (bom.BOM, error)
	UpdateSettings(ctx context.Context, id int64, currency string, volume int32) (bom.BOM, error)
	SetAnnualHours(ctx context.Context, id int64, hours decimal.Decimal) (bom.BOM, error)
	DeleteBOM(ctx context.Context, id int64) error
	AddNode(ctx context.Context, bomID int64, n bom.NewNode) (bom.Node, error)
	SetNodeOffer(ctx context.Context, bomID, nodeID, offerID int64) (bom.Node, error)
	SetNodeQuantity(ctx context.Context, bomID, nodeID int64, qty decimal.Decimal) (bom.Node, error)
	SetNodeProductionDays(ctx context.Context, bomID, nodeID int64, days int32) (bom.Node, error)
	MoveNode(ctx context.Context, bomID, nodeID, newParentID int64) (bom.Node, error)
	DeleteNode(ctx context.Context, bomID, nodeID int64) error
	FreezePrices(ctx context.Context, bomID int64, at time.Time) (bom.BOM, error)
	UsePriceSnapshot(ctx context.Context, bomID, snapshotID int64) (bom.BOM, error)
	UnfreezePrices(ctx context.Context, bomID int64) (bom.BOM, error)
	Totals(ctx context.Context, bomID int64) (bom.TreeTotals, error)
	Stock(ctx context.Context, bomID int64) ([]bom.PartStock, error)
	SetPartStock(ctx context.Context, bomID, partID int64, qty decimal.Decimal) (bom.PartStock, error)
	SetSchedule(ctx context.Context, bomID int64, s bom.Schedule) (bom.Schedule, error)
	SetNodeStrategy(ctx context.Context, bomID, nodeID int64, s *bom.Strategy) (bom.Node, error)
	ReplaceNodePart(ctx context.Context, bomID, nodeID, partID, offerID int64) (bom.Node, error)
	Suggestions(ctx context.Context, bomID, nodeID int64) (bom.Suggestions, error)
	CostAnomalies(ctx context.Context, bomID int64, threshold decimal.Decimal) (bom.CostAnomalies, error)
	SetDestinationCountry(ctx context.Context, id int64, country *string) (bom.BOM, error)
	LandedCosts(ctx context.Context, bomID, partID int64, offerIDs []int64) (bom.LandedComparison, error)
	MakeOrBuy(ctx context.Context, bomID, nodeID int64, req bom.MakeOrBuyRequest) (bom.MakeOrBuyResult, error)
	ClearSchedule(ctx context.Context, bomID int64) error
	PutTargetBudget(ctx context.Context, bomID, nodeID int64, amount money.Money) (bom.TargetBudget, error)
	DeleteTargetBudget(ctx context.Context, bomID, nodeID int64) error
}

// SnapshotService est ce que l'API attend du domaine snapshot ; *snapshot.Service la satisfait.
type SnapshotService interface {
	Publish(ctx context.Context, bomID int64, at time.Time, req snapshot.PublishRequest) (snapshot.Snapshot, error)
	Get(ctx context.Context, id int64) (snapshot.Published, error)
	NewDraft(ctx context.Context, id int64) (bom.BOM, error)
}

var (
	_ BOMService      = (*bom.Service)(nil)
	_ SnapshotService = (*snapshot.Service)(nil)
)

// errBadRequest : corps ou paramètre de requête illisible (JSON invalide, champ inconnu, id non numérique).
var errBadRequest = errors.New("bad request")

const maxBodyBytes = 1 << 20

type api struct {
	boms      BOMService
	snapshots SnapshotService
	// costs, snapCosts : méthode de coût et ventilation (ADR 0031) ; routes montées seulement si fournies.
	costs     CostService
	snapCosts SnapshotCostService
	// versions, review : comparaison, versions, contrôles et pistes (ADR 0032, 0033) ; montées si fournies.
	versions VersionsService
	review   ReviewService
	now      func() time.Time
}

func (a *api) routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/boms", a.listBOMs)
	mux.HandleFunc("POST /api/v1/boms", a.createBOM)
	mux.HandleFunc("PATCH /api/v1/boms/{id}", a.patchBOM)
	mux.HandleFunc("DELETE /api/v1/boms/{id}", a.deleteBOM)
	mux.HandleFunc("GET /api/v1/boms/{id}/tree", a.tree)
	mux.HandleFunc("POST /api/v1/boms/{id}/nodes", a.addNode)
	mux.HandleFunc("PATCH /api/v1/boms/{id}/nodes/{nodeID}", a.patchNode)
	mux.HandleFunc("DELETE /api/v1/boms/{id}/nodes/{nodeID}", a.deleteNode)
	mux.HandleFunc("PUT /api/v1/boms/{id}/nodes/{nodeID}/budget", a.putBudget)
	mux.HandleFunc("DELETE /api/v1/boms/{id}/nodes/{nodeID}/budget", a.deleteBudget)
	mux.HandleFunc("GET /api/v1/boms/{id}/nodes/{nodeID}/suggestions", a.suggestions)
	mux.HandleFunc("POST /api/v1/boms/{id}/nodes/{nodeID}/make-or-buy", a.makeOrBuy)
	mux.HandleFunc("GET /api/v1/boms/{id}/anomalies", a.anomalies)
	mux.HandleFunc("GET /api/v1/parts/{id}/landed-cost", a.landedCost)
	mux.HandleFunc("GET /api/v1/boms/{id}/stock", a.stock)
	mux.HandleFunc("PUT /api/v1/boms/{id}/stock/{partID}", a.putStock)
	mux.HandleFunc("PUT /api/v1/boms/{id}/schedule", a.putSchedule)
	mux.HandleFunc("DELETE /api/v1/boms/{id}/schedule", a.deleteSchedule)
	mux.HandleFunc("POST /api/v1/boms/{id}/freeze-prices", a.freezePrices)
	mux.HandleFunc("PUT /api/v1/boms/{id}/freeze-prices", a.usePriceSnapshot)
	mux.HandleFunc("DELETE /api/v1/boms/{id}/freeze-prices", a.unfreezePrices)
	mux.HandleFunc("POST /api/v1/boms/{id}/publish", a.publish)
	mux.HandleFunc("GET /api/v1/snapshots/{id}", a.getSnapshot)
	mux.HandleFunc("POST /api/v1/snapshots/{id}/drafts", a.newDraft)
	if a.costs != nil {
		mux.HandleFunc("GET /api/v1/boms/{id}/cost-method", a.getCostMethod)
		mux.HandleFunc("PUT /api/v1/boms/{id}/cost-method", a.putCostMethod)
		mux.HandleFunc("GET /api/v1/boms/{id}/nodes/{nodeID}/cost-breakdown", a.costBreakdown)
		mux.HandleFunc("GET /api/v1/boms/{id}/budgets", a.budgets)
	}
	if a.snapCosts != nil {
		mux.HandleFunc("GET /api/v1/snapshots/{id}/nodes/{nodeID}/cost-breakdown", a.snapshotCostBreakdown)
	}
}

// --- BOM ---------------------------------------------------------------------------------------------

func (a *api) listBOMs(w http.ResponseWriter, r *http.Request) {
	bs, err := a.boms.ListBOMs(r.Context())
	if err != nil {
		writeError(w, r, err)
		return
	}
	out := make([]bomJSON, len(bs))
	for i, b := range bs {
		out[i] = toBOMJSON(b)
	}
	writeJSON(w, http.StatusOK, out)
}

type createBOMRequest struct {
	Name     string `json:"name"`
	Currency string `json:"currency"`
	Volume   int32  `json:"volume"`
	// AnnualHours : absent → 1 607 h (ADR 0024).
	AnnualHours *decimal.Decimal `json:"annual_hours"`
}

func (a *api) createBOM(w http.ResponseWriter, r *http.Request) {
	var req createBOMRequest
	if err := decode(r, &req); err != nil {
		writeError(w, r, err)
		return
	}
	b, err := a.boms.CreateBOM(r.Context(), bom.NewBOM{
		Name: req.Name, Currency: req.Currency, Volume: req.Volume, AnnualHours: req.AnnualHours,
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, toBOMJSON(b))
}

// patchBOMRequest : soit name (renommer), soit currency et volume (réglages), soit annual_hours (ADR 0024),
// soit destination_country (code pays ou null, ADR 0025), un seul groupe par requête pour qu'un PATCH ne soit
// jamais appliqué à moitié.
type patchBOMRequest struct {
	Name        *string          `json:"name"`
	Currency    *string          `json:"currency"`
	Volume      *int32           `json:"volume"`
	AnnualHours *decimal.Decimal `json:"annual_hours"`
	Destination json.RawMessage  `json:"destination_country"`
}

func (a *api) patchBOM(w http.ResponseWriter, r *http.Request) {
	id, err := bomPathID(r, "id")
	if err != nil {
		writeError(w, r, err)
		return
	}
	var req patchBOMRequest
	if err := decode(r, &req); err != nil {
		writeError(w, r, err)
		return
	}
	groups := 0
	for _, present := range []bool{req.Name != nil, req.Currency != nil || req.Volume != nil, req.AnnualHours != nil, req.Destination != nil} {
		if present {
			groups++
		}
	}
	var b bom.BOM
	switch {
	case groups != 1:
		err = fmt.Errorf("send either name, or currency and volume together, or annual_hours, or destination_country: %w", errBadRequest)
	case req.Name != nil:
		b, err = a.boms.RenameBOM(r.Context(), id, *req.Name)
	case req.Currency != nil && req.Volume != nil:
		b, err = a.boms.UpdateSettings(r.Context(), id, *req.Currency, *req.Volume)
	case req.AnnualHours != nil:
		b, err = a.boms.SetAnnualHours(r.Context(), id, *req.AnnualHours)
	case req.Destination != nil:
		var c *string
		if string(req.Destination) != "null" {
			c = new(string)
			if jerr := json.Unmarshal(req.Destination, c); jerr != nil {
				err = fmt.Errorf("destination_country must be a string or null: %w", errBadRequest)
				break
			}
		}
		b, err = a.boms.SetDestinationCountry(r.Context(), id, c)
	default:
		err = fmt.Errorf("send currency and volume together: %w", errBadRequest)
	}
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toBOMJSON(b))
}

func (a *api) deleteBOM(w http.ResponseWriter, r *http.Request) {
	id, err := bomPathID(r, "id")
	if err == nil {
		err = a.boms.DeleteBOM(r.Context(), id)
	}
	if err != nil {
		writeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *api) tree(w http.ResponseWriter, r *http.Request) {
	id, err := bomPathID(r, "id")
	if err != nil {
		writeError(w, r, err)
		return
	}
	t, err := a.boms.Totals(r.Context(), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	nodes := make([]treeNodeJSON, len(t.Nodes))
	for i, n := range t.Nodes {
		nodes[i] = treeNodeJSON{nodeJSON: toNodeJSON(n), Totals: nodeTotals(t.Totals, n.ID)}
	}
	writeJSON(w, http.StatusOK, treeJSON{
		BOM: toBOMJSON(t.BOM), Nodes: nodes, Explanation: toExplanationJSON(t.Totals.Explanation), Stock: toStockJSON(t.Stock),
		Planning: toPlanningJSON(t.Planning), Budgets: toBudgetsJSON(t.Budgets),
	})
}

// --- Nœuds -------------------------------------------------------------------------------------------

type addNodeRequest struct {
	ParentID       *int64          `json:"parent_id"`
	Name           string          `json:"name"`
	PartID         *int64          `json:"part_id"`
	OfferID        *int64          `json:"offer_id"`
	Quantity       decimal.Decimal `json:"quantity"`
	ProductionDays int32           `json:"production_days"`
}

func (a *api) addNode(w http.ResponseWriter, r *http.Request) {
	bomID, err := bomPathID(r, "id")
	if err != nil {
		writeError(w, r, err)
		return
	}
	var req addNodeRequest
	if err := decode(r, &req); err != nil {
		writeError(w, r, err)
		return
	}
	if req.ParentID == nil {
		writeError(w, r, fmt.Errorf("parent_id is required: %w", errBadRequest))
		return
	}
	n, err := a.boms.AddNode(r.Context(), bomID, bom.NewNode{
		ParentID: *req.ParentID, Name: req.Name, PartID: req.PartID, OfferID: req.OfferID,
		Quantity: req.Quantity, ProductionDays: req.ProductionDays,
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, toNodeJSON(n))
}

// patchNodeRequest : exactement une opération par requête (changer l'offre, la quantité, le parent, les jours
// de production, la stratégie, ou part_id et offer_id ensemble pour appliquer une suggestion, ADR 0028),
// chacune étant une opération distincte du service : pas de PATCH appliqué à moitié.
// strategy : "cost" | "lead_time" | "co2" | "balanced", ou null pour la retirer (RawMessage : absent ≠ null).
type patchNodeRequest struct {
	OfferID        *int64           `json:"offer_id"`
	PartID         *int64           `json:"part_id"`
	Quantity       *decimal.Decimal `json:"quantity"`
	ParentID       *int64           `json:"parent_id"`
	ProductionDays *int32           `json:"production_days"`
	Strategy       json.RawMessage  `json:"strategy"`
}

func (a *api) patchNode(w http.ResponseWriter, r *http.Request) {
	bomID, err := bomPathID(r, "id")
	if err != nil {
		writeError(w, r, err)
		return
	}
	nodeID, err := bomPathID(r, "nodeID")
	if err != nil {
		writeError(w, r, err)
		return
	}
	var req patchNodeRequest
	if err := decode(r, &req); err != nil {
		writeError(w, r, err)
		return
	}
	replace := req.PartID != nil && req.OfferID != nil
	set := 0
	for _, present := range []bool{
		req.OfferID != nil || req.PartID != nil, req.Quantity != nil, req.ParentID != nil, req.ProductionDays != nil,
		req.Strategy != nil,
	} {
		if present {
			set++
		}
	}
	if set != 1 || (req.PartID != nil && !replace) {
		writeError(w, r, fmt.Errorf(
			"send exactly one of offer_id, part_id with offer_id, quantity, parent_id, production_days, strategy: %w", errBadRequest))
		return
	}
	ctx := r.Context()
	var n bom.Node
	switch {
	case req.Strategy != nil:
		var st *bom.Strategy
		if st, err = decodeStrategy(req.Strategy); err == nil {
			n, err = a.boms.SetNodeStrategy(ctx, bomID, nodeID, st)
		}
	case replace:
		n, err = a.boms.ReplaceNodePart(ctx, bomID, nodeID, *req.PartID, *req.OfferID)
	case req.OfferID != nil:
		n, err = a.boms.SetNodeOffer(ctx, bomID, nodeID, *req.OfferID)
	case req.Quantity != nil:
		n, err = a.boms.SetNodeQuantity(ctx, bomID, nodeID, *req.Quantity)
	case req.ParentID != nil:
		n, err = a.boms.MoveNode(ctx, bomID, nodeID, *req.ParentID)
	default:
		n, err = a.boms.SetNodeProductionDays(ctx, bomID, nodeID, *req.ProductionDays)
	}
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toNodeJSON(n))
}

// decodeStrategy : null → nil (retirer) ; une chaîne → la stratégie, validée par le service.
func decodeStrategy(raw json.RawMessage) (*bom.Strategy, error) {
	if string(raw) == "null" {
		return nil, nil
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, fmt.Errorf("strategy must be a string or null: %w", errBadRequest)
	}
	st := bom.Strategy(s)
	return &st, nil
}

func (a *api) suggestions(w http.ResponseWriter, r *http.Request) {
	bomID, err := bomPathID(r, "id")
	if err != nil {
		writeError(w, r, err)
		return
	}
	nodeID, err := bomPathID(r, "nodeID")
	if err != nil {
		writeError(w, r, err)
		return
	}
	sg, err := a.boms.Suggestions(r.Context(), bomID, nodeID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toSuggestionsJSON(sg))
}

func (a *api) deleteNode(w http.ResponseWriter, r *http.Request) {
	bomID, err := bomPathID(r, "id")
	if err != nil {
		writeError(w, r, err)
		return
	}
	nodeID, err := bomPathID(r, "nodeID")
	if err == nil {
		err = a.boms.DeleteNode(r.Context(), bomID, nodeID)
	}
	if err != nil {
		writeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- Anomalies de coût (ADR 0027) ---------------------------------------------------------------------

// anomalies : ?threshold=3 (décimal > 1, défaut 3).
func (a *api) anomalies(w http.ResponseWriter, r *http.Request) {
	id, err := bomPathID(r, "id")
	if err != nil {
		writeError(w, r, err)
		return
	}
	threshold := pricing.DefaultAnomalyThreshold
	if raw := r.URL.Query().Get("threshold"); raw != "" {
		if threshold, err = decimal.NewFromString(raw); err != nil {
			writeError(w, r, fmt.Errorf("threshold %q is not a decimal: %w", raw, errBadRequest))
			return
		}
	}
	res, err := a.boms.CostAnomalies(r.Context(), id, threshold)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toAnomaliesJSON(res))
}

// --- Coût total rendu (ADR 0025) -----------------------------------------------------------------------

// landedCost : GET /api/v1/parts/{id}/landed-cost?bom_id=…&offer_ids=1,2 (ADR 0025) — offres d'une même pièce
// comparées côte à côte pour cette BOM.
func (a *api) landedCost(w http.ResponseWriter, r *http.Request) {
	partID, err := bomPathID(r, "id")
	if err != nil {
		writeError(w, r, err)
		return
	}
	q := r.URL.Query()
	bomID, err := strconv.ParseInt(q.Get("bom_id"), 10, 64)
	if err != nil || bomID <= 0 {
		writeError(w, r, fmt.Errorf("bom_id %q is not a positive integer: %w", q.Get("bom_id"), errBadRequest))
		return
	}
	var offers []int64
	for _, raw := range strings.Split(q.Get("offer_ids"), ",") {
		id, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
		if err != nil || id <= 0 {
			writeError(w, r, fmt.Errorf("offer_ids must be positive integers separated by commas: %w", errBadRequest))
			return
		}
		offers = append(offers, id)
	}
	res, err := a.boms.LandedCosts(r.Context(), bomID, partID, offers)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toLandedJSON(res))
}

// --- Make or buy (ADR 0026) -----------------------------------------------------------------------------

type makeOrBuyRequest struct {
	FixedCosts       *money.Money `json:"fixed_costs"`
	LogisticsPerUnit *money.Money `json:"logistics_per_unit"`
	Tier1Price       *money.Money `json:"tier1_price"`
	Tier1Origin      string       `json:"tier1_origin"`
	Tier1Packaging   *money.Money `json:"tier1_packaging"`
}

// makeOrBuy : simulation sans stockage ; tous les champs sont requis (un zéro se saisit, il n'est pas supposé).
func (a *api) makeOrBuy(w http.ResponseWriter, r *http.Request) {
	bomID, err := bomPathID(r, "id")
	if err != nil {
		writeError(w, r, err)
		return
	}
	nodeID, err := bomPathID(r, "nodeID")
	if err != nil {
		writeError(w, r, err)
		return
	}
	var req makeOrBuyRequest
	if err := decode(r, &req); err != nil {
		writeError(w, r, err)
		return
	}
	if req.FixedCosts == nil || req.LogisticsPerUnit == nil || req.Tier1Price == nil || req.Tier1Packaging == nil {
		writeError(w, r, fmt.Errorf("fixed_costs, logistics_per_unit, tier1_price and tier1_packaging are required: %w", errBadRequest))
		return
	}
	res, err := a.boms.MakeOrBuy(r.Context(), bomID, nodeID, bom.MakeOrBuyRequest{
		FixedCosts: *req.FixedCosts, LogisticsPerUnit: *req.LogisticsPerUnit, Tier1Price: *req.Tier1Price,
		Tier1Origin: req.Tier1Origin, Tier1Packaging: *req.Tier1Packaging,
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toMakeOrBuyJSON(res))
}

// --- Stock (ADR 0022) ---------------------------------------------------------------------------------

func (a *api) stock(w http.ResponseWriter, r *http.Request) {
	id, err := bomPathID(r, "id")
	if err != nil {
		writeError(w, r, err)
		return
	}
	st, err := a.boms.Stock(r.Context(), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toStockJSON(st))
}

type putStockRequest struct {
	Quantity *decimal.Decimal `json:"quantity"`
}

func (a *api) putStock(w http.ResponseWriter, r *http.Request) {
	bomID, err := bomPathID(r, "id")
	if err != nil {
		writeError(w, r, err)
		return
	}
	partID, err := bomPathID(r, "partID")
	if err != nil {
		writeError(w, r, err)
		return
	}
	var req putStockRequest
	if err := decode(r, &req); err != nil {
		writeError(w, r, err)
		return
	}
	if req.Quantity == nil {
		writeError(w, r, fmt.Errorf("quantity is required: %w", errBadRequest))
		return
	}
	st, err := a.boms.SetPartStock(r.Context(), bomID, partID, *req.Quantity)
	if errors.Is(err, catalog.ErrPartNotFound) { // la pièce est dans le chemin : ressource introuvable
		writeErrorBody(w, http.StatusNotFound, err.Error(), "part_not_found")
		return
	}
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, stockJSON{PartID: st.PartID, Quantity: st.Quantity.String()})
}

// --- Budgets cibles (ADR 0042) ---------------------------------------------------------------------------

type targetBudgetJSON struct {
	NodeID int64       `json:"node_id"`
	Target money.Money `json:"target"`
}

// putBudget : {"amount":"3000.00","currency":"EUR"} ; 409 si les sous-budgets d'un ensemble dépasseraient son budget.
func (a *api) putBudget(w http.ResponseWriter, r *http.Request) {
	bomID, err := bomPathID(r, "id")
	var nodeID int64
	if err == nil {
		nodeID, err = bomPathID(r, "nodeID")
	}
	var req money.Money
	if err == nil {
		err = decode(r, &req)
	}
	var b bom.TargetBudget
	if err == nil {
		b, err = a.boms.PutTargetBudget(r.Context(), bomID, nodeID, req)
	}
	if err != nil {
		writeError(w, r, err)
		return
	}
	target, err := money.New(b.Amount, b.Currency)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, targetBudgetJSON{NodeID: b.NodeID, Target: target})
}

func (a *api) deleteBudget(w http.ResponseWriter, r *http.Request) {
	bomID, err := bomPathID(r, "id")
	var nodeID int64
	if err == nil {
		nodeID, err = bomPathID(r, "nodeID")
	}
	if err == nil {
		err = a.boms.DeleteTargetBudget(r.Context(), bomID, nodeID)
	}
	if err != nil {
		writeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- Planning (ADR 0023) -----------------------------------------------------------------------------

// putScheduleRequest : dates calendaires "AAAA-MM-JJ" ; target_on absent ou null = pas de date cible.
type putScheduleRequest struct {
	StartOn  *string `json:"start_on"`
	TargetOn *string `json:"target_on"`
}

func (a *api) putSchedule(w http.ResponseWriter, r *http.Request) {
	id, err := bomPathID(r, "id")
	if err != nil {
		writeError(w, r, err)
		return
	}
	var req putScheduleRequest
	if err := decode(r, &req); err != nil {
		writeError(w, r, err)
		return
	}
	if req.StartOn == nil {
		writeError(w, r, fmt.Errorf("start_on is required: %w", errBadRequest))
		return
	}
	var sch bom.Schedule
	if sch.StartOn, err = parseDay("start_on", *req.StartOn); err != nil {
		writeError(w, r, err)
		return
	}
	if req.TargetOn != nil {
		t, err := parseDay("target_on", *req.TargetOn)
		if err != nil {
			writeError(w, r, err)
			return
		}
		sch.TargetOn = &t
	}
	out, err := a.boms.SetSchedule(r.Context(), id, sch)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toScheduleJSON(&out))
}

func (a *api) deleteSchedule(w http.ResponseWriter, r *http.Request) {
	id, err := bomPathID(r, "id")
	if err == nil {
		err = a.boms.ClearSchedule(r.Context(), id)
	}
	if err != nil {
		writeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// parseDay lit une date calendaire "AAAA-MM-JJ" (minuit UTC) ; l'erreur enveloppe errBadRequest.
func parseDay(field, raw string) (time.Time, error) {
	t, err := time.Parse(time.DateOnly, raw)
	if err != nil {
		return time.Time{}, fmt.Errorf("%s %q is not a date (YYYY-MM-DD): %w", field, raw, errBadRequest)
	}
	return t, nil
}

// --- Prix figés, publication, snapshots --------------------------------------------------------------

func (a *api) freezePrices(w http.ResponseWriter, r *http.Request) {
	id, err := bomPathID(r, "id")
	if err != nil {
		writeError(w, r, err)
		return
	}
	b, err := a.boms.FreezePrices(r.Context(), id, a.now())
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toBOMJSON(b))
}

// usePriceSnapshot attache un snapshot de prix existant au brouillon : {"price_snapshot_id": N}.
// Couverture incomplète → 409 avec les offres et paires manquantes ; snapshot inconnu → 404.
func (a *api) usePriceSnapshot(w http.ResponseWriter, r *http.Request) {
	id, err := bomPathID(r, "id")
	if err != nil {
		writeError(w, r, err)
		return
	}
	var req struct {
		PriceSnapshotID *int64 `json:"price_snapshot_id"`
	}
	if err := readBody(w, r, &req, false); err != nil {
		writeError(w, r, err)
		return
	}
	if req.PriceSnapshotID == nil || *req.PriceSnapshotID <= 0 {
		writeError(w, r, fmt.Errorf("price_snapshot_id is required and must be a positive integer: %w", errBadRequest))
		return
	}
	b, err := a.boms.UsePriceSnapshot(r.Context(), id, *req.PriceSnapshotID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toBOMJSON(b))
}

func (a *api) unfreezePrices(w http.ResponseWriter, r *http.Request) {
	id, err := bomPathID(r, "id")
	if err != nil {
		writeError(w, r, err)
		return
	}
	b, err := a.boms.UnfreezePrices(r.Context(), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toBOMJSON(b))
}

func (a *api) publish(w http.ResponseWriter, r *http.Request) {
	id, err := bomPathID(r, "id")
	if err != nil {
		writeError(w, r, err)
		return
	}
	var req publishRequest
	if err := decodeOptional(r, &req); err != nil {
		writeError(w, r, err)
		return
	}
	s, err := a.snapshots.Publish(r.Context(), id, a.now(), snapshot.PublishRequest{
		Note: req.Note, ProposedBy: req.ProposedBy, ValidatedBy: req.ValidatedBy, Justifications: req.Justifications,
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, toSnapshotJSON(s))
}

func (a *api) getSnapshot(w http.ResponseWriter, r *http.Request) {
	id, err := bomPathID(r, "id")
	if err != nil {
		writeError(w, r, err)
		return
	}
	p, err := a.snapshots.Get(r.Context(), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	nodes := make([]treeNodeJSON, len(p.Nodes))
	for i, n := range p.Nodes {
		nj := nodeJSON{
			ID: n.Key, ParentID: n.ParentKey, Name: n.Name,
			Quantity: n.Quantity.String(), ProductionDays: n.ProductionDays, Strategy: strategyJSON(n.Strategy),
		}
		if n.Part != nil {
			nj.PartID, nj.OfferID = &n.Part.PartID, &n.Part.OfferID
		}
		nodes[i] = treeNodeJSON{nodeJSON: nj, Totals: nodeTotals(p.Totals, n.Key)}
	}
	writeJSON(w, http.StatusOK, publishedJSON{
		Snapshot: toSnapshotJSON(p.Snapshot), Nodes: nodes, Explanation: toExplanationJSON(p.Totals.Explanation),
		Stock: toStockJSON(p.Stock), Planning: toPlanningJSON(p.Planning), Checks: toFrozenChecksJSON(p.Checks),
	})
}

func (a *api) newDraft(w http.ResponseWriter, r *http.Request) {
	id, err := bomPathID(r, "id")
	if err != nil {
		writeError(w, r, err)
		return
	}
	b, err := a.snapshots.NewDraft(r.Context(), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, toBOMJSON(b))
}

// --- Décodage et erreurs -----------------------------------------------------------------------------

// bomPathID lit un identifiant de chemin ; l'erreur enveloppe errBadRequest (statusOf → 400).
func bomPathID(r *http.Request, name string) (int64, error) {
	raw := r.PathValue(name)
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("%s %q is not a positive integer: %w", name, raw, errBadRequest)
	}
	return id, nil
}

// decode lit un seul objet JSON (1 Mio max), en refusant les champs inconnus : une faute de frappe dans un
// nom de champ est une erreur, pas un champ ignoré en silence.
func decode(r *http.Request, v any) error {
	dec := json.NewDecoder(io.LimitReader(r.Body, maxBodyBytes))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return fmt.Errorf("invalid JSON body: %w: %w", err, errBadRequest)
	}
	if dec.More() {
		return fmt.Errorf("invalid JSON body: trailing data: %w", errBadRequest)
	}
	return nil
}

// publishRequest : corps facultatif de la publication tracée (ADR 0033).
type publishRequest struct {
	Note           string            `json:"note"`
	ProposedBy     string            `json:"proposed_by"`
	ValidatedBy    string            `json:"validated_by"`
	Justifications map[string]string `json:"justifications"`
}

// decodeOptional lit un objet JSON comme decode, mais un corps vide laisse v à sa valeur zéro.
func decodeOptional(r *http.Request, v any) error {
	body, err := io.ReadAll(io.LimitReader(r.Body, maxBodyBytes))
	if err != nil {
		return fmt.Errorf("read body: %w: %w", err, errBadRequest)
	}
	if len(strings.TrimSpace(string(body))) == 0 {
		return nil
	}
	dec := json.NewDecoder(strings.NewReader(string(body)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return fmt.Errorf("invalid JSON body: %w: %w", err, errBadRequest)
	}
	if dec.More() {
		return fmt.Errorf("invalid JSON body: trailing data: %w", errBadRequest)
	}
	return nil
}

// statusOf : statut HTTP d'une erreur du domaine, lu dans errorTable (un seul endroit, avec le code stable).
func statusOf(err error) int {
	if e, ok := lookupError(err); ok {
		return e.status
	}
	return http.StatusInternalServerError
}

// writeError écrit {"error": "...", "code": "..."} (code stable, voir errorTable) ; le détail d'une erreur serveur est journalisé, pas renvoyé.
func writeError(w http.ResponseWriter, r *http.Request, err error) {
	status := statusOf(err)
	msg := err.Error()
	if status == http.StatusInternalServerError {
		slog.ErrorContext(r.Context(), "request failed", "method", r.Method, "path", r.URL.Path, "err", err)
		msg = "internal error"
	}
	writeErrorBody(w, status, msg, errorCode(err))
}
