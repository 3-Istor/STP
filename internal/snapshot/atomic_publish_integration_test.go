package snapshot_test

import (
	"context"
	"fmt"
	"slices"
	"testing"

	"bomstudio/internal/bom"
	"bomstudio/internal/bom/bomstore"
	"bomstudio/internal/snapshot"
	"bomstudio/internal/snapshot/snapshotstore"
)

// racingStore simule une requête concurrente (PATCH d'un autre utilisateur) qui modifie le brouillon juste
// après la première lecture de son arbre par la publication.
type racingStore struct {
	bom.Store
	fired bool
	race  func()
}

func (r *racingStore) fire() {
	if !r.fired {
		r.fired = true
		r.race()
	}
}

func (r *racingStore) Tree(ctx context.Context, bomID int64) ([]bom.Node, error) {
	out, err := r.Store.Tree(ctx, bomID)
	r.fire()
	return out, err
}

func (r *racingStore) Draft(ctx context.Context, bomID int64) (bom.Draft, error) {
	out, err := r.Store.Draft(ctx, bomID)
	r.fire()
	return out, err
}

// SI le brouillon change entre deux lectures de la publication (quantité modifiée, pièce ajoutée)
// ALORS le snapshot DOIT être celui d'une seule lecture : nœuds figés, contrôles figés et coût viennent du même
// état du brouillon (v2-lot-g T1). Avant : contrôles sur l'ancien arbre, nœuds et coût sur le nouveau.
func TestPublishReadsTheDraftOnce(t *testing.T) {
	f := newFixture(t)
	ctx := t.Context()
	b, roue := f.newDraft(t)
	before := draftShape(t, f, b.ID)

	race := &racingStore{Store: bomstore.New(f.tx), race: func() {
		if _, err := f.boms.SetNodeQuantity(ctx, b.ID, roue, dec("10")); err != nil {
			t.Fatal(err)
		}
		_, nodes, err := f.boms.Tree(ctx, b.ID)
		if err != nil {
			t.Fatal(err)
		}
		f.addPart(t, b.ID, nodes[0].ID, f.ecrou, f.ecrouO, "3")
	}}
	boms := bom.NewService(race, f.cat, f.prices)
	snaps := snapshot.NewService(snapshotstore.New(f.tx), boms, f.cat, f.prices)

	s, err := snaps.Publish(ctx, b.ID, t1, anyWarning)
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	if !race.fired {
		t.Fatal("la modification concurrente n'a pas eu lieu : le test ne prouve rien")
	}
	p := f.get(t, s.ID)
	if got := frozenShape(p); !slices.Equal(got, before) {
		t.Fatalf("nœuds figés = %v,\nwant l'état lu au début de la publication %v", got, before)
	}
	// Coût figé = coût de l'état lu (4 × 100 USD × 0,92 + 20 × 1 €), pas celui du brouillon modifié.
	if got, want := rootSummary(t, p), "388.00 EUR, CO2 42 kg, 32.2 kg, 35 j"; got != want {
		t.Fatalf("snapshot : %s, want %s", got, want)
	}
	// Les contrôles figés ont vu le même arbre que les nœuds figés.
	want := fmt.Sprintf("%d nœuds", len(before))
	for _, c := range p.Checks {
		if c.Code == bom.CheckOffersActive && c.Detail != want {
			t.Fatalf("contrôle %s : %q, want %q (même lecture que les nœuds figés)", c.Code, c.Detail, want)
		}
	}
	if after := draftShape(t, f, b.ID); slices.Equal(after, before) {
		t.Fatal("le brouillon n'a pas été modifié : le test ne prouve rien")
	}
}
