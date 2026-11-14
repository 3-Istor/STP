package bom

import (
	"errors"
	"slices"
	"testing"

	"github.com/shopspring/decimal"
)

// laborPart : pièce avec des heures de main-d'œuvre par unité ("" : heures absentes du catalogue).
func laborPart(nodeID int64, parent *int64, qty string, offerID int64, hours string) CalcNode {
	n := part(nodeID, parent, qty, offerID, 0, "0")
	if hours != "" {
		h := dec(hours)
		n.Part.LaborHours = &h
	}
	return n
}

// TestComputeTotalsLabor : heures et ETP remontés dans l'arbre (ADR 0024).
func TestComputeTotalsLabor(t *testing.T) {
	base := dec("1607")
	type want struct {
		node    int64
		hours   string // "" : pas d'heures attendues (LaborErr)
		fte     string // "" : pas d'ETP attendu
		missing []int64
	}
	tests := []struct {
		name        string
		nodes       []CalcNode
		volume      int32
		annualHours *decimal.Decimal
		want        []want
	}{
		{
			name:   "QUAND la BOM totalise 16,07 h par voiture, 100 voitures, base 1 607 h ALORS 1 ETP",
			nodes:  []CalcNode{sub(1, nil, "1", 0), laborPart(2, id(1), "1", 10, "10"), laborPart(3, id(1), "1", 11, "6.07")},
			volume: 100, annualHours: &base,
			want: []want{{node: 1, hours: "16.07", fte: "1"}, {node: 3, hours: "6.07", fte: "0.3777224642190416925948973242"}},
		},
		{
			name: "les heures suivent la quantité réelle (quantités multipliées le long du chemin)",
			nodes: []CalcNode{
				sub(1, nil, "1", 0), sub(2, id(1), "4", 0), laborPart(3, id(2), "2.5", 10, "0.5"),
			},
			volume: 1607, annualHours: &base,
			want: []want{{node: 3, hours: "5", fte: "5"}, {node: 2, hours: "5"}, {node: 1, hours: "5", fte: "5"}},
		},
		{
			name: "SI une pièce n'a pas d'heures ALORS ses ancêtres n'ont ni heures ni ETP, la branche sœur garde les siennes",
			nodes: []CalcNode{
				sub(1, nil, "1", 0), sub(2, id(1), "1", 0), laborPart(3, id(2), "1", 10, ""),
				laborPart(4, id(1), "1", 11, "2"),
			},
			volume: 100, annualHours: &base,
			want: []want{
				{node: 3, missing: []int64{3}}, {node: 2, missing: []int64{3}}, {node: 1, missing: []int64{3}},
				{node: 4, hours: "2"},
			},
		},
		{
			name:  "un sous-ensemble seul n'a pas d'heures propres : 0 h, 0 ETP",
			nodes: []CalcNode{sub(1, nil, "1", 3)}, volume: 100, annualHours: &base,
			want: []want{{node: 1, hours: "0", fte: "0"}},
		},
		{
			name:  "SANS base horaire (snapshot publié avant ADR 0024) ALORS des heures mais pas d'ETP",
			nodes: []CalcNode{sub(1, nil, "1", 0), laborPart(2, id(1), "1", 10, "8")}, volume: 100,
			want: []want{{node: 1, hours: "8"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prices := map[int64]PriceResult{}
			for _, n := range tt.nodes {
				if n.Part != nil {
					prices[n.Part.OfferID] = price(t, n.Part.OfferID, "1", "EUR")
				}
			}
			got, err := ComputeTotals(TotalsInput{
				Nodes: tt.nodes, Currency: "EUR", Prices: prices, Volume: tt.volume, AnnualHours: tt.annualHours,
			})
			if err != nil {
				t.Fatal(err)
			}
			for _, w := range tt.want {
				nt, _ := got.Node(w.node)
				if w.hours == "" {
					if nt.LaborHours != nil || !errors.Is(nt.LaborErr, ErrMissingLabor) {
						t.Errorf("node %d: hours %v, err %v; want none and ErrMissingLabor", w.node, nt.LaborHours, nt.LaborErr)
					}
					if nt.FTE != nil {
						t.Errorf("node %d: fte %s, want none", w.node, nt.FTE)
					}
				} else if nt.LaborHours == nil || !nt.LaborHours.Equal(dec(w.hours)) {
					t.Errorf("node %d: hours %v, want %s (err %v)", w.node, nt.LaborHours, w.hours, nt.LaborErr)
				}
				switch {
				case w.fte != "" && (nt.FTE == nil || !nt.FTE.Equal(dec(w.fte))):
					t.Errorf("node %d: fte %v, want %s", w.node, nt.FTE, w.fte)
				case w.fte == "" && w.hours != "" && tt.annualHours == nil && nt.FTE != nil:
					t.Errorf("node %d: fte %s, want none without annual hours", w.node, nt.FTE)
				}
				if w.missing != nil && !slices.Equal(nt.MissingLabor, w.missing) {
					t.Errorf("node %d: missing labor %v, want %v", w.node, nt.MissingLabor, w.missing)
				}
			}
		})
	}
}

func TestSetAnnualHours(t *testing.T) {
	tests := []struct {
		name    string
		hours   string
		wantErr error
	}{
		{"base horaire positive acceptée", "1600.5", nil},
		{"base horaire nulle refusée (division par zéro)", "0", ErrInvalidInput},
		{"base horaire négative refusée", "-1", ErrInvalidInput},
		{"base d'une année bissextile acceptée", "8784", nil},
		{"base au-delà d'une année bissextile refusée", "8784.01", ErrInvalidInput},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := NewService(newMemStore(), nil, nil)
			b, err := svc.CreateBOM(t.Context(), NewBOM{Name: "B", Currency: "EUR", Volume: 1})
			if err != nil {
				t.Fatal(err)
			}
			if !b.AnnualHours.Equal(DefaultAnnualHours) {
				t.Fatalf("default annual hours %s, want %s", b.AnnualHours, DefaultAnnualHours)
			}
			got, err := svc.SetAnnualHours(t.Context(), b.ID, dec(tt.hours))
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err %v, want %v", err, tt.wantErr)
			}
			if err == nil && !got.AnnualHours.Equal(dec(tt.hours)) {
				t.Errorf("annual hours %s, want %s", got.AnnualHours, tt.hours)
			}
		})
	}
}

// Snapshot publié avant ADR 0024 : l'explication dit « heures non figées », pas « pièce sans heures ».
func TestComputeTotalsLaborNotFrozen(t *testing.T) {
	nodes := []CalcNode{sub(1, nil, "1", 0), laborPart(2, id(1), "1", 10, "")}
	got, err := ComputeTotals(TotalsInput{
		Nodes: nodes, Currency: "EUR", Prices: map[int64]PriceResult{10: price(t, 10, "1", "EUR")},
		Volume: 100, LaborNotFrozen: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, nodeID := range []int64{1, 2} {
		nt, _ := got.Node(nodeID)
		if !errors.Is(nt.LaborErr, ErrLaborNotFrozen) || errors.Is(nt.LaborErr, ErrMissingLabor) {
			t.Errorf("node %d: labor err %v, want ErrLaborNotFrozen only", nodeID, nt.LaborErr)
		}
		if nt.LaborHours != nil || nt.FTE != nil || len(nt.MissingLabor) != 0 {
			t.Errorf("node %d: hours %v fte %v missing %v, want none", nodeID, nt.LaborHours, nt.FTE, nt.MissingLabor)
		}
	}
}
