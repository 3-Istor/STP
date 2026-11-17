package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"bomstudio/internal/bom"
	"bomstudio/internal/catalog"
	"bomstudio/internal/money"
	"bomstudio/internal/pricing"
	"bomstudio/internal/snapshot"
)

var testNow = time.Date(2026, 11, 3, 10, 0, 0, 0, time.UTC)

// fakeBOMs enregistre chaque appel (méthode et arguments décodés) et renvoie err si elle est posée.
type fakeBOMs struct {
	err   error
	calls []string
	tree  bom.TreeTotals
}

func (f *fakeBOMs) record(format string, args ...any) error {
	f.calls = append(f.calls, fmt.Sprintf(format, args...))
	return f.err
}

func ptrStr(p *int64) string {
	if p == nil {
		return "nil"
	}
	return fmt.Sprint(*p)
}

var (
	draft = bom.BOM{ID: 7, Name: "Citadine", Currency: "EUR", Volume: 1000, CreatedAt: testNow}
	part  = bom.Node{ID: 3, BOMID: 7, ParentID: ptr(int64(1)), PartID: ptr(int64(10)), OfferID: ptr(int64(20)),
		Quantity: decimal.RequireFromString("2.5")}
)

func ptr[T any](v T) *T { return &v }

func (f *fakeBOMs) CreateBOM(_ context.Context, b bom.NewBOM) (bom.BOM, error) {
	return draft, f.record("CreateBOM %s %s %d", b.Name, b.Currency, b.Volume)
}
func (f *fakeBOMs) ListBOMs(context.Context) ([]bom.BOM, error) {
	return []bom.BOM{draft}, f.record("ListBOMs")
}
func (f *fakeBOMs) RenameBOM(_ context.Context, id int64, name string) (bom.BOM, error) {
	return draft, f.record("RenameBOM %d %s", id, name)
}
func (f *fakeBOMs) UpdateSettings(_ context.Context, id int64, c string, v int32) (bom.BOM, error) {
	return draft, f.record("UpdateSettings %d %s %d", id, c, v)
}
func (f *fakeBOMs) SetAnnualHours(_ context.Context, id int64, h decimal.Decimal) (bom.BOM, error) {
	return draft, f.record("SetAnnualHours %d %s", id, h)
}
func (f *fakeBOMs) Stock(_ context.Context, id int64) ([]bom.PartStock, error) {
	return []bom.PartStock{{PartID: 10, Quantity: decimal.RequireFromString("40")}}, f.record("Stock %d", id)
}
func (f *fakeBOMs) SetPartStock(_ context.Context, id, partID int64, q decimal.Decimal) (bom.PartStock, error) {
	err := f.record("SetPartStock %d %d %s", id, partID, q)
	if partID == 404 && err == nil {
		err = fmt.Errorf("part 404: %w", catalog.ErrPartNotFound)
	}
	return bom.PartStock{PartID: partID, Quantity: q}, err
}
func (f *fakeBOMs) SetSchedule(_ context.Context, id int64, s bom.Schedule) (bom.Schedule, error) {
	target := "nil"
	if s.TargetOn != nil {
		target = s.TargetOn.Format(time.DateOnly)
	}
	return s, f.record("SetSchedule %d %s %s", id, s.StartOn.Format(time.DateOnly), target)
}
func (f *fakeBOMs) ClearSchedule(_ context.Context, id int64) error {
	return f.record("ClearSchedule %d", id)
}
func (f *fakeBOMs) SetNodeStrategy(_ context.Context, bomID, nodeID int64, st *bom.Strategy) (bom.Node, error) {
	v := "nil"
	if st != nil {
		v = string(*st)
	}
	return bom.Node{ID: nodeID, BOMID: bomID, Strategy: st}, f.record("SetNodeStrategy %d %d %s", bomID, nodeID, v)
}
func (f *fakeBOMs) ReplaceNodePart(_ context.Context, bomID, nodeID, partID, offerID int64) (bom.Node, error) {
	return bom.Node{ID: nodeID, BOMID: bomID, PartID: &partID, OfferID: &offerID},
		f.record("ReplaceNodePart %d %d %d %d", bomID, nodeID, partID, offerID)
}
func (f *fakeBOMs) Suggestions(_ context.Context, bomID, nodeID int64) (bom.Suggestions, error) {
	cost, _ := money.Parse("12.5", "EUR")
	return bom.Suggestions{
		NodeID: nodeID, Strategy: bom.StrategyCO2, StrategyFrom: 1, RealQuantity: decimal.NewFromInt(2), Currency: "EUR",
		Ranked: []bom.Ranked{{Candidate: bom.Candidate{PartID: 10, OfferID: 20, Cost: &cost, LeadDays: 3, CO2Kg: decimal.RequireFromString("1.5"), HasFacts: true},
			Score: decimal.RequireFromString("0.123456")}},
		Excluded: []bom.Candidate{{PartID: 11, OfferID: 21, Excluded: fmt.Errorf("offer 21: %w", pricing.ErrNoQuote)}},
	}, f.record("Suggestions %d %d", bomID, nodeID)
}
func (f *fakeBOMs) CostAnomalies(_ context.Context, bomID int64, th decimal.Decimal) (bom.CostAnomalies, error) {
	cost, _ := money.Parse("340", "EUR")
	med, _ := money.Parse("100", "EUR")
	ratio := decimal.RequireFromString("3.4")
	return bom.CostAnomalies{Threshold: th, Currency: "EUR", Nodes: []bom.NodeAnomaly{
		{NodeID: 3, Anomaly: pricing.Anomaly{PartID: 10, Category: "Freins", Status: pricing.AnomalyDetected, Cost: &cost,
			Median: &med, Ratio: &ratio, Peers: 6}},
		{NodeID: 4, Anomaly: pricing.Anomaly{PartID: 11, Category: "Vis", Status: pricing.AnomalyNotEvaluable, Peers: 2,
			Err: fmt.Errorf("2 of 5 required: %w", pricing.ErrTooFewPeers)}},
	}}, f.record("CostAnomalies %d %s", bomID, th)
}
func (f *fakeBOMs) SetDestinationCountry(_ context.Context, id int64, c *string) (bom.BOM, error) {
	v := "nil"
	if c != nil {
		v = *c
	}
	b := draft
	b.DestinationCountry = c
	return b, f.record("SetDestinationCountry %d %s", id, v)
}
func (f *fakeBOMs) LandedCosts(_ context.Context, bomID, partID int64, offers []int64) (bom.LandedComparison, error) {
	buy, _ := money.Parse("100", "EUR")
	duty, _ := money.Parse("4.5", "EUR")
	fr := "FR"
	return bom.LandedComparison{PartID: partID, Currency: "EUR", Offers: []bom.OfferLanded{{
		OfferID: offers[0], Origin: "CN", Destination: &fr,
		Landed: pricing.Landed{
			Purchase: pricing.Component{Amount: &buy}, Customs: pricing.Component{Amount: &duty},
			Freight:   pricing.Component{Err: fmt.Errorf("freight CN→FR: %w", pricing.ErrNoFreightRate)},
			Packaging: pricing.Component{Err: pricing.ErrNoPackaging}, Err: pricing.ErrNoFreightRate,
		},
	}}}, f.record("LandedCosts %d %d %v", bomID, partID, offers)
}
func (f *fakeBOMs) MakeOrBuy(_ context.Context, bomID, nodeID int64, req bom.MakeOrBuyRequest) (bom.MakeOrBuyResult, error) {
	be := int64(2000)
	mk, _ := money.Parse("160", "EUR")
	return bom.MakeOrBuyResult{NodeID: nodeID, RealQuantity: decimal.NewFromInt(1), Volume: 1000, Currency: "EUR",
			MakeOrBuy: pricing.MakeOrBuy{MakePerCar: &mk, BreakEvenVolume: &be}},
		f.record("MakeOrBuy %d %d %s %s %s %s %s", bomID, nodeID, req.FixedCosts, req.LogisticsPerUnit, req.Tier1Price, req.Tier1Origin, req.Tier1Packaging)
}
func (f *fakeBOMs) DeleteBOM(_ context.Context, id int64) error {
	return f.record("DeleteBOM %d", id)
}
func (f *fakeBOMs) AddNode(_ context.Context, bomID int64, n bom.NewNode) (bom.Node, error) {
	return part, f.record("AddNode %d parent=%d name=%q part=%s offer=%s qty=%s days=%d",
		bomID, n.ParentID, n.Name, ptrStr(n.PartID), ptrStr(n.OfferID), n.Quantity, n.ProductionDays)
}
func (f *fakeBOMs) SetNodeOffer(_ context.Context, bomID, nodeID, offerID int64) (bom.Node, error) {
	return part, f.record("SetNodeOffer %d %d %d", bomID, nodeID, offerID)
}
func (f *fakeBOMs) SetNodeQuantity(_ context.Context, bomID, nodeID int64, q decimal.Decimal) (bom.Node, error) {
	return part, f.record("SetNodeQuantity %d %d %s", bomID, nodeID, q)
}
func (f *fakeBOMs) SetNodeProductionDays(_ context.Context, bomID, nodeID int64, d int32) (bom.Node, error) {
	return part, f.record("SetNodeProductionDays %d %d %d", bomID, nodeID, d)
}
func (f *fakeBOMs) MoveNode(_ context.Context, bomID, nodeID, parentID int64) (bom.Node, error) {
	return part, f.record("MoveNode %d %d %d", bomID, nodeID, parentID)
}
func (f *fakeBOMs) DeleteNode(_ context.Context, bomID, nodeID int64) error {
	return f.record("DeleteNode %d %d", bomID, nodeID)
}
func (f *fakeBOMs) FreezePrices(_ context.Context, id int64, at time.Time) (bom.BOM, error) {
	b := draft
	b.PriceSnapshotID = ptr(int64(5))
	return b, f.record("FreezePrices %d %s", id, at.Format(time.RFC3339))
}
func (f *fakeBOMs) UsePriceSnapshot(_ context.Context, id, snapshotID int64) (bom.BOM, error) {
	if snapshotID == 404 {
		return bom.BOM{}, fmt.Errorf("use price snapshot %d: %w", snapshotID, pricing.ErrPriceSnapshotNotFound)
	}
	if snapshotID == 409 {
		return bom.BOM{}, fmt.Errorf("use price snapshot %d: offers [11]: %w", snapshotID, pricing.ErrNoQuote)
	}
	b := draft
	b.PriceSnapshotID = ptr(snapshotID)
	return b, f.record("UsePriceSnapshot %d %d", id, snapshotID)
}
func (f *fakeBOMs) UnfreezePrices(_ context.Context, id int64) (bom.BOM, error) {
	return draft, f.record("UnfreezePrices %d", id)
}
func (f *fakeBOMs) Totals(_ context.Context, id int64) (bom.TreeTotals, error) {
	return f.tree, f.record("Totals %d", id)
}

