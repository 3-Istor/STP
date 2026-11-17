package httpapi

import (
	"errors"
	"time"

	"github.com/shopspring/decimal"

	"bomstudio/internal/bom"
	"bomstudio/internal/money"
	"bomstudio/internal/pricing"
	"bomstudio/internal/snapshot"
	"bomstudio/internal/supplier"
)

// Représentations JSON de l'API : champs en snake_case, montants en {"amount": "92.00", "currency": "EUR"}
// (money.Money), quantités, CO2, masses et taux en chaînes décimales : jamais un nombre JSON.
// Les montants sont exacts (non arrondis) : l'arrondi se fait à l'affichage (ADR 0004).

type bomJSON struct {
	ID               int64          `json:"id"`
	Name             string         `json:"name"`
	Currency         string         `json:"currency"`
	Volume           int32          `json:"volume"`
	AnnualHours      string         `json:"annual_hours"`
	Destination      *string        `json:"destination_country"` // null : non renseigné (ADR 0025)
	PriceSnapshotID  *int64         `json:"price_snapshot_id"`   // null : prix en direct
	SourceSnapshotID *int64         `json:"source_snapshot_id"`
	CostMethod       costMethodJSON `json:"cost_method"` // ADR 0031
	CreatedAt        time.Time      `json:"created_at"`
}

func toBOMJSON(b bom.BOM) bomJSON {
	return bomJSON{
		ID: b.ID, Name: b.Name, Currency: b.Currency, Volume: b.Volume, AnnualHours: b.AnnualHours.String(),
		Destination: b.DestinationCountry, PriceSnapshotID: b.PriceSnapshotID, SourceSnapshotID: b.SourceSnapshotID,
		CostMethod: toCostMethodJSON(b.CostMethod), CreatedAt: b.CreatedAt,
	}
}

type nodeJSON struct {
	ID             int64   `json:"id"`
	ParentID       *int64  `json:"parent_id"` // null : racine
	Name           string  `json:"name"`      // sous-ensemble ; vide pour une pièce
	PartID         *int64  `json:"part_id"`
	OfferID        *int64  `json:"offer_id"`
	Quantity       string  `json:"quantity"`
	ProductionDays int32   `json:"production_days"`
	Strategy       *string `json:"strategy"` // stratégie attachée à ce nœud (ADR 0028) ; null = héritée ou aucune
}

func toNodeJSON(n bom.Node) nodeJSON {
	return nodeJSON{
		ID: n.ID, ParentID: n.ParentID, Name: n.Name, PartID: n.PartID, OfferID: n.OfferID,
		Quantity: n.Quantity.String(), ProductionDays: n.ProductionDays, Strategy: strategyJSON(n.Strategy),
	}
}

func strategyJSON(s *bom.Strategy) *string {
	if s == nil {
		return nil
	}
	v := string(*s)
	return &v
}

// candidateJSON : une offre évaluée pour la quantité réelle du nœud ; scores normalisés 0 (meilleur) à 1.
type candidateJSON struct {
	PartID       int64        `json:"part_id"`
	OfferID      int64        `json:"offer_id"`
	Current      bool         `json:"current"`
	Cost         *money.Money `json:"cost"`
	LeadTimeDays *int         `json:"lead_time_days"` // null si un attribut de la pièce manque
	CO2Kg        *string      `json:"co2_kg"`
	Quote        *quoteJSON   `json:"quote"`
	Rate         *rateJSON    `json:"rate"`
	Score        *string      `json:"score"`
	CostScore    *string      `json:"cost_score"`
	LeadScore    *string      `json:"lead_time_score"`
	CO2Score     *string      `json:"co2_score"`
	MassScore    *string      `json:"mass_score"`    // profil de véhicule (ADR 0036)
	QualityScore *string      `json:"quality_score"` // idem ; 0 quand le critère ne pèse pas
	MassKg       *string      `json:"mass_kg"`
	Quality      *string      `json:"quality"`
	Excluded     *string      `json:"excluded"` // raison de l'exclusion (prix, taux, attribut ou note qualité manquant)
}

