package httpapi

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/shopspring/decimal"

	"bomstudio/internal/bom"
	"bomstudio/internal/money"
	"bomstudio/internal/pricing"
	"bomstudio/internal/snapshot"
)

// Analyses (étapes Chiffrer et Sourcer) et profils de véhicule (ADR 0036, 0037) : handlers fins, les calculs
// sont dans bom. Montants exacts (arrondi à l'affichage, ADR 0004) ; pourcentages arrondis à 2 décimales.

// AnalysisService est ce que l'API attend du domaine pour les analyses ; *bom.Service la satisfait.
type AnalysisService interface {
	FXSensitivity(ctx context.Context, bomID int64, shockPercent decimal.Decimal) (bom.FXSensitivity, error)
	PriceDrivers(ctx context.Context, bomID int64, limit int) (bom.PriceDrivers, error)
	Sourcing(ctx context.Context, bomID int64) (bom.Sourcing, error)
	PartAnomalies(ctx context.Context, bomID int64, method bom.AnomalyMethod, threshold *decimal.Decimal) (bom.PartAnomalies, error)
	VehicleProfile(ctx context.Context, bomID int64) (*bom.VehicleProfile, error)
	SetVehicleProfile(ctx context.Context, bomID int64, preset string, weights *bom.Weights) (bom.VehicleProfile, error)
}

// SnapshotProfileService : profil figé d'un snapshot ; *snapshot.Service la satisfait.
type SnapshotProfileService interface {
	Profile(ctx context.Context, id int64) (*bom.VehicleProfile, error)
}

var (
	_ AnalysisService        = (*bom.Service)(nil)
	_ SnapshotProfileService = (*snapshot.Service)(nil)
)

type analysisHandler struct {
	service   AnalysisService
	snapshots SnapshotProfileService // nil : route du profil figé non montée
}

// mountAnalysis monte les routes du lot E si les services les satisfont (appelé par NewRouter).
func mountAnalysis(mux *http.ServeMux, boms BOMService, snaps SnapshotService) {
	svc, ok := boms.(AnalysisService)
	if !ok {
		return
	}
	h := analysisHandler{service: svc}
	h.snapshots, _ = snaps.(SnapshotProfileService)
	h.register(mux)
}

func (h analysisHandler) register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/boms/{id}/analysis/fx-sensitivity", h.fxSensitivity)
	mux.HandleFunc("GET /api/v1/boms/{id}/analysis/price-drivers", h.priceDrivers)
	mux.HandleFunc("GET /api/v1/boms/{id}/analysis/sourcing", h.sourcing)
	mux.HandleFunc("GET /api/v1/boms/{id}/analysis/anomalies", h.anomalies)
	mux.HandleFunc("GET /api/v1/vehicle-profiles", h.presets)
	mux.HandleFunc("GET /api/v1/boms/{id}/profile", h.getProfile)
	mux.HandleFunc("PUT /api/v1/boms/{id}/profile", h.putProfile)
	if h.snapshots != nil {
		mux.HandleFunc("GET /api/v1/snapshots/{id}/profile", h.snapshotProfile)
	}
}

// writeAnalysisError : source non câblée → 503 ; le reste suit writeError.
func writeAnalysisError(w http.ResponseWriter, r *http.Request, err error) {
	writeError(w, r, err) // ErrAnalysisUnavailable et ErrProfilesUnavailable (503) sont dans errorTable
}

// pct : pourcentage exact du domaine → 2 décimales, arrondi half-even une seule fois (ADR 0004, v2-lot-g T5).
func pct(d decimal.Decimal) string { return percentOut(d, 2) }

// percentOut : seul arrondi d'un pourcentage, en sortie : half-even à places décimales (AGENTS §3, ADR 0004).
func percentOut(d decimal.Decimal, places int32) string {
	return d.RoundBank(places).StringFixed(places)
}

func pctPtr(d *decimal.Decimal) *string {
	if d == nil {
		return nil
	}
	v := pct(*d)
	return &v
}

func errPtr(err error) *string {
	if err == nil {
		return nil
	}
	v := err.Error()
	return &v
}

