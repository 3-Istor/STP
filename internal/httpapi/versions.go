package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"bomstudio/internal/bom"
	"bomstudio/internal/money"
	"bomstudio/internal/snapshot"
)

// Versions et publication (ADR 0032, 0033) : comparaison, contrôles avant publication, versions d'une BOM et
// pistes de réduction. Handlers fins : décoder, appeler le domaine, encoder.

// VersionsService : comparaison et versions ; *snapshot.Service la satisfait.
type VersionsService interface {
	Compare(ctx context.Context, from, to snapshot.Ref) (snapshot.Comparison, error)
	Versions(ctx context.Context, bomID int64) ([]snapshot.Version, error)
}

// ReviewService : contrôles avant publication et pistes de réduction ; *bom.Service la satisfait.
type ReviewService interface {
	PublicationChecks(ctx context.Context, bomID int64) ([]bom.PublicationCheck, error)
	Ideas(ctx context.Context, bomID int64) (bom.IdeasSummary, error)
	CreateIdea(ctx context.Context, bomID int64, in bom.IdeaInput) (bom.Idea, error)
	UpdateIdea(ctx context.Context, bomID, id int64, p bom.IdeaPatch) (bom.Idea, error)
	DeleteIdea(ctx context.Context, bomID, id int64) error
}

var (
	_ VersionsService = (*snapshot.Service)(nil)
	_ ReviewService   = (*bom.Service)(nil)
)

func (a *api) versionRoutes(mux *http.ServeMux) {
	if a.versions != nil {
		mux.HandleFunc("GET /api/v1/compare", a.compare)
		mux.HandleFunc("GET /api/v1/boms/{id}/versions", a.listVersions)
	}
	if a.review != nil {
		mux.HandleFunc("GET /api/v1/boms/{id}/publication-checks", a.publicationChecks)
		mux.HandleFunc("GET /api/v1/boms/{id}/ideas", a.listIdeas)
		mux.HandleFunc("POST /api/v1/boms/{id}/ideas", a.createIdea)
		mux.HandleFunc("PATCH /api/v1/boms/{id}/ideas/{ideaID}", a.patchIdea)
		mux.HandleFunc("DELETE /api/v1/boms/{id}/ideas/{ideaID}", a.deleteIdea)
	}
}

// --- Comparaison --------------------------------------------------------------------------------------

type versionTotalsJSON struct {
	Cost         money.Money `json:"cost"`
	CO2Kg        string      `json:"co2_kg"`
	MassKg       string      `json:"mass_kg"`
	LeadTimeDays int         `json:"lead_time_days"`
}

type compareSideJSON struct {
	Ref             string            `json:"ref"`
	Label           string            `json:"label"`
	Name            string            `json:"name"`
	Method          costMethodJSON    `json:"method"`
	PriceSnapshotID *int64            `json:"price_snapshot_id"`
	Totals          versionTotalsJSON `json:"totals"`
}

type causeJSON struct {
	Kind        string      `json:"kind"`
	Amount      money.Money `json:"amount"`
	Explanation string      `json:"explanation"`
	NodeIDs     []int64     `json:"node_ids"`
	FromNodeIDs []int64     `json:"from_node_ids"`
	NodeCount   int         `json:"node_count"`
}

type changedPartJSON struct {
	Change     string      `json:"change"`
	Path       string      `json:"path"`
	NodeID     *int64      `json:"node_id"`
	FromNodeID *int64      `json:"from_node_id"`
	PartID     int64       `json:"part_id"`
	Before     string      `json:"before"`
	After      string      `json:"after"`
	Delta      money.Money `json:"delta"`
}

// compareJSON : montants exacts (arrondi à l'affichage, ADR 0004) ; Σ causes[].amount = delta.cost exactement.
type compareJSON struct {
	From         compareSideJSON   `json:"from"`
	To           compareSideJSON   `json:"to"`
	Delta        versionTotalsJSON `json:"delta"`
	Causes       []causeJSON       `json:"causes"`
	ChangedParts []changedPartJSON `json:"changed_parts"`
}

func toCompareSide(v snapshot.VersionTotals) compareSideJSON {
	return compareSideJSON{Ref: v.Ref.String(), Label: v.Label, Name: v.Name, Method: toCostMethodJSON(v.Method),
		PriceSnapshotID: v.PriceSnapshotID, Totals: versionTotalsJSON{Cost: v.Cost, CO2Kg: v.CO2Kg.String(),
			MassKg: v.MassKg.String(), LeadTimeDays: v.LeadTimeDays}}
}

func ids(v []int64) []int64 {
	if v == nil {
		return []int64{}
	}
	return v
}

