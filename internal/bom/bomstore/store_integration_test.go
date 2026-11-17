package bomstore_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"

	"bomstudio/internal/bom"
	"bomstudio/internal/bom/bomstore"
)

// testDB ouvre une transaction sur la base de DATABASE_URL, annulée en fin de test : rien n'est
// laissé en base. Sans base joignable, le test est sauté en local mais échoue en CI (CI=true).
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
	// Les paquets de test tournent en parallèle sur la même base : sans ce verrou, un TRUNCATE ... CASCADE
	// (test d'immutabilité des snapshots) et des insertions dans boms / bom_nodes se bloquent mutuellement
	// (deadlock 40P01). Le verrou consultatif, libéré au rollback, fait passer ces tests l'un après l'autre.
	if _, err := tx.Exec(t.Context(), `SELECT pg_advisory_xact_lock($1)`, bomTablesTestLock); err != nil {
		t.Fatalf("advisory lock: %v", err)
	}
	return tx
}

// bomTablesTestLock : clé du verrou consultatif partagé par les tests d'intégration qui écrivent dans
// les tables de BOM (même valeur dans internal/snapshot et internal/bom/bomstore).
const bomTablesTestLock = 69

// savepoint isole un cas de test : une erreur SQL attendue n'invalide pas la transaction du test.
func savepoint(t *testing.T, tx pgx.Tx) pgx.Tx {
	t.Helper()
	sp, err := tx.Begin(t.Context())
	if err != nil {
		t.Fatalf("savepoint: %v", err)
	}
	t.Cleanup(func() { _ = sp.Rollback(context.Background()) })
	return sp
}

// catalogFixture : deux pièces ; la première a deux offres, la seconde une.
type catalogFixture struct {
	part1, part2           int64
	offer1a, offer1b, off2 int64
}

func newCatalog(t *testing.T, tx pgx.Tx) catalogFixture {
	t.Helper()
	ctx := t.Context()
	scan := func(sql string, args []any, dest ...any) {
		t.Helper()
		if err := tx.QueryRow(ctx, sql, args...).Scan(dest...); err != nil {
			t.Fatalf("fixture %q: %v", sql, err)
		}
	}
	var supplier int64
	var f catalogFixture
	scan(`INSERT INTO suppliers (name) VALUES ('Fournisseur test') RETURNING id`, nil, &supplier)
	scan(`INSERT INTO parts (name) VALUES ('Écrou') RETURNING id`, nil, &f.part1)
	scan(`INSERT INTO parts (name) VALUES ('Jante') RETURNING id`, nil, &f.part2)
	const offer = `INSERT INTO offers (part_id, supplier_id, currency, price_min, price_max, lead_time_days, origin_country)
		VALUES ($1, $2, 'EUR', 1, 2, 5, 'FR') RETURNING id`
	scan(offer, []any{f.part1, supplier}, &f.offer1a)
	scan(offer, []any{f.part1, supplier}, &f.offer1b)
	scan(offer, []any{f.part2, supplier}, &f.off2)
	return f
}

// sampleTree : racine R → A → B → C, et R → D. Renvoie la BOM et les ids par nom.
func sampleTree(t *testing.T, s *bomstore.Store) (bom.BOM, map[string]int64) {
	t.Helper()
	ctx := t.Context()
	b, err := s.CreateBOM(ctx, bom.NewBOM{Name: "Voiture", Currency: "EUR", Volume: 1000})
	if err != nil {
		t.Fatalf("create bom: %v", err)
	}
	nodes, err := s.Tree(ctx, b.ID)
	if err != nil {
		t.Fatalf("tree: %v", err)
	}
	ids := map[string]int64{"R": nodes[0].ID}
	for _, n := range []struct{ name, parent string }{{"A", "R"}, {"B", "A"}, {"C", "B"}, {"D", "R"}} {
		got, err := s.AddNode(ctx, b.ID, bom.NewNode{ParentID: ids[n.parent], Name: n.name, Quantity: decimal.NewFromInt(1)})
		if err != nil {
			t.Fatalf("add %s: %v", n.name, err)
		}
		ids[n.name] = got.ID
	}
	return b, ids
}