func ratesJSON(rs []pricing.Rate) []rateJSON {
	out := make([]rateJSON, len(rs))
	for i, r := range rs {
		out[i] = toRateJSON(r)
	}
	return out
}

func queryDecimal(r *http.Request, name string, def decimal.Decimal) (decimal.Decimal, error) {
	raw := r.URL.Query().Get(name)
	if raw == "" {
		return def, nil
	}
	d, err := decimal.NewFromString(raw)
	if err != nil {
		return decimal.Decimal{}, fmt.Errorf("%s %q is not a decimal: %w", name, raw, errBadRequest)
	}
	return d, nil
}

// --- Sensibilité au change ------------------------------------------------------------------------------

// fxExposureJSON : une devise d'origine. share_percent : part du coût convertie depuis cette devise ;
// impact : variation du coût de la voiture pour le choc ; budget_percent_after : null sans budget utilisable.
type fxExposureJSON struct {
	Currency           string         `json:"currency"`
	SharePercent       string         `json:"share_percent"`
	Impact             money.Money    `json:"impact"`
	BudgetPercentAfter *string        `json:"budget_percent_after"`
	ShockPercent       string         `json:"shock_percent"`
	Exposed            money.Money    `json:"exposed"`
	CostBefore         money.Money    `json:"cost_before"`
	CostAfter          money.Money    `json:"cost_after"`
	Budget             *money.Money   `json:"budget"`
	BudgetError        *string        `json:"budget_error"`
	Rates              []rateJSON     `json:"rates"`
	CostMethod         costMethodJSON `json:"cost_method"`
	PriceSnapshotID    *int64         `json:"price_snapshot_id"`
}

// fxSensitivity : ?shock_percent=5 (décimal non nul, ]-100 ; 100], défaut 5).
func (h analysisHandler) fxSensitivity(w http.ResponseWriter, r *http.Request) {
	id, err := bomPathID(r, "id")
	var shock decimal.Decimal
	if err == nil {
		shock, err = queryDecimal(r, "shock_percent", decimal.NewFromInt(5))
	}
	var res bom.FXSensitivity
	if err == nil {
		res, err = h.service.FXSensitivity(r.Context(), id, shock)
	}
	if err != nil {
		writeAnalysisError(w, r, err)
		return
	}
	out := make([]fxExposureJSON, len(res.Currencies))
	var budgetErr *string
	if res.BudgetErr != "" {
		budgetErr = &res.BudgetErr
	}
	for i, e := range res.Currencies {
		out[i] = fxExposureJSON{
			Currency: string(e.Currency), SharePercent: pct(e.SharePercent), Impact: e.Impact,
			BudgetPercentAfter: pctPtr(e.BudgetPercentAfter), ShockPercent: res.ShockPercent.String(), Exposed: e.Exposed,
			CostBefore: res.Total, CostAfter: e.CostAfter, Budget: res.Budget, BudgetError: budgetErr, Rates: ratesJSON(e.Rates),
			CostMethod: toCostMethodJSON(res.Explanation.Method), PriceSnapshotID: res.Explanation.PriceSnapshotID,
		}
	}
	writeJSON(w, http.StatusOK, out)
}

// --- Pièces qui pèsent le plus ---------------------------------------------------------------------------

type driverOfferJSON struct {
	OfferID   int64        `json:"offer_id"`
	Quantity  string       `json:"quantity"`
	Min       *money.Money `json:"min"`
	Max       *money.Money `json:"max"`
	Quotes    int          `json:"quotes"`
	Amplitude *money.Money `json:"amplitude"`
	Error     *string      `json:"error"`
}

// priceDriverJSON : amplitude = (max − min des cotations de la fenêtre) × quantité réelle, convertie ; les
// pièces non évaluables suivent les classées, amplitude null et error renseignée.
type priceDriverJSON struct {
	PartID      int64             `json:"part_id"`
	Name        string            `json:"name"`
	Amplitude   *money.Money      `json:"amplitude"`
	Rank        *int              `json:"rank"`
	Error       *string           `json:"error"`
	Offers      []driverOfferJSON `json:"offers"`
	WindowStart time.Time         `json:"window_start"`
	WindowEnd   time.Time         `json:"window_end"`
}