type fakeSnapshots struct {
	err       error
	calls     []string
	published snapshot.Published
}

func (f *fakeSnapshots) record(format string, args ...any) error {
	f.calls = append(f.calls, fmt.Sprintf(format, args...))
	return f.err
}

var published = snapshot.Snapshot{ID: 4, BOMID: 7, PriceSnapshotID: 5, Name: "Citadine", Currency: "EUR",
	Volume: 1000, PublishedAt: testNow}

func (f *fakeSnapshots) Publish(_ context.Context, bomID int64, at time.Time, req snapshot.PublishRequest) (snapshot.Snapshot, error) {
	if req.Note != "" || req.ProposedBy != "" || req.ValidatedBy != "" || len(req.Justifications) > 0 {
		return published, f.record("Publish %d %s note=%q proposed=%q validated=%q justifications=%v", bomID, at.Format(time.RFC3339),
			req.Note, req.ProposedBy, req.ValidatedBy, req.Justifications)
	}
	return published, f.record("Publish %d %s", bomID, at.Format(time.RFC3339))
}
func (f *fakeSnapshots) Get(_ context.Context, id int64) (snapshot.Published, error) {
	return f.published, f.record("Get %d", id)
}
func (f *fakeSnapshots) NewDraft(_ context.Context, id int64) (bom.BOM, error) {
	return draft, f.record("NewDraft %d", id)
}