func (a *api) compare(w http.ResponseWriter, r *http.Request) {
	from, err := snapshot.ParseRef(r.URL.Query().Get("from"))
	var to snapshot.Ref
	if err == nil {
		to, err = snapshot.ParseRef(r.URL.Query().Get("to"))
	}
	var c snapshot.Comparison
	if err == nil {
		c, err = a.versions.Compare(r.Context(), from, to)
	}
	if err != nil {
		writeError(w, r, err)
		return
	}
	out := compareJSON{From: toCompareSide(c.From), To: toCompareSide(c.To),
		Delta: versionTotalsJSON{Cost: c.Delta.Cost, CO2Kg: c.Delta.CO2Kg.String(), MassKg: c.Delta.MassKg.String(),
			LeadTimeDays: c.Delta.LeadTimeDays},
		Causes: make([]causeJSON, len(c.Causes)), ChangedParts: make([]changedPartJSON, len(c.ChangedParts))}
	for i, k := range c.Causes {
		out.Causes[i] = causeJSON{Kind: string(k.Kind), Amount: k.Amount, Explanation: k.Explanation, NodeIDs: ids(k.NodeIDs),
			FromNodeIDs: ids(k.FromNodeIDs), NodeCount: k.NodeCount}
	}
	for i, p := range c.ChangedParts {
		out.ChangedParts[i] = changedPartJSON{Change: p.Change, Path: p.Path, NodeID: p.NodeID, FromNodeID: p.FromNodeID,
			PartID: p.PartID, Before: p.Before, After: p.After, Delta: p.Delta}
	}
	writeJSON(w, http.StatusOK, out)
}

// --- Versions -----------------------------------------------------------------------------------------

type versionJSON struct {
	Kind             string         `json:"kind"` // draft | snapshot
	ID               int64          `json:"id"`
	BOMID            int64          `json:"bom_id"`
	Ref              string         `json:"ref"` // à passer à /compare
	Label            string         `json:"label"`
	Name             string         `json:"name"`
	At               time.Time      `json:"at"` // création (brouillon) ou publication (snapshot), à la seconde
	SourceSnapshotID *int64         `json:"source_snapshot_id"`
	Method           costMethodJSON `json:"method"`
	Cost             *money.Money   `json:"cost"`
	CostError        *string        `json:"cost_error"`
	TargetBudget     *money.Money   `json:"target_budget"`
	BudgetPercent    *string        `json:"budget_percent"`
	PriceSnapshotID  *int64         `json:"price_snapshot_id"`
	ProposedBy       *string        `json:"proposed_by"`
	ValidatedBy      *string        `json:"validated_by"`
	Note             *string        `json:"note"`
	PartCount        int            `json:"part_count"`
	NodeCount        int            `json:"node_count"`
}

func (a *api) listVersions(w http.ResponseWriter, r *http.Request) {
	id, err := bomPathID(r, "id")
	var vs []snapshot.Version
	if err == nil {
		vs, err = a.versions.Versions(r.Context(), id)
	}
	if err != nil {
		writeError(w, r, err)
		return
	}
	out := make([]versionJSON, len(vs))
	for i, v := range vs {
		ref := snapshot.Ref{Kind: snapshot.RefBOM, ID: v.ID}
		if v.Kind == "snapshot" {
			ref.Kind = snapshot.RefSnapshot
		}
		j := versionJSON{Kind: v.Kind, ID: v.ID, BOMID: v.BOMID, Ref: ref.String(), Label: v.Label, Name: v.Name,
			At: v.At.UTC().Truncate(time.Second), SourceSnapshotID: v.SourceSnapshotID, Method: toCostMethodJSON(v.Method),
			Cost: v.Cost, TargetBudget: v.TargetBudgetRoot, PriceSnapshotID: v.PriceSnapshotID, ProposedBy: v.ProposedBy,
			ValidatedBy: v.ValidatedBy, Note: v.Note, PartCount: v.PartCount, NodeCount: v.NodeCount}
		if v.CostError != "" {
			e := v.CostError
			j.CostError = &e
		}
		if v.BudgetPercent != nil {
			p := v.BudgetPercent.StringFixed(1)
			j.BudgetPercent = &p
		}
		out[i] = j
	}
	writeJSON(w, http.StatusOK, out)
}

// --- Contrôles avant publication ------------------------------------------------------------------------

type checkJSON struct {
	Code    string  `json:"code"`
	Level   string  `json:"level"` // ok | warn | block
	Title   string  `json:"title"`
	Detail  string  `json:"detail"`
	NodeIDs []int64 `json:"node_ids"`
}

func (a *api) publicationChecks(w http.ResponseWriter, r *http.Request) {
	id, err := bomPathID(r, "id")
	var cs []bom.PublicationCheck
	if err == nil {
		cs, err = a.review.PublicationChecks(r.Context(), id)
	}
	if err != nil {
		writeError(w, r, err)
		return
	}
	out := make([]checkJSON, len(cs))
	for i, c := range cs {
		out[i] = checkJSON{Code: c.Code, Level: string(c.Level), Title: c.Title, Detail: c.Detail, NodeIDs: ids(c.NodeIDs)}
	}
	writeJSON(w, http.StatusOK, out)
}

