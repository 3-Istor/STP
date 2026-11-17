package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"

	"bomstudio/internal/bom"
	"bomstudio/internal/bom/bomstore"
	"bomstudio/internal/catalog"
	"bomstudio/internal/catalog/catalogstore"
	"bomstudio/internal/money"
	"bomstudio/internal/pricing"
	"bomstudio/internal/pricing/pricingstore"
	"bomstudio/internal/snapshot"
	"bomstudio/internal/snapshot/snapshotstore"
	"bomstudio/internal/supplier"
	"bomstudio/internal/supplier/supplierstore"
)

// txPricing : le stub de prix, dont chaque gel a aussi sa ligne price_snapshots (FK des snapshots de BOM).
type txPricing struct {
	*pricing.Stub
	tx    pgx.Tx
	stubs map[int64]int64
}

func (p *txPricing) FreezePrices(ctx context.Context, bomID int64, offers []int64, cur []money.Currency, at time.Time) (int64, error) {
	stubID, err := p.Stub.FreezePrices(ctx, bomID, offers, cur, at)
	if err != nil {
		return 0, err
	}
	var id int64
	if err := p.tx.QueryRow(ctx, `INSERT INTO price_snapshots (scope, taken_at) VALUES ('catalog', $1) RETURNING id`, at).Scan(&id); err != nil {
		return 0, err
	}
	p.stubs[id] = stubID
	return id, nil
}

func (p *txPricing) SnapshotOfferPrice(ctx context.Context, id, offerID int64) (pricing.Quote, error) {
	return p.Stub.SnapshotOfferPrice(ctx, p.stubs[id], offerID)
}

func (p *txPricing) SnapshotRate(ctx context.Context, id int64, from, to money.Currency) (pricing.Rate, error) {
	return p.Stub.SnapshotRate(ctx, p.stubs[id], from, to)
}

