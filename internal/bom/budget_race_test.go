package bom

import (
	"context"
	"testing"
	"time"

	"bomstudio/internal/money"
	"bomstudio/internal/pricing"
)

// racyStore : juste après la première lecture des budgets (hors transaction), un autre écrivain pose un budget
// dans une autre devise, comme en concurrence.
type racyStore struct {
	*memStore
	bomID  int64
	inject TargetBudget
	done   bool
}

func (r *racyStore) TargetBudgets(ctx context.Context, bomID int64) ([]TargetBudget, error) {
	out, err := r.memStore.TargetBudgets(ctx, bomID)
	if !r.done {
		r.done = true
		r.mu.Lock()
		r.budgets[bomID] = append(r.budgets[bomID], r.inject)
		r.mu.Unlock()
	}
	return out, err
}

// Un budget d'une devise apparu entre la lecture des taux et le verrou ne fait pas refuser l'écriture pour un
// taux « manquant » : l'écriture recommence avec les taux de cette devise (une seule tentative perdue).
func TestBudgetWriteRetriesWhenACurrencyAppearsDuringTheWrite(t *testing.T) {
	ctx := t.Context()
	day := time.Date(2026, 3, 14, 0, 0, 0, 0, time.UTC)
	rates := pricing.NewStub(nil, []pricing.Rate{
		{ID: 1, From: "GBP", To: "EUR", Rate: dec("1.2"), Source: "sim", QuotedAt: day},
		{ID: 2, From: "EUR", To: "GBP", Rate: dec("0.8333333333"), Source: "sim", QuotedAt: day},
	})
	mem := newMemStore()
	mem.budgets = map[int64][]TargetBudget{}
	racy := &racyStore{memStore: mem}
	svc := NewService(racy, nil, rates)
	b, err := svc.CreateBOM(ctx, NewBOM{Name: "B", Currency: "EUR", Volume: 1})
	if err != nil {
		t.Fatal(err)
	}
	_, nodes, _ := svc.Tree(ctx, b.ID)
	root := nodes[0].ID
	add := func(name string) int64 {
		n, err := svc.AddNode(ctx, b.ID, NewNode{ParentID: root, Name: name, Quantity: dec("1")})
		if err != nil {
			t.Fatal(err)
		}
		return n.ID
	}
	engine, cabin := add("Moteur"), add("Habitacle")
	eur, _ := money.Parse("1000", "EUR")
	global, _ := money.Parse("2000", "EUR")
	racy.done = true // pas de course pour le budget global
	if _, err := svc.PutTargetBudget(ctx, b.ID, root, global); err != nil {
		t.Fatal(err)
	}
	// Le sous-budget en GBP du frère (100 GBP = 120 EUR) compte dans le budget global : il faut son taux.
	racy.done, racy.bomID, racy.inject = false, b.ID, TargetBudget{NodeID: cabin, Amount: dec("100"), Currency: "GBP"}
	if _, err := svc.PutTargetBudget(ctx, b.ID, engine, eur); err != nil {
		t.Fatalf("write refused: %v (a GBP budget appearing meanwhile must not be a missing rate)", err)
	}
	if !racy.done {
		t.Fatal("the concurrent write was never simulated")
	}
}

// budgetFX d'une BOM aux prix figés lit les taux du snapshot de prix (pas ceux du jour) ; un taux absent du
// snapshot n'est pas une erreur ici : la conversion qui en aurait besoin échouera, nommée.
func TestBudgetFXInSnapshotMode(t *testing.T) {
	ctx := t.Context()
	day := time.Date(2026, 3, 14, 0, 0, 0, 0, time.UTC)
	usd, _ := money.Parse("10", "USD")
	rates := pricing.NewStub([]pricing.Quote{{OfferID: 1, Price: usd, Source: "sim", QuotedAt: day}},
		[]pricing.Rate{{ID: 1, From: "USD", To: "EUR", Rate: dec("0.8"), Source: "sim", QuotedAt: day}})
	id, err := rates.FreezePrices(ctx, 1, []int64{1}, []money.Currency{"EUR"}, day)
	if err != nil {
		t.Fatal(err)
	}
	rates.AddRate(pricing.Rate{ID: 2, From: "USD", To: "EUR", Rate: dec("0.9"), Source: "sim", QuotedAt: day.AddDate(0, 0, 1)})
	svc := NewService(newMemStore(), nil, rates)
	budgets := []TargetBudget{{NodeID: 1, Amount: dec("100"), Currency: "USD"}}

	frozen, err := svc.budgetFX(ctx, BOM{ID: 1, Currency: "EUR", PriceSnapshotID: &id}, budgets)
	if err != nil {
		t.Fatal(err)
	}
	if frozen.Context.Mode != "snapshot" || frozen.Context.SnapshotID == nil || *frozen.Context.SnapshotID != id ||
		len(frozen.Rates) != 1 || frozen.Rates[0].Rate != "0.8" {
		t.Fatalf("snapshot mode: %+v, want the frozen 0.8 USD→EUR (not today's 0.9), inverse absent without error", frozen)
	}
	live, err := svc.budgetFX(ctx, BOM{ID: 1, Currency: "EUR"}, budgets)
	if err != nil || live.Context.Mode != "live" || len(live.Rates) == 0 || live.Rates[0].Rate != "0.9" {
		t.Fatalf("live mode: %+v (%v), want today's 0.9", live, err)
	}
}