func serve(t *testing.T, boms *fakeBOMs, snaps *fakeSnapshots, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	h := NewRouter(Services{BOMs: boms, Snapshots: snaps, Now: func() time.Time { return testNow }})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, path, strings.NewReader(body)))
	return rec
}

// TestRoutes : chaque route décode sa requête, appelle la bonne méthode avec les bons arguments et renvoie
// le bon code.
func TestRoutes(t *testing.T) {
	const at = "2026-11-03T10:00:00Z"
	tests := []struct {
		name       string
		method     string
		path       string
		body       string
		wantStatus int
		wantCall   string // "" : aucun appel au service attendu
		wantBody   string // sous-chaîne attendue dans la réponse
	}{
		{"list boms", "GET", "/api/v1/boms", "", 200, "ListBOMs", `"currency":"EUR"`},
		{"create bom", "POST", "/api/v1/boms", `{"name":"Citadine","currency":"EUR","volume":1000}`, 201,
			"CreateBOM Citadine EUR 1000", `"price_snapshot_id":null`},
		{"rename bom", "PATCH", "/api/v1/boms/7", `{"name":"Berline"}`, 200, "RenameBOM 7 Berline", `"id":7`},
		{"update settings", "PATCH", "/api/v1/boms/7", `{"currency":"USD","volume":500}`, 200,
			"UpdateSettings 7 USD 500", `"id":7`},
		{"patch bom name and settings together", "PATCH", "/api/v1/boms/7", `{"name":"B","currency":"USD","volume":5}`,
			400, "", `"error":`},
		{"patch bom currency without volume", "PATCH", "/api/v1/boms/7", `{"currency":"USD"}`, 400, "", `"error":`},
		{"set annual hours", "PATCH", "/api/v1/boms/7", `{"annual_hours":"1600.5"}`, 200,
			"SetAnnualHours 7 1600.5", `"annual_hours":`},
		{"patch bom annual hours with name", "PATCH", "/api/v1/boms/7", `{"name":"B","annual_hours":"1600"}`,
			400, "", `"error":`},
		{"set destination", "PATCH", "/api/v1/boms/7", `{"destination_country":"FR"}`, 200, "SetDestinationCountry 7 FR",
			`"destination_country":"FR"`},
		{"clear destination", "PATCH", "/api/v1/boms/7", `{"destination_country":null}`, 200, "SetDestinationCountry 7 nil",
			`"destination_country":null`},
		{"destination with name", "PATCH", "/api/v1/boms/7", `{"name":"B","destination_country":"FR"}`, 400, "", `"error":`},
		{"landed cost", "GET", "/api/v1/parts/10/landed-cost?bom_id=7&offer_ids=20,21", "", 200, "LandedCosts 7 10 [20 21]",
			`"freight":{"amount":null,"error":"freight CN→FR: no freight rate for this origin and destination","error_code":"no_freight_rate"}`},
		{"landed cost total null when a component is missing", "GET", "/api/v1/parts/10/landed-cost?bom_id=7&offer_ids=20", "", 200,
			"LandedCosts 7 10 [20]", `"total":null`},
		{"landed cost bad offers", "GET", "/api/v1/parts/10/landed-cost?bom_id=7&offer_ids=20,x", "", 400, "", `offer_ids`},
		{"landed cost without bom", "GET", "/api/v1/parts/10/landed-cost?offer_ids=20", "", 400, "", `bom_id`},
		{"make or buy", "POST", "/api/v1/boms/7/nodes/4/make-or-buy",
			`{"fixed_costs":{"amount":"100000","currency":"EUR"},"logistics_per_unit":{"amount":"5","currency":"EUR"},` +
				`"tier1_price":{"amount":"100","currency":"USD"},"tier1_origin":"CN","tier1_packaging":{"amount":"0","currency":"EUR"}}`,
			200, "MakeOrBuy 7 4 100000.00 EUR 5.00 EUR 100.00 USD CN 0.00 EUR", `"make_per_car":{"amount":"160.00","currency":"EUR"}`},
		{"make or buy missing field", "POST", "/api/v1/boms/7/nodes/4/make-or-buy", `{"tier1_origin":"CN"}`, 400, "", `required`},
		{"patch bom empty body", "PATCH", "/api/v1/boms/7", `{}`, 400, "", `"error":`},
		{"delete bom", "DELETE", "/api/v1/boms/7", "", 204, "DeleteBOM 7", ""},
		{"tree", "GET", "/api/v1/boms/7/tree", "", 200, "Totals 7", `"nodes":`},
		{"add sub-assembly", "POST", "/api/v1/boms/7/nodes", `{"parent_id":1,"name":"Roues","quantity":"4","production_days":2}`,
			201, `AddNode 7 parent=1 name="Roues" part=nil offer=nil qty=4 days=2`, `"quantity":"2.5"`},
		{"add part", "POST", "/api/v1/boms/7/nodes", `{"parent_id":2,"part_id":10,"offer_id":20,"quantity":"0.25"}`,
			201, `AddNode 7 parent=2 name="" part=10 offer=20 qty=0.25 days=0`, `"offer_id":20`},
		{"add node without parent", "POST", "/api/v1/boms/7/nodes", `{"name":"Roues","quantity":"4"}`, 400, "", `parent_id is required`},
		{"change offer", "PATCH", "/api/v1/boms/7/nodes/3", `{"offer_id":21}`, 200, "SetNodeOffer 7 3 21", `"id":3`},
		{"change quantity", "PATCH", "/api/v1/boms/7/nodes/3", `{"quantity":"1.5"}`, 200, "SetNodeQuantity 7 3 1.5", `"id":3`},
		{"move node", "PATCH", "/api/v1/boms/7/nodes/3", `{"parent_id":2}`, 200, "MoveNode 7 3 2", `"id":3`},
		{"set production days", "PATCH", "/api/v1/boms/7/nodes/3", `{"production_days":3}`, 200,
			"SetNodeProductionDays 7 3 3", `"id":3`},
		{"set strategy", "PATCH", "/api/v1/boms/7/nodes/3", `{"strategy":"co2"}`, 200, "SetNodeStrategy 7 3 co2", `"strategy":"co2"`},
		{"clear strategy", "PATCH", "/api/v1/boms/7/nodes/3", `{"strategy":null}`, 200, "SetNodeStrategy 7 3 nil", `"strategy":null`},
		{"strategy not a string", "PATCH", "/api/v1/boms/7/nodes/3", `{"strategy":3}`, 400, "", `string or null`},
		{"replace part", "PATCH", "/api/v1/boms/7/nodes/3", `{"part_id":12,"offer_id":22}`, 200, "ReplaceNodePart 7 3 12 22", `"part_id":12`},
		{"part without offer", "PATCH", "/api/v1/boms/7/nodes/3", `{"part_id":12}`, 400, "", `exactly one`},
		{"replace part and quantity", "PATCH", "/api/v1/boms/7/nodes/3", `{"part_id":12,"offer_id":22,"quantity":"2"}`, 400, "", `exactly one`},
		{"suggestions", "GET", "/api/v1/boms/7/nodes/3/suggestions", "", 200, "Suggestions 7 3",
			`"strategy":"co2","strategy_from":1`},
		{"suggestions score rounded on output", "GET", "/api/v1/boms/7/nodes/3/suggestions", "", 200, "Suggestions 7 3",
			`"cost":{"amount":"12.50","currency":"EUR"},"lead_time_days":3,"co2_kg":"1.5","quote":null,"rate":null,"score":"0.1235"`},
		{"suggestions excluded with reason", "GET", "/api/v1/boms/7/nodes/3/suggestions", "", 200, "Suggestions 7 3",
			`"offer_id":21,"current":false,"cost":null,"lead_time_days":null,"co2_kg":null`},
		{"patch node two fields", "PATCH", "/api/v1/boms/7/nodes/3", `{"offer_id":21,"quantity":"2"}`, 400, "", `exactly one`},
		{"patch node no field", "PATCH", "/api/v1/boms/7/nodes/3", `{}`, 400, "", `exactly one`},
		{"delete node", "DELETE", "/api/v1/boms/7/nodes/3", "", 204, "DeleteNode 7 3", ""},
		{"anomalies default threshold", "GET", "/api/v1/boms/7/anomalies", "", 200, "CostAnomalies 7 3",
			`"status":"anomaly","unit_cost":{"amount":"340.00","currency":"EUR"},"median":{"amount":"100.00","currency":"EUR"},"ratio":"3.40"`},
		{"anomalies not evaluable has a reason", "GET", "/api/v1/boms/7/anomalies?threshold=2.5", "", 200, "CostAnomalies 7 2.5",
			`"status":"not_evaluable","unit_cost":null,"median":null,"ratio":null,"peers":2,"missing":0,"reason":"2 of 5 required`},
		{"anomalies bad threshold", "GET", "/api/v1/boms/7/anomalies?threshold=abc", "", 400, "", `not a decimal`},
		{"stock", "GET", "/api/v1/boms/7/stock", "", 200, "Stock 7", `[{"part_id":10,"quantity":"40"}]`},
		{"set part stock", "PUT", "/api/v1/boms/7/stock/10", `{"quantity":"12.5"}`, 200, "SetPartStock 7 10 12.5",
			`{"part_id":10,"quantity":"12.5"}`},
		{"set stock of unknown part", "PUT", "/api/v1/boms/7/stock/404", `{"quantity":"1"}`, 404, "SetPartStock 7 404 1", `"error":`},
		{"set part stock without quantity", "PUT", "/api/v1/boms/7/stock/10", `{}`, 400, "", `quantity is required`},
		{"set schedule", "PUT", "/api/v1/boms/7/schedule", `{"start_on":"2026-12-01","target_on":"2027-02-01"}`, 200,
			"SetSchedule 7 2026-12-01 2027-02-01", `{"start_on":"2026-12-01","target_on":"2027-02-01"}`},
		{"set schedule without target", "PUT", "/api/v1/boms/7/schedule", `{"start_on":"2026-12-01"}`, 200,
			"SetSchedule 7 2026-12-01 nil", `"target_on":null`},
		{"set schedule without start", "PUT", "/api/v1/boms/7/schedule", `{"target_on":"2027-02-01"}`, 400, "", `start_on is required`},
		{"set schedule bad date", "PUT", "/api/v1/boms/7/schedule", `{"start_on":"01/12/2026"}`, 400, "", `not a date`},
		{"clear schedule", "DELETE", "/api/v1/boms/7/schedule", "", 204, "ClearSchedule 7", ""},
		{"freeze prices", "POST", "/api/v1/boms/7/freeze-prices", "", 200, "FreezePrices 7 " + at, `"price_snapshot_id":5`},
		{"unfreeze prices", "DELETE", "/api/v1/boms/7/freeze-prices", "", 200, "UnfreezePrices 7", `"price_snapshot_id":null`},
		{"use existing price snapshot", "PUT", "/api/v1/boms/7/freeze-prices", `{"price_snapshot_id":12}`, 200, "UsePriceSnapshot 7 12", `"price_snapshot_id":12`},
		{"use price snapshot without id", "PUT", "/api/v1/boms/7/freeze-prices", `{}`, 400, "", `price_snapshot_id is required`},
		{"use price snapshot zero id", "PUT", "/api/v1/boms/7/freeze-prices", `{"price_snapshot_id":0}`, 400, "", `positive integer`},
		{"use unknown price snapshot", "PUT", "/api/v1/boms/7/freeze-prices", `{"price_snapshot_id":404}`, 404, "", `price snapshot not found`},
		{"use price snapshot not covering the bom", "PUT", "/api/v1/boms/7/freeze-prices", `{"price_snapshot_id":409}`, 409, "", `offers [11]`},
		{"publish", "POST", "/api/v1/boms/7/publish", "", 201, "Publish 7 " + at, `"published_at":"` + at + `"`},
		{"publish with trace", "POST", "/api/v1/boms/7/publish",
			`{"note":"Jalon","proposed_by":"Arthur P.","validated_by":"Joe","justifications":{"budgets":"revue en V3"}}`, 201,
			`Publish 7 ` + at + ` note="Jalon" proposed="Arthur P." validated="Joe" justifications=map[budgets:revue en V3]`, `"id":4`},
		{"publish unknown field", "POST", "/api/v1/boms/7/publish", `{"notes":"x"}`, 400, "", "unknown field"},
		{"get snapshot", "GET", "/api/v1/snapshots/4", "", 200, "Get 4", `"snapshot":`},
		{"new draft from snapshot", "POST", "/api/v1/snapshots/4/drafts", "", 201, "NewDraft 4", `"id":7`},
		{"unknown field rejected", "POST", "/api/v1/boms", `{"name":"C","currency":"EUR","volume":1,"colour":"red"}`,
			400, "", `unknown field`},
		{"malformed JSON", "POST", "/api/v1/boms", `{"name":`, 400, "", `invalid JSON body`},
		{"trailing data", "POST", "/api/v1/boms", `{"name":"C","currency":"EUR","volume":1} {}`, 400, "", `trailing data`},
		{"non numeric id", "DELETE", "/api/v1/boms/abc", "", 400, "", `not a positive integer`},
		{"non numeric node id", "DELETE", "/api/v1/boms/7/nodes/x", "", 400, "", `not a positive integer`},
		{"zero id", "GET", "/api/v1/boms/0/tree", "", 400, "", `not a positive integer`},
		{"wrong method", "PUT", "/api/v1/boms/7", "", 405, "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			boms, snaps := &fakeBOMs{tree: sampleTree(t)}, &fakeSnapshots{published: samplePublished(t)}
			rec := serve(t, boms, snaps, tt.method, tt.path, tt.body)

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d (body %s)", rec.Code, tt.wantStatus, rec.Body)
			}
			calls := append(boms.calls, snaps.calls...)
			switch {
			case tt.wantCall == "" && len(calls) != 0:
				t.Errorf("calls = %q, want none", calls)
			case tt.wantCall != "" && (len(calls) != 1 || calls[0] != tt.wantCall):
				t.Errorf("calls = %q, want [%q]", calls, tt.wantCall)
			}
			if !strings.Contains(rec.Body.String(), tt.wantBody) {
				t.Errorf("body = %s, want it to contain %s", rec.Body, tt.wantBody)
			}
			if rec.Code >= 400 && rec.Code != 405 {
				var e map[string]string
				if err := json.Unmarshal(rec.Body.Bytes(), &e); err != nil || e["error"] == "" {
					t.Errorf("error body = %s, want {\"error\": \"...\"}", rec.Body)
				}
			}
		})
	}
}