// priceDrivers : ?limit=10 (1 à 100).
func (h analysisHandler) priceDrivers(w http.ResponseWriter, r *http.Request) {
	id, err := bomPathID(r, "id")
	limit := 10
	if raw := r.URL.Query().Get("limit"); err == nil && raw != "" {
		if limit, err = strconv.Atoi(raw); err != nil {
			err = fmt.Errorf("limit %q is not an integer: %w", raw, errBadRequest)
		}
	}
	var res bom.PriceDrivers
	if err == nil {
		res, err = h.service.PriceDrivers(r.Context(), id, limit)
	}
	if err != nil {
		writeAnalysisError(w, r, err)
		return
	}
	out := make([]priceDriverJSON, 0, len(res.Drivers)+len(res.Excluded))
	conv := func(d bom.PriceDriver) priceDriverJSON {
		j := priceDriverJSON{PartID: d.PartID, Name: d.Name, Amplitude: d.Amplitude, Error: errPtr(d.Err),
			WindowStart: res.WindowStart, WindowEnd: res.WindowEnd, Offers: make([]driverOfferJSON, len(d.Offers))}
		for i, o := range d.Offers {
			j.Offers[i] = driverOfferJSON{OfferID: o.OfferID, Quantity: o.Quantity.String(), Min: o.Range.Min, Max: o.Range.Max,
				Quotes: o.Range.Quotes, Amplitude: o.Amplitude, Error: errPtr(o.Err)}
		}
		return j
	}
	for i, d := range res.Drivers {
		j := conv(d)
		rank := i + 1
		j.Rank = &rank
		out = append(out, j)
	}
	for _, d := range res.Excluded {
		out = append(out, conv(d))
	}
	writeJSON(w, http.StatusOK, out)
}

// --- Sourcing --------------------------------------------------------------------------------------------

type costShareJSON struct {
	Key          string      `json:"key"`
	Cost         money.Money `json:"cost"`
	SharePercent string      `json:"share_percent"`
	Parts        int         `json:"parts"`
}

type contractStateJSON struct {
	Status     string  `json:"status"` // active | expired | not_started | none
	ContractID *int64  `json:"contract_id"`
	EndsOn     *string `json:"ends_on"`
}

type supplierShareJSON struct {
	SupplierID   int64             `json:"supplier_id"`
	Name         string            `json:"name"`
	Countries    []string          `json:"countries"`
	Currencies   []money.Currency  `json:"currencies"`
	Cost         money.Money       `json:"cost"`
	SharePercent string            `json:"share_percent"`
	Parts        int               `json:"parts"`
	Contract     contractStateJSON `json:"contract"`
}

type monoSourceJSON struct {
	PartID       int64       `json:"part_id"`
	Name         string      `json:"name"`
	ActiveOffers int         `json:"active_offers"`
	SupplierIDs  []int64     `json:"supplier_ids"`
	Cost         money.Money `json:"cost"`
	SharePercent string      `json:"share_percent"`
}

type lostDiscountJSON struct {
	PartID          int64        `json:"part_id"`
	Name            string       `json:"name"`
	OfferID         int64        `json:"offer_id"`
	OrderedQuantity string       `json:"ordered_quantity"`
	DiscountPercent string       `json:"discount_percent"`
	TierMin         *string      `json:"tier_min"`
	Amount          *money.Money `json:"amount"` // par voiture
	Error           *string      `json:"error"`
}

type expiredContractJSON struct {
	ContractID   int64              `json:"contract_id"`
	SupplierID   int64              `json:"supplier_id"`
	SupplierName string             `json:"supplier_name"`
	EndsOn       string             `json:"ends_on"`
	LostAmount   *money.Money       `json:"lost_amount"`
	Parts        []lostDiscountJSON `json:"parts"`
}

type rateGapJSON struct {
	Currency   string       `json:"currency"`
	BudgetRate *rateJSON    `json:"budget_rate"`
	LatestRate *rateJSON    `json:"latest_rate"`
	GapPercent *string      `json:"gap_percent"`
	Effect     *money.Money `json:"effect"` // coût au dernier taux − coût au taux budget, par voiture
	Error      *string      `json:"error"`
}