// suggestionsJSON : strategy null = aucune stratégie dans la lignée (ordre par offre).
type suggestionsJSON struct {
	NodeID          int64           `json:"node_id"`
	Strategy        *string         `json:"strategy"`
	StrategyFrom    *int64          `json:"strategy_from"`
	RealQuantity    string          `json:"real_quantity"`
	Currency        string          `json:"currency"`
	PriceSnapshotID *int64          `json:"price_snapshot_id"`
	Candidates      []candidateJSON `json:"candidates"`
	Excluded        []candidateJSON `json:"excluded"`
	// Profile : profil de véhicule qui classe quand aucune stratégie de nœud ne s'applique (ADR 0036) ;
	// weights : poids appliqués (stratégie, profil, ou tous nuls).
	Profile *profileJSON `json:"profile"`
	Weights *weightsJSON `json:"weights"`
}

func toSuggestionsJSON(s bom.Suggestions) suggestionsJSON {
	score := func(d decimal.Decimal) *string { v := d.Round(4).String(); return &v } // sortie : 4 décimales
	out := suggestionsJSON{
		NodeID: s.NodeID, RealQuantity: s.RealQuantity.String(), Currency: string(s.Currency),
		PriceSnapshotID: s.PriceSnapshotID, Candidates: make([]candidateJSON, len(s.Ranked)),
		Excluded: make([]candidateJSON, len(s.Excluded)), Weights: toWeightsJSON(s.Weights),
	}
	if s.Profile != nil {
		p := toProfileJSON(s.Profile)
		out.Profile = &p
	}
	if s.Strategy != "" {
		st, from := string(s.Strategy), s.StrategyFrom
		out.Strategy, out.StrategyFrom = &st, &from
	}
	for i, r := range s.Ranked {
		c := toCandidateJSON(r.Candidate)
		c.Score, c.CostScore, c.LeadScore, c.CO2Score = score(r.Score), score(r.CostScore), score(r.LeadScore), score(r.CO2Score)
		c.MassScore, c.QualityScore = score(r.MassScore), score(r.QualityScore)
		out.Candidates[i] = c
	}
	for i, c := range s.Excluded {
		out.Excluded[i] = toCandidateJSON(c)
	}
	return out
}