// parents renvoie, pour chaque nœud de l'arbre, l'id de son parent (0 pour la racine).
func parents(t *testing.T, s *bomstore.Store, bomID int64) map[int64]int64 {
	t.Helper()
	nodes, err := s.Tree(t.Context(), bomID)
	if err != nil {
		t.Fatalf("tree: %v", err)
	}
	out := make(map[int64]int64, len(nodes))
	for _, n := range nodes {
		var p int64
		if n.ParentID != nil {
			p = *n.ParentID
		}
		out[n.ID] = p
	}
	return out
}

func TestCreateBOMHasRootCurrencyAndVolume(t *testing.T) {
	tx := testDB(t)
	s := bomstore.New(tx)
	ctx := t.Context()

	b, err := s.CreateBOM(ctx, bom.NewBOM{Name: "Citadine", Currency: "EUR", Volume: 5000})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if b.Currency != "EUR" || b.Volume != 5000 || b.PriceSnapshotID != nil {
		t.Fatalf("bom = %+v, want EUR, 5000, prix en direct", b)
	}
	nodes, err := s.Tree(ctx, b.ID)
	if err != nil {
		t.Fatalf("tree: %v", err)
	}
	if len(nodes) != 1 || nodes[0].ParentID != nil || nodes[0].Name != "Citadine" {
		t.Fatalf("tree = %+v, want une seule racine nommée Citadine", nodes)
	}

	if _, err := s.UpdateSettings(ctx, b.ID, "USD", 10); err != nil {
		t.Fatalf("update settings: %v", err)
	}
	got, err := s.GetBOM(ctx, b.ID)
	if err != nil || got.Currency != "USD" || got.Volume != 10 {
		t.Fatalf("get = %+v, %v, want USD et 10", got, err)
	}
	if _, err := s.GetBOM(ctx, -1); !errors.Is(err, bom.ErrBOMNotFound) {
		t.Fatalf("get inconnue : err = %v, want ErrBOMNotFound", err)
	}
	if _, err := s.Tree(ctx, -1); !errors.Is(err, bom.ErrBOMNotFound) {
		t.Fatalf("tree inconnue : err = %v, want ErrBOMNotFound", err)
	}
}

// SI je déplace un nœud sous l'un de ses descendants ALORS le système DOIT refuser (bom-tree-editor).
func TestMoveNode(t *testing.T) {
	tx := testDB(t)
	tests := []struct {
		name, node, newParent string
		wantErr               error
	}{
		{"sous un petit-enfant", "A", "C", bom.ErrCycle},
		{"sous un enfant direct", "A", "B", bom.ErrCycle},
		{"sous lui-même", "A", "A", bom.ErrCycle},
		{"la racine sous une feuille", "R", "D", bom.ErrCycle},
		{"sous un nœud d'une autre branche", "D", "C", nil},
		{"un sous-arbre remonté vers la racine", "B", "R", nil},
		{"sous un nœud inexistant", "A", "inconnu", bom.ErrNodeNotFound},
		{"sous un nœud d'une autre BOM", "A", "autre", bom.ErrNodeNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := bomstore.New(savepoint(t, tx))
			b, ids := sampleTree(t, s)
			_, otherIDs := sampleTree(t, s)
			ids["autre"] = otherIDs["A"]
			ids["inconnu"] = -1
			before := parents(t, s, b.ID)

			moved, err := s.MoveNode(t.Context(), b.ID, ids[tt.node], ids[tt.newParent], nil)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			after := parents(t, s, b.ID)
			if err != nil {
				for id, p := range before {
					if after[id] != p {
						t.Fatalf("refus mais arbre modifié : nœud %d parent %d → %d", id, p, after[id])
					}
				}
				return
			}
			if moved.ParentID == nil || *moved.ParentID != ids[tt.newParent] || after[ids[tt.node]] != ids[tt.newParent] {
				t.Fatalf("parent de %s = %d, want %d", tt.node, after[ids[tt.node]], ids[tt.newParent])
			}
		})
	}
}