type sourcingJSON struct {
	Currency         string                `json:"currency"`
	AsOf             time.Time             `json:"as_of"`
	Total            money.Money           `json:"total"`
	ByCountry        []costShareJSON       `json:"by_country"`
	ByCurrency       []costShareJSON       `json:"by_currency"`
	SupplierCount    int                   `json:"supplier_count"`
	Top5SharePercent string                `json:"top5_share_percent"`
	Suppliers        []supplierShareJSON   `json:"suppliers"`
	MonoSourceCount  int                   `json:"mono_source_count"`
	MonoSource       []monoSourceJSON      `json:"mono_source"`
	ExpiredContracts []expiredContractJSON `json:"expired_contracts"`
	Rates            []rateGapJSON         `json:"rates"`
	CostMethod       costMethodJSON        `json:"cost_method"`
	PriceSnapshotID  *int64                `json:"price_snapshot_id"`
}

func (h analysisHandler) sourcing(w http.ResponseWriter, r *http.Request) {
	id, err := bomPathID(r, "id")
	var s bom.Sourcing
	if err == nil {
		s, err = h.service.Sourcing(r.Context(), id)
	}
	if err != nil {
		writeAnalysisError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toSourcingJSON(s))
}

func toSourcingJSON(s bom.Sourcing) sourcingJSON {
	shares := func(in []bom.CostShare) []costShareJSON {
		out := make([]costShareJSON, len(in))
		for i, c := range in {
			out[i] = costShareJSON{Key: c.Key, Cost: c.Cost, SharePercent: pct(c.SharePercent), Parts: c.Parts}
		}
		return out
	}
	out := sourcingJSON{
		Currency: string(s.Currency), AsOf: s.AsOf, Total: s.Total, ByCountry: shares(s.ByCountry), ByCurrency: shares(s.ByCurrency),
		SupplierCount: len(s.Suppliers), Top5SharePercent: pct(s.TopSharePercent), Suppliers: make([]supplierShareJSON, len(s.Suppliers)),
		MonoSourceCount: len(s.MonoSource), MonoSource: make([]monoSourceJSON, len(s.MonoSource)),
		ExpiredContracts: make([]expiredContractJSON, len(s.ExpiredContracts)), Rates: make([]rateGapJSON, len(s.Rates)),
		CostMethod: toCostMethodJSON(s.Explanation.Method), PriceSnapshotID: s.Explanation.PriceSnapshotID,
	}
	for i, sp := range s.Suppliers {
		c := contractStateJSON{Status: sp.Contract.Status, ContractID: sp.Contract.ContractID}
		if sp.Contract.EndsOn != "" {
			end := sp.Contract.EndsOn
			c.EndsOn = &end
		}
		out.Suppliers[i] = supplierShareJSON{SupplierID: sp.SupplierID, Name: sp.Name, Countries: sp.Countries, Currencies: sp.Currencies,
			Cost: sp.Cost, SharePercent: pct(sp.SharePercent), Parts: sp.Parts, Contract: c}
	}
	for i, m := range s.MonoSource {
		out.MonoSource[i] = monoSourceJSON{PartID: m.PartID, Name: m.Name, ActiveOffers: m.ActiveOffers, SupplierIDs: m.SupplierIDs,
			Cost: m.Cost, SharePercent: pct(m.SharePercent)}
	}
	for i, ec := range s.ExpiredContracts {
		j := expiredContractJSON{ContractID: ec.ContractID, SupplierID: ec.SupplierID, SupplierName: ec.SupplierName, EndsOn: ec.EndsOn,
			LostAmount: ec.LostAmount, Parts: make([]lostDiscountJSON, len(ec.Offers))}
		for k, l := range ec.Offers {
			lj := lostDiscountJSON{PartID: l.PartID, Name: l.Name, OfferID: l.OfferID, OrderedQuantity: l.Quantity.String(),
				DiscountPercent: l.Percent.String(), Amount: l.Amount, Error: errPtr(l.Err)}
			if l.TierMin != nil {
				v := l.TierMin.String()
				lj.TierMin = &v
			}
			j.Parts[k] = lj
		}
		out.ExpiredContracts[i] = j
	}
	for i, g := range s.Rates {
		j := rateGapJSON{Currency: string(g.Currency), GapPercent: pctPtr(g.GapPercent), Effect: g.Effect, Error: errPtr(g.Err)}
		if g.Budget != nil {
			b := toRateJSON(*g.Budget)
			j.BudgetRate = &b
		}
		if g.Latest != nil {
			l := toRateJSON(*g.Latest)
			j.LatestRate = &l
		}
		out.Rates[i] = j
	}
	return out
}

