package snapshot_test

import (
	"errors"
	"strconv"
	"testing"

	"bomstudio/internal/bom"
	"bomstudio/internal/money"
	"bomstudio/internal/pricing"
	"bomstudio/internal/pricing/pricingstore"
	"bomstudio/internal/snapshot"
	"bomstudio/internal/supplier"
	"bomstudio/internal/supplier/supplierstore"
)

// costFixture branche les faits de la méthode (ADR 0031) sur la fixture : destination IT, douane et transport
// FR→IT, emballage et outillage des offres, taux budget 2026 USD→EUR, contrat 4 % dès 1 000 sur la roue USD.
// Méthode full + budget, volume programme 50 000.
func costFixture(t *testing.T) (fixture, bom.BOM, *pricingstore.Logistics) {
	t.Helper()
	f := newFixture(t)
	ctx := t.Context()
	logi := pricingstore.NewLogistics(f.tx)
	f.boms.WithCostSources(logi, supplier.NewService(supplierstore.New(f.tx)))
	var supplierID int64
	if err := f.tx.QueryRow(ctx, `SELECT supplier_id FROM offers WHERE id = $1`, f.roueUSD).Scan(&supplierID); err != nil {
		t.Fatal(err)
	}
	f.cat.mu.Lock()
	for id, o := range f.cat.offers {
		o.SupplierID, o.OriginCountry = supplierID, "FR"
		f.cat.offers[id] = o
	}
	f.cat.mu.Unlock()

	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := f.tx.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
	}
	exec(`INSERT INTO customs_duties VALUES ('FR', 'IT', 2)`)
	exec(`INSERT INTO freight_rates VALUES ('FR', 'IT', 0.10, 'EUR')`)
	exec(`UPDATE offers SET packaging_amount = 1, packaging_currency = 'USD', tooling_amount = 50000, tooling_currency = 'EUR' WHERE id = $1`, f.roueUSD)
	exec(`UPDATE offers SET packaging_amount = 0.01, packaging_currency = 'EUR', tooling_amount = 0, tooling_currency = 'EUR' WHERE id = $1`, f.ecrouO)
	exec(`INSERT INTO budget_rates VALUES (2026, 'USD', 'EUR', 0.90) ON CONFLICT (year, from_currency, to_currency) DO UPDATE SET rate = 0.90`)
	var contract int64
	if err := f.tx.QueryRow(ctx, `INSERT INTO contracts (supplier_id, starts_on, ends_on, conditions)
		VALUES ($1, '2026-01-01', '2026-12-31', 'test') RETURNING id`, supplierID).Scan(&contract); err != nil {
		t.Fatal(err)
	}
	exec(`INSERT INTO contract_offers VALUES ($1, $2, $3)`, contract, f.roueUSD, supplierID)
	exec(`INSERT INTO contract_tiers VALUES ($1, 1000, 4)`, contract)

	b, _ := f.newDraft(t)
	it := "IT"
	if _, err := f.boms.SetDestinationCountry(ctx, b.ID, &it); err != nil {
		t.Fatal(err)
	}
	pv := int32(50000)
	b, err := f.boms.SetCostMethod(ctx, b.ID, bom.CostMethod{Basis: bom.BasisFull, FX: bom.FXBudget, ProgramVolume: &pv})
	if err != nil {
		t.Fatal(err)
	}
	return f, b, logi
}

func rootCost(t *testing.T, tt bom.Totals) money.Money {
	t.Helper()
	for _, n := range tt.Nodes {
		if r, _ := tt.Node(n.NodeID); n.NodeID == tt.Nodes[0].NodeID {
			if r.Cost == nil {
				t.Fatalf("root has no cost: %v", r.CostErr)
			}
			return *r.Cost
		}
	}
	t.Fatal("no root")
	return money.Money{}
}

