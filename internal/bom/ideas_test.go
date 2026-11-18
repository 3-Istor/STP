package bom

import (
	"context"
	"errors"
	"testing"

	"bomstudio/internal/money"
)

func TestSummarizeIdeas(t *testing.T) {
	eur := func(s string) money.Money { return *moneyPtr(t, s, "EUR") }
	ideas := []Idea{
		{ID: 1, Gain: eur("91"), Status: IdeaInProgress}, {ID: 2, Gain: eur("286"), Status: IdeaToValidate},
		{ID: 3, Gain: eur("102"), Status: IdeaInProgress}, {ID: 4, Gain: eur("68"), Status: IdeaNew},
		{ID: 5, Gain: eur("93"), Status: IdeaNew}, {ID: 6, Gain: eur("500"), Status: IdeaDone},
		{ID: 7, Gain: eur("40"), Status: IdeaDropped},
	}
	cost := eur("26326")
	budget := &TargetBudget{NodeID: 1, Amount: dec("25500"), Currency: "EUR"}
	tests := []struct {
		name                       string
		ideas                      []Idea
		cost                       *money.Money
		budget                     *TargetBudget
		open, gap, remaining, gerr string
		err                        error
	}{
		{name: "maquette I4-6 : écart 826, pistes ouvertes 640, reste 186 (réalisées et abandonnées exclues)",
			ideas: ideas, cost: &cost, budget: budget, open: "640", gap: "826", remaining: "186"},
		{name: "pistes au-delà de l'écart : reste négatif (écart couvert)",
			ideas: append(ideas, Idea{ID: 8, Gain: eur("200"), Status: IdeaNew}), cost: &cost, budget: budget,
			open: "840", gap: "826", remaining: "-14"},
		{name: "sans budget global : pas d'écart, dit pourquoi", ideas: ideas, cost: &cost, open: "640", gerr: "pas de budget global"},
		{name: "coût incomplet : pas d'écart", ideas: ideas, budget: budget, open: "640", gerr: "coût incomplet"},
		{name: "aucune piste : total 0", cost: &cost, budget: budget, open: "0", gap: "826", remaining: "826"},
		{name: "piste dans une autre devise : erreur nommée", ideas: []Idea{{ID: 9, Gain: *moneyPtr(t, "1", "USD"), Status: IdeaNew}},
			err: money.ErrCurrencyMismatch},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := SummarizeIdeas("EUR", tt.ideas, tt.cost, tt.budget)
			if !errors.Is(err, tt.err) {
				t.Fatalf("err = %v, want %v", err, tt.err)
			}
			if tt.err != nil {
				return
			}
			if !got.OpenTotal.Amount().Equal(dec(tt.open)) || got.GapError != tt.gerr {
				t.Errorf("open = %s, gap error %q; want %s, %q", got.OpenTotal, got.GapError, tt.open, tt.gerr)
			}
			if tt.gap == "" {
				if got.GapToTarget != nil || got.Remaining != nil {
					t.Errorf("gap = %v, remaining = %v, want none", got.GapToTarget, got.Remaining)
				}
				return
			}
			if got.GapToTarget == nil || !got.GapToTarget.Amount().Equal(dec(tt.gap)) || !got.Remaining.Amount().Equal(dec(tt.remaining)) {
				t.Errorf("gap = %v, remaining = %v, want %s, %s", got.GapToTarget, got.Remaining, tt.gap, tt.remaining)
			}
		})
	}
}

// Sans vérification de famille câblée, lier un brouillon à une piste est refusé (503), jamais accepté en silence ;
// une piste sans brouillon lié reste possible (v2-lot-g T6).
func TestIdeaDraftLinkNeedsTheFamilyCheck(t *testing.T) {
	svc, bomID, _ := costService(t, t0, true)
	svc.WithIdeas(memIdeas{})
	other, err := svc.CreateBOM(t.Context(), NewBOM{Name: "B", Currency: "EUR", Volume: 1})
	if err != nil {
		t.Fatal(err)
	}
	eur := *moneyPtr(t, "5", "EUR")
	if _, err := svc.CreateIdea(t.Context(), bomID, IdeaInput{Title: "x", Gain: eur, DraftBOMID: &other.ID}); !errors.Is(err, ErrIdeasUnavailable) {
		t.Fatalf("err = %v, want ErrIdeasUnavailable", err)
	}
	if _, err := svc.CreateIdea(t.Context(), bomID, IdeaInput{Title: "x", Gain: eur}); err != nil {
		t.Fatalf("idea without draft: %v", err)
	}
}

// memIdeas : stockage minimal des pistes (création seulement).
type memIdeas struct{ IdeaStore }

func (memIdeas) CreateIdea(_ context.Context, bomID int64, in IdeaInput) (Idea, error) {
	return Idea{ID: 1, BOMID: bomID, Title: in.Title, Gain: in.Gain, Status: in.Status, DraftBOMID: in.DraftBOMID}, nil
}