// --- Anomalies par pièce ---------------------------------------------------------------------------------

// partAnomalyJSON : reference_cost = médiane (median) ou médiane €/kg × masse (per_kg) ; excess = coût
// unitaire − référence ; impact = excess × quantité réelle (surcoût estimé par voiture).
type partAnomalyJSON struct {
	PartID        int64        `json:"part_id"`
	Name          string       `json:"name"`
	Category      string       `json:"category"`
	OfferID       int64        `json:"offer_id"`
	NodeIDs       []int64      `json:"node_ids"`
	Quantity      string       `json:"quantity"`
	Status        string       `json:"status"`
	UnitCost      *money.Money `json:"unit_cost"`
	PerKg         *money.Money `json:"per_kg"`
	MedianPerKg   *money.Money `json:"median_per_kg"`
	ReferenceCost *money.Money `json:"reference_cost"`
	Ratio         *string      `json:"ratio"`
	Excess        *money.Money `json:"excess"`
	Impact        *money.Money `json:"impact"`
	Peers         int          `json:"peers"`
	Missing       int          `json:"missing"`
	Reason        *string      `json:"reason"`
	// Family : famille de comparaison de la méthode « material » (catégorie · matière) ; null sinon.
	Family *string `json:"family"`
}

type partAnomaliesJSON struct {
	Method          string            `json:"method"`
	Threshold       string            `json:"threshold"`
	Currency        string            `json:"currency"`
	PriceSnapshotID *int64            `json:"price_snapshot_id"`
	Rates           []rateJSON        `json:"rates"`
	AnomalyCount    int               `json:"anomaly_count"`
	Parts           []partAnomalyJSON `json:"parts"`
}

// anomalies : ?method=per_kg|median|offers|history|material (défaut per_kg, ADR 0037 et 0041) & threshold (décimal > 1, défaut de la méthode).
func (h analysisHandler) anomalies(w http.ResponseWriter, r *http.Request) {
	id, err := bomPathID(r, "id")
	method := bom.AnomalyMethod(r.URL.Query().Get("method"))
	if method == "" {
		method = bom.AnomalyPerKg
	}
	var threshold *decimal.Decimal
	if raw := r.URL.Query().Get("threshold"); err == nil && raw != "" {
		var d decimal.Decimal
		if d, err = decimal.NewFromString(raw); err != nil {
			err = fmt.Errorf("threshold %q is not a decimal: %w", raw, errBadRequest)
		}
		threshold = &d
	}
	var res bom.PartAnomalies
	if err == nil {
		res, err = h.service.PartAnomalies(r.Context(), id, method, threshold)
	}
	if err != nil {
		writeAnalysisError(w, r, err)
		return
	}
	out := partAnomaliesJSON{Method: string(res.Method), Threshold: res.Threshold.String(), Currency: string(res.Currency),
		PriceSnapshotID: res.PriceSnapshotID, Rates: ratesJSON(res.Rates), Parts: make([]partAnomalyJSON, len(res.Parts))}
	for i, p := range res.Parts {
		j := partAnomalyJSON{PartID: p.PartID, Name: p.Name, Category: p.Category, OfferID: p.OfferID, NodeIDs: p.NodeIDs,
			Quantity: p.Quantity.String(), Status: string(p.Status), UnitCost: p.Cost, PerKg: p.PerKg, MedianPerKg: p.MedianPerKg,
			ReferenceCost: p.Median, Excess: p.Excess, Impact: p.Impact, Peers: p.Peers, Missing: p.Missing, Reason: errPtr(p.Err)}
		if p.Family != "" {
			f := p.Family
			j.Family = &f
		}
		if p.Ratio != nil {
			v := p.Ratio.Truncate(2).StringFixed(2) // tronqué : jamais arrondi au-dessus du seuil (ADR 0027)
			j.Ratio = &v
		}
		if p.Status == pricing.AnomalyDetected {
			out.AnomalyCount++
		}
		out.Parts[i] = j
	}
	writeJSON(w, http.StatusOK, out)
}