// Publier puis modifier douane, transport, emballage, outillage, taux budget et contrat : le snapshot rejoue
// exactement le même coût (au centime et au-delà), le brouillon suit les nouvelles tables.
func TestSnapshotReplaysFullCostAfterTablesChange(t *testing.T) {
	f, b, _ := costFixture(t)
	ctx := t.Context()
	draft, err := f.boms.Totals(ctx, b.ID)
	if err != nil {
		t.Fatal(err)
	}
	before := rootCost(t, draft.Totals)
	// roue : 360 − 14.40 + 6.912 + 3.20 + 3.60 + 1 ; écrou : 20 + 0.40 + 0.02 + 0.20
	if before.Round().String() != "380.93 EUR" {
		t.Fatalf("draft full/budget cost = %s, want 380.93 EUR", before)
	}
	s := f.publish(t, b.ID)
	if s.CostMethod == nil || s.CostMethod.Basis != bom.BasisFull || s.CostAsOf == nil || !s.CostAsOf.Equal(t0) {
		t.Fatalf("frozen method %+v as of %v, want full as of %s", s.CostMethod, s.CostAsOf, t0)
	}

	changes := []string{
		`UPDATE customs_duties SET duty_percent = 10 WHERE origin_country = 'FR' AND destination_country = 'IT'`,
		`UPDATE freight_rates SET amount_per_kg = 1 WHERE origin_country = 'FR' AND destination_country = 'IT'`,
		`UPDATE offers SET packaging_amount = 5, tooling_amount = 900000 WHERE id = ` + itoa(f.roueUSD),
		`UPDATE budget_rates SET rate = 0.80 WHERE year = 2026 AND from_currency = 'USD' AND to_currency = 'EUR'`,
		`UPDATE contracts SET ends_on = '2026-01-31' WHERE id IN (SELECT contract_id FROM contract_offers WHERE offer_id = ` + itoa(f.roueUSD) + `)`,
	}
	for _, sql := range changes {
		if _, err := f.tx.Exec(ctx, sql); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
	}
	after, err := f.boms.Totals(ctx, b.ID)
	if err != nil {
		t.Fatal(err)
	}
	if moved := rootCost(t, after.Totals); moved.Amount().Equal(before.Amount()) {
		t.Fatalf("draft cost did not follow the tables (%s): the replay check below would prove nothing", moved)
	}
	got := f.get(t, s.ID)
	if replay := rootCost(t, got.Totals); !replay.Amount().Equal(before.Amount()) {
		t.Errorf("snapshot replays %s, want exactly %s", replay.Amount(), before.Amount())
	}
	if got.Totals.Explanation.Method.Basis != bom.BasisFull || len(got.Totals.Explanation.BudgetRates) != 1 ||
		!got.Totals.Explanation.BudgetRates[0].Rate.Equal(dec("0.90")) {
		t.Errorf("explanation = %+v, want full with frozen budget rate 0.90", got.Totals.Explanation)
	}
	bd, err := f.snaps.CostBreakdown(ctx, s.ID, got.Nodes[0].Key)
	if err != nil || !bd.Total.Amount().Equal(before.Amount()) {
		t.Errorf("snapshot breakdown total = %v, %v", bd.Total, err)
	}

	// Un nouveau brouillon garde la méthode figée.
	nd, err := f.snaps.NewDraft(ctx, s.ID)
	if err != nil {
		t.Fatal(err)
	}
	if nd.CostMethod.Basis != bom.BasisFull || nd.CostMethod.FX != bom.FXBudget || *nd.CostMethod.ProgramVolume != 50000 {
		t.Errorf("new draft method = %+v", nd.CostMethod)
	}

	// Les faits figés sont immuables, même en SQL direct.
	for _, sql := range []string{
		`UPDATE bom_snapshot_offer_costs SET customs_percent = 0 WHERE snapshot_id = ` + itoa(s.ID),
		`DELETE FROM bom_snapshot_budget_rates WHERE snapshot_id = ` + itoa(s.ID),
	} {
		sp, err := f.tx.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := sp.Exec(ctx, sql); err == nil {
			t.Errorf("%s: accepted, want rejected by trigger", sql)
		}
		_ = sp.Rollback(ctx)
	}
}

// Une composante manquante de la méthode refuse la publication : rien n'est figé à zéro.
func TestPublishRefusesIncompleteCost(t *testing.T) {
	tests := []struct {
		name    string
		sql     string
		wantErr error
	}{
		{"emballage non renseigné", `UPDATE offers SET packaging_amount = NULL, packaging_currency = NULL WHERE id = $1`, pricing.ErrNoPackaging},
		{"outillage non renseigné", `UPDATE offers SET tooling_amount = NULL, tooling_currency = NULL WHERE id = $1`, bom.ErrNoTooling},
		{"taux budget absent", `DELETE FROM budget_rates WHERE year = 2026 AND from_currency = 'USD' AND $1::bigint > 0`, bom.ErrNoBudgetRate},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, b, _ := costFixture(t)
			if _, err := f.tx.Exec(t.Context(), tt.sql, f.roueUSD); err != nil {
				t.Fatal(err)
			}
			_, err := f.snaps.Publish(t.Context(), b.ID, t1, anyWarning)
			if !errors.Is(err, snapshot.ErrIncompleteCost) || !errors.Is(err, tt.wantErr) {
				t.Fatalf("publish err = %v, want ErrIncompleteCost and %v", err, tt.wantErr)
			}
		})
	}
}

func itoa(v int64) string { return strconv.FormatInt(v, 10) }
