package snapshot_test

import (
	"errors"
	"testing"

	"bomstudio/internal/bom"
)

// SI les budgets figés d'un snapshot ne respectent pas la règle de l'ADR 0042 (un sous-budget qui dépasse son
// ensemble : donnée incohérente) ALORS le nouveau brouillon échoue avec l'erreur nommée, et aucun brouillon
// partiel ne reste.
func TestNewDraftRefusesFrozenBudgetsThatBreakTheRule(t *testing.T) {
	f := newFixture(t)
	ctx := t.Context()
	b, roue := f.newDraft(t)
	ecrou := roueChild(t, f, b.ID, roue)
	// Écrit en SQL : le service refuserait ce brouillon (l'Écrou à 80 € sous la Roue à 60 €).
	for _, row := range []struct {
		node   int64
		amount string
	}{{roue, "60"}, {ecrou, "80"}} {
		if _, err := f.tx.Exec(ctx, `INSERT INTO bom_target_budgets(bom_id, node_id, amount, currency) VALUES ($1,$2,$3,'EUR')`, b.ID, row.node, row.amount); err != nil {
			t.Fatal(err)
		}
	}
	s := f.publish(t, b.ID)
	if _, err := f.snaps.NewDraft(ctx, s.ID); !errors.Is(err, bom.ErrBudgetExceedsParent) {
		t.Fatalf("new draft: %v, want ErrBudgetExceedsParent", err)
	}
	var n int
	if err := f.tx.QueryRow(ctx, `SELECT count(*) FROM boms WHERE source_snapshot_id = $1`, s.ID).Scan(&n); err != nil || n != 0 {
		t.Fatalf("%d partial draft(s) left (%v)", n, err)
	}
}

// roueChild : l'Écrou, enfant de la pièce Roue dans le brouillon.
func roueChild(t *testing.T, f fixture, bomID, roue int64) int64 {
	t.Helper()
	_, nodes, err := f.boms.Tree(t.Context(), bomID)
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range nodes {
		if n.ParentID != nil && *n.ParentID == roue {
			return n.ID
		}
	}
	t.Fatal("no child under the Roue part")
	return 0
}
