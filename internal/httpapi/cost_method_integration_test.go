package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

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

// ADR 0031, routes de la méthode de coût sur une vraie base : une pièce GB (2 kg, 100 GBP le 2026-02-15) ×3
// dans une BOM EUR de volume 200 livrée en IT, contrat 5 % dès 500, taux budget 2026 GBP→EUR 1.15.
func TestCostMethodRoutes(t *testing.T) {
	h, tx := catalogAPI(t) // transaction annulée en fin de test
	ctx := t.Context()
	scan := func(sql string, args []any, dest ...any) {
		t.Helper()
		if err := tx.QueryRow(ctx, sql, args...).Scan(dest...); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
	}
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := tx.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
	}
	// Les tables de BOM sont partagées avec d'autres paquets de test (même verrou que bomstore, snapshot).
	exec(`SELECT pg_advisory_xact_lock(69)`)
	var sup, part, offer int64
	scan(`INSERT INTO suppliers (name) VALUES ('Arden test') RETURNING id`, nil, &sup)
	scan(`INSERT INTO parts (name, mass_kg, co2_kg_per_unit, production_days) VALUES ('Porte avant nue', 2, 0, 0) RETURNING id`, nil, &part)
	scan(`INSERT INTO offers (part_id, supplier_id, currency, price_min, price_max, lead_time_days, origin_country,
		packaging_amount, packaging_currency, tooling_amount, tooling_currency)
		VALUES ($1, $2, 'GBP', 90, 110, 10, 'GB', 1, 'GBP', 60000, 'EUR') RETURNING id`, []any{part, sup}, &offer)
	var contract int64
	scan(`INSERT INTO contracts (supplier_id, starts_on, ends_on, conditions) VALUES ($1, '2026-01-01', '2026-03-31', 'x') RETURNING id`,
		[]any{sup}, &contract)
	exec(`INSERT INTO contract_offers VALUES ($1, $2, $3)`, contract, offer, sup)
	exec(`INSERT INTO contract_tiers VALUES ($1, 500, 5)`, contract)
	exec(`INSERT INTO customs_duties VALUES ('GB', 'IT', 4) ON CONFLICT (origin_country, destination_country) DO UPDATE SET duty_percent = 4`)
	exec(`INSERT INTO freight_rates VALUES ('GB', 'IT', 0.50, 'EUR') ON CONFLICT (origin_country, destination_country) DO UPDATE SET amount_per_kg = 0.50, currency = 'EUR'`)
	exec(`INSERT INTO budget_rates VALUES (2026, 'GBP', 'EUR', 1.15) ON CONFLICT (year, from_currency, to_currency) DO UPDATE SET rate = 1.15`)

	quotedAt := time.Date(2026, 2, 15, 10, 0, 0, 0, time.UTC)
	price, _ := money.Parse("100", "GBP")
	prices := pricing.NewStub([]pricing.Quote{{OfferID: offer, Price: price, Source: "sim", QuotedAt: quotedAt}},
		[]pricing.Rate{{From: "GBP", To: "EUR", Rate: decimal.RequireFromString("1.20"), Source: "sim", QuotedAt: quotedAt}})
	cat := catalog.NewService(catalogstore.New(tx), time.Now)
	boms := bom.NewService(bomstore.New(tx), cat, prices).
		WithCostSources(pricingstore.NewLogistics(tx), supplier.NewService(supplierstore.New(tx)))
	h = NewRouter(Services{BOMs: boms, Snapshots: snapshot.NewService(snapshotstore.New(tx), boms, cat, prices)})

	var created bomJSON
	if err := json.Unmarshal(apiCall(t, h, "POST", "/api/v1/boms", `{"name":"Multipla","currency":"EUR","volume":200}`,
		http.StatusCreated).Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	it := "IT"
	if _, err := boms.SetDestinationCountry(ctx, created.ID, &it); err != nil {
		t.Fatal(err)
	}
	_, nodes, _ := boms.Tree(ctx, created.ID)
	root := nodes[0].ID
	door, err := boms.AddNode(ctx, created.ID, bom.NewNode{ParentID: root, PartID: &part, OfferID: &offer, Quantity: decimal.NewFromInt(3)})
	if err != nil {
		t.Fatal(err)
	}
	base := fmt.Sprintf("/api/v1/boms/%d", created.ID)

	t.Run("méthode par défaut : purchase + latest", func(t *testing.T) {
		got := apiCall(t, h, "GET", base+"/cost-method", "", http.StatusOK).Body.String()
		if want := `{"basis":"purchase","fx":"latest","program_volume":null}` + "\n"; got != want {
			t.Errorf("GET cost-method = %s, want %s", got, want)
		}
	})

	puts := []struct {
		name, body string
		status     int
	}{
		{"full sans volume programme : 400", `{"basis":"full","fx":"budget","program_volume":null}`, http.StatusBadRequest},
		{"base inconnue : 400", `{"basis":"gross","fx":"latest","program_volume":null}`, http.StatusBadRequest},
		{"champ inconnu : 400", `{"basis":"full","fx":"budget","program_volume":1000,"x":1}`, http.StatusBadRequest},
		{"full + budget, volume 1 000 : 200", `{"basis":"full","fx":"budget","program_volume":1000}`, http.StatusOK},
	}
	for _, tt := range puts {
		t.Run(tt.name, func(t *testing.T) { apiCall(t, h, "PUT", base+"/cost-method", tt.body, tt.status) })
	}
	apiCall(t, h, "GET", "/api/v1/boms/999999999/cost-method", "", http.StatusNotFound)

	t.Run("l'arbre suit la méthode et l'explication la porte", func(t *testing.T) {
		var tree treeJSON
		if err := json.Unmarshal(apiCall(t, h, "GET", base+"/tree", "", http.StatusOK).Body.Bytes(), &tree); err != nil {
			t.Fatal(err)
		}
		e := tree.Explanation
		if e.CostMethod.Basis != "full" || e.CostMethod.FX != "budget" || e.CostAsOf == nil || !e.CostAsOf.Equal(quotedAt) ||
			len(e.BudgetRates) != 1 || e.BudgetRates[0].Rate != "1.15" {
			t.Errorf("explanation = %+v", e)
		}
		// 345 − 17.25 + 13.11 + 3 + 3.45 + 60
		if c := tree.Nodes[0].Totals.Cost; c == nil || c.Round().String() != "407.31 EUR" {
			t.Errorf("root cost = %v, want 407.31 EUR", c)
		}
	})

	t.Run("ventilation d'une pièce : lignes par nature, source et date, Σ = total", func(t *testing.T) {
		var bd breakdownJSON
		if err := json.Unmarshal(apiCall(t, h, "GET", fmt.Sprintf("%s/nodes/%d/cost-breakdown", base, door.ID), "", http.StatusOK).Body.Bytes(), &bd); err != nil {
			t.Fatal(err)
		}
		want := []struct{ kind, amount string }{
			{"price", "360.00"}, {"fx", "-15.00"}, {"discount", "-17.25"}, {"customs", "13.11"},
			{"freight", "3.00"}, {"packaging", "3.45"}, {"tooling", "60.00"},
		}
		if len(bd.Lines) != len(want) {
			t.Fatalf("%d lines, want %d: %+v", len(bd.Lines), len(want), bd.Lines)
		}
		sum := decimal.Zero
		for i, w := range want {
			l := bd.Lines[i]
			if l.Kind != w.kind || l.Amount.Round().Amount().StringFixed(2) != w.amount || l.Label == "" || l.Source == "" {
				t.Errorf("line %d = %+v, want %s %s", i, l, w.kind, w.amount)
			}
			sum = sum.Add(l.Amount.Amount())
		}
		if !sum.Equal(bd.Total.Amount()) || bd.Lines[0].AsOf == nil || bd.Method.Basis != "full" {
			t.Errorf("Σ %s ≠ total %s, or missing as_of/method", sum, bd.Total)
		}
	})

	t.Run("ventilation de la racine : agrégée par nature", func(t *testing.T) {
		var bd breakdownJSON
		if err := json.Unmarshal(apiCall(t, h, "GET", fmt.Sprintf("%s/nodes/%d/cost-breakdown", base, root), "", http.StatusOK).Body.Bytes(), &bd); err != nil {
			t.Fatal(err)
		}
		if len(bd.Lines) != 7 || bd.Lines[0].AsOf != nil || bd.Total.Round().String() != "407.31 EUR" {
			t.Errorf("root breakdown = %+v", bd)
		}
	})
	apiCall(t, h, "GET", fmt.Sprintf("%s/nodes/999999999/cost-breakdown", base), "", http.StatusNotFound)

	t.Run("budgets cibles : vert, orange, rouge selon le coût de la méthode", func(t *testing.T) {
		exec(`INSERT INTO bom_target_budgets VALUES ($1, $2, 500, 'EUR'), ($1, $3, 300, 'EUR')`, created.ID, root, door.ID)
		var got []budgetJSON
		if err := json.Unmarshal(apiCall(t, h, "GET", base+"/budgets", "", http.StatusOK).Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		if len(got) != 2 || got[0].State != "ok" || got[1].State != "over" || got[1].Delta == nil {
			t.Errorf("budgets = %+v", got)
		}
	})

	t.Run("taux budget absent : erreur nommée sur la pièce, 409 sur la ventilation", func(t *testing.T) {
		exec(`DELETE FROM budget_rates WHERE year = 2026 AND from_currency = 'GBP' AND to_currency = 'EUR'`)
		var tree treeJSON
		if err := json.Unmarshal(apiCall(t, h, "GET", base+"/tree", "", http.StatusOK).Body.Bytes(), &tree); err != nil {
			t.Fatal(err)
		}
		for _, n := range tree.Nodes {
			wantCode := "missing_cost"
			if n.ID == door.ID {
				wantCode = "no_budget_rate"
			}
			if n.Totals.Cost != nil || n.Totals.CostErrorCode == nil || *n.Totals.CostErrorCode != wantCode {
				t.Errorf("node %d: cost %v, code %v; want no cost and %s", n.ID, n.Totals.Cost, n.Totals.CostErrorCode, wantCode)
			}
		}
		apiCall(t, h, "GET", fmt.Sprintf("%s/nodes/%d/cost-breakdown", base, door.ID), "", http.StatusConflict)
	})
}