// --- Pistes de réduction -------------------------------------------------------------------------------

type ideaJSON struct {
	ID            int64       `json:"id"`
	Title         string      `json:"title"`
	Owner         string      `json:"owner"`
	EstimatedGain money.Money `json:"estimated_gain"`
	Status        string      `json:"status"`
	DraftBOMID    *int64      `json:"draft_bom_id"`
	CreatedAt     time.Time   `json:"created_at"`
}

func toIdeaJSON(i bom.Idea) ideaJSON {
	return ideaJSON{ID: i.ID, Title: i.Title, Owner: i.Owner, EstimatedGain: i.Gain, Status: string(i.Status),
		DraftBOMID: i.DraftBOMID, CreatedAt: i.CreatedAt}
}

type ideasJSON struct {
	Ideas       []ideaJSON   `json:"ideas"`
	OpenTotal   money.Money  `json:"open_total"`
	GapToTarget *money.Money `json:"gap_to_target"` // coût − budget global ; null sans budget global ou sans coût
	Remaining   *money.Money `json:"remaining"`     // écart − pistes ouvertes ; négatif = écart couvert
	GapError    *string      `json:"gap_error"`
}

func (a *api) listIdeas(w http.ResponseWriter, r *http.Request) {
	id, err := bomPathID(r, "id")
	var s bom.IdeasSummary
	if err == nil {
		s, err = a.review.Ideas(r.Context(), id)
	}
	if err != nil {
		writeError(w, r, err)
		return
	}
	out := ideasJSON{Ideas: make([]ideaJSON, len(s.Ideas)), OpenTotal: s.OpenTotal, GapToTarget: s.GapToTarget, Remaining: s.Remaining}
	for i, idea := range s.Ideas {
		out.Ideas[i] = toIdeaJSON(idea)
	}
	if s.GapError != "" {
		e := s.GapError
		out.GapError = &e
	}
	writeJSON(w, http.StatusOK, out)
}

type createIdeaRequest struct {
	Title         string       `json:"title"`
	Owner         string       `json:"owner"`
	EstimatedGain *money.Money `json:"estimated_gain"`
	Status        string       `json:"status"`
	DraftBOMID    *int64       `json:"draft_bom_id"`
}

func (a *api) createIdea(w http.ResponseWriter, r *http.Request) {
	id, err := bomPathID(r, "id")
	var req createIdeaRequest
	if err == nil {
		err = decode(r, &req)
	}
	if err == nil && req.EstimatedGain == nil {
		err = fmt.Errorf("estimated_gain is required: %w", errBadRequest)
	}
	var idea bom.Idea
	if err == nil {
		idea, err = a.review.CreateIdea(r.Context(), id, bom.IdeaInput{Title: req.Title, Owner: req.Owner, Gain: *req.EstimatedGain,
			Status: bom.IdeaStatus(req.Status), DraftBOMID: req.DraftBOMID})
	}
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, toIdeaJSON(idea))
}

// patchIdeaRequest : champs absents inchangés ; "draft_bom_id": null détache le brouillon.
type patchIdeaRequest struct {
	Title         *string         `json:"title"`
	Owner         *string         `json:"owner"`
	EstimatedGain *money.Money    `json:"estimated_gain"`
	Status        *string         `json:"status"`
	DraftBOMID    json.RawMessage `json:"draft_bom_id"`
}

func (a *api) patchIdea(w http.ResponseWriter, r *http.Request) {
	id, err := bomPathID(r, "id")
	var ideaID int64
	if err == nil {
		ideaID, err = bomPathID(r, "ideaID")
	}
	var req patchIdeaRequest
	if err == nil {
		err = decode(r, &req)
	}
	p := bom.IdeaPatch{Title: req.Title, Owner: req.Owner, Gain: req.EstimatedGain}
	if req.Status != nil {
		st := bom.IdeaStatus(*req.Status)
		p.Status = &st
	}
	if err == nil && len(req.DraftBOMID) > 0 {
		if bytes.Equal(bytes.TrimSpace(req.DraftBOMID), []byte("null")) {
			p.ClearDraft = true
		} else if e := json.Unmarshal(req.DraftBOMID, &p.DraftBOMID); e != nil {
			err = fmt.Errorf("draft_bom_id must be an integer or null: %w", errBadRequest)
		}
	}
	var idea bom.Idea
	if err == nil {
		idea, err = a.review.UpdateIdea(r.Context(), id, ideaID, p)
	}
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toIdeaJSON(idea))
}

func (a *api) deleteIdea(w http.ResponseWriter, r *http.Request) {
	id, err := bomPathID(r, "id")
	var ideaID int64
	if err == nil {
		ideaID, err = bomPathID(r, "ideaID")
	}
	if err == nil {
		err = a.review.DeleteIdea(r.Context(), id, ideaID)
	}
	if err != nil {
		writeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
