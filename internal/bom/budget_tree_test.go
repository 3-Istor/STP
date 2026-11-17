package bom

import (
	"errors"
	"strings"
	"testing"
	"time"

	"bomstudio/internal/money"
	"bomstudio/internal/pricing"
)

// TestCheckBudgetTree : Σ des budgets des sous-ensembles budgétés les plus proches ≤ budget de l'ensemble (ADR 0042),
// jugé seulement sur les ensembles touchés par l'écriture, budgets convertis au taux du jour (ADR 0044).
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
	in := func(node int64, amount, currency string) TargetBudget {
		return TargetBudget{NodeID: node, Amount: dec(amount), Currency: currency}
	}
	eur := func(node int64, amount string) TargetBudget { return in(node, amount, "EUR") }
	day := time.Date(2026, 3, 14, 0, 0, 0, 0, time.UTC)
	// 1 JPY = 0,006 EUR (direct) ; 1 EUR = 1,25 USD (seulement l'inverse de USD→EUR : 1 USD = 0,8 EUR).
	fx := BudgetFX{Currency: "EUR", Context: pricing.PriceContext{Mode: "live", AsOf: day}, Rates: []pricing.FXQuote{
		{ID: 1, BaseCurrency: "JPY", QuoteCurrency: "EUR", Rate: "0.006", Source: "sim", QuotedAt: day},
		{ID: 2, BaseCurrency: "EUR", QuoteCurrency: "USD", Rate: "1.25", Source: "sim", QuotedAt: day},
	}}
	tests := []struct {
		name    string
		budgets []TargetBudget
		touched []int64
		wantErr error
		wantMsg string // extrait attendu du message de refus
	}{
		{"aucun budget", nil, []int64{4}, nil, ""},
		{"ensemble à 3000, sous-ensembles 2000 + 1000 : pile le budget, accepté",
			[]TargetBudget{eur(2, "3000"), eur(4, "2000"), eur(7, "1000")}, []int64{7}, nil, ""},
		{"QUAND les sous-ensembles font 2000 + 1000.01 sous un ensemble à 3000 ALORS refus",
			[]TargetBudget{eur(2, "3000"), eur(4, "2000"), eur(7, "1000.01")}, []int64{7}, ErrBudgetExceedsParent, "over by 0.01 EUR"},
		{"un nœud sans budget est transparent : la pompe compte pour le moteur",
			[]TargetBudget{eur(2, "500"), eur(7, "501")}, []int64{7}, ErrBudgetExceedsParent, `"Moteur"`},
		{"seuls les plus proches comptent : la culasse est dans le bloc, pas comptée deux fois",
			[]TargetBudget{eur(2, "3000"), eur(4, "3000"), eur(6, "3000")}, []int64{6}, nil, ""},
		{"baisser un ensemble sous ses sous-budgets : jugé sur l'ensemble lui-même",
			[]TargetBudget{eur(4, "100"), eur(6, "150")}, []int64{4}, ErrBudgetExceedsParent, `"Bloc"`},
		{"racine : Σ des sous-systèmes au-dessus du budget global",
			[]TargetBudget{eur(1, "4000"), eur(2, "3000"), eur(3, "1500")}, []int64{3}, ErrBudgetExceedsParent, `"Voiture"`},
		{"QUAND un sous-budget est en JPY ALORS il est converti au taux du jour : 500 000 JPY = 3000 EUR, pile",
			[]TargetBudget{eur(2, "3000"), in(4, "500000", "JPY")}, []int64{4}, nil, ""},
		{"500 001 JPY = 3000,006 EUR > 3000 EUR : refus qui dit le taux et sa date",
			[]TargetBudget{eur(2, "3000"), in(4, "500001", "JPY")}, []int64{4}, ErrBudgetExceedsParent, "1 JPY = 0.006 EUR (sim, 2026-03-14)"},
		{"taux inverse : 3750 USD = 3000 EUR sous un ensemble à 3000 EUR, accepté",
			[]TargetBudget{eur(2, "3000"), in(4, "3750", "USD")}, []int64{4}, nil, ""},
		{"ensemble en devise étrangère : 1 000 000 JPY (6000 EUR) couvre 5000 EUR de sous-budgets",
			[]TargetBudget{in(2, "1000000", "JPY"), eur(4, "5000")}, []int64{4}, nil, ""},
		{"taux absent (GBP) : refus nommé, jamais un budget ignoré",
			[]TargetBudget{eur(2, "3000"), in(4, "10", "GBP")}, []int64{4}, pricing.ErrNoRate, ""},
		{"un dépassement ailleurs dans l'arbre ne bloque pas une écriture qui ne le touche pas",
			[]TargetBudget{eur(4, "100"), eur(6, "150"), eur(3, "50")}, []int64{3}, nil, ""},
		{"un budget en devise sans taux ailleurs ne bloque pas non plus",
			[]TargetBudget{in(4, "10", "GBP"), eur(3, "50")}, []int64{3}, nil, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := CheckBudgetTree(nodes, tt.budgets, tt.touched, fx)
			if (tt.wantErr == nil) != (err == nil) || (tt.wantErr != nil && !errors.Is(err, tt.wantErr)) {
				t.Fatalf("err %v, want %v", err, tt.wantErr)
			}
			if tt.wantMsg != "" && !strings.Contains(err.Error(), tt.wantMsg) {
				t.Fatalf("message %q, want it to contain %q", err, tt.wantMsg)
			}
		})
	}
	// Une répartition qui ne peut pas être jugée (taux absent pour un sous-budget) est dite, jamais « tenue ».
	unverified := AllocationWarnings(nodes, []TargetBudget{eur(2, "3000"), in(4, "10", "GBP")}, fx)
	if !strings.Contains(unverified[2], "allocation not verifiable") {
		t.Fatalf("allocation of node 2 = %q, want it marked as not verifiable", unverified[2])
	}
	// Dépassement exact sous l'arrondi d'affichage : le refus le dit au lieu d'écrire « over by 0 ».
	err := CheckBudgetTree(nodes, []TargetBudget{eur(2, "3000"), in(4, "500000.5", "JPY")}, []int64{4}, fx)
	if !errors.Is(err, ErrBudgetExceedsParent) || !strings.Contains(err.Error(), "less than 1 minor unit of EUR") {
		t.Fatalf("err %v, want a refusal that names a sub-cent excess", err)
	}
	// Budget global converti (pistes, sensibilité au change, versions) : direct, inverse, ou raison si pas de taux.
	for _, c := range []struct {
		budget       TargetBudget
		want, reason string
	}{
		{in(1, "500000", "JPY"), "3000", ""},
		{in(1, "3750", "USD"), "3000", ""},
		{eur(1, "26500"), "26500", ""},
		{in(1, "10", "GBP"), "", "no exchange rate"},
	} {
		got, reason, err := ConvertTargetBudget(c.budget, fx)
		if err != nil || !strings.Contains(reason, c.reason) || (c.want != "" && (got.Currency != "EUR" || !got.Amount.Equal(dec(c.want)))) {
			t.Fatalf("convert %v: %v %q %v, want %s EUR (reason %q)", c.budget, got, reason, err, c.want, c.reason)
		}
	}
	// Après coup, le change peut faire dépasser une répartition déjà acceptée : signalé, pas bloqué.
	warnings := AllocationWarnings(nodes, []TargetBudget{eur(2, "3000"), in(4, "600000", "JPY"), eur(3, "10")}, fx)
	if len(warnings) != 1 || !strings.Contains(warnings[2], "over by 600.00 EUR") {
		t.Fatalf("allocation warnings %v, want only Moteur over by 600 EUR", warnings)
	}
}

