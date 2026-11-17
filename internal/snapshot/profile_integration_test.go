package snapshot_test

import (
	"testing"

	"bomstudio/internal/bom"
	"bomstudio/internal/bom/bomstore"
)

// ADR 0036 : la publication fige le profil du brouillon ; le changer ensuite ne touche pas le snapshot ; un
// brouillon créé depuis le snapshot reprend le profil figé. Sans profil, rien n'est figé.
func TestSnapshotFreezesVehicleProfile(t *testing.T) {
	f := newFixture(t)
	ctx := t.Context()
	f.boms.WithProfiles(bomstore.New(f.tx))
	b, _ := f.newDraft(t)

	none := f.publish(t, b.ID)
	if got := f.get(t, none.ID).Snapshot.Profile; got != nil {
		t.Fatalf("snapshot of a bom without profile has %+v", got)
	}

	family, err := f.boms.SetVehicleProfile(ctx, b.ID, "family", nil)
	if err != nil {
		t.Fatal(err)
	}
	s := f.publish(t, b.ID)
	if _, err := f.boms.SetVehicleProfile(ctx, b.ID, "custom", &bom.Weights{Mass: 100}); err != nil {
		t.Fatal(err)
	}
	if got := f.get(t, s.ID).Snapshot.Profile; got == nil || *got != family {
		t.Fatalf("frozen profile %+v, want %+v (draft changed after publication)", got, family)
	}
	draft, err := f.snaps.NewDraft(ctx, s.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := f.boms.VehicleProfile(ctx, draft.ID); err != nil || got == nil || *got != family {
		t.Fatalf("new draft profile %+v, %v; want %+v", got, err, family)
	}
	if _, err := f.tx.Exec(ctx, `UPDATE bom_snapshot_vehicle_profiles SET weight_cost = 0 WHERE snapshot_id = $1`, s.ID); err == nil {
		t.Fatal("frozen profile was updated: the database must reject it")
	}
}
