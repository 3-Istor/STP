package sampledata

import (
	"reflect"
	"testing"

	"github.com/shopspring/decimal"
)

// Le plan des versions de démo vient du jeu généré : même graine, même plan ; le nouveau fournisseur des portes
// avant nues vend bien la même pièce, et ce n'est pas l'offre déjà utilisée.
func TestDemoPlan(t *testing.T) {
	ds, err := Generate("42")
	if err != nil {
		t.Fatal(err)
	}
	p, err := Demo(ds)
	if err != nil {
		t.Fatal(err)
	}
	again, err := Demo(ds)
	if err != nil || !reflect.DeepEqual(p, again) {
		t.Fatalf("plan is not deterministic: %v", err)
	}
	used := map[string]bool{}
	for _, b := range ds.BOMs {
		for _, n := range b.Nodes {
			if b.Key == p.BOMKey && n.OfferKey != "" {
				used[n.OfferKey] = true
			}
		}
	}
	offers := map[string]Offer{}
	for _, o := range ds.Offers {
		offers[o.Key] = o
	}
	total := decimal.Zero
	for _, i := range p.Ideas {
		total = total.Add(decimal.RequireFromString(i.Gain))
	}
	tests := []struct {
		name string
		ok   bool
	}{
		{"deux portes avant changent d'offre", len(p.OfferChanges) == 2},
		{"la nouvelle offre vend la porte avant nue et n'était pas utilisée",
			offers[p.OfferChanges[0].OfferKey].PartKey == "mpl.porte-av" && !used[p.OfferChanges[0].OfferKey]},
		{"un haut-parleur de plus dans chacune des 4 portes", len(p.Additions) == 4 && p.Additions[0].Want == 2},
		{"5 pistes, 640 € comme la maquette I4-6, une seule liée au brouillon", len(p.Ideas) == 5 &&
			total.Equal(decimal.NewFromInt(640)) && p.Ideas[0].LinkDraft && !p.Ideas[1].LinkDraft},
		{"trace de V1", p.Note == "Jalon de lancement" && p.ProposedBy == "Arthur P." && p.ValidatedBy == "Joe"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !tt.ok {
				t.Errorf("plan = %+v", p)
			}
		})
	}
}