// Versions et publication sur une vraie base (ADR 0032, 0033) : une porte vendue par deux fournisseurs, une
// BOM publiée en V1 avec sa trace, un brouillon B qui change de fournisseur, la comparaison, les versions et
// les pistes de réduction.
func TestVersionsAndPublicationRoutes(t *testing.T) {
	_, tx := catalogAPI(t)
	ctx := t.Context()
	scan := func(sql string, args []any, dest ...any) {
		t.Helper()
		if err := tx.QueryRow(ctx, sql, args...).Scan(dest...); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
	}
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(69)`); err != nil {
		t.Fatal(err)
	}
	var arden, tier1, door, offA, offT int64
	scan(`INSERT INTO suppliers (name) VALUES ('Arden test') RETURNING id`, nil, &arden)
	scan(`INSERT INTO suppliers (name) VALUES ('Tier 1 PL') RETURNING id`, nil, &tier1)
	scan(`INSERT INTO parts (name, mass_kg, co2_kg_per_unit, production_days) VALUES ('Porte avant nue', 2, 1, 0) RETURNING id`, nil, &door)
	const offer = `INSERT INTO offers (part_id, supplier_id, currency, price_min, price_max, lead_time_days, origin_country)
		VALUES ($1, $2, $3, 1, 200, 10, 'PL') RETURNING id`
	scan(offer, []any{door, arden, "GBP"}, &offA)
	scan(offer, []any{door, tier1, "EUR"}, &offT)
	at := time.Date(2026, 2, 15, 10, 0, 0, 0, time.UTC)
	gbp, _ := money.Parse("100", "GBP")
	eur, _ := money.Parse("110", "EUR")
	prices := &txPricing{Stub: pricing.NewStub(
		[]pricing.Quote{{OfferID: offA, Price: gbp, Source: "sim", QuotedAt: at}, {OfferID: offT, Price: eur, Source: "sim", QuotedAt: at}},
		[]pricing.Rate{{From: "GBP", To: "EUR", Rate: decimal.RequireFromString("1.20"), Source: "sim", QuotedAt: at}}),
		tx: tx, stubs: map[int64]int64{}}
	cat := catalog.NewService(catalogstore.New(tx), time.Now)
	sup := supplier.NewService(supplierstore.New(tx))
	boms := bom.NewService(bomstore.New(tx), cat, prices).WithCostSources(pricingstore.NewLogistics(tx), sup).WithIdeas(bomstore.New(tx))
	snaps := snapshot.NewService(snapshotstore.New(tx), boms, cat, prices).WithSupplierNames(sup)
	h := NewRouter(Services{BOMs: boms, Snapshots: snaps, Now: func() time.Time { return at }})

	var a bomJSON
	if err := json.Unmarshal(apiCall(t, h, "POST", "/api/v1/boms", `{"name":"Multipla","currency":"EUR","volume":200}`,
		http.StatusCreated).Body.Bytes(), &a); err != nil {
		t.Fatal(err)
	}
	_, nodes, _ := boms.Tree(ctx, a.ID)
	if _, err := boms.AddNode(ctx, a.ID, bom.NewNode{ParentID: nodes[0].ID, PartID: &door, OfferID: &offA, Quantity: decimal.NewFromInt(2)}); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO bom_target_budgets VALUES ($1, $2, 250, 'EUR')`, a.ID, nodes[0].ID); err != nil {
		t.Fatal(err)
	}
	base := fmt.Sprintf("/api/v1/boms/%d", a.ID)

	var checks []checkJSON
	if err := json.Unmarshal(apiCall(t, h, "GET", base+"/publication-checks", "", http.StatusOK).Body.Bytes(), &checks); err != nil {
		t.Fatal(err)
	}
	levels := map[string]string{}
	for _, c := range checks {
		levels[c.Code] = c.Level
		if c.NodeIDs == nil || c.Title == "" {
			t.Errorf("check %+v: node_ids must be [] and title set", c)
		}
	}
	if len(checks) != 7 || levels["prices_frozen"] != "warn" || levels["offers_active"] != "ok" || levels["expired_contracts"] != "ok" {
		t.Fatalf("checks = %+v", checks)
	}

	apiCall(t, h, "POST", base+"/publish", "", http.StatusUnprocessableEntity)
	var v1 snapshotJSON
	if err := json.Unmarshal(apiCall(t, h, "POST", base+"/publish",
		`{"note":"Jalon de lancement","proposed_by":"Arthur P.","validated_by":"Joe","justifications":{"prices_frozen":"figé à la publication"}}`,
		http.StatusCreated).Body.Bytes(), &v1); err != nil {
		t.Fatal(err)
	}
	if v1.ValidatedBy == nil || *v1.ValidatedBy != "Joe" {
		t.Errorf("published trace = %+v", v1)
	}
	var pub publishedJSON
	if err := json.Unmarshal(apiCall(t, h, "GET", fmt.Sprintf("/api/v1/snapshots/%d", v1.ID), "", http.StatusOK).Body.Bytes(), &pub); err != nil {
		t.Fatal(err)
	}
	if len(pub.Checks) != 7 || pub.Snapshot.Note == nil || *pub.Snapshot.Note != "Jalon de lancement" {
		t.Errorf("snapshot checks/note = %+v / %v", pub.Checks, pub.Snapshot.Note)
	}

	var b bomJSON
	if err := json.Unmarshal(apiCall(t, h, "POST", fmt.Sprintf("/api/v1/snapshots/%d/drafts", v1.ID), "", http.StatusCreated).Body.Bytes(), &b); err != nil {
		t.Fatal(err)
	}
	_, bn, _ := boms.Tree(ctx, b.ID)
	if _, err := boms.SetNodeOffer(ctx, b.ID, bn[1].ID, offT); err != nil {
		t.Fatal(err)
	}

	t.Run("comparaison V1 → B : Σ causes = écart, explication nommée", func(t *testing.T) {
		var c compareJSON
		body := apiCall(t, h, "GET", fmt.Sprintf("/api/v1/compare?from=snapshot:%d&to=bom:%d", v1.ID, b.ID), "", http.StatusOK).Body.Bytes()
		if err := json.Unmarshal(body, &c); err != nil {
			t.Fatal(err)
		}
		// 2 × 100 × 1.20 = 240 → 2 × 110 = 220
		sum := decimal.Zero
		for _, k := range c.Causes {
			sum = sum.Add(k.Amount.Amount())
		}
		if c.From.Label != "V1" || c.To.Label != "Brouillon B" || !c.Delta.Cost.Amount().Equal(decimal.NewFromInt(-20)) ||
			!sum.Equal(c.Delta.Cost.Amount()) || len(c.Causes) != 1 || c.Causes[0].Kind != "offer_change" ||
			!strings.Contains(c.Causes[0].Explanation, "Porte avant nue : Arden test → Tier 1 PL") {
			t.Errorf("compare = %s", body)
		}
		if len(c.ChangedParts) != 1 || c.ChangedParts[0].Path != "Porte avant nue" || c.ChangedParts[0].Before != "Arden test · 2 u." {
			t.Errorf("changed parts = %+v", c.ChangedParts)
		}
	})
	for _, q := range []struct {
		query  string
		status int
	}{
		{"from=snapshot:x&to=bom:1", http.StatusBadRequest},
		{"to=bom:1", http.StatusBadRequest},
		{fmt.Sprintf("from=snapshot:%d&to=bom:999999999", v1.ID), http.StatusNotFound},
	} {
		apiCall(t, h, "GET", "/api/v1/compare?"+q.query, "", q.status)
	}

	t.Run("versions : A, V1, B avec trace, dates à la seconde et % du budget", func(t *testing.T) {
		var vs []versionJSON
		if err := json.Unmarshal(apiCall(t, h, "GET", fmt.Sprintf("/api/v1/boms/%d/versions", b.ID), "", http.StatusOK).Body.Bytes(), &vs); err != nil {
			t.Fatal(err)
		}
		got := map[string]versionJSON{}
		for _, v := range vs {
			got[v.Label] = v
			if v.At.Nanosecond() != 0 {
				t.Errorf("%s at %s: not to the second", v.Label, v.At)
			}
		}
		if len(vs) != 3 || got["V1"].ProposedBy == nil || got["V1"].Ref != fmt.Sprintf("snapshot:%d", v1.ID) ||
			got["Brouillon B"].BudgetPercent == nil || *got["Brouillon B"].BudgetPercent != "88.0" {
			t.Errorf("versions = %+v", vs)
		}
	})

	t.Run("pistes de réduction : création, total ouvert, reste à trouver, modification, suppression", func(t *testing.T) {
		ideas := fmt.Sprintf("/api/v1/boms/%d/ideas", b.ID)
		var i1 ideaJSON
		if err := json.Unmarshal(apiCall(t, h, "POST", ideas, fmt.Sprintf(
			`{"title":"Portes nues → Tier 1 PL","owner":"Brian P.","estimated_gain":{"amount":"20","currency":"EUR"},"status":"in_progress","draft_bom_id":%d}`,
			b.ID), http.StatusCreated).Body.Bytes(), &i1); err != nil {
			t.Fatal(err)
		}
		apiCall(t, h, "POST", ideas, `{"title":"Jante","estimated_gain":{"amount":"5","currency":"EUR"}}`, http.StatusCreated)
		apiCall(t, h, "POST", ideas, `{"title":"x"}`, http.StatusBadRequest)
		apiCall(t, h, "POST", ideas, `{"title":"x","estimated_gain":{"amount":"5","currency":"USD"}}`, http.StatusBadRequest)
		apiCall(t, h, "POST", ideas, `{"title":"x","estimated_gain":{"amount":"5","currency":"EUR"},"status":"maybe"}`, http.StatusBadRequest)
		var sum ideasJSON
		if err := json.Unmarshal(apiCall(t, h, "GET", ideas, "", http.StatusOK).Body.Bytes(), &sum); err != nil {
			t.Fatal(err)
		}
		// B : 220 € pour 250 € : écart −30, pistes 25, reste −55.
		if len(sum.Ideas) != 2 || sum.OpenTotal.String() != "25.00 EUR" || sum.GapToTarget == nil ||
			sum.GapToTarget.String() != "-30.00 EUR" || sum.Remaining.String() != "-55.00 EUR" {
			t.Errorf("ideas = %+v", sum)
		}
		var patched ideaJSON
		if err := json.Unmarshal(apiCall(t, h, "PATCH", fmt.Sprintf("%s/%d", ideas, i1.ID), `{"status":"done","draft_bom_id":null}`,
			http.StatusOK).Body.Bytes(), &patched); err != nil {
			t.Fatal(err)
		}
		if patched.Status != "done" || patched.DraftBOMID != nil || patched.Title != i1.Title {
			t.Errorf("patched = %+v", patched)
		}
		apiCall(t, h, "PATCH", fmt.Sprintf("%s/%d", ideas, i1.ID), `{"draft_bom_id":"x"}`, http.StatusBadRequest)
		apiCall(t, h, "DELETE", fmt.Sprintf("%s/%d", ideas, i1.ID), "", http.StatusNoContent)
		apiCall(t, h, "DELETE", fmt.Sprintf("%s/%d", ideas, i1.ID), "", http.StatusNotFound)
	})

	t.Run("contrôle bloquant : 409", func(t *testing.T) {
		if _, err := tx.Exec(ctx, `UPDATE offers SET archived_at = now() WHERE id = $1`, offT); err != nil {
			t.Fatal(err)
		}
		apiCall(t, h, "POST", fmt.Sprintf("/api/v1/boms/%d/publish", b.ID),
			`{"justifications":{"prices_frozen":"x","budgets":"y","target_date":"z"}}`, http.StatusConflict)
	})
}