// SI je supprime un sous-ensemble ALORS ses enfants sont supprimés ; la racine ne se supprime pas.
func TestDeleteNode(t *testing.T) {
	tx := testDB(t)
	tests := []struct {
		name      string
		node      string
		wantErr   error
		wantNodes []string
	}{
		{"un sous-ensemble emporte son sous-arbre", "A", nil, []string{"R", "D"}},
		{"une feuille", "C", nil, []string{"R", "A", "B", "D"}},
		{"la racine est refusée", "R", bom.ErrRootNode, []string{"R", "A", "B", "C", "D"}},
		{"un nœud inexistant", "inconnu", bom.ErrNodeNotFound, []string{"R", "A", "B", "C", "D"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := bomstore.New(savepoint(t, tx))
			b, ids := sampleTree(t, s)
			ids["inconnu"] = -1

			err := s.DeleteNode(t.Context(), b.ID, ids[tt.node])
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			left := parents(t, s, b.ID)
			if len(left) != len(tt.wantNodes) {
				t.Fatalf("%d nœuds restants, want %v", len(left), tt.wantNodes)
			}
			for _, n := range tt.wantNodes {
				if _, ok := left[ids[n]]; !ok {
					t.Fatalf("nœud %s absent, want %v", n, tt.wantNodes)
				}
			}
		})
	}
}

// SI une BOM a un snapshot publié ALORS sa suppression est refusée (R2) ; sinon elle part avec son arbre.
func TestDeleteBOM(t *testing.T) {
	tx := testDB(t)
	tests := []struct {
		name         string
		withSnapshot bool
		wantErr      error
	}{
		{"sans snapshot", false, nil},
		{"avec un snapshot publié", true, bom.ErrHasSnapshots},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sp := savepoint(t, tx)
			s := bomstore.New(sp)
			ctx := t.Context()
			b, _ := sampleTree(t, s)
			if tt.withSnapshot {
				var priceSnap int64
				if err := sp.QueryRow(ctx, `INSERT INTO price_snapshots (scope, scope_id, taken_at)
					VALUES ('bom', $1, now()) RETURNING id`, b.ID).Scan(&priceSnap); err != nil {
					t.Fatalf("price snapshot: %v", err)
				}
				if _, err := sp.Exec(ctx, `INSERT INTO bom_snapshots (bom_id, price_snapshot_id, name, currency, volume)
					VALUES ($1, $2, 'v1', 'EUR', 1000)`, b.ID, priceSnap); err != nil {
					t.Fatalf("bom snapshot: %v", err)
				}
			}

			// la suppression refusée est isolée dans un savepoint : on vérifie ensuite que la BOM est intacte
			del := savepoint(t, sp)
			err := bomstore.New(del).DeleteBOM(ctx, b.ID)
			if err != nil {
				_ = del.Rollback(ctx)
			}
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			_, getErr := s.Tree(ctx, b.ID)
			if tt.wantErr == nil && !errors.Is(getErr, bom.ErrBOMNotFound) {
				t.Fatalf("après suppression : tree err = %v, want ErrBOMNotFound", getErr)
			}
			if tt.wantErr != nil && getErr != nil {
				t.Fatalf("après refus : tree err = %v, want BOM intacte", getErr)
			}
		})
	}
	if err := bomstore.New(savepoint(t, tx)).DeleteBOM(t.Context(), -1); !errors.Is(err, bom.ErrBOMNotFound) {
		t.Fatalf("BOM inconnue : err = %v, want ErrBOMNotFound", err)
	}
}