// TestServiceTargetBudgets : pose, refus sans écriture, retrait qui remonterait les sous-budgets, déplacement.
func TestServiceTargetBudgets(t *testing.T) {
	ctx := t.Context()
	day := time.Date(2026, 3, 14, 0, 0, 0, 0, time.UTC)
	// Taux du jour : 1 USD = 0,8 EUR et l'inverse (pour la BOM passée en USD plus bas).
	rates := pricing.NewStub(nil, []pricing.Rate{
		{ID: 1, From: "USD", To: "EUR", Rate: dec("0.8"), Source: "sim", QuotedAt: day},
		{ID: 2, From: "EUR", To: "USD", Rate: dec("1.25"), Source: "sim", QuotedAt: day},
	})
	svc := NewService(newMemStore(), nil, rates)
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
	misc := add(root, "Divers") // sans budget
	eur := func(s string) money.Money {
		m, err := money.Parse(s, "EUR")
		if err != nil {
			t.Fatal(err)
		}
		return m
	}
	usd := func(s string) money.Money {
		m, err := money.Parse(s, "USD")
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
		{"habitacle en USD : 625 USD = 500 EUR au taux du jour, 2000 + 1000 + 500 = 3500, accepté", func() error {
			_, err := svc.PutTargetBudget(ctx, b.ID, cabin, usd("625"))
			return err
		}, nil, nil},
		{"626 USD = 500,80 EUR > 500 restant sous la racine : refus", func() error {
			_, err := svc.PutTargetBudget(ctx, b.ID, cabin, usd("626"))
			return err
		}, ErrBudgetExceedsParent, func() {
			if got := budgetOf(cabin); got != "625" {
				t.Fatalf("cabin budget %s, want 625 kept", got)
			}
		}},
		{"devise sans taux (JPY) : refus nommé", func() error {
			jpy, _ := money.Parse("1", "JPY")
			_, err := svc.PutTargetBudget(ctx, b.ID, cabin, jpy)
			return err
		}, pricing.ErrNoRate, nil},
		{"la BOM passe en USD : rien ne bloque, un déplacement sans rapport passe", func() error {
			if _, err := svc.UpdateSettings(ctx, b.ID, "USD", 1); err != nil {
				return err
			}
			_, err := svc.MoveNode(ctx, b.ID, misc, engine)
			return err
		}, nil, nil},
		{"mais la pompe (1000 EUR = 1250 USD) sous l'habitacle (625 USD) : refus au taux du jour", func() error {
			_, err := svc.MoveNode(ctx, b.ID, pump, cabin)
			return err
		}, ErrBudgetExceedsParent, nil},
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
