package bom

import (
	"testing"

	"github.com/shopspring/decimal"

	"bomstudio/internal/money"
)

func TestBuildPlanActions(t *testing.T) {
	d := decimal.RequireFromString
	sv := func(node, from, to int64, gain string) PlanSaving {
		return PlanSaving{NodeID: node, FromOfferID: from, ToOfferID: to, Gain: d(gain), CO2Delta: d("-1")}
	}
	tests := []struct {
		name          string
		overrun       string
		savings       []PlanSaving
		wantOrder     []int64 // ToOfferID, dans l'ordre
		wantNodes     []int   // nombre de nœuds par action
		wantCovers    int     // index de l'action qui couvre, -1 : aucune
		wantRemaining string
	}{
		{"tri par gain décroissant", "10", []PlanSaving{sv(1, 1, 10, "4"), sv(2, 2, 20, "9"), sv(3, 3, 30, "6")},
			[]int64{20, 30, 10}, []int{1, 1, 1}, 1, "0"},
		{"même substitution regroupée", "10", []PlanSaving{sv(1, 1, 10, "4"), sv(2, 1, 10, "4"), sv(3, 3, 30, "5")},
			[]int64{10, 30}, []int{2, 1}, 1, "0"},
		{"rien ne couvre : reste à trouver", "20", []PlanSaving{sv(1, 1, 10, "4"), sv(2, 2, 20, "5")},
			[]int64{20, 10}, []int{1, 1}, -1, "11"},
		{"au plus 3 actions après celle qui couvre", "1",
			[]PlanSaving{sv(1, 1, 10, "9"), sv(2, 2, 20, "8"), sv(3, 3, 30, "7"), sv(4, 4, 40, "6"), sv(5, 5, 50, "5"), sv(6, 6, 60, "4")},
			[]int64{10, 20, 30, 40}, []int{1, 1, 1, 1}, 0, "0"},
		{"égalité de gain : plus petite offre cible", "5", []PlanSaving{sv(1, 1, 30, "5"), sv(2, 2, 20, "5")},
			[]int64{20, 30}, []int{1, 1}, 0, "0"},
		{"aucune économie", "5", nil, nil, nil, -1, "5"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, rem := BuildPlanActions(d(tt.overrun), tt.savings)
			if len(got) != len(tt.wantOrder) {
				t.Fatalf("%d actions, want %d: %+v", len(got), len(tt.wantOrder), got)
			}
			covers := -1
			for i, a := range got {
				if a.ToOfferID != tt.wantOrder[i] || len(a.Nodes) != tt.wantNodes[i] {
					t.Errorf("action %d: offer %d with %d nodes, want %d with %d", i, a.ToOfferID, len(a.Nodes), tt.wantOrder[i], tt.wantNodes[i])
				}
				if a.Covers {
					covers = i
				}
			}
			if covers != tt.wantCovers {
				t.Errorf("covers at %d, want %d", covers, tt.wantCovers)
			}
			if !rem.Equal(d(tt.wantRemaining)) {
				t.Errorf("remaining %s, want %s", rem, tt.wantRemaining)
			}
		})
	}
	t.Run("plafond d'actions quand rien ne couvre", func(t *testing.T) {
		var all []PlanSaving
		for i := int64(1); i <= 30; i++ {
			all = append(all, sv(i, i, 100+i, "1"))
		}
		got, rem := BuildPlanActions(d("100"), all)
		if len(got) != maxPlanActions || !rem.Equal(d("70")) { // le reste compte toutes les actions, pas seulement celles rendues
			t.Errorf("%d actions, remaining %s", len(got), rem)
		}
	})
	t.Run("gain et deltas cumulés sur les nœuds regroupés", func(t *testing.T) {
		got, _ := BuildPlanActions(d("1"), []PlanSaving{sv(1, 1, 10, "4"), sv(2, 1, 10, "3")})
		if !got[0].Gain.Equal(d("7")) || !got[0].CO2Delta.Equal(d("-2")) || !got[0].Cumulative.Equal(d("7")) {
			t.Errorf("gain %s co2 %s cumulative %s", got[0].Gain, got[0].CO2Delta, got[0].Cumulative)
		}
	})
}

func TestBestSaving(t *testing.T) {
	c := func(offer int64, cost string, current bool, excl error) Candidate {
		out := Candidate{PartID: offer, OfferID: offer, Current: current, Excluded: excl}
		if cost != "" {
			m, _ := money.Parse(cost, "EUR")
			out.Cost = &m
		}
		return out
	}
	tests := []struct {
		name   string
		cands  []Candidate
		wantTo int64 // 0 : aucune économie
	}{
		{"la moins chère gagne", []Candidate{c(1, "10", true, nil), c(2, "8", false, nil), c(3, "7", false, nil)}, 3},
		{"égalité : plus petite offre", []Candidate{c(1, "10", true, nil), c(2, "7", false, nil), c(3, "7", false, nil)}, 2},
		{"pas moins chère que l'actuelle", []Candidate{c(1, "10", true, nil), c(2, "10", false, nil), c(3, "12", false, nil)}, 0},
		{"exclue ignorée", []Candidate{c(1, "10", true, nil), c(2, "", false, ErrArchived)}, 0},
		{"offre actuelle sans coût : pas de plan", []Candidate{c(1, "", true, nil), c(2, "5", false, nil)}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, to, ok := bestSaving(tt.cands)
			if ok != (tt.wantTo != 0) || (ok && to.OfferID != tt.wantTo) {
				t.Errorf("got offer %d ok=%v, want %d", to.OfferID, ok, tt.wantTo)
			}
		})
	}
}
