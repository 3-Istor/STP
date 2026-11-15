package viz3dstore_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"

	"bomstudio/internal/catalog"
	"bomstudio/internal/viz3d"
	"bomstudio/internal/viz3d/viz3dstore"
)

// bomTablesTestLock : clé du verrou consultatif partagé par les tests d'intégration qui écrivent dans
// les tables de BOM (même valeur dans internal/bom/bomstore et internal/snapshot).
const bomTablesTestLock = 69

// testDB ouvre une transaction sur la base de DATABASE_URL, annulée en fin de test : rien n'est laissé
// en base. Sans base joignable, le test est sauté en local mais échoue en CI (CI=true).
func testDB(t *testing.T) pgx.Tx {
	t.Helper()
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("DATABASE_URL absente : test d'intégration sauté")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	conn, err := pgx.Connect(ctx, url)
	if err == nil {
		err = conn.Ping(ctx)
	}
	if err != nil {
		if os.Getenv("CI") != "" {
			t.Fatalf("connect DATABASE_URL: %v", err)
		}
		t.Skipf("base injoignable (lancer `make migrate`) : %v", err)
	}
	t.Cleanup(func() { _ = conn.Close(context.Background()) })
	tx, err := conn.Begin(t.Context())
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	t.Cleanup(func() { _ = tx.Rollback(context.Background()) })
	if _, err := tx.Exec(t.Context(), `SELECT pg_advisory_xact_lock($1)`, bomTablesTestLock); err != nil {
		t.Fatalf("advisory lock: %v", err)
	}
	return tx
}

// savepoint isole un cas : une erreur SQL attendue n'invalide pas la transaction du test.
func savepoint(t *testing.T, tx pgx.Tx) pgx.Tx {
	t.Helper()
	sp, err := tx.Begin(t.Context())
	if err != nil {
		t.Fatalf("savepoint: %v", err)
	}
	t.Cleanup(func() { _ = sp.Rollback(context.Background()) })
	return sp
}

// fixture : deux pièces et une BOM (sa racine suffit : le store viz3d ne lit pas l'arbre).
type fixture struct{ part1, part2, bomID int64 }

func newFixture(t *testing.T, tx pgx.Tx) fixture {
	t.Helper()
	var f fixture
	scan := func(sql string, dest any) {
		t.Helper()
		if err := tx.QueryRow(t.Context(), sql).Scan(dest); err != nil {
			t.Fatalf("fixture %q: %v", sql, err)
		}
	}
	scan(`INSERT INTO parts (name, category, zone) VALUES ('Pneu', 'Roues', 'Liaison au sol') RETURNING id`, &f.part1)
	scan(`INSERT INTO parts (name, category, zone) VALUES ('Jante', 'Roues', 'Liaison au sol') RETURNING id`, &f.part2)
	scan(`INSERT INTO boms (name, currency, volume) VALUES ('Voiture', 'EUR', 100) RETURNING id`, &f.bomID)
	return f
}

func dec(s string) decimal.Decimal { return decimal.RequireFromString(s) }

