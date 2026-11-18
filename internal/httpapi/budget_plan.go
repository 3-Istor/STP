package httpapi

import (
	"net/http"

	"github.com/shopspring/decimal"

	"bomstudio/internal/bom"
	"bomstudio/internal/money"
)

// planSideJSON : une extrémité d'une substitution ; material et quality null si non renseignés.
type planSideJSON struct {
	PartID   int64   `json:"part_id"`
	OfferID  int64   `json:"offer_id"`
	Part     string  `json:"part"`
	Supplier string  `json:"supplier"`
	Material *string `json:"material"`
	Quality  *string `json:"quality"`
}

type planNodeJSON struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// planActionJSON : gain, deltas (après − avant) et cumul sont par véhicule ; covers : première action qui couvre le dépassement.
type planActionJSON struct {
	Nodes          []planNodeJSON `json:"nodes"`
	From           planSideJSON   `json:"from"`
	To             planSideJSON   `json:"to"`
	Gain           money.Money    `json:"gain"`
	CumulativeGain money.Money    `json:"cumulative_gain"`
	CO2DeltaKg     string         `json:"co2_delta_kg"`
	MassDeltaKg    string         `json:"mass_delta_kg"`
	LeadDeltaDays  int            `json:"lead_time_delta_days"`
	Covers         bool           `json:"covers"`
}

type budgetPlanJSON struct {
	NodeID    int64            `json:"node_id"`
	Currency  string           `json:"currency"`
	Budget    money.Money      `json:"budget"`
	Cost      money.Money      `json:"cost"`
	Overrun   money.Money      `json:"overrun"`
	Remaining money.Money      `json:"remaining"`
	Covered   bool             `json:"covered"`
	Actions   []planActionJSON `json:"actions"`
}

func toBudgetPlanJSON(p bom.BudgetPlan) budgetPlanJSON {
	out := budgetPlanJSON{NodeID: p.NodeID, Currency: string(p.Currency), Budget: p.Budget, Cost: p.Cost,
		Overrun: p.Overrun, Remaining: p.Remaining, Covered: p.Covered, Actions: make([]planActionJSON, len(p.Actions))}
	opt := func(s string) *string {
		if s == "" {
			return nil
		}
		return &s
	}
	q := func(d *decimal.Decimal) *string {
		if d == nil {
			return nil
		}
		s := d.String()
		return &s
	}
	for i, a := range p.Actions {
		gain, _ := money.New(a.Gain, string(p.Currency))
		cum, _ := money.New(a.Cumulative, string(p.Currency))
		nodes := make([]planNodeJSON, len(a.Nodes))
		for j, n := range a.Nodes {
			nodes[j] = planNodeJSON{ID: n.ID, Name: n.Name}
		}
		out.Actions[i] = planActionJSON{
			Nodes: nodes, Gain: gain, CumulativeGain: cum, Covers: a.Covers, LeadDeltaDays: a.LeadDelta,
			CO2DeltaKg: a.CO2Delta.String(), MassDeltaKg: a.MassDelta.String(),
			From: planSideJSON{PartID: a.FromPartID, OfferID: a.FromOfferID, Part: a.FromPart, Supplier: a.FromSupplier,
				Material: opt(a.FromMaterial), Quality: q(a.FromQuality)},
			To: planSideJSON{PartID: a.ToPartID, OfferID: a.ToOfferID, Part: a.ToPart, Supplier: a.ToSupplier,
				Material: opt(a.ToMaterial), Quality: q(a.ToQuality)},
		}
	}
	return out
}

func (a *api) budgetPlan(w http.ResponseWriter, r *http.Request) {
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
	p, err := a.boms.BudgetPlan(r.Context(), bomID, nodeID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toBudgetPlanJSON(p))
}
