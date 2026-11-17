package bom

import (
	"errors"
	"testing"

	"bomstudio/internal/money"
)

// TestCheckBudgetTree : Σ des budgets des sous-ensembles budgétés les plus proches ≤ budget de l'ensemble (ADR 0040).
func TestCheckBudgetTree(t *testing.T) {
	id := func(v int64) *int64 { return &v }
	// 1 racine ─ 2 Moteur ─ 4 Bloc ─ 6 Culasse
	//          │          └ 5 Sans budget ─ 7 Pompe
	//          └ 3 Habitacle
	nodes := []Node{
		{ID: 1, Name: "Voiture"}, {ID: 2, ParentID: id(1), Name: "Moteur"}, {ID: 3, ParentID: id(1), Name: "Habitacle"},
		{ID: 4, ParentID: id(2), Name: "Bloc"}, {ID: 5, ParentID: id(2), Name: "Sans budget"},
		{ID: 6, ParentID: id(4), Name: "Culasse"}, {ID: 7, ParentID: id(5), Name: "Pompe"},
	}
	eur := func(node int64, amount string) TargetBudget {
		return TargetBudget{NodeID: node, Amount: dec(amount), Currency: "EUR"}
	}
	tests := []struct {
		name    string
		budgets []TargetBudget
		wantErr error
	}{
		{"aucun budget", nil, nil},
		{"ensemble à 3000, sous-ensembles 2000 + 1000 : pile le budget, accepté",
			[]TargetBudget{eur(2, "3000"), eur(4, "2000"), eur(7, "1000")}, nil},
		{"QUAND les sous-ensembles font 2000 + 1000.01 sous un ensemble à 3000 ALORS refus",
			[]TargetBudget{eur(2, "3000"), eur(4, "2000"), eur(7, "1000.01")}, ErrBudgetExceedsParent},
		{"un nœud sans budget est transparent : la pompe compte pour le moteur",
			[]TargetBudget{eur(2, "500"), eur(7, "501")}, ErrBudgetExceedsParent},
		{"seuls les plus proches comptent : la culasse est dans le bloc, pas comptée deux fois",
			[]TargetBudget{eur(2, "3000"), eur(4, "3000"), eur(6, "3000")}, nil},
		{"niveau plus bas : culasse au-dessus du bloc",
			[]TargetBudget{eur(4, "100"), eur(6, "150")}, ErrBudgetExceedsParent},
		{"racine : Σ des sous-systèmes au-dessus du budget global",
			[]TargetBudget{eur(1, "4000"), eur(2, "3000"), eur(3, "1500")}, ErrBudgetExceedsParent},
		{"sous-budgets sans budget d'ensemble : rien à plafonner",
			[]TargetBudget{eur(4, "9000"), eur(7, "9000")}, nil},
		{"budget dans une autre devise : refus, jamais additionné",
			[]TargetBudget{eur(2, "3000"), {NodeID: 4, Amount: dec("10"), Currency: "USD"}}, money.ErrCurrencyMismatch},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := CheckBudgetTree(nodes, tt.budgets, "EUR")
			if (tt.wantErr == nil) != (err == nil) || (tt.wantErr != nil && !errors.Is(err, tt.wantErr)) {
				t.Fatalf("err %v, want %v", err, tt.wantErr)
			}
		})
	}
}