func TestModelsAndPartModels(t *testing.T) {
	tx := testDB(t)
	s := viz3dstore.New(tx)
	f := newFixture(t, tx)
	ctx := t.Context()

	prim, err := s.CreateModel(ctx, viz3d.Model{Name: "Pneu (wheel)", Source: viz3d.SourceGenerated, Format: viz3d.FormatPrimitive, Confidence: dec("0.2")})
	if err != nil {
		t.Fatal(err)
	}
	glb, err := s.CreateModel(ctx, viz3d.Model{Name: "Pneu 205/55", Source: viz3d.SourceUpload, StoragePath: "models/ab/abc.glb",
		Format: viz3d.FormatGLB, License: "CC-BY", Dimensions: &viz3d.Dimensions{X: dec("0.65"), Y: dec("0.22"), Z: dec("0.65")}, Confidence: dec("1")})
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.Model(ctx, glb.ID)
	if err != nil || got.StoragePath != "models/ab/abc.glb" || got.Dimensions == nil || !got.Dimensions.Y.Equal(dec("0.22")) || got.License != "CC-BY" {
		t.Fatalf("model = %+v, %v", got, err)
	}
	if p, err := s.Model(ctx, prim.ID); err != nil || p.Dimensions != nil || p.StoragePath != "" {
		t.Fatalf("primitive = %+v, %v", p, err)
	}
	if _, err := s.Model(ctx, -1); !errors.Is(err, viz3d.ErrModelNotFound) {
		t.Fatalf("modèle inconnu : err = %v", err)
	}

	t.Run("contraintes de la base", func(t *testing.T) {
		tests := []struct {
			name  string
			model viz3d.Model
		}{
			{"primitive avec fichier", viz3d.Model{Name: "x", Source: "generated", Format: viz3d.FormatPrimitive, StoragePath: "a.glb", Confidence: dec("0.2")}},
			{"GLB sans fichier", viz3d.Model{Name: "x", Source: "upload", Format: viz3d.FormatGLB, Confidence: dec("1")}},
			{"confiance > 1", viz3d.Model{Name: "x", Source: "upload", Format: viz3d.FormatGLB, StoragePath: "a.glb", Confidence: dec("1.5")}},
			{"format inconnu", viz3d.Model{Name: "x", Source: "upload", Format: "obj", StoragePath: "a.obj", Confidence: dec("1")}},
			{"dimension négative", viz3d.Model{Name: "x", Source: "upload", Format: viz3d.FormatGLB, StoragePath: "a.glb", Confidence: dec("1"),
				Dimensions: &viz3d.Dimensions{X: dec("-1"), Y: dec("1"), Z: dec("1")}}},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				if _, err := viz3dstore.New(savepoint(t, tx)).CreateModel(ctx, tt.model); err == nil {
					t.Fatal("insertion acceptée, attendu un refus de la base")
				}
			})
		}
	})

	t.Run("association remplacée, lue seule et par lot", func(t *testing.T) {
		if _, err := s.SetPartModel(ctx, viz3d.PartModel{PartID: f.part1, ModelID: prim.ID, Match: viz3d.MatchGenerated, Confidence: dec("0.2")}); err != nil {
			t.Fatal(err)
		}
		pm, err := s.SetPartModel(ctx, viz3d.PartModel{PartID: f.part1, ModelID: glb.ID, Match: viz3d.MatchExact, Confidence: dec("1")})
		if err != nil || pm.ModelID != glb.ID || pm.Match != viz3d.MatchExact {
			t.Fatalf("remplacement = %+v, %v", pm, err)
		}
		r, err := s.PartModel(ctx, f.part1)
		if err != nil || r.Model.ID != glb.ID || r.PartModel.Match != viz3d.MatchExact || !r.PartModel.Confidence.Equal(dec("1")) {
			t.Fatalf("lecture = %+v, %v", r, err)
		}
		if _, err := s.PartModel(ctx, f.part2); !errors.Is(err, viz3d.ErrNoModel) {
			t.Fatalf("pièce sans modèle : err = %v", err)
		}
		all, err := s.PartModels(ctx, []int64{f.part1, f.part2, -5})
		if err != nil || len(all) != 1 || all[f.part1].Model.StoragePath != "models/ab/abc.glb" {
			t.Fatalf("lot = %+v, %v", all, err)
		}
		if empty, err := s.PartModels(ctx, nil); err != nil || len(empty) != 0 {
			t.Fatalf("lot vide = %+v, %v", empty, err)
		}
	})

	t.Run("clés étrangères et contrainte traduites", func(t *testing.T) {
		tests := []struct {
			name    string
			pm      viz3d.PartModel
			wantErr error // nil : une erreur quelconque de la base suffit
		}{
			{"pièce inconnue", viz3d.PartModel{PartID: -1, ModelID: glb.ID, Match: viz3d.MatchExact, Confidence: dec("1")}, catalog.ErrPartNotFound},
			{"modèle inconnu", viz3d.PartModel{PartID: f.part2, ModelID: -1, Match: viz3d.MatchExact, Confidence: dec("1")}, viz3d.ErrModelNotFound},
			{"match_type inconnu", viz3d.PartModel{PartID: f.part2, ModelID: glb.ID, Match: "perfect", Confidence: dec("1")}, nil},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				_, err := viz3dstore.New(savepoint(t, tx)).SetPartModel(ctx, tt.pm)
				if err == nil || (tt.wantErr != nil && !errors.Is(err, tt.wantErr)) {
					t.Fatalf("err = %v, attendu %v", err, tt.wantErr)
				}
			})
		}
	})
}