// TestErrorMapping : chaque erreur métier, même enveloppée, devient le bon code HTTP et un message explicite ;
// une erreur inconnue devient 500 sans divulguer son détail.
func TestErrorMapping(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantMsg    string
	}{
		{"bom not found", bom.ErrBOMNotFound, 404, "bom not found"},
		{"node not found", bom.ErrNodeNotFound, 404, "bom node not found"},
		{"snapshot not found", snapshot.ErrSnapshotNotFound, 404, "bom snapshot not found"},
		{"cycle", bom.ErrCycle, 409, "move would create a cycle"},
		{"has snapshots", bom.ErrHasSnapshots, 409, "bom has published snapshots"},
		{"root node", bom.ErrRootNode, 409, "root node cannot be deleted"},
		{"missing price at freeze", pricing.ErrNoQuote, 409, "no price quote"},
		{"missing rate at publish", pricing.ErrNoRate, 409, "no exchange rate"},
		{"publication blocked", snapshot.ErrPublicationBlocked, 409, "publication blocked"},
		{"warning not justified", snapshot.ErrJustificationRequired, 422, "justification"},
		{"invalid input", bom.ErrInvalidInput, 400, "invalid bom input"},
		{"offer of another part", bom.ErrOfferNotForPart, 400, "offer does not sell this part"},
		{"not a part", bom.ErrNotAPart, 400, "node is not a part"},
		{"unknown offer", catalog.ErrOfferNotFound, 400, "offer not found"},
		{"unknown part", catalog.ErrPartNotFound, 400, "part not found"},
		{"unknown currency", money.ErrUnknownCurrency, 400, "unknown currency"},
		{"joined errors keep their sentinel", errors.Join(pricing.ErrNoQuote, pricing.ErrNoRate), 409, "no price quote"},
		{"unexpected failure", errors.New("connection refused"), 500, "internal error"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wrapped := fmt.Errorf("delete bom 7: %w", tt.err)
			rec := serve(t, &fakeBOMs{err: wrapped}, &fakeSnapshots{}, "DELETE", "/api/v1/boms/7", "")

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			var body map[string]string
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode %s: %v", rec.Body, err)
			}
			if !strings.Contains(body["error"], tt.wantMsg) {
				t.Errorf("error = %q, want it to contain %q", body["error"], tt.wantMsg)
			}
			if tt.wantStatus == 500 && strings.Contains(body["error"], "connection refused") {
				t.Errorf("error = %q leaks the internal cause", body["error"])
			}
		})
	}
}

