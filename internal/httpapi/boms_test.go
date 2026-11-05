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

func (f *fakeSnapshots) Publish(_ context.Context, bomID int64, at time.Time) (snapshot.Snapshot, error) {
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
		{"patch node two fields", "PATCH", "/api/v1/boms/7/nodes/3", `{"offer_id":21,"quantity":"2"}`, 400, "", `exactly one`},
		{"patch node no field", "PATCH", "/api/v1/boms/7/nodes/3", `{}`, 400, "", `exactly one`},
		{"delete node", "DELETE", "/api/v1/boms/7/nodes/3", "", 204, "DeleteNode 7 3", ""},
		{"freeze prices", "POST", "/api/v1/boms/7/freeze-prices", "", 200, "FreezePrices 7 " + at, `"price_snapshot_id":5`},
		{"unfreeze prices", "DELETE", "/api/v1/boms/7/freeze-prices", "", 200, "UnfreezePrices 7", `"price_snapshot_id":null`},
		{"publish", "POST", "/api/v1/boms/7/publish", "", 201, "Publish 7 " + at, `"published_at":"` + at + `"`},
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
