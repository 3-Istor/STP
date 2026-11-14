package bom

import (
	"errors"
	"testing"

	"github.com/shopspring/decimal"

	"bomstudio/internal/catalog"
)

// stockPart : pièce partID (offre offerID, livraison leadDays, production prodDays).
func stockPart(nodeID int64, parent *int64, qty string, partID, offerID int64, leadDays, prodDays int) CalcNode {
	n := part(nodeID, parent, qty, offerID, leadDays, "0")
	n.Part.PartID = partID
	n.ProductionDays = prodDays
	return n
}

// TestComputeTotalsStock : couverture par le stock de la BOM (ADR 0022). Quantité comparée = quantité
// réelle × volume ; répartition entre nœuds de la même pièce par id croissant ; coût inchangé.
func TestComputeTotalsStock(t *testing.T) {
	type want struct {
		node     int64
		covered  string // "" : pas de stock pour cette pièce (StockCovered nil)
		inStock  bool
		ownLead  int
		leadDays int
	}
	tests := []struct {
		name   string
		nodes  []CalcNode
		volume int32
		stock  map[int64]decimal.Decimal
		want   []want
	}{
		{
			name:   "QUAND une pièce est en stock en quantité suffisante ALORS son délai est nul et elle est « en stock »",
			nodes:  []CalcNode{sub(1, nil, "1", 1), stockPart(2, id(1), "4", 100, 10, 30, 5)},
			volume: 10, stock: map[int64]decimal.Decimal{100: dec("40")},
			want: []want{{node: 2, covered: "40", inStock: true, ownLead: 0, leadDays: 0}, {node: 1, leadDays: 1}},
		},
		{
			name:   "SI le stock ne couvre qu'une partie ALORS le délai reste celui de l'offre et la quantité couverte est donnée",
			nodes:  []CalcNode{sub(1, nil, "1", 0), stockPart(2, id(1), "4", 100, 10, 30, 5)},
			volume: 10, stock: map[int64]decimal.Decimal{100: dec("39.5")},
			want: []want{{node: 2, covered: "39.5", ownLead: 35, leadDays: 35}, {node: 1, leadDays: 35}},
		},
		{
			name: "un stock partagé par deux nœuds de la même pièce sert d'abord le plus petit id",
			nodes: []CalcNode{
				sub(1, nil, "1", 0), stockPart(3, id(1), "2", 100, 10, 30, 0), stockPart(2, id(1), "3", 100, 10, 30, 0),
			},
			volume: 1, stock: map[int64]decimal.Decimal{100: dec("4")},
			want: []want{
				{node: 2, covered: "3", inStock: true, ownLead: 0}, {node: 3, covered: "1", ownLead: 30, leadDays: 30},
			},
		},
		{
			name:   "la quantité réelle multiplie les quantités le long du chemin avant de comparer au stock",
			nodes:  []CalcNode{sub(1, nil, "1", 0), sub(2, id(1), "4", 0), stockPart(3, id(2), "5", 100, 10, 8, 0)},
			volume: 2, stock: map[int64]decimal.Decimal{100: dec("39")},
			want: []want{{node: 3, covered: "39", ownLead: 8, leadDays: 8}},
		},
		{
			name:   "sans stock déclaré pour la pièce, rien ne change",
			nodes:  []CalcNode{sub(1, nil, "1", 0), stockPart(2, id(1), "1", 100, 10, 7, 1)},
			volume: 10, stock: map[int64]decimal.Decimal{999: dec("100")},
			want: []want{{node: 2, ownLead: 8, leadDays: 8}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prices := map[int64]PriceResult{10: price(t, 10, "2", "EUR")}
			got, err := ComputeTotals(TotalsInput{Nodes: tt.nodes, Currency: "EUR", Prices: prices, Volume: tt.volume, Stock: tt.stock})
			if err != nil {
				t.Fatal(err)
			}
			for _, w := range tt.want {
				nt, _ := got.Node(w.node)
				switch {
				case w.covered == "" && nt.StockCovered != nil:
					t.Errorf("node %d: covered %s, want none", w.node, nt.StockCovered)
				case w.covered != "" && (nt.StockCovered == nil || !nt.StockCovered.Equal(dec(w.covered))):
					t.Errorf("node %d: covered %v, want %s", w.node, nt.StockCovered, w.covered)
				}
				if nt.InStock != w.inStock {
					t.Errorf("node %d: in stock %v, want %v", w.node, nt.InStock, w.inStock)
				}
				if w.covered != "" && nt.OwnLeadTimeDays != w.ownLead {
					t.Errorf("node %d: own lead %d, want %d", w.node, nt.OwnLeadTimeDays, w.ownLead)
				}
				if nt.LeadTimeDays != w.leadDays && (w.leadDays != 0 || w.inStock) {
					t.Errorf("node %d: lead %d, want %d", w.node, nt.LeadTimeDays, w.leadDays)
				}
			}
			// Le coût ne dépend pas du stock (ADR 0022) : 2 € × quantité réelle, couverte ou non.
			for _, n := range tt.nodes {
				if n.Part == nil {
					continue
				}
				nt, _ := got.Node(n.ID)
				want := dec("2").Mul(nt.RealQuantity)
				if nt.OwnCost == nil || !nt.OwnCost.Amount().Equal(want) {
					t.Errorf("node %d: own cost %v, want %s EUR", n.ID, nt.OwnCost, want)
				}
			}
		})
	}
}

func TestSetPartStock(t *testing.T) {
	archived := quotedAt
	tests := []struct {
		name      string
		partID    int64
		qty       string
		wantErr   error
		wantStock int // lignes de stock après l'appel
	}{
		{"une quantité positive est enregistrée", 1, "12.5", nil, 1},
		{"zéro retire le stock", 1, "0", nil, 0},
		{"une quantité négative est refusée", 1, "-1", ErrInvalidInput, 0},
		{"une quantité démesurée est refusée (400, pas une erreur de la base)", 1, "1e200", ErrInvalidInput, 0},
		{"une quantité trop précise est refusée", 1, "1e-20", ErrInvalidInput, 0},
		{"une pièce archivée est refusée", 2, "3", ErrInvalidInput, 0},
		{"une pièce absente du catalogue est refusée", 404, "3", catalog.ErrPartNotFound, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cat, err := catalog.NewStub([]catalog.Part{{ID: 1, Name: "Roue"}, {ID: 2, Name: "Ancienne", ArchivedAt: &archived}}, nil)
			if err != nil {
				t.Fatal(err)
			}
			svc := NewService(newMemStore(), cat, nil)
			b, err := svc.CreateBOM(t.Context(), NewBOM{Name: "B", Currency: "EUR", Volume: 1})
			if err != nil {
				t.Fatal(err)
			}
			if tt.qty == "0" { // un stock existant à retirer
				if _, err := svc.SetPartStock(t.Context(), b.ID, 1, dec("5")); err != nil {
					t.Fatal(err)
				}
			}
			_, err = svc.SetPartStock(t.Context(), b.ID, tt.partID, dec(tt.qty))
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err %v, want %v", err, tt.wantErr)
			}
			st, err := svc.Stock(t.Context(), b.ID)
			if err != nil {
				t.Fatal(err)
			}
			if len(st) != tt.wantStock {
				t.Errorf("stock %v, want %d row(s)", st, tt.wantStock)
			}
		})
	}
	if _, err := NewService(newMemStore(), nil, nil).Stock(t.Context(), 99); !errors.Is(err, ErrBOMNotFound) {
		t.Errorf("stock of unknown bom: err %v, want ErrBOMNotFound", err)
	}
}