// Génération : prise atomique (refus tant qu'elle est en attente ou en cours), progression, bilan JSON,
// reprise listée après un arrêt.
func TestGenerationLifecycle(t *testing.T) {
	tx := testDB(t)
	s := viz3dstore.New(tx)
	f := newFixture(t, tx)
	ctx := t.Context()

	if _, err := s.Generation(ctx, f.bomID); !errors.Is(err, viz3d.ErrNoGeneration) {
		t.Fatalf("avant lancement : err = %v", err)
	}
	g, err := s.ClaimGeneration(ctx, f.bomID, 2, "fp-1")
	if err != nil || g.Status != viz3d.StatusQueued || g.Total != 2 || g.Fingerprint != "fp-1" || g.Result != nil {
		t.Fatalf("claim = %+v, %v", g, err)
	}
	tests := []struct {
		name    string
		run     func(t *testing.T) error
		wantErr error
	}{
		{"relance en attente : refusée", func(*testing.T) error { _, err := s.ClaimGeneration(ctx, f.bomID, 2, "fp-1"); return err }, viz3d.ErrGenerationRunning},
		{"progression : running 1/2", func(*testing.T) error {
			g, err := s.SetGenerationProgress(ctx, f.bomID, 1)
			if err == nil && (g.Status != viz3d.StatusRunning || g.Done != 1) {
				return errors.New("progress not recorded")
			}
			return err
		}, nil},
		{"relance en cours : refusée", func(*testing.T) error { _, err := s.ClaimGeneration(ctx, f.bomID, 2, "fp-1"); return err }, viz3d.ErrGenerationRunning},
		{"reprise : listée tant qu'elle est en cours", func(*testing.T) error {
			pending, err := s.PendingGenerations(ctx)
			if err != nil {
				return err
			}
			for _, p := range pending {
				if p.BOMID == f.bomID && p.Done == 1 {
					return nil
				}
			}
			return errors.New("generation not pending")
		}, nil},
		{"fin : done 2/2 avec bilan", func(*testing.T) error {
			res := &viz3d.GenerationResult{Finder: "generated-v1", Generated: 2, Failed: []viz3d.FailedPart{}}
			g, err := s.FinishGeneration(ctx, f.bomID, viz3d.StatusDone, 2, "", res)
			if err != nil {
				return err
			}
			if g.Status != viz3d.StatusDone || g.Done != 2 || g.Result == nil || g.Result.Generated != 2 || g.Result.Finder != "generated-v1" {
				return errors.New("finish not recorded")
			}
			return nil
		}, nil},
		{"finie : plus listée pour reprise", func(*testing.T) error {
			pending, err := s.PendingGenerations(ctx)
			if err != nil {
				return err
			}
			for _, p := range pending {
				if p.BOMID == f.bomID {
					return errors.New("finished generation still pending")
				}
			}
			return nil
		}, nil},
		{"relance après done : acceptée, état remis à zéro avec la nouvelle empreinte", func(*testing.T) error {
			g, err := s.ClaimGeneration(ctx, f.bomID, 3, "fp-2")
			if err != nil {
				return err
			}
			if g.Status != viz3d.StatusQueued || g.Done != 0 || g.Total != 3 || g.Fingerprint != "fp-2" || g.Result != nil || g.Error != "" {
				return errors.New("claim did not reset the row")
			}
			return nil
		}, nil},
		{"échec : failed avec l'erreur nommée", func(*testing.T) error {
			g, err := s.FinishGeneration(ctx, f.bomID, viz3d.StatusFailed, 1, "store down", nil)
			if err != nil {
				return err
			}
			if g.Status != viz3d.StatusFailed || g.Error != "store down" || g.Result != nil {
				return errors.New("failure not recorded")
			}
			return nil
		}, nil},
		{"relance après failed : acceptée", func(*testing.T) error { _, err := s.ClaimGeneration(ctx, f.bomID, 3, "fp-2"); return err }, nil},
		{"progression au-delà du total : refusée par la base", func(t *testing.T) error {
			_, err := viz3dstore.New(savepoint(t, tx)).SetGenerationProgress(ctx, f.bomID, 4)
			if err == nil {
				return errors.New("done > total accepted")
			}
			return nil
		}, nil},
		{"BOM sans génération : ErrNoGeneration à la progression", func(*testing.T) error { _, err := s.SetGenerationProgress(ctx, -1, 0); return err }, viz3d.ErrNoGeneration},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.run(t); !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, attendu %v", err, tt.wantErr)
			}
		})
	}
}
