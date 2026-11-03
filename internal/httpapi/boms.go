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
	DeleteBOM(ctx context.Context, id int64) error
	AddNode(ctx context.Context, bomID int64, n bom.NewNode) (bom.Node, error)
	SetNodeOffer(ctx context.Context, bomID, nodeID, offerID int64) (bom.Node, error)
	SetNodeQuantity(ctx context.Context, bomID, nodeID int64, qty decimal.Decimal) (bom.Node, error)
	SetNodeProductionDays(ctx context.Context, bomID, nodeID int64, days int32) (bom.Node, error)
	MoveNode(ctx context.Context, bomID, nodeID, newParentID int64) (bom.Node, error)
	DeleteNode(ctx context.Context, bomID, nodeID int64) error
	FreezePrices(ctx context.Context, bomID int64, at time.Time) (bom.BOM, error)
	UnfreezePrices(ctx context.Context, bomID int64) (bom.BOM, error)
	Totals(ctx context.Context, bomID int64) (bom.TreeTotals, error)
}

// SnapshotService est ce que l'API attend du domaine snapshot ; *snapshot.Service la satisfait.
type SnapshotService interface {
	Publish(ctx context.Context, bomID int64, at time.Time) (snapshot.Snapshot, error)
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
	now       func() time.Time
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
	mux.HandleFunc("POST /api/v1/boms/{id}/freeze-prices", a.freezePrices)
	mux.HandleFunc("DELETE /api/v1/boms/{id}/freeze-prices", a.unfreezePrices)
	mux.HandleFunc("POST /api/v1/boms/{id}/publish", a.publish)
	mux.HandleFunc("GET /api/v1/snapshots/{id}", a.getSnapshot)
	mux.HandleFunc("POST /api/v1/snapshots/{id}/drafts", a.newDraft)
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
}

func (a *api) createBOM(w http.ResponseWriter, r *http.Request) {
	var req createBOMRequest
	if err := decode(r, &req); err != nil {
		writeError(w, r, err)
		return
	}
	b, err := a.boms.CreateBOM(r.Context(), bom.NewBOM{Name: req.Name, Currency: req.Currency, Volume: req.Volume})
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, toBOMJSON(b))
}

// patchBOMRequest : soit name (renommer), soit currency et volume (réglages), un seul groupe par requête
// pour qu'un PATCH ne soit jamais appliqué à moitié.
type patchBOMRequest struct {
	Name     *string `json:"name"`
	Currency *string `json:"currency"`
	Volume   *int32  `json:"volume"`
}

func (a *api) patchBOM(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, r, err)
		return
	}
	var req patchBOMRequest
	if err := decode(r, &req); err != nil {
		writeError(w, r, err)
		return
	}
	settings := req.Currency != nil || req.Volume != nil
	var b bom.BOM
	switch {
	case req.Name != nil && !settings:
		b, err = a.boms.RenameBOM(r.Context(), id, *req.Name)
	case req.Name == nil && req.Currency != nil && req.Volume != nil:
		b, err = a.boms.UpdateSettings(r.Context(), id, *req.Currency, *req.Volume)
	default:
		err = fmt.Errorf("send either name, or currency and volume together: %w", errBadRequest)
	}
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toBOMJSON(b))
}

func (a *api) deleteBOM(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
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
	id, err := pathID(r, "id")
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
		BOM: toBOMJSON(t.BOM), Nodes: nodes, Explanation: toExplanationJSON(t.Totals.Explanation),
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
	bomID, err := pathID(r, "id")
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

// patchNodeRequest : exactement un champ par requête (changer l'offre, la quantité, le parent ou les jours
// de production), chacun étant une opération distincte du service : pas de PATCH appliqué à moitié.
type patchNodeRequest struct {
	OfferID        *int64           `json:"offer_id"`
	Quantity       *decimal.Decimal `json:"quantity"`
	ParentID       *int64           `json:"parent_id"`
	ProductionDays *int32           `json:"production_days"`
}

func (a *api) patchNode(w http.ResponseWriter, r *http.Request) {
	bomID, err := pathID(r, "id")
	if err != nil {
		writeError(w, r, err)
		return
	}
	nodeID, err := pathID(r, "nodeID")
	if err != nil {
		writeError(w, r, err)
		return
	}
	var req patchNodeRequest
	if err := decode(r, &req); err != nil {
		writeError(w, r, err)
		return
	}
	set := 0
	for _, present := range []bool{req.OfferID != nil, req.Quantity != nil, req.ParentID != nil, req.ProductionDays != nil} {
		if present {
			set++
		}
	}
	if set != 1 {
		writeError(w, r, fmt.Errorf("send exactly one of offer_id, quantity, parent_id, production_days: %w", errBadRequest))
		return
	}
	ctx := r.Context()
	var n bom.Node
	switch {
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

func (a *api) deleteNode(w http.ResponseWriter, r *http.Request) {
	bomID, err := pathID(r, "id")
	if err != nil {
		writeError(w, r, err)
		return
	}
	nodeID, err := pathID(r, "nodeID")
	if err == nil {
		err = a.boms.DeleteNode(r.Context(), bomID, nodeID)
	}
	if err != nil {
		writeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- Prix figés, publication, snapshots --------------------------------------------------------------

func (a *api) freezePrices(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
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

func (a *api) unfreezePrices(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
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
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, r, err)
		return
	}
	s, err := a.snapshots.Publish(r.Context(), id, a.now())
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, toSnapshotJSON(s))
}

func (a *api) getSnapshot(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
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
			Quantity: n.Quantity.String(), ProductionDays: n.ProductionDays,
		}
		if n.Part != nil {
			nj.PartID, nj.OfferID = &n.Part.PartID, &n.Part.OfferID
		}
		nodes[i] = treeNodeJSON{nodeJSON: nj, Totals: nodeTotals(p.Totals, n.Key)}
	}
	writeJSON(w, http.StatusOK, publishedJSON{
		Snapshot: toSnapshotJSON(p.Snapshot), Nodes: nodes, Explanation: toExplanationJSON(p.Totals.Explanation),
	})
}

func (a *api) newDraft(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
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

func pathID(r *http.Request, name string) (int64, error) {
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

// statusOf traduit une erreur métier en code HTTP. Une erreur inconnue est une erreur serveur.
func statusOf(err error) int {
	is := func(targets ...error) bool {
		for _, t := range targets {
			if errors.Is(err, t) {
				return true
			}
		}
		return false
	}
	switch {
	case is(bom.ErrBOMNotFound, bom.ErrNodeNotFound, snapshot.ErrSnapshotNotFound):
		return http.StatusNotFound
	case is(bom.ErrCycle, bom.ErrHasSnapshots, bom.ErrRootNode, pricing.ErrNoQuote, pricing.ErrNoRate):
		return http.StatusConflict
	case is(errBadRequest, bom.ErrInvalidInput, bom.ErrOfferNotForPart, bom.ErrNotAPart,
		catalog.ErrPartNotFound, catalog.ErrOfferNotFound, money.ErrUnknownCurrency):
		return http.StatusBadRequest
	default:
		return http.StatusInternalServerError
	}
}

// writeError écrit {"error": "..."} ; le détail d'une erreur serveur est journalisé, pas renvoyé.
func writeError(w http.ResponseWriter, r *http.Request, err error) {
	status := statusOf(err)
	msg := err.Error()
	if status == http.StatusInternalServerError {
		slog.ErrorContext(r.Context(), "request failed", "method", r.Method, "path", r.URL.Path, "err", err)
		msg = "internal error"
	}
	writeJSON(w, status, map[string]string{"error": msg})
}