// TestErrorsOnEveryRoute : une erreur du service n'est jamais avalée, quelle que soit la route.
func TestErrorsOnEveryRoute(t *testing.T) {
	tests := []struct{ method, path, body string }{
		{"GET", "/api/v1/boms", ""},
		{"POST", "/api/v1/boms", `{"name":"C","currency":"EUR","volume":1}`},
		{"PATCH", "/api/v1/boms/7", `{"name":"B"}`},
		{"PATCH", "/api/v1/boms/7", `{"currency":"EUR","volume":1}`},
		{"DELETE", "/api/v1/boms/7", ""},
		{"GET", "/api/v1/boms/7/tree", ""},
		{"POST", "/api/v1/boms/7/nodes", `{"parent_id":1,"name":"R","quantity":"1"}`},
		{"PATCH", "/api/v1/boms/7/nodes/3", `{"offer_id":1}`},
		{"PATCH", "/api/v1/boms/7/nodes/3", `{"quantity":"1"}`},
		{"PATCH", "/api/v1/boms/7/nodes/3", `{"parent_id":1}`},
		{"PATCH", "/api/v1/boms/7/nodes/3", `{"production_days":1}`},
		{"DELETE", "/api/v1/boms/7/nodes/3", ""},
		{"POST", "/api/v1/boms/7/freeze-prices", ""},
		{"DELETE", "/api/v1/boms/7/freeze-prices", ""},
		{"POST", "/api/v1/boms/7/publish", ""},
		{"GET", "/api/v1/snapshots/4", ""},
		{"POST", "/api/v1/snapshots/4/drafts", ""},
	}
	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path+" "+tt.body, func(t *testing.T) {
			err := fmt.Errorf("op: %w", bom.ErrBOMNotFound)
			rec := serve(t, &fakeBOMs{err: err}, &fakeSnapshots{err: err}, tt.method, tt.path, tt.body)
			if rec.Code != http.StatusNotFound {
				t.Errorf("status = %d, want 404 (body %s)", rec.Code, rec.Body)
			}
		})
	}
}