func toCandidateJSON(c bom.Candidate) candidateJSON {
	out := candidateJSON{PartID: c.PartID, OfferID: c.OfferID, Current: c.Current, Cost: c.Cost}
	if c.HasFacts {
		lead, co2, mass := c.LeadDays, c.CO2Kg.String(), c.MassKg.String()
		out.LeadTimeDays, out.CO2Kg, out.MassKg = &lead, &co2, &mass
	}
	if c.Quality != nil {
		q := c.Quality.String()
		out.Quality = &q
	}
	if q := c.Quote; q != nil {
		out.Quote = &quoteJSON{OfferID: q.OfferID, Price: q.Price, Source: q.Source, QuotedAt: q.QuotedAt}
	}
	if r := c.Rate; r != nil {
		rj := toRateJSON(*r)
		out.Rate = &rj
	}
	if c.Excluded != nil {
		msg := c.Excluded.Error()
		out.Excluded = &msg
	}
	return out
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
	// LaborHours : heures par voiture du sous-arbre ; null quand labor_error est renseigné (ADR 0024).
	LaborHours   *string `json:"labor_hours"`
	LaborError   *string `json:"labor_error"`
	MissingLabor []int64 `json:"missing_labor"`
	// FTE : arrondi half-even à 2 décimales (sortie, ADR 0004/0024) ; null sans heures ou sans base horaire.
	FTE *string `json:"fte"`
	// StockCovered : quantité couverte par le stock de la BOM (sur la quantité commandée) ; null si la pièce
	// n'a pas de stock. InStock : couverture totale, délai propre nul (ADR 0022).
	StockCovered *string `json:"stock_covered"`
	InStock      bool    `json:"in_stock"`
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
	out.MissingLabor = nt.MissingLabor
	if out.MissingLabor == nil {
		out.MissingLabor = []int64{}
	}
	if h := nt.LaborHours; h != nil {
		v := h.String()
		out.LaborHours = &v
	}
	if nt.LaborErr != nil {
		msg := nt.LaborErr.Error()
		out.LaborError = &msg
	}
	if c := nt.StockCovered; c != nil {
		v := c.String()
		out.StockCovered = &v
	}
	out.InStock = nt.InStock
	if f := nt.FTE; f != nil {
		v := f.RoundBank(2).StringFixedBank(2)
		out.FTE = &v
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

// costErrorCode : code stable de la cause d'un coût absent, pour l'écran ; "missing_cost" = un descendant.
func costErrorCode(err error) string {
	codes := []struct {
		target error
		code   string
	}{
		{bom.ErrMissingCost, "missing_cost"},
		{pricing.ErrNoQuote, "no_quote"},
		{pricing.ErrNoRate, "no_rate"},
		{bom.ErrNoBudgetRate, "no_budget_rate"},
		{bom.ErrNoTooling, "no_tooling"},
		{bom.ErrNoProgramVolume, "no_program_volume"},
		{bom.ErrNoCostDate, "no_cost_date"},
		{bom.ErrNoCostFacts, "no_cost_facts"},
		{supplier.ErrSelectionRequired, "contract_selection_required"},
		{pricing.ErrNoDestination, "no_destination"},
		{pricing.ErrNoCustomsDuty, "no_customs_duty"},
		{pricing.ErrNoFreightRate, "no_freight_rate"},
		{pricing.ErrNoPackaging, "no_packaging"},
	}
	for _, c := range codes {
		if errors.Is(err, c.target) {
			return c.code
		}
	}
	return "missing_cost"
}

type treeNodeJSON struct {
	nodeJSON
	Totals *totalsJSON `json:"totals"`
}

// explanationJSON dit sur quoi reposent tous les totaux : devise, prix en direct (price_snapshot_id null)
// ou snapshot de prix, taux utilisés et leur date.
// cost_method : méthode appliquée (ADR 0031) ; cost_as_of : date de calcul (contrats, année du taux budget),
// null pour purchase + latest ; budget_rates : taux budget utilisés.
type explanationJSON struct {
	Currency        string         `json:"currency"`
	PriceSnapshotID *int64         `json:"price_snapshot_id"`
	Rates           []rateJSON     `json:"rates"`
	CostMethod      costMethodJSON `json:"cost_method"`
	CostAsOf        *time.Time     `json:"cost_as_of"`
	BudgetRates     []rateJSON     `json:"budget_rates"`
}

func toExplanationJSON(e bom.Explanation) explanationJSON {
	conv := func(in []pricing.Rate) []rateJSON {
		out := make([]rateJSON, len(in))
		for i, r := range in {
			out[i] = toRateJSON(r)
		}
		return out
	}
	return explanationJSON{Currency: string(e.Currency), PriceSnapshotID: e.PriceSnapshotID, Rates: conv(e.Rates),
		CostMethod: toCostMethodJSON(e.Method), CostAsOf: e.CostAsOf, BudgetRates: conv(e.BudgetRates)}
}

type treeJSON struct {
	BOM         bomJSON         `json:"bom"`
	Nodes       []treeNodeJSON  `json:"nodes"`
	Explanation explanationJSON `json:"explanation"`
	Stock       []stockJSON     `json:"stock"`
	Planning    planningJSON    `json:"planning"`
	// Budgets : budgets cibles du brouillon évalués sur les totaux de cette réponse (même calcul que /budgets).
	Budgets []budgetJSON `json:"budgets"`
}

// scheduleJSON : dates calendaires "AAAA-MM-JJ" (ADR 0023).
type scheduleJSON struct {
	StartOn  string  `json:"start_on"`
	TargetOn *string `json:"target_on"`
}

func toScheduleJSON(s *bom.Schedule) *scheduleJSON {
	if s == nil {
		return nil
	}
	out := &scheduleJSON{StartOn: s.StartOn.Format(time.DateOnly)}
	if s.TargetOn != nil {
		v := s.TargetOn.Format(time.DateOnly)
		out.TargetOn = &v
	}
	return out
}

type latePartJSON struct {
	NodeID      int64 `json:"node_id"`
	ChainDays   int   `json:"chain_days"`
	OverrunDays int   `json:"overrun_days"`
}

// planningJSON : disponibilité, chemin critique et pièces responsables d'une cible impossible (ADR 0023).
// available_on est null sans date de départ ; target_met et allowed_days sont null sans date cible.
type planningJSON struct {
	Schedule     *scheduleJSON  `json:"schedule"`
	LeadTimeDays int            `json:"lead_time_days"`
	AvailableOn  *string        `json:"available_on"`
	Critical     []int64        `json:"critical"`
	AllowedDays  *int           `json:"allowed_days"`
	TargetMet    *bool          `json:"target_met"`
	Late         []latePartJSON `json:"late"`
}

func toPlanningJSON(p bom.Planning) planningJSON {
	out := planningJSON{
		Schedule: toScheduleJSON(p.Schedule), LeadTimeDays: p.LeadTimeDays, Critical: p.Critical,
		AllowedDays: p.AllowedDays, TargetMet: p.TargetMet, Late: make([]latePartJSON, len(p.Late)),
	}
	if out.Critical == nil {
		out.Critical = []int64{}
	}
	if p.AvailableOn != nil {
		v := p.AvailableOn.Format(time.DateOnly)
		out.AvailableOn = &v
	}
	for i, l := range p.Late {
		out.Late[i] = latePartJSON{NodeID: l.NodeID, ChainDays: l.ChainDays, OverrunDays: l.OverrunDays}
	}
	return out
}

// anomalyJSON : verdict d'un nœud pièce. status : "anomaly" | "normal" | "not_evaluable" (jamais caché en
// « normal ») ; ratio = coût ÷ médiane, arrondi à 2 décimales en sortie ; reason dit pourquoi non évaluable.
type anomalyJSON struct {
	NodeID   int64        `json:"node_id"`
	PartID   int64        `json:"part_id"`
	Category string       `json:"category"`
	Status   string       `json:"status"`
	UnitCost *money.Money `json:"unit_cost"`
	Median   *money.Money `json:"median"`
	Ratio    *string      `json:"ratio"`
	Peers    int          `json:"peers"`
	Missing  int          `json:"missing"` // pièces de la catégorie écartées faute de prix ou de taux
	Reason   *string      `json:"reason"`
}

type anomaliesJSON struct {
	Threshold       string        `json:"threshold"`
	Currency        string        `json:"currency"`
	PriceSnapshotID *int64        `json:"price_snapshot_id"`
	Rates           []rateJSON    `json:"rates"`
	Nodes           []anomalyJSON `json:"nodes"`
}

func toAnomaliesJSON(a bom.CostAnomalies) anomaliesJSON {
	out := anomaliesJSON{
		Threshold: a.Threshold.String(), Currency: string(a.Currency), PriceSnapshotID: a.PriceSnapshotID,
		Nodes: make([]anomalyJSON, len(a.Nodes)), Rates: make([]rateJSON, len(a.Rates)),
	}
	for i, r := range a.Rates {
		out.Rates[i] = toRateJSON(r)
	}
	for i, n := range a.Nodes {
		j := anomalyJSON{
			NodeID: n.NodeID, PartID: n.PartID, Category: n.Category, Status: string(n.Status),
			UnitCost: n.Cost, Median: n.Median, Peers: n.Peers, Missing: n.Missing,
		}
		if n.Ratio != nil {
			// Tronqué (jamais arrondi vers le haut) : 2,995 s'affiche 2,99, pas 3,00 sous un seuil de 3.
			v := n.Ratio.Truncate(2).StringFixed(2)
			j.Ratio = &v
		}
		if n.Err != nil {
			msg := n.Err.Error()
			j.Reason = &msg
		}
		out.Nodes[i] = j
	}
	return out
}

// componentJSON : une composante du coût rendu ; amount null quand error est renseignée (jamais un zéro).
// error_code : no_destination, no_customs_duty, no_freight_rate, no_packaging, no_mass, no_quote, no_rate,
// ou other ; il dit quelle table renseigner.
type componentJSON struct {
	Amount    *money.Money `json:"amount"`
	Error     *string      `json:"error"`
	ErrorCode *string      `json:"error_code"`
}

func toComponentJSON(c pricing.Component) componentJSON {
	out := componentJSON{Amount: c.Amount}
	if c.Err != nil {
		msg, code := c.Err.Error(), landedErrorCode(c.Err)
		out.Error, out.ErrorCode = &msg, &code
	}
	return out
}

func landedErrorCode(err error) string {
	for _, c := range []struct {
		target error
		code   string
	}{
		{pricing.ErrNoDestination, "no_destination"}, {pricing.ErrNoCustomsDuty, "no_customs_duty"},
		{pricing.ErrNoFreightRate, "no_freight_rate"}, {pricing.ErrNoPackaging, "no_packaging"},
		{pricing.ErrNoMass, "no_mass"}, {pricing.ErrNoQuote, "no_quote"}, {pricing.ErrNoRate, "no_rate"},
	} {
		if errors.Is(err, c.target) {
			return c.code
		}
	}
	return "other"
}

type offerLandedJSON struct {
	OfferID     int64         `json:"offer_id"`
	Origin      string        `json:"origin_country"`
	Destination *string       `json:"destination_country"`
	Purchase    componentJSON `json:"purchase"`
	Customs     componentJSON `json:"customs"`
	Freight     componentJSON `json:"freight"`
	Packaging   componentJSON `json:"packaging"`
	Total       *money.Money  `json:"total"`
	Error       *string       `json:"error"`
	// Sur quoi repose le résultat : valeurs de table retenues (logistics_tables "live" : lues en direct, non
	// figées avec les prix) et taux utilisés avec leur date.
	DutyPercent     *string      `json:"duty_percent"`
	FreightPerKg    *money.Money `json:"freight_per_kg"`
	PackagingPer    *money.Money `json:"packaging_per_unit"`
	Rates           []rateJSON   `json:"rates"`
	LogisticsTables string       `json:"logistics_tables"`
}

type landedJSON struct {
	PartID          int64             `json:"part_id"`
	Currency        string            `json:"currency"`
	PriceSnapshotID *int64            `json:"price_snapshot_id"`
	Offers          []offerLandedJSON `json:"offers"`
}

func toLandedJSON(l bom.LandedComparison) landedJSON {
	out := landedJSON{PartID: l.PartID, Currency: string(l.Currency), PriceSnapshotID: l.PriceSnapshotID,
		Offers: make([]offerLandedJSON, len(l.Offers))}
	for i, o := range l.Offers {
		j := offerLandedJSON{
			OfferID: o.OfferID, Origin: o.Origin, Destination: o.Destination,
			Purchase: toComponentJSON(o.Purchase), Customs: toComponentJSON(o.Customs),
			Freight: toComponentJSON(o.Freight), Packaging: toComponentJSON(o.Packaging), Total: o.Total,
			FreightPerKg: o.FreightPerKg, PackagingPer: o.PackagingPer, LogisticsTables: "live",
			Rates: make([]rateJSON, len(o.Rates)),
		}
		for k, r := range o.Rates {
			j.Rates[k] = toRateJSON(r)
		}
		if o.DutyPercent != nil {
			v := o.DutyPercent.String()
			j.DutyPercent = &v
		}
		if o.Err != nil {
			msg := o.Err.Error()
			j.Error = &msg
		}
		out.Offers[i] = j
	}
	return out
}

type costLineJSON struct {
	Label string `json:"label"`
	componentJSON
}

// makeOrBuyJSON : difference = fabriquer − acheter par voiture (> 0 : acheter est moins cher à ce volume) ;
// break_even_volume null si jamais rentable ou si une composante manque (error le dit).
type makeOrBuyJSON struct {
	NodeID          int64          `json:"node_id"`
	RealQuantity    string         `json:"real_quantity"`
	Volume          int32          `json:"volume"`
	Currency        string         `json:"currency"`
	PriceSnapshotID *int64         `json:"price_snapshot_id"`
	Make            []costLineJSON `json:"make"`
	Buy             []costLineJSON `json:"buy"`
	FixedPerCar     *money.Money   `json:"fixed_per_car"`
	MakePerCar      *money.Money   `json:"make_per_car"`
	BuyPerCar       *money.Money   `json:"buy_per_car"`
	Difference      *money.Money   `json:"difference"`
	BreakEvenVolume *int64         `json:"break_even_volume"`
	NeverProfitable bool           `json:"never_profitable"`
	Rates           []rateJSON     `json:"rates"`
	Error           *string        `json:"error"`
}

func toMakeOrBuyJSON(r bom.MakeOrBuyResult) makeOrBuyJSON {
	lines := func(ls []pricing.CostLine) []costLineJSON {
		out := make([]costLineJSON, len(ls))
		for i, l := range ls {
			out[i] = costLineJSON{Label: l.Label, componentJSON: toComponentJSON(l.Component)}
		}
		return out
	}
	out := makeOrBuyJSON{
		NodeID: r.NodeID, RealQuantity: r.RealQuantity.String(), Volume: r.Volume, Currency: string(r.Currency),
		PriceSnapshotID: r.PriceSnapshotID, Make: lines(r.Make), Buy: lines(r.Buy),
		FixedPerCar: r.FixedPerCar, MakePerCar: r.MakePerCar, BuyPerCar: r.BuyPerCar, Difference: r.Difference,
		BreakEvenVolume: r.BreakEvenVolume, NeverProfitable: r.NeverProfitable, Rates: make([]rateJSON, len(r.Rates)),
	}
	for i, rt := range r.Rates {
		out.Rates[i] = toRateJSON(rt)
	}
	if r.Err != nil {
		msg := r.Err.Error()
		out.Error = &msg
	}
	return out
}

// stockJSON : stock d'une pièce propre à la BOM (ADR 0022).
type stockJSON struct {
	PartID   int64  `json:"part_id"`
	Quantity string `json:"quantity"`
}

func toStockJSON(st []bom.PartStock) []stockJSON {
	out := make([]stockJSON, len(st))
	for i, s := range st {
		out[i] = stockJSON{PartID: s.PartID, Quantity: s.Quantity.String()}
	}
	return out
}

type snapshotJSON struct {
	ID              int64     `json:"id"`
	BOMID           int64     `json:"bom_id"`
	PriceSnapshotID int64     `json:"price_snapshot_id"`
	Name            string    `json:"name"`
	Currency        string    `json:"currency"`
	Volume          int32     `json:"volume"`
	AnnualHours     *string   `json:"annual_hours"` // null : snapshot publié avant ADR 0024
	Destination     *string   `json:"destination_country"`
	PublishedAt     time.Time `json:"published_at"`
	// Trace de la publication (ADR 0033) ; null si non saisie ou publiée avant.
	Note        *string `json:"note"`
	ProposedBy  *string `json:"proposed_by"`
	ValidatedBy *string `json:"validated_by"`
}

func toSnapshotJSON(s snapshot.Snapshot) snapshotJSON {
	out := snapshotJSON{
		ID: s.ID, BOMID: s.BOMID, PriceSnapshotID: s.PriceSnapshotID, Name: s.Name,
		Currency: s.Currency, Volume: s.Volume, PublishedAt: s.PublishedAt, Destination: s.DestinationCountry,
		Note: s.Note, ProposedBy: s.ProposedBy, ValidatedBy: s.ValidatedBy,
	}
	if h := s.AnnualHours; h != nil {
		v := h.String()
		out.AnnualHours = &v
	}
	return out
}

// publishedJSON : snapshot publié ; ses nœuds ont la même forme que ceux d'un brouillon (id = clé figée).
type publishedJSON struct {
	Snapshot    snapshotJSON    `json:"snapshot"`
	Nodes       []treeNodeJSON  `json:"nodes"`
	Explanation explanationJSON `json:"explanation"`
	Stock       []stockJSON     `json:"stock"`
	Planning    planningJSON    `json:"planning"`
	// Checks : contrôles avant publication figés, avec la justification de chaque avertissement (ADR 0033).
	Checks []frozenCheckJSON `json:"checks"`
}

type frozenCheckJSON struct {
	Code          string  `json:"code"`
	Level         string  `json:"level"`
	Title         string  `json:"title"`
	Detail        string  `json:"detail"`
	Justification *string `json:"justification"`
}

func toFrozenChecksJSON(cs []snapshot.FrozenCheck) []frozenCheckJSON {
	out := make([]frozenCheckJSON, len(cs))
	for i, c := range cs {
		out[i] = frozenCheckJSON{Code: c.Code, Level: c.Level, Title: c.Title, Detail: c.Detail, Justification: c.Justification}
	}
	return out
}