// --- Profils de véhicule ---------------------------------------------------------------------------------

type weightsJSON struct {
	Cost     int `json:"cost"`
	CO2      int `json:"co2"`
	LeadTime int `json:"lead_time"`
	Mass     int `json:"mass"`
	Quality  int `json:"quality"`
}

func toWeightsJSON(w bom.Weights) *weightsJSON {
	return &weightsJSON{Cost: w.Cost, CO2: w.CO2, LeadTime: w.LeadTime, Mass: w.Mass, Quality: w.Quality}
}

// profileJSON : {"preset":"family","weights":{…}} ; preset et weights null : aucun profil (classement par
// stratégie de nœud seulement, ADR 0028).
type profileJSON struct {
	Preset  *string      `json:"preset"`
	Weights *weightsJSON `json:"weights"`
}

func toProfileJSON(p *bom.VehicleProfile) profileJSON {
	if p == nil {
		return profileJSON{}
	}
	preset := string(p.Preset)
	return profileJSON{Preset: &preset, Weights: toWeightsJSON(p.Weights)}
}

type presetJSON struct {
	Preset  string       `json:"preset"`
	Label   string       `json:"label"`
	Hint    string       `json:"hint"`
	Weights *weightsJSON `json:"weights"`
	Custom  bool         `json:"custom"` // poids libres (somme 100)
}

func (h analysisHandler) presets(w http.ResponseWriter, _ *http.Request) {
	defs := bom.VehicleProfilePresets()
	out := make([]presetJSON, len(defs))
	for i, d := range defs {
		out[i] = presetJSON{Preset: string(d.Preset), Label: d.Label, Hint: d.Hint, Weights: toWeightsJSON(d.Weights),
			Custom: d.Preset == bom.ProfileCustom}
	}
	writeJSON(w, http.StatusOK, out)
}

func (h analysisHandler) getProfile(w http.ResponseWriter, r *http.Request) {
	id, err := bomPathID(r, "id")
	var p *bom.VehicleProfile
	if err == nil {
		p, err = h.service.VehicleProfile(r.Context(), id)
	}
	if err != nil {
		writeAnalysisError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toProfileJSON(p))
}

// putProfile : {"preset":"family"} ou {"preset":"custom","weights":{…}} (entiers, somme 100).
func (h analysisHandler) putProfile(w http.ResponseWriter, r *http.Request) {
	id, err := bomPathID(r, "id")
	var req profileJSON
	if err == nil {
		err = decode(r, &req)
	}
	if err == nil && req.Preset == nil {
		err = fmt.Errorf("preset is required: %w", errBadRequest)
	}
	var p bom.VehicleProfile
	if err == nil {
		var weights *bom.Weights
		if req.Weights != nil {
			weights = &bom.Weights{Cost: req.Weights.Cost, CO2: req.Weights.CO2, LeadTime: req.Weights.LeadTime,
				Mass: req.Weights.Mass, Quality: req.Weights.Quality}
		}
		p, err = h.service.SetVehicleProfile(r.Context(), id, *req.Preset, weights)
	}
	if err != nil {
		writeAnalysisError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toProfileJSON(&p))
}

func (h analysisHandler) snapshotProfile(w http.ResponseWriter, r *http.Request) {
	id, err := bomPathID(r, "id")
	var p *bom.VehicleProfile
	if err == nil {
		p, err = h.snapshots.Profile(r.Context(), id)
	}
	if err != nil {
		writeAnalysisError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toProfileJSON(p))
}