// Ajout d'une pièce avec l'offre et la quantité choisies (R3), changement d'offre, garde-fous de la base.
func TestAddPartAndChangeOffer(t *testing.T) {
	tx := testDB(t)
	cat := newCatalog(t, tx)
	s := bomstore.New(tx)
	ctx := t.Context()
	b, ids := sampleTree(t, s)
	_, otherIDs := sampleTree(t, s)

	qty := decimal.RequireFromString("4.5")
	part, err := s.AddNode(ctx, b.ID, bom.NewNode{ParentID: ids["D"], PartID: &cat.part1, OfferID: &cat.offer1a, Quantity: qty})
	if err != nil {
		t.Fatalf("add part: %v", err)
	}
	if !part.IsPart() || *part.OfferID != cat.offer1a || !part.Quantity.Equal(qty) || *part.ParentID != ids["D"] {
		t.Fatalf("part = %+v, want offre %d, quantité %s sous D", part, cat.offer1a, qty)
	}
	if got, err := s.SetNodeOffer(ctx, b.ID, part.ID, cat.offer1b); err != nil || *got.OfferID != cat.offer1b {
		t.Fatalf("set offer = %+v, %v, want offre %d", got, err, cat.offer1b)
	}

	tests := []struct {
		name    string
		run     func(s *bomstore.Store) error
		wantErr error // nil : une erreur quelconque est attendue (garde-fou de la base)
	}{
		{"parent dans une autre BOM", func(s *bomstore.Store) error {
			_, err := s.AddNode(ctx, b.ID, bom.NewNode{ParentID: otherIDs["A"], Name: "X", Quantity: decimal.NewFromInt(1)})
			return err
		}, bom.ErrNodeNotFound},
		{"offre sur un sous-ensemble", func(s *bomstore.Store) error {
			_, err := s.SetNodeOffer(ctx, b.ID, ids["A"], cat.offer1a)
			return err
		}, bom.ErrNotAPart},
		{"offre d'une autre pièce", func(s *bomstore.Store) error {
			_, err := s.SetNodeOffer(ctx, b.ID, part.ID, cat.off2)
			return err
		}, nil},
		{"quantité nulle", func(s *bomstore.Store) error {
			_, err := s.SetNodeQuantity(ctx, b.ID, part.ID, decimal.Zero)
			return err
		}, nil},
		{"nœud inconnu", func(s *bomstore.Store) error {
			_, err := s.SetNodeQuantity(ctx, b.ID, -1, decimal.NewFromInt(2))
			return err
		}, bom.ErrNodeNotFound},
	}
	for _, tt := range tests {
		t.Run("SI "+tt.name+" ALORS refus", func(t *testing.T) {
			err := tt.run(bomstore.New(savepoint(t, tx)))
			if err == nil {
				t.Fatal("err = nil, want un refus")
			}
			if tt.wantErr != nil && !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

// ADR 0022 : le stock est propre à la BOM ; une BOM inconnue est ErrBOMNotFound, une quantité négative est
// refusée par la base, et supprimer la BOM supprime son stock.
func TestStoreStock(t *testing.T) {
	tx := testDB(t)
	ctx := t.Context()
	cat := newCatalog(t, tx)
	s := bomstore.New(tx)
	a, err := s.CreateBOM(ctx, bom.NewBOM{Name: "A", Currency: "EUR", Volume: 1})
	if err != nil {
		t.Fatal(err)
	}
	other, err := s.CreateBOM(ctx, bom.NewBOM{Name: "B", Currency: "EUR", Volume: 1})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetStock(ctx, a.ID, bom.PartStock{PartID: cat.part1, Quantity: dec("3")}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetStock(ctx, a.ID, bom.PartStock{PartID: cat.part1, Quantity: dec("7.5")}); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	st, err := s.Stock(ctx, a.ID)
	if err != nil || len(st) != 1 || !st[0].Quantity.Equal(dec("7.5")) {
		t.Fatalf("stock A = %v, %v ; want part1 × 7.5", st, err)
	}
	if st, _ := s.Stock(ctx, other.ID); len(st) != 0 {
		t.Fatalf("stock B = %v, want none (pas de partage entre BOM)", st)
	}
	sp := savepoint(t, tx)
	if _, err := bomstore.New(sp).SetStock(ctx, 999999, bom.PartStock{PartID: cat.part1, Quantity: dec("1")}); !errors.Is(err, bom.ErrBOMNotFound) {
		t.Fatalf("unknown bom: err = %v, want ErrBOMNotFound", err)
	}
	_ = sp.Rollback(ctx)
	sp = savepoint(t, tx)
	if _, err := bomstore.New(sp).SetStock(ctx, a.ID, bom.PartStock{PartID: cat.part1, Quantity: dec("-1")}); err == nil {
		t.Fatal("negative stock accepted by the database")
	}
	_ = sp.Rollback(ctx)
	if err := s.DeleteBOM(ctx, a.ID); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM bom_part_stock WHERE bom_id = $1`, a.ID).Scan(&n); err != nil || n != 0 {
		t.Fatalf("stock left after delete: %d, %v", n, err)
	}
}

func dec(s string) decimal.Decimal { return decimal.RequireFromString(s) }

// ADR 0028 : stratégie d'un nœud (contrainte de la base) et remplacement de la pièce d'un nœud.
func TestStoreStrategyAndReplacePart(t *testing.T) {
	tx := testDB(t)
	ctx := t.Context()
	cat := newCatalog(t, tx)
	s := bomstore.New(tx)
	b, err := s.CreateBOM(ctx, bom.NewBOM{Name: "S", Currency: "EUR", Volume: 1})
	if err != nil {
		t.Fatal(err)
	}
	nodes, err := s.Tree(ctx, b.ID)
	if err != nil {
		t.Fatal(err)
	}
	root := nodes[0].ID
	p, err := s.AddNode(ctx, b.ID, bom.NewNode{ParentID: root, PartID: &cat.part1, OfferID: &cat.offer1a, Quantity: dec("2")})
	if err != nil {
		t.Fatal(err)
	}
	st := bom.StrategyBalanced
	got, err := s.SetNodeStrategy(ctx, b.ID, root, &st)
	if err != nil || got.Strategy == nil || *got.Strategy != st {
		t.Fatalf("set strategy: %+v, %v", got, err)
	}
	if got, err = s.SetNodeStrategy(ctx, b.ID, root, nil); err != nil || got.Strategy != nil {
		t.Fatalf("clear strategy: %+v, %v", got, err)
	}
	sp := savepoint(t, tx)
	bad := bom.Strategy("cheapest")
	if _, err := bomstore.New(sp).SetNodeStrategy(ctx, b.ID, root, &bad); err == nil {
		t.Fatal("unknown strategy accepted by the database")
	}
	_ = sp.Rollback(ctx)

	n, err := s.SetNodePart(ctx, b.ID, p.ID, cat.part2, cat.off2)
	if err != nil || *n.PartID != cat.part2 || *n.OfferID != cat.off2 || !n.Quantity.Equal(dec("2")) {
		t.Fatalf("set part: %+v, %v", n, err)
	}
	if _, err := s.SetNodePart(ctx, b.ID, root, cat.part2, cat.off2); !errors.Is(err, bom.ErrNotAPart) {
		t.Fatalf("set part of a sub-assembly: err %v, want ErrNotAPart", err)
	}
	if _, err := s.SetNodePart(ctx, b.ID, 999999, cat.part2, cat.off2); !errors.Is(err, bom.ErrNodeNotFound) {
		t.Fatalf("set part of unknown node: err %v, want ErrNodeNotFound", err)
	}
}

// ADR 0031 : la méthode de coût se relit telle qu'enregistrée ; la base refuse full sans volume programme.
func TestStoreCostMethod(t *testing.T) {
	tx := testDB(t)
	ctx := t.Context()
	s := bomstore.New(tx)
	b, err := s.CreateBOM(ctx, bom.NewBOM{Name: "M", Currency: "EUR", Volume: 10})
	if err != nil {
		t.Fatal(err)
	}
	if b.CostMethod != (bom.CostMethod{Basis: bom.BasisPurchase, FX: bom.FXLatest}) {
		t.Fatalf("default method = %+v", b.CostMethod)
	}
	pv := int32(60000)
	tests := []struct {
		name    string
		m       bom.CostMethod
		wantErr bool
	}{
		{"full + budget avec volume programme", bom.CostMethod{Basis: bom.BasisFull, FX: bom.FXBudget, ProgramVolume: &pv}, false},
		{"full sans volume : refus de la base", bom.CostMethod{Basis: bom.BasisFull, FX: bom.FXLatest}, true},
		{"base inconnue : refus de la base", bom.CostMethod{Basis: "gross", FX: bom.FXLatest}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sp := savepoint(t, tx)
			got, err := bomstore.New(sp).SetCostMethod(ctx, b.ID, tt.m)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if err == nil && (got.CostMethod.Basis != tt.m.Basis || *got.CostMethod.ProgramVolume != pv) {
				t.Errorf("method = %+v", got.CostMethod)
			}
		})
	}
	if _, err := s.SetCostMethod(ctx, 1<<40, bom.CostMethod{Basis: bom.BasisPurchase, FX: bom.FXLatest}); !errors.Is(err, bom.ErrBOMNotFound) {
		t.Errorf("unknown bom: %v", err)
	}
	if got, err := s.TargetBudgets(ctx, b.ID); err != nil || len(got) != 0 {
		t.Errorf("budgets = %v, %v", got, err)
	}
}

// TestEditTargetBudget : la règle des budgets hiérarchiques (ADR 0040) est jugée dans la transaction ; un refus
// n'écrit rien (ni pose, ni retrait, ni déplacement).
func TestEditTargetBudget(t *testing.T) {
	tx := testDB(t)
	s := bomstore.New(tx)
	ctx := t.Context()
	b, ids := sampleTree(t, s) // R ─ A ─ B ─ C ; R ─ D
	check := func(nodes []bom.Node, budgets []bom.TargetBudget) error { return bom.CheckBudgetTree(nodes, budgets, "EUR") }
	eur := func(node, amount string) *bom.TargetBudget {
		return &bom.TargetBudget{NodeID: ids[node], Amount: decimal.RequireFromString(amount), Currency: "EUR"}
	}
	budgets := func() map[int64]string {
		list, err := s.TargetBudgets(ctx, b.ID)
		if err != nil {
			t.Fatal(err)
		}
		out := map[int64]string{}
		for _, x := range list {
			out[x.NodeID] = x.Amount.String()
		}
		return out
	}
	steps := []struct {
		name    string
		do      func() error
		wantErr error
		want    map[string]string // budgets attendus après l'étape
	}{
		{"A à 3000", func() error { return s.EditTargetBudget(ctx, b.ID, ids["A"], eur("A", "3000"), check) }, nil, map[string]string{"A": "3000"}},
		{"C à 3000.01 sous A : refus, rien d'écrit", func() error { return s.EditTargetBudget(ctx, b.ID, ids["C"], eur("C", "3000.01"), check) },
			bom.ErrBudgetExceedsParent, map[string]string{"A": "3000"}},
		{"C à 3000", func() error { return s.EditTargetBudget(ctx, b.ID, ids["C"], eur("C", "3000"), check) }, nil, map[string]string{"A": "3000", "C": "3000"}},
		{"D à 500 et R à 3500", func() error {
			if err := s.EditTargetBudget(ctx, b.ID, ids["D"], eur("D", "500"), check); err != nil {
				return err
			}
			return s.EditTargetBudget(ctx, b.ID, ids["R"], eur("R", "3500"), check)
		}, nil, map[string]string{"A": "3000", "C": "3000", "D": "500", "R": "3500"}},
		{"D déplacé sous B : 3000 + 500 > 3000 dans A, refus, D reste sous R", func() error {
			_, err := s.MoveNode(ctx, b.ID, ids["D"], ids["B"], check)
			return err
		}, bom.ErrBudgetExceedsParent, map[string]string{"A": "3000", "C": "3000", "D": "500", "R": "3500"}},
		{"retrait de A : C et D remontent sous R, 3500 ≤ 3500", func() error { return s.EditTargetBudget(ctx, b.ID, ids["A"], nil, check) },
			nil, map[string]string{"C": "3000", "D": "500", "R": "3500"}},
		{"nœud inconnu", func() error { return s.EditTargetBudget(ctx, b.ID, -1, eur("A", "1"), check) }, bom.ErrNodeNotFound,
			map[string]string{"C": "3000", "D": "500", "R": "3500"}},
	}
	for _, st := range steps {
		if err := st.do(); !errors.Is(err, st.wantErr) || (st.wantErr == nil && err != nil) {
			t.Fatalf("%s: err %v, want %v", st.name, err, st.wantErr)
		}
		got := budgets()
		want := map[int64]string{}
		for k, v := range st.want {
			want[ids[k]] = v
		}
		if fmt.Sprint(got) != fmt.Sprint(want) {
			t.Fatalf("%s: budgets %v, want %v", st.name, got, want)
		}
	}
	if p := parents(t, s, b.ID); p[ids["D"]] != ids["R"] {
		t.Fatalf("D under %d after refused move, want R", p[ids["D"]])
	}
}
