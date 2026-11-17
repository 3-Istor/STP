package httpapi

import (
	"errors"
	"net/http"

	"bomstudio/internal/bom"
	"bomstudio/internal/catalog"
	"bomstudio/internal/money"
	"bomstudio/internal/pricing"
	"bomstudio/internal/snapshot"
	"bomstudio/internal/supplier"
)

// apiError : une erreur nommée du domaine, son statut HTTP et son code stable (champ `code` de {"error": …}).
// Le code ne change jamais ; le texte `error` peut changer. L'interface traduit le code en français.
type apiError struct {
	err    error
	status int
	code   string
}

// errorTable est l'unique endroit qui relie une erreur du domaine à un statut et à un code. L'ordre compte :
// la première erreur qui correspond (errors.Is) l'emporte.
var errorTable = []apiError{
	{bom.ErrBOMNotFound, http.StatusNotFound, "bom_not_found"},
	{bom.ErrNodeNotFound, http.StatusNotFound, "node_not_found"},
	{snapshot.ErrSnapshotNotFound, http.StatusNotFound, "snapshot_not_found"},
	{pricing.ErrUnknownOffer, http.StatusNotFound, "unknown_offer"},
	{pricing.ErrPriceSnapshotNotFound, http.StatusNotFound, "price_snapshot_not_found"},
	{bom.ErrIdeaNotFound, http.StatusNotFound, "idea_not_found"},

	{bom.ErrCycle, http.StatusConflict, "cycle"},
	{bom.ErrHasSnapshots, http.StatusConflict, "has_snapshots"},
	{bom.ErrRootNode, http.StatusConflict, "root_node"},
	{pricing.ErrNoQuote, http.StatusConflict, "no_quote"},
	{pricing.ErrNoRate, http.StatusConflict, "no_rate"},
	{catalog.ErrMissingAttribute, http.StatusConflict, "missing_attribute"},
	{bom.ErrMissingCost, http.StatusConflict, "missing_cost"},
	{bom.ErrNoBudgetRate, http.StatusConflict, "no_budget_rate"},
	{bom.ErrNoTooling, http.StatusConflict, "no_tooling"},
	{bom.ErrNoProgramVolume, http.StatusConflict, "no_program_volume"},
	{bom.ErrNoCostDate, http.StatusConflict, "no_cost_date"},
	{bom.ErrNoCostFacts, http.StatusConflict, "no_cost_facts"},
	{snapshot.ErrIncompleteCost, http.StatusConflict, "incomplete_cost"},
	{supplier.ErrSelectionRequired, http.StatusConflict, "contract_selection_required"},
	{pricing.ErrNoCustomsDuty, http.StatusConflict, "no_customs_duty"},
	{pricing.ErrNoFreightRate, http.StatusConflict, "no_freight_rate"},
	{pricing.ErrNoPackaging, http.StatusConflict, "no_packaging"},
	{pricing.ErrNoDestination, http.StatusConflict, "no_destination"},
	{snapshot.ErrPublicationBlocked, http.StatusConflict, "publication_blocked"},
	{bom.ErrBudgetExceedsParent, http.StatusConflict, "budget_exceeds_parent"},
	{bom.ErrBridgeUnavailable, http.StatusConflict, "bridge_unavailable"},
	{bom.ErrBridgeIncomplete, http.StatusConflict, "bridge_incomplete"},
	{money.ErrCurrencyMismatch, http.StatusConflict, "currency_mismatch"},

	{snapshot.ErrJustificationRequired, http.StatusUnprocessableEntity, "justification_required"},

	{errBadRequest, http.StatusBadRequest, "bad_request"},
	{bom.ErrInvalidInput, http.StatusBadRequest, "invalid_input"},
	{pricing.ErrInvalidThreshold, http.StatusBadRequest, "invalid_threshold"},
	{bom.ErrOfferNotForPart, http.StatusBadRequest, "offer_not_for_part"},
	{bom.ErrNotAPart, http.StatusBadRequest, "not_a_part"},
	{catalog.ErrPartNotFound, http.StatusBadRequest, "part_not_found"},
	{catalog.ErrOfferNotFound, http.StatusBadRequest, "offer_not_found"},
	{money.ErrUnknownCurrency, http.StatusBadRequest, "unknown_currency"},
	{pricing.ErrInvalidLogistics, http.StatusBadRequest, "invalid_logistics"},

	{bom.ErrSuggestionsUnavailable, http.StatusServiceUnavailable, "suggestions_unavailable"},
	{bom.ErrLogisticsUnavailable, http.StatusServiceUnavailable, "logistics_unavailable"},
	{bom.ErrCostSourcesUnavailable, http.StatusServiceUnavailable, "cost_sources_unavailable"},
	{bom.ErrIdeasUnavailable, http.StatusServiceUnavailable, "ideas_unavailable"},
	{bom.ErrAnalysisUnavailable, http.StatusServiceUnavailable, "analysis_unavailable"},
	{bom.ErrProfilesUnavailable, http.StatusServiceUnavailable, "profiles_unavailable"},
}

func lookupError(err error) (apiError, bool) {
	for _, e := range errorTable {
		if errors.Is(err, e.err) {
			return e, true
		}
	}
	return apiError{}, false
}

// writeErrorBody écrit {"error", "code"} : le seul endroit qui construit un corps d'erreur (voir le test de garde).
func writeErrorBody(w http.ResponseWriter, status int, message, code string) {
	writeJSON(w, status, map[string]string{"error": message, "code": code})
}

// codeOrStatus : code de la table si l'erreur y est, sinon un code générique du statut (mappeurs locaux).
func codeOrStatus(err error, status int) string {
	if e, ok := lookupError(err); ok {
		return e.code
	}
	return statusCode(status)
}

func statusCode(status int) string {
	switch status {
	case http.StatusBadRequest:
		return "bad_request"
	case http.StatusNotFound:
		return "not_found"
	case http.StatusMethodNotAllowed:
		return "method_not_allowed"
	case http.StatusConflict:
		return "conflict"
	case http.StatusServiceUnavailable:
		return "unavailable"
	}
	return "internal"
}

// errorCode : code stable d'une erreur ; "internal" pour une erreur serveur inconnue.
func errorCode(err error) string {
	if e, ok := lookupError(err); ok {
		return e.code
	}
	return "internal"
}