// sampleTree : racine EUR, une pièce à 100 EUR et une à 100 USD (taux 0,92), une pièce sans prix sous un
// sous-ensemble. Les totaux viennent du vrai calcul (bom.ComputeTotals).
func sampleTree(t *testing.T) bom.TreeTotals {
	t.Helper()
	one := decimal.NewFromInt(1)
	nodes := []bom.Node{
		{ID: 1, BOMID: 7, Name: "Citadine", Quantity: one},
		{ID: 2, BOMID: 7, ParentID: ptr(int64(1)), PartID: ptr(int64(10)), OfferID: ptr(int64(20)), Quantity: one},
		{ID: 3, BOMID: 7, ParentID: ptr(int64(1)), PartID: ptr(int64(11)), OfferID: ptr(int64(21)), Quantity: one},
		{ID: 4, BOMID: 7, ParentID: ptr(int64(1)), Name: "Freins", Quantity: one},
		{ID: 5, BOMID: 7, ParentID: ptr(int64(4)), PartID: ptr(int64(12)), OfferID: ptr(int64(22)), Quantity: one},
	}
	facts := func(offer int64) *bom.PartFacts {
		return &bom.PartFacts{OfferID: offer, LeadTimeDays: 30, CO2Kg: decimal.RequireFromString("1.5"), MassKg: one}
	}
	calc := make([]bom.CalcNode, len(nodes))
	for i, n := range nodes {
		calc[i] = bom.CalcNode{ID: n.ID, ParentID: n.ParentID, Quantity: n.Quantity}
		if n.OfferID != nil {
			calc[i].Part = facts(*n.OfferID)
		}
	}
	eur, usd := mustMoney(t, "100", "EUR"), mustMoney(t, "100", "USD")
	totals, err := bom.ComputeTotals(bom.TotalsInput{
		Nodes: calc, Currency: "EUR",
		Prices: map[int64]bom.PriceResult{
			20: {Quote: pricing.Quote{OfferID: 20, Price: eur, Source: "sim", QuotedAt: testNow}},
			21: {Quote: pricing.Quote{OfferID: 21, Price: usd, Source: "sim", QuotedAt: testNow}},
			22: {Err: fmt.Errorf("offer 22: %w", pricing.ErrNoQuote)},
		},
		Rates: map[money.Currency]bom.RateResult{
			"USD": {Rate: pricing.Rate{From: "USD", To: "EUR", Rate: decimal.RequireFromString("0.92"), Source: "sim", QuotedAt: testNow}},
		},
	})
	if err != nil {
		t.Fatalf("compute totals: %v", err)
	}
	return bom.TreeTotals{BOM: draft, Nodes: nodes, Totals: totals}
}