// TestServiceTargetBudgets : pose, refus sans écriture, retrait qui remonterait les sous-budgets, déplacement.
func TestServiceTargetBudgets(t *testing.T) {
	ctx := t.Context()
	svc := NewService(newMemStore(), nil, nil)
	b, err := svc.CreateBOM(ctx, NewBOM{Name: "B", Currency: "EUR", Volume: 1})
	if err != nil {
		t.Fatal(err)
	}
	_, nodes, _ := svc.Tree(ctx, b.ID)
	root := nodes[0].ID
	add := func(parent int64, name string) int64 {
		n, err := svc.AddNode(ctx, b.ID, NewNode{ParentID: parent, Name: name, Quantity: dec("1")})
		if err != nil {
			t.Fatal(err)
		}
		return n.ID
	}
	engine := add(root, "Moteur")
	block := add(engine, "Bloc")
	pump := add(engine, "Pompe")
	cabin := add(root, "Habitacle")
	eur := func(s string) money.Money {
		m, err := money.Parse(s, "EUR")
		if err != nil {
			t.Fatal(err)
		}
		return m
	}
	budgetOf := func(node int64) string {
		list, err := svc.TargetBudgetList(ctx, b.ID)
		if err != nil {
			t.Fatal(err)
		}
		for _, x := range list {
			if x.NodeID == node {
				return x.Amount.String()
			}
		}
		return ""
	}
	steps := []struct {
		name    string
		do      func() error
		wantErr error
		check   func()
	}{
		{"budget de l'ensemble Moteur", func() error { _, err := svc.PutTargetBudget(ctx, b.ID, engine, eur("3000")); return err }, nil, nil},
		{"bloc à 2000", func() error { _, err := svc.PutTargetBudget(ctx, b.ID, block, eur("2000")); return err }, nil, nil},
		{"pompe à 1000.01 : refus, rien d'écrit", func() error { _, err := svc.PutTargetBudget(ctx, b.ID, pump, eur("1000.01")); return err },
			ErrBudgetExceedsParent, func() {
				if got := budgetOf(pump); got != "" {
					t.Fatalf("refused budget written: %s", got)
				}
			}},
		{"pompe à 1000 : pile 3000, accepté", func() error { _, err := svc.PutTargetBudget(ctx, b.ID, pump, eur("1000")); return err }, nil, nil},
		{"baisser le moteur sous ses sous-budgets : refus, 3000 gardé", func() error { _, err := svc.PutTargetBudget(ctx, b.ID, engine, eur("2999")); return err },
			ErrBudgetExceedsParent, func() {
				if got := budgetOf(engine); got != "3000" {
					t.Fatalf("engine budget %s, want 3000 kept", got)
				}
			}},
		{"budget global 3500", func() error { _, err := svc.PutTargetBudget(ctx, b.ID, root, eur("3500")); return err }, nil, nil},
		{"habitacle à 600 : 3000 + 600 > 3500, refus", func() error { _, err := svc.PutTargetBudget(ctx, b.ID, cabin, eur("600")); return err }, ErrBudgetExceedsParent, nil},
		{"habitacle à 500", func() error { _, err := svc.PutTargetBudget(ctx, b.ID, cabin, eur("500")); return err }, nil, nil},
		{"déplacer l'habitacle sous le moteur : 2000 + 1000 + 500 > 3000, refus, il reste sous la racine",
			func() error { _, err := svc.MoveNode(ctx, b.ID, cabin, engine); return err }, ErrBudgetExceedsParent, func() {
				_, nodes, _ := svc.Tree(ctx, b.ID)
				for _, n := range nodes {
					if n.ID == cabin && *n.ParentID != root {
						t.Fatalf("cabin moved under %d", *n.ParentID)
					}
				}
			}},
		{"retirer le budget du moteur : 2000 + 1000 + 500 = 3500 sous la racine, accepté",
			func() error { return svc.DeleteTargetBudget(ctx, b.ID, engine) }, nil, func() {
				if got := budgetOf(engine); got != "" {
					t.Fatalf("engine budget still %s", got)
				}
			}},
		{"montant négatif : entrée invalide", func() error { _, err := svc.PutTargetBudget(ctx, b.ID, cabin, eur("-1")); return err }, ErrInvalidInput, nil},
		{"autre devise que la BOM : entrée invalide", func() error {
			usd, _ := money.Parse("10", "USD")
			_, err := svc.PutTargetBudget(ctx, b.ID, cabin, usd)
			return err
		}, ErrInvalidInput, nil},
		{"nœud inconnu", func() error { _, err := svc.PutTargetBudget(ctx, b.ID, 9999, eur("1")); return err }, ErrNodeNotFound, nil},
	}
	for _, st := range steps { // étapes dans l'ordre : chacune part de l'état laissé par la précédente
		err := st.do()
		if (st.wantErr == nil) != (err == nil) || (st.wantErr != nil && !errors.Is(err, st.wantErr)) {
			t.Fatalf("%s: err %v, want %v", st.name, err, st.wantErr)
		}
		if st.check != nil {
			st.check()
		}
	}
}
