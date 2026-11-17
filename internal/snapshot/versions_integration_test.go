package snapshot_test

import (
	"errors"
	"strings"
	"testing"

	"bomstudio/internal/bom"
	"bomstudio/internal/bom/bomstore"
	"bomstudio/internal/money"
	"bomstudio/internal/pricing"
	"bomstudio/internal/snapshot"
)

// versionsFixture : brouillon A (Roue USD ×4 > Écrou ×5, budget global 450 €), publié en V1 avec sa trace,
// puis brouillon B créé depuis V1 : Roue passée à l'offre EUR (95 €), Écrou recoté 1,10 €, Volant ajouté (30 €).
func versionsFixture(t *testing.T) (fixture, bom.BOM, snapshot.Snapshot, bom.BOM) {
	t.Helper()
	ctx := t.Context()
	f := newFixture(t)
	a, _ := f.newDraft(t)
	_, nodes, err := f.boms.Tree(ctx, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.tx.Exec(ctx, `INSERT INTO bom_target_budgets (bom_id, node_id, amount, currency) VALUES ($1, $2, 450, 'EUR')`,
		a.ID, nodes[0].ID); err != nil {
		t.Fatal(err)
	}
	v1, err := f.snaps.Publish(ctx, a.ID, t1, snapshot.PublishRequest{Note: "Jalon de lancement", ProposedBy: "Arthur P.",
		ValidatedBy: "Joe", Justifications: anyWarning.Justifications})
	if err != nil {
		t.Fatal(err)
	}
	b, err := f.snaps.NewDraft(ctx, v1.ID)
	if err != nil {
		t.Fatal(err)
	}
	f.prices.AddQuote(pricing.Quote{OfferID: f.roueEUR, Price: m(t, "95", "EUR"), Source: "sim", QuotedAt: t2})
	f.prices.AddQuote(pricing.Quote{OfferID: f.ecrouO, Price: m(t, "1.10", "EUR"), Source: "sim", QuotedAt: t2})
	f.prices.AddQuote(pricing.Quote{OfferID: f.volantO, Price: m(t, "30", "EUR"), Source: "sim", QuotedAt: t2})
	_, bn, err := f.boms.Tree(ctx, b.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range bn {
		if n.PartID != nil && *n.PartID == f.roue {
			if _, err := f.boms.SetNodeOffer(ctx, b.ID, n.ID, f.roueEUR); err != nil {
				t.Fatal(err)
			}
		}
	}
	f.addPart(t, b.ID, bn[0].ID, f.volant, f.volantO, "1")
	return f, a, v1, b
}

func TestCompareSnapshotToDraft(t *testing.T) {
	f, _, v1, b := versionsFixture(t)
	c, err := f.snaps.Compare(t.Context(), snapshot.Ref{Kind: snapshot.RefSnapshot, ID: v1.ID}, snapshot.Ref{Kind: snapshot.RefBOM, ID: b.ID})
	if err != nil {
		t.Fatal(err)
	}
	// V1 : 4 × 100 USD × 0.92 + 20 × 1 = 388 ; B : 4 × 95 + 20 × 1.10 + 30 = 432.
	if c.From.Label != "V1" || c.To.Label != "Brouillon B" || !c.From.Cost.Amount().Equal(dec("388")) || !c.To.Cost.Amount().Equal(dec("432")) {
		t.Fatalf("from %s %s, to %s %s", c.From.Label, c.From.Cost, c.To.Label, c.To.Cost)
	}
	want := map[bom.CauseKind]string{bom.CauseStructure: "30", bom.CauseOfferChange: "12", bom.CausePrice: "2"}
	sum := dec("0")
	for _, cause := range c.Causes {
		sum = sum.Add(cause.Amount.Amount())
		w, ok := want[cause.Kind]
		if !ok || !cause.Amount.Amount().Equal(dec(w)) {
			t.Errorf("cause %s = %s, want %q", cause.Kind, cause.Amount, w)
		}
		if cause.Explanation == "" || cause.NodeCount == 0 {
			t.Errorf("cause %s: empty explanation or nodes: %+v", cause.Kind, cause)
		}
	}
	if len(c.Causes) != len(want) || !sum.Equal(c.Delta.Cost.Amount()) || !c.Delta.Cost.Amount().Equal(dec("44")) {
		t.Errorf("%d causes summing to %s, delta %s; want 3 causes = 44", len(c.Causes), sum, c.Delta.Cost)
	}
	for _, cause := range c.Causes {
		if cause.Kind == bom.CauseStructure && !strings.Contains(cause.Explanation, "Volant") {
			t.Errorf("structure explanation %q does not name the added part", cause.Explanation)
		}
	}
	if len(c.ChangedParts) != 2 || c.ChangedParts[0].Change != "offer" || c.ChangedParts[0].Path != "Roues > Roue" ||
		c.ChangedParts[1].Change != "added" || c.ChangedParts[1].Before != "—" {
		t.Errorf("changed parts = %+v", c.ChangedParts)
	}
}

func TestCompareErrors(t *testing.T) {
	f, a, v1, _ := versionsFixture(t)
	other, err := f.boms.CreateBOM(t.Context(), bom.NewBOM{Name: "Utilitaire", Currency: "USD", Volume: 10})
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name     string
		from, to snapshot.Ref
		want     error
	}{
		{"snapshot inconnu", snapshot.Ref{Kind: snapshot.RefSnapshot, ID: -1}, snapshot.Ref{Kind: snapshot.RefBOM, ID: a.ID}, snapshot.ErrSnapshotNotFound},
		{"BOM inconnue", snapshot.Ref{Kind: snapshot.RefSnapshot, ID: v1.ID}, snapshot.Ref{Kind: snapshot.RefBOM, ID: -1}, bom.ErrBOMNotFound},
		{"devises différentes", snapshot.Ref{Kind: snapshot.RefSnapshot, ID: v1.ID}, snapshot.Ref{Kind: snapshot.RefBOM, ID: other.ID}, money.ErrCurrencyMismatch},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := f.snaps.Compare(t.Context(), tt.from, tt.to); !errors.Is(err, tt.want) {
				t.Errorf("err = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestParseRef(t *testing.T) {
	tests := []struct {
		in   string
		want snapshot.Ref
		ok   bool
	}{
		{"snapshot:12", snapshot.Ref{Kind: "snapshot", ID: 12}, true},
		{"bom:3", snapshot.Ref{Kind: "bom", ID: 3}, true},
		{"bom:0", snapshot.Ref{}, false},
		{"brouillon:3", snapshot.Ref{}, false},
		{"12", snapshot.Ref{}, false},
		{"bom:x", snapshot.Ref{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := snapshot.ParseRef(tt.in)
			if (err == nil) != tt.ok || got != tt.want {
				t.Errorf("ParseRef(%q) = %+v, %v", tt.in, got, err)
			}
			if !tt.ok && !errors.Is(err, bom.ErrInvalidInput) {
				t.Errorf("err = %v, want ErrInvalidInput", err)
			}
		})
	}
}

func TestVersionsOfFamily(t *testing.T) {
	f, a, v1, b := versionsFixture(t)
	for _, id := range []int64{a.ID, b.ID} { // même famille vue de l'origine ou du brouillon dérivé
		vs, err := f.snaps.Versions(t.Context(), id)
		if err != nil {
			t.Fatal(err)
		}
		if len(vs) != 3 {
			t.Fatalf("%d versions, want 3: %+v", len(vs), vs)
		}
		labels := map[string]snapshot.Version{}
		for _, v := range vs {
			labels[v.Label] = v
		}
		va, vv, vb := labels["Brouillon A"], labels["V1"], labels["Brouillon B"]
		if va.ID != a.ID || vv.ID != v1.ID || vb.ID != b.ID || vb.SourceSnapshotID == nil || *vb.SourceSnapshotID != v1.ID {
			t.Fatalf("versions = %+v", vs)
		}
		if vv.ProposedBy == nil || *vv.ProposedBy != "Arthur P." || vv.Note == nil || *vv.Note != "Jalon de lancement" {
			t.Errorf("V1 trace = %v %v", vv.ProposedBy, vv.Note)
		}
		// budget global copié dans B : 432 / 450 = 96 % ; V1 : 388 / 450.
		if vb.BudgetPercent == nil || !vb.BudgetPercent.Equal(dec("96")) || vv.Cost == nil || !vv.Cost.Amount().Equal(dec("388")) {
			t.Errorf("B budget %% = %v, V1 cost = %v", vb.BudgetPercent, vv.Cost)
		}
	}
}

func TestReductionIdeas(t *testing.T) {
	f, a, _, b := versionsFixture(t)
	ctx := t.Context()
	f.boms.WithIdeas(bomstore.New(f.tx)).WithDraftFamily(f.snaps)
	eur := func(s string) money.Money { return m(t, s, "EUR") }
	// Piste de B chiffrée dans un autre brouillon de la famille (A) : jamais B lui-même (v2-lot-g T6).
	i1, err := f.boms.CreateIdea(ctx, b.ID, bom.IdeaInput{Title: "Roue → offre EUR", Owner: "Brian P.", Gain: eur("12"),
		Status: bom.IdeaInProgress, DraftBOMID: &a.ID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.boms.CreateIdea(ctx, b.ID, bom.IdeaInput{Title: "Volant : 2e source", Gain: eur("5")}); err != nil {
		t.Fatal(err)
	}
	sum, err := f.boms.Ideas(ctx, b.ID)
	if err != nil {
		t.Fatal(err)
	}
	// B : 432 € pour 450 € de budget : écart −18 €, pistes ouvertes 17 €, reste −35 €.
	if len(sum.Ideas) != 2 || !sum.OpenTotal.Amount().Equal(dec("17")) || sum.GapToTarget == nil ||
		!sum.GapToTarget.Amount().Equal(dec("-18")) || !sum.Remaining.Amount().Equal(dec("-35")) {
		t.Fatalf("summary = %+v", sum)
	}
	dropped := bom.IdeaDropped
	if _, err := f.boms.UpdateIdea(ctx, b.ID, i1.ID, bom.IdeaPatch{Status: &dropped, ClearDraft: true}); err != nil {
		t.Fatal(err)
	}
	if sum, err = f.boms.Ideas(ctx, b.ID); err != nil || !sum.OpenTotal.Amount().Equal(dec("5")) || sum.Ideas[0].DraftBOMID != nil {
		t.Fatalf("after drop: %+v, %v", sum, err)
	}
	tests := []struct {
		name string
		call func() error
		want error
	}{
		{"gain dans une autre devise", func() error {
			_, err := f.boms.CreateIdea(ctx, b.ID, bom.IdeaInput{Title: "x", Gain: m(t, "1", "USD")})
			return err
		}, bom.ErrInvalidInput},
		{"titre vide", func() error {
			_, err := f.boms.CreateIdea(ctx, b.ID, bom.IdeaInput{Title: "  ", Gain: eur("1")})
			return err
		}, bom.ErrInvalidInput},
		{"statut inconnu", func() error {
			s := bom.IdeaStatus("maybe")
			_, err := f.boms.UpdateIdea(ctx, b.ID, i1.ID, bom.IdeaPatch{Status: &s})
			return err
		}, bom.ErrInvalidInput},
		{"brouillon lié inexistant", func() error {
			bad := int64(-1)
			_, err := f.boms.UpdateIdea(ctx, b.ID, i1.ID, bom.IdeaPatch{DraftBOMID: &bad})
			return err
		}, bom.ErrInvalidInput},
		{"brouillon lié = la BOM elle-même (son gain serait compté deux fois)", func() error {
			_, err := f.boms.UpdateIdea(ctx, b.ID, i1.ID, bom.IdeaPatch{DraftBOMID: &b.ID})
			return err
		}, bom.ErrInvalidInput},
		{"brouillon lié d'une autre famille", func() error {
			other, err := f.boms.CreateBOM(ctx, bom.NewBOM{Name: "Autre voiture", Currency: "EUR", Volume: 10})
			if err != nil {
				return err
			}
			_, err = f.boms.CreateIdea(ctx, b.ID, bom.IdeaInput{Title: "x", Gain: eur("1"), DraftBOMID: &other.ID})
			return err
		}, bom.ErrInvalidInput},
		{"gain d'un milliard ou plus", func() error {
			_, err := f.boms.CreateIdea(ctx, b.ID, bom.IdeaInput{Title: "x", Gain: eur("1000000000")})
			return err
		}, bom.ErrInvalidInput},
		{"gain juste sous la borne : accepté", func() error {
			_, err := f.boms.CreateIdea(ctx, b.ID, bom.IdeaInput{Title: "x", Gain: eur("999999999.99"), DraftBOMID: &a.ID})
			return err
		}, nil},
		{"piste d'une autre BOM", func() error { return f.boms.DeleteIdea(ctx, b.ID+1000, i1.ID) }, bom.ErrIdeaNotFound},
		{"suppression puis relecture", func() error {
			if err := f.boms.DeleteIdea(ctx, b.ID, i1.ID); err != nil {
				return err
			}
			_, err := f.boms.UpdateIdea(ctx, b.ID, i1.ID, bom.IdeaPatch{})
			return err
		}, bom.ErrIdeaNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.call(); !errors.Is(err, tt.want) {
				t.Errorf("err = %v, want %v", err, tt.want)
			}
		})
	}
}

// QUAND le budget de la BOM change après la publication de V1
// ALORS le % du budget de V1 et les budgets d'un brouillon créé depuis V1 NE DOIVENT PAS bouger : les budgets
// cibles sont des réglages figés au snapshot (AGENTS §3, v2-lot-g T3).
func TestSnapshotBudgetsAreFrozen(t *testing.T) {
	f, a, v1, _ := versionsFixture(t)
	ctx := t.Context()
	_, nodes, err := f.boms.Tree(ctx, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	root := nodes[0].ID
	if err := f.boms.SetTargetBudget(ctx, a.ID, bom.TargetBudget{NodeID: root, Amount: dec("1000"), Currency: "EUR"}); err != nil {
		t.Fatal(err)
	}
	if err := f.boms.SetTargetBudget(ctx, a.ID, bom.TargetBudget{NodeID: nodes[1].ID, Amount: dec("10"), Currency: "EUR"}); err != nil {
		t.Fatal(err)
	}

	vs, err := f.snaps.Versions(ctx, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range vs {
		if v.Kind != "snapshot" || v.ID != v1.ID {
			continue
		}
		// 388 € pour le budget figé de 450 € : 86,22 %, pas 38,8 % (budget actuel de 1 000 €).
		if v.BudgetPercent == nil || !v.BudgetPercent.Round(2).Equal(dec("86.22")) {
			t.Errorf("V1 budget %% = %v, want 86.22 (budget figé)", v.BudgetPercent)
		}
		if v.TargetBudgetRoot == nil || !v.TargetBudgetRoot.Amount().Equal(dec("450")) {
			t.Errorf("V1 budget global = %v, want 450 EUR", v.TargetBudgetRoot)
		}
	}

	c, err := f.snaps.NewDraft(ctx, v1.ID)
	if err != nil {
		t.Fatal(err)
	}
	budgets, err := f.boms.TargetBudgetList(ctx, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	_, cn, err := f.boms.Tree(ctx, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(budgets) != 1 || budgets[0].NodeID != cn[0].ID || !budgets[0].Amount.Equal(dec("450")) || budgets[0].Currency != "EUR" {
		t.Fatalf("budgets du brouillon issu de V1 = %+v, want seulement le budget global figé de 450 EUR", budgets)
	}
}