func mustMoney(t *testing.T, amount, currency string) money.Money {
	t.Helper()
	m, err := money.Parse(amount, currency)
	if err != nil {
		t.Fatalf("money %s %s: %v", amount, currency, err)
	}
	return m
}

// samplePublished : snapshot figé (racine + une pièce à 80 EUR × 2), totaux calculés avec ses attributs figés.
func samplePublished(t *testing.T) snapshot.Published {
	t.Helper()
	nodes := []snapshot.Node{
		{Key: 1, Name: "Citadine", Quantity: decimal.NewFromInt(1)},
		{Key: 2, ParentKey: ptr(int64(1)), Quantity: decimal.NewFromInt(2), ProductionDays: 3,
			Part: &snapshot.Part{PartID: 10, OfferID: 20, CO2Kg: decimal.RequireFromString("4.2"), MassKg: decimal.NewFromInt(1), LeadTimeDays: 7}},
	}
	totals, err := bom.ComputeTotals(bom.TotalsInput{
		Nodes: []bom.CalcNode{
			{ID: 1, Quantity: decimal.NewFromInt(1)},
			{ID: 2, ParentID: ptr(int64(1)), Quantity: decimal.NewFromInt(2), ProductionDays: 3,
				Part: &bom.PartFacts{OfferID: 20, LeadTimeDays: 7, CO2Kg: decimal.RequireFromString("4.2"), MassKg: decimal.NewFromInt(1)}},
		},
		Currency:        "EUR",
		Prices:          map[int64]bom.PriceResult{20: {Quote: pricing.Quote{OfferID: 20, Price: mustMoney(t, "80", "EUR"), QuotedAt: testNow}}},
		PriceSnapshotID: ptr(int64(5)),
	})
	if err != nil {
		t.Fatalf("compute totals: %v", err)
	}
	return snapshot.Published{Snapshot: published, Nodes: nodes, Totals: totals}
}

