package httpapi

import (
	"errors"
	"time"

	"bomstudio/internal/bom"
	"bomstudio/internal/money"
	"bomstudio/internal/pricing"
	"bomstudio/internal/snapshot"
)

// Représentations JSON de l'API : champs en snake_case, montants en {"amount": "92.00", "currency": "EUR"}
// (money.Money), quantités, CO2, masses et taux en chaînes décimales : jamais un nombre JSON.
// Les montants sont exacts (non arrondis) : l'arrondi se fait à l'affichage (ADR 0004).

type bomJSON struct {
	ID               int64     `json:"id"`
	Name             string    `json:"name"`
	Currency         string    `json:"currency"`
	Volume           int32     `json:"volume"`
	PriceSnapshotID  *int64    `json:"price_snapshot_id"` // null : prix en direct
	SourceSnapshotID *int64    `json:"source_snapshot_id"`
	CreatedAt        time.Time `json:"created_at"`
}

func toBOMJSON(b bom.BOM) bomJSON {
	return bomJSON{
		ID: b.ID, Name: b.Name, Currency: b.Currency, Volume: b.Volume,
		PriceSnapshotID: b.PriceSnapshotID, SourceSnapshotID: b.SourceSnapshotID, CreatedAt: b.CreatedAt,
	}
}

type nodeJSON struct {
	ID             int64  `json:"id"`
	ParentID       *int64 `json:"parent_id"` // null : racine
	Name           string `json:"name"`      // sous-ensemble ; vide pour une pièce
	PartID         *int64 `json:"part_id"`
	OfferID        *int64 `json:"offer_id"`
	Quantity       string `json:"quantity"`
	ProductionDays int32  `json:"production_days"`
}

func toNodeJSON(n bom.Node) nodeJSON {
	return nodeJSON{
		ID: n.ID, ParentID: n.ParentID, Name: n.Name, PartID: n.PartID, OfferID: n.OfferID,
		Quantity: n.Quantity.String(), ProductionDays: n.ProductionDays,
	}
}

type quoteJSON struct {
	OfferID  int64       `json:"offer_id"`
	Price    money.Money `json:"price"`
	Source   string      `json:"source"`
	QuotedAt time.Time   `json:"quoted_at"`
}

type rateJSON struct {
	From     string    `json:"from"`
	To       string    `json:"to"`
	Rate     string    `json:"rate"`
	Source   string    `json:"source"`
	QuotedAt time.Time `json:"quoted_at"`
}

func toRateJSON(r pricing.Rate) rateJSON {
	return rateJSON{From: string(r.From), To: string(r.To), Rate: r.Rate.String(), Source: r.Source, QuotedAt: r.QuotedAt}
}

// totalsJSON : total d'un nœud. cost est null quand cost_error est renseigné (jamais un coût à zéro).
type totalsJSON struct {
	RealQuantity string       `json:"real_quantity"`
	Cost         *money.Money `json:"cost"`
	// CostErrorCode : "no_quote", "no_rate" (la pièce elle-même) ou "missing_cost" (un descendant) ; null sinon.
	CostErrorCode   *string      `json:"cost_error_code"`
	CostError       *string      `json:"cost_error"`
	MissingCost     []int64      `json:"missing_cost"`
	CO2Kg           string       `json:"co2_kg"`
	MassKg          string       `json:"mass_kg"`
	LeadTimeDays    int          `json:"lead_time_days"`
	OwnCost         *money.Money `json:"own_cost"`
	OwnLeadTimeDays int          `json:"own_lead_time_days"`
	Quote           *quoteJSON   `json:"quote"`
	Rate            *rateJSON    `json:"rate"`
}

// nodeTotals renvoie le total du nœud id, ou nil si le calcul n'en a pas (ne devrait pas arriver).
func nodeTotals(t bom.Totals, id int64) *totalsJSON {
	nt, ok := t.Node(id)
	if !ok {
		return nil
	}
	out := &totalsJSON{
		RealQuantity: nt.RealQuantity.String(), Cost: nt.Cost, MissingCost: nt.MissingCost,
		CO2Kg: nt.CO2Kg.String(), MassKg: nt.MassKg.String(), LeadTimeDays: nt.LeadTimeDays,
		OwnCost: nt.OwnCost, OwnLeadTimeDays: nt.OwnLeadTimeDays,
	}
	if out.MissingCost == nil {
		out.MissingCost = []int64{}
	}
	if nt.CostErr != nil {
		code, msg := costErrorCode(nt.CostErr), nt.CostErr.Error()
		out.CostErrorCode, out.CostError = &code, &msg
	}
	if q := nt.Quote; q != nil {
		out.Quote = &quoteJSON{OfferID: q.OfferID, Price: q.Price, Source: q.Source, QuotedAt: q.QuotedAt}
	}
	if r := nt.Rate; r != nil {
		rj := toRateJSON(*r)
		out.Rate = &rj
	}
	return out
}

func costErrorCode(err error) string {
	switch {
	case errors.Is(err, pricing.ErrNoQuote):
		return "no_quote"
	case errors.Is(err, pricing.ErrNoRate):
		return "no_rate"
	default:
		return "missing_cost"
	}
}

type treeNodeJSON struct {
	nodeJSON
	Totals *totalsJSON `json:"totals"`
}

// explanationJSON dit sur quoi reposent tous les totaux : devise, prix en direct (price_snapshot_id null)
// ou snapshot de prix, taux utilisés et leur date.
type explanationJSON struct {
	Currency        string     `json:"currency"`
	PriceSnapshotID *int64     `json:"price_snapshot_id"`
	Rates           []rateJSON `json:"rates"`
}

func toExplanationJSON(e bom.Explanation) explanationJSON {
	rates := make([]rateJSON, len(e.Rates))
	for i, r := range e.Rates {
		rates[i] = toRateJSON(r)
	}
	return explanationJSON{Currency: string(e.Currency), PriceSnapshotID: e.PriceSnapshotID, Rates: rates}
}

type treeJSON struct {
	BOM         bomJSON         `json:"bom"`
	Nodes       []treeNodeJSON  `json:"nodes"`
	Explanation explanationJSON `json:"explanation"`
}

type snapshotJSON struct {
	ID              int64     `json:"id"`
	BOMID           int64     `json:"bom_id"`
	PriceSnapshotID int64     `json:"price_snapshot_id"`
	Name            string    `json:"name"`
	Currency        string    `json:"currency"`
	Volume          int32     `json:"volume"`
	PublishedAt     time.Time `json:"published_at"`
}

func toSnapshotJSON(s snapshot.Snapshot) snapshotJSON {
	return snapshotJSON{
		ID: s.ID, BOMID: s.BOMID, PriceSnapshotID: s.PriceSnapshotID, Name: s.Name,
		Currency: s.Currency, Volume: s.Volume, PublishedAt: s.PublishedAt,
	}
}

// publishedJSON : snapshot publié ; ses nœuds ont la même forme que ceux d'un brouillon (id = clé figée).
type publishedJSON struct {
	Snapshot    snapshotJSON    `json:"snapshot"`
	Nodes       []treeNodeJSON  `json:"nodes"`
	Explanation explanationJSON `json:"explanation"`
}