// TestTreeBody : l'arbre renvoie chaque nœud avec ses totaux ; montants, quantités et CO2 sont des chaînes,
// un prix manquant est une erreur nommée (cost null), jamais un zéro ; l'explication dit d'où viennent les prix.
func TestTreeBody(t *testing.T) {
	rec := serve(t, &fakeBOMs{tree: sampleTree(t)}, &fakeSnapshots{}, "GET", "/api/v1/boms/7/tree", "")
	if rec.Code != 200 {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
	var got struct {
		BOM   struct{ ID int64 } `json:"bom"`
		Nodes []struct {
			ID     int64          `json:"id"`
			Totals map[string]any `json:"totals"`
		} `json:"nodes"`
		Explanation struct {
			Currency        string           `json:"currency"`
			PriceSnapshotID *int64           `json:"price_snapshot_id"`
			Rates           []map[string]any `json:"rates"`
		} `json:"explanation"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	byID := map[int64]map[string]any{}
	for _, n := range got.Nodes {
		byID[n.ID] = n.Totals
	}
	amountOf := func(v any) string {
		m, ok := v.(map[string]any)
		if !ok {
			return fmt.Sprint(v)
		}
		return fmt.Sprint(m["amount"], " ", m["currency"])
	}
	tests := []struct {
		name string
		got  any
		want any
	}{
		{"bom id", got.BOM.ID, int64(7)},
		{"node count", len(got.Nodes), 5},
		{"eur part cost", amountOf(byID[2]["cost"]), "100.00 EUR"},
		{"usd part converted cost", amountOf(byID[3]["cost"]), "92.00 EUR"},
		{"usd part rate used", byID[3]["rate"].(map[string]any)["rate"], "0.92"},
		{"quote price kept in its currency", amountOf(byID[3]["quote"].(map[string]any)["price"]), "100.00 USD"},
		{"part without price has no cost", byID[5]["cost"], nil},
		{"part without price is named", byID[5]["cost_error_code"], "no_quote"},
		{"parent of missing price", byID[4]["cost_error_code"], "missing_cost"},
		{"root lists missing parts", fmt.Sprint(byID[1]["missing_cost"]), "[5]"},
		{"root cost hidden, not zero", byID[1]["cost"], nil},
		{"co2 is a string", byID[1]["co2_kg"], "4.5"},
		{"real quantity is a string", byID[2]["real_quantity"], "1"},
		{"complete node has empty missing list", fmt.Sprint(byID[2]["missing_cost"]), "[]"},
		{"lead time", byID[1]["lead_time_days"], float64(30)},
		{"explanation currency", got.Explanation.Currency, "EUR"},
		{"live prices", got.Explanation.PriceSnapshotID == nil, true},
		{"explanation rates", len(got.Explanation.Rates), 1},
		{"rate date", got.Explanation.Rates[0]["quoted_at"], "2026-11-03T10:00:00Z"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("got %v (%T), want %v (%T)", tt.got, tt.got, tt.want, tt.want)
			}
		})
	}
}

// TestSnapshotBody : un snapshot publié a la même forme qu'un brouillon (id = clé figée) et ses totaux
// reposent sur son snapshot de prix.
func TestSnapshotBody(t *testing.T) {
	rec := serve(t, &fakeBOMs{}, &fakeSnapshots{published: samplePublished(t)}, "GET", "/api/v1/snapshots/4", "")
	if rec.Code != 200 {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
	body := rec.Body.String()
	for _, want := range []string{
		`"snapshot":{"id":4,"bom_id":7,"price_snapshot_id":5`,
		`"id":2,"parent_id":1,"name":"","part_id":10,"offer_id":20,"quantity":"2","production_days":3`,
		`"cost":{"amount":"160.00","currency":"EUR"}`,
		`"co2_kg":"8.4"`,
		`"lead_time_days":10`,
		`"explanation":{"currency":"EUR","price_snapshot_id":5`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("body = %s\nwant it to contain %s", body, want)
		}
	}
}

// TestTreeBodyLaborAndStock : heures, ETP (arrondi half-even à 2 décimales en sortie, ADR 0024) et stock
// (ADR 0022) dans le JSON de l'arbre. 0,375 ETP → "0.38" (une troncature donnerait "0.37").
func TestTreeBodyLaborAndStock(t *testing.T) {
	one := decimal.NewFromInt(1)
	hours := decimal.RequireFromString("0.375")
	nodes := []bom.Node{
		{ID: 1, BOMID: 7, Name: "Citadine", Quantity: one},
		{ID: 2, BOMID: 7, ParentID: ptr(int64(1)), PartID: ptr(int64(10)), OfferID: ptr(int64(20)), Quantity: one},
		{ID: 3, BOMID: 7, ParentID: ptr(int64(1)), PartID: ptr(int64(11)), OfferID: ptr(int64(21)), Quantity: one},
	}
	calc := []bom.CalcNode{
		{ID: 1, Quantity: one},
		{ID: 2, ParentID: ptr(int64(1)), Quantity: one, Part: &bom.PartFacts{PartID: 10, OfferID: 20, LeadTimeDays: 9, LaborHours: &hours}},
		{ID: 3, ParentID: ptr(int64(1)), Quantity: one, Part: &bom.PartFacts{PartID: 11, OfferID: 21, LeadTimeDays: 4}},
	}
	stock := []bom.PartStock{{PartID: 10, Quantity: decimal.NewFromInt(5)}}
	totals, err := bom.ComputeTotals(bom.TotalsInput{
		Nodes: calc, Currency: "EUR", Volume: 1, AnnualHours: &one, Stock: bom.StockMap(stock),
		Prices: map[int64]bom.PriceResult{
			20: {Quote: pricing.Quote{OfferID: 20, Price: mustMoney(t, "1", "EUR"), QuotedAt: testNow}},
			21: {Quote: pricing.Quote{OfferID: 21, Price: mustMoney(t, "1", "EUR"), QuotedAt: testNow}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	tree := bom.TreeTotals{BOM: draft, Nodes: nodes, Stock: stock, Totals: totals}
	rec := serve(t, &fakeBOMs{tree: tree}, &fakeSnapshots{}, "GET", "/api/v1/boms/7/tree", "")
	if rec.Code != 200 {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
	var got struct {
		Nodes []struct {
			ID     int64          `json:"id"`
			Totals map[string]any `json:"totals"`
		} `json:"nodes"`
		Stock []map[string]any `json:"stock"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	byID := map[int64]map[string]any{}
	for _, n := range got.Nodes {
		byID[n.ID] = n.Totals
	}
	tests := []struct {
		name string
		got  any
		want any
	}{
		{"part labor hours", byID[2]["labor_hours"], "0.375"},
		{"part fte rounded half-even", byID[2]["fte"], "0.38"},
		{"part without hours: no hours", byID[3]["labor_hours"], nil},
		{"part without hours: named error", byID[3]["labor_error"] != nil, true},
		{"root lists the part without hours", fmt.Sprint(byID[1]["missing_labor"]), "[3]"},
		{"root has no fte", byID[1]["fte"], nil},
		{"stocked part covered", byID[2]["stock_covered"], "1"},
		{"stocked part in stock", byID[2]["in_stock"], true},
		{"stocked part own lead time is zero", byID[2]["own_lead_time_days"], float64(0)},
		{"part without stock: covered null", byID[3]["stock_covered"], nil},
		{"root lead time ignores the stocked part", byID[1]["lead_time_days"], float64(4)},
		{"stock list", fmt.Sprint(got.Stock), "[map[part_id:10 quantity:5]]"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("got %v (%T), want %v (%T)", tt.got, tt.got, tt.want, tt.want)
			}
		})
	}
}
