package viz3d

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/shopspring/decimal"

	"bomstudio/internal/bom"
	"bomstudio/internal/catalog"
)

// memStore : viz3d.Store en mémoire, pour tester le service sans base. Il reproduit le contrat du store
// Postgres (une génération par BOM, refus si en attente ou en cours, modèle par pièce remplacé).
type memStore struct {
	mu          sync.Mutex
	models      map[int64]Model
	partModels  map[int64]PartModel
	generations map[int64]Generation
	nextID      int64
	// failAfter : après ce nombre d'associations, SetPartModel échoue (simule une panne à mi-chemin).
	failAfter int
	sets      int
}

func newMemStore() *memStore {
	return &memStore{models: map[int64]Model{}, partModels: map[int64]PartModel{}, generations: map[int64]Generation{}, failAfter: -1}
}

var errStoreDown = errors.New("store down")

func (m *memStore) CreateModel(_ context.Context, mo Model) (Model, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.nextID++
	mo.ID = m.nextID
	m.models[mo.ID] = mo
	return mo, nil
}

func (m *memStore) Model(_ context.Context, id int64) (Model, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	mo, ok := m.models[id]
	if !ok {
		return Model{}, ErrModelNotFound
	}
	return mo, nil
}

func (m *memStore) SetPartModel(_ context.Context, pm PartModel) (PartModel, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.failAfter >= 0 && m.sets >= m.failAfter {
		return PartModel{}, errStoreDown
	}
	if _, ok := m.models[pm.ModelID]; !ok {
		return PartModel{}, ErrModelNotFound
	}
	m.sets++
	m.partModels[pm.PartID] = pm
	return pm, nil
}

func (m *memStore) PartModel(_ context.Context, partID int64) (Resolved, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	pm, ok := m.partModels[partID]
	if !ok {
		return Resolved{}, ErrNoModel
	}
	return Resolved{PartModel: pm, Model: m.models[pm.ModelID]}, nil
}

func (m *memStore) PartModels(_ context.Context, ids []int64) (map[int64]Resolved, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := map[int64]Resolved{}
	for _, id := range ids {
		if pm, ok := m.partModels[id]; ok {
			out[id] = Resolved{PartModel: pm, Model: m.models[pm.ModelID]}
		}
	}
	return out, nil
}

func (m *memStore) ClaimGeneration(_ context.Context, bomID int64, total int, fp string) (Generation, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if g, ok := m.generations[bomID]; ok && (g.Status == StatusQueued || g.Status == StatusRunning) {
		return Generation{}, ErrGenerationRunning
	}
	g := Generation{BOMID: bomID, Status: StatusQueued, Total: total, Fingerprint: fp}
	m.generations[bomID] = g
	return g, nil
}

func (m *memStore) Generation(_ context.Context, bomID int64) (Generation, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	g, ok := m.generations[bomID]
	if !ok {
		return Generation{}, ErrNoGeneration
	}
	return g, nil
}

func (m *memStore) SetGenerationProgress(_ context.Context, bomID int64, done int) (Generation, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	g := m.generations[bomID]
	g.Status, g.Done = StatusRunning, done
	m.generations[bomID] = g
	return g, nil
}

func (m *memStore) FinishGeneration(_ context.Context, bomID int64, st Status, done int, errText string, res *GenerationResult) (Generation, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	g := m.generations[bomID]
	g.Status, g.Done, g.Error, g.Result = st, done, errText, res
	m.generations[bomID] = g
	return g, nil
}

func (m *memStore) PendingGenerations(context.Context) ([]Generation, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []Generation
	for _, g := range m.generations {
		if g.Status == StatusQueued || g.Status == StatusRunning {
			out = append(out, g)
		}
	}
	slices.SortFunc(out, func(a, b Generation) int { return int(a.BOMID - b.BOMID) })
	return out, nil
}

// memBlobs : BlobStore en mémoire.
type memBlobs struct {
	mu    sync.Mutex
	files map[string][]byte
}

func (b *memBlobs) Put(_ context.Context, key string, r io.Reader) error {
	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.files == nil {
		b.files = map[string][]byte{}
	}
	b.files[key] = data
	return nil
}

func (b *memBlobs) Get(_ context.Context, key string) (io.ReadCloser, int64, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	d, ok := b.files[key]
	if !ok {
		return nil, 0, ErrBlobNotFound
	}
	return io.NopCloser(bytes.NewReader(d)), int64(len(d)), nil
}

func (b *memBlobs) Exists(_ context.Context, key string) (bool, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	_, ok := b.files[key]
	return ok, nil
}

// multiTrees : arbres par BOM, modifiables entre deux appels (pour rendre une scène « stale »).
type multiTrees struct {
	mu    sync.Mutex
	trees map[int64][]bom.Node
}

func (f *multiTrees) Tree(_ context.Context, bomID int64) (bom.BOM, []bom.Node, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	n, ok := f.trees[bomID]
	if !ok {
		return bom.BOM{}, nil, bom.ErrBOMNotFound
	}
	return bom.BOM{ID: bomID}, slices.Clone(n), nil
}

// countingFinder enveloppe un chercheur et compte ses appels ; failOn fait échouer une pièce donnée.
type countingFinder struct {
	inner  ModelFinder
	calls  []int64
	failOn int64
	err    error
}

func (c *countingFinder) Name() string { return c.inner.Name() }
func (c *countingFinder) Find(ctx context.Context, p catalog.Part) (Candidate, error) {
	c.calls = append(c.calls, p.ID)
	if p.ID == c.failOn {
		return Candidate{}, c.err
	}
	return c.inner.Find(ctx, p)
}

// sync : les générations tournent dans l'appel (pas de goroutine) : le test lit l'état final sans attendre.
func syncLauncher(job func(ctx context.Context)) { job(context.Background()) }

func newTestService(store Store, blobs BlobStore, finder ModelFinder) (*Service, *multiTrees) {
	nodes, parts, _ := fixture()
	trees := &multiTrees{trees: map[int64][]bom.Node{1: nodes}}
	s := NewService(trees, fakeParts{parts: parts}).WithModels(store, blobs, finder, func(id int64) string { return fmt.Sprintf("/m/%d", id) }).
		WithLauncher(syncLauncher)
	return s, trees
}

func TestFingerprint(t *testing.T) {
	nodes, _, _ := fixture()
	base := Fingerprint(nodes)
	shuffled := slices.Clone(nodes)
	slices.Reverse(shuffled)
	moved := slices.Clone(nodes)
	moved[3].ParentID = ptr(int64(3))
	qty := slices.Clone(nodes)
	qty[5].Quantity = dec("5")
	offer := slices.Clone(nodes)
	offer[5].OfferID = ptr(int64(999))

	tests := []struct {
		name  string
		nodes []bom.Node
		same  bool
	}{
		{"même arbre dans un autre ordre : même empreinte", shuffled, true},
		{"un nœud déplacé : empreinte différente", moved, false},
		{"une quantité changée : empreinte différente", qty, false},
		{"un nœud retiré : empreinte différente", nodes[:len(nodes)-1], false},
		{"une offre changée : même empreinte (la scène n'en dépend pas)", offer, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Fingerprint(tt.nodes) == base; got != tt.same {
				t.Fatalf("égalité d'empreinte = %v, attendu %v", got, tt.same)
			}
		})
	}
}

// Génération : chaque pièce distincte reçoit un modèle, le bilan dit lequel, et la scène les reflète.
func TestGeneration(t *testing.T) {
	t.Run("génération complète : une primitive par pièce distincte, scène enrichie", func(t *testing.T) {
		store := newMemStore()
		finder := &countingFinder{inner: GeneratedFinder{}}
		s, _ := newTestService(store, &memBlobs{}, finder)
		ctx := t.Context()

		g, err := s.StartGeneration(ctx, 1)
		if err != nil {
			t.Fatal(err)
		}
		if g.Status != StatusQueued || g.Total != 9 {
			t.Fatalf("lancement = %+v, attendu queued / 9 pièces distinctes", g)
		}
		st, err := s.GenerationStatus(ctx, 1)
		if err != nil {
			t.Fatal(err)
		}
		if st.Status != StatusDone || st.Done != 9 || st.Result == nil || st.Result.Generated != 8 || len(st.Result.Failed) != 1 ||
			st.Result.Failed[0].PartID != 105 || st.Result.Finder != "generated-v1" {
			t.Fatalf("état final = %+v (bilan %+v)", st, st.Result)
		}
		// La pièce absente du catalogue est tentée une fois, pas associée ; les 8 autres ont un modèle.
		if len(finder.calls) != 8 || len(store.partModels) != 8 {
			t.Fatalf("%d appels au chercheur, %d associations ; attendu 8 et 8", len(finder.calls), len(store.partModels))
		}
		a, err := s.Assembly(ctx, 1)
		if err != nil {
			t.Fatal(err)
		}
		if a.Generation == nil || a.Generation.Status != StatusDone {
			t.Fatalf("scène sans état de génération : %+v", a.Generation)
		}
		for _, p := range a.Placements {
			if p.ModelID == nil || p.Match != MatchGenerated || p.ModelURL != nil {
				t.Fatalf("placement %d : model_id %v, match %s, url %v ; attendu primitive persistée sans URL", p.NodeID, p.ModelID, p.Match, p.ModelURL)
			}
		}
	})

	t.Run("relance pendant la génération : refusée", func(t *testing.T) {
		store := newMemStore()
		s, _ := newTestService(store, &memBlobs{}, GeneratedFinder{})
		// Un lanceur qui ne fait rien : la génération reste « queued ».
		s.WithLauncher(func(func(ctx context.Context)) {})
		if _, err := s.StartGeneration(t.Context(), 1); err != nil {
			t.Fatal(err)
		}
		if _, err := s.StartGeneration(t.Context(), 1); !errors.Is(err, ErrGenerationRunning) {
			t.Fatalf("err = %v, attendu ErrGenerationRunning", err)
		}
	})

	t.Run("BOM inconnue : ErrBOMNotFound au lancement et au statut", func(t *testing.T) {
		s, _ := newTestService(newMemStore(), &memBlobs{}, GeneratedFinder{})
		if _, err := s.StartGeneration(t.Context(), 42); !errors.Is(err, bom.ErrBOMNotFound) {
			t.Fatalf("start : err = %v", err)
		}
		if _, err := s.GenerationStatus(t.Context(), 42); !errors.Is(err, bom.ErrBOMNotFound) {
			t.Fatalf("status : err = %v", err)
		}
	})

	t.Run("sans génération : statut none, scène sans modèle persisté", func(t *testing.T) {
		s, _ := newTestService(newMemStore(), &memBlobs{}, GeneratedFinder{})
		st, err := s.GenerationStatus(t.Context(), 1)
		if err != nil || st.Status != StatusNone {
			t.Fatalf("statut = %+v, %v", st, err)
		}
		a, err := s.Assembly(t.Context(), 1)
		if err != nil || a.Generation == nil || a.Generation.Status != StatusNone || a.Placements[0].ModelID != nil {
			t.Fatalf("scène = %+v, %v", a.Generation, err)
		}
	})

	t.Run("sans stockage (tranche 1) : ErrModelsUnavailable, la scène reste rendue", func(t *testing.T) {
		nodes, parts, _ := fixture()
		s := NewService(&multiTrees{trees: map[int64][]bom.Node{1: nodes}}, fakeParts{parts: parts})
		if _, err := s.StartGeneration(t.Context(), 1); !errors.Is(err, ErrModelsUnavailable) {
			t.Fatalf("err = %v", err)
		}
		a, err := s.Assembly(t.Context(), 1)
		if err != nil || a.Generation != nil || len(a.Placements) == 0 {
			t.Fatalf("scène = %+v, %v", a, err)
		}
	})
}

// Reprise : une génération interrompue à mi-chemin (store en panne, puis arrêt) reprend en sautant les
// pièces déjà résolues ; le chercheur n'est pas rappelé pour elles.
func TestGenerationResume(t *testing.T) {
	store := newMemStore()
	store.failAfter = 3
	finder := &countingFinder{inner: GeneratedFinder{}}
	s, _ := newTestService(store, &memBlobs{}, finder)
	ctx := t.Context()

	if _, err := s.StartGeneration(ctx, 1); err != nil {
		t.Fatal(err)
	}
	st, err := s.GenerationStatus(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if st.Status != StatusFailed || !strings.Contains(st.Error, "store down") || len(store.partModels) != 3 {
		t.Fatalf("après la panne : %+v, %d associations", st, len(store.partModels))
	}
	firstCalls := len(finder.calls)

	// Le store revient ; une relance (ou la reprise au démarrage) termine le travail restant.
	store.failAfter = -1
	if _, err := s.StartGeneration(ctx, 1); err != nil {
		t.Fatal(err)
	}
	st, err = s.GenerationStatus(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if st.Status != StatusDone || st.Done != 9 || len(store.partModels) != 8 {
		t.Fatalf("après la reprise : %+v, %d associations", st, len(store.partModels))
	}
	// 8 pièces lisibles : 4 tentées lors de la première passe (3 réussies + la 4e qui a échoué),
	// puis seulement celles sans modèle lors de la seconde (5) : jamais 8 + 8.
	if got := len(finder.calls) - firstCalls; got != 5 {
		t.Fatalf("%d appels au chercheur lors de la reprise (premier passage : %d), attendu 5", got, firstCalls)
	}
	for _, pm := range store.partModels {
		if pm.Match != MatchGenerated {
			t.Fatalf("association inattendue : %+v", pm)
		}
	}
}

// ResumeGenerations relance ce qui était en attente ou en cours au redémarrage, rien d'autre.
func TestResumeGenerations(t *testing.T) {
	store := newMemStore()
	store.generations[1] = Generation{BOMID: 1, Status: StatusRunning, Total: 9, Fingerprint: "old"}
	store.generations[2] = Generation{BOMID: 2, Status: StatusDone, Total: 1, Fingerprint: "x"}
	s, trees := newTestService(store, &memBlobs{}, GeneratedFinder{})
	trees.trees[2] = trees.trees[1]
	if err := s.ResumeGenerations(t.Context()); err != nil {
		t.Fatal(err)
	}
	if g := store.generations[1]; g.Status != StatusDone || g.Done != 9 {
		t.Fatalf("génération 1 non reprise : %+v", g)
	}
	if g := store.generations[2]; g.Status != StatusDone || g.Done != 0 {
		t.Fatalf("génération 2 (déjà finie) a été relancée : %+v", g)
	}
}

// Une BOM modifiée après la génération donne une scène « stale » ; l'état stocké reste « done ».
func TestGenerationStale(t *testing.T) {
	store := newMemStore()
	s, trees := newTestService(store, &memBlobs{}, GeneratedFinder{})
	ctx := t.Context()
	if _, err := s.StartGeneration(ctx, 1); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name   string
		change func()
		want   Status
	}{
		{"arbre inchangé : done", func() {}, StatusDone},
		{"offre changée : done (la scène n'en dépend pas)", func() {
			trees.mu.Lock()
			defer trees.mu.Unlock()
			trees.trees[1][5].OfferID = ptr(int64(7))
		}, StatusDone},
		{"quantité changée : stale", func() {
			trees.mu.Lock()
			defer trees.mu.Unlock()
			trees.trees[1][5].Quantity = dec("3")
		}, StatusStale},
		{"nœud ajouté : stale", func() {
			trees.mu.Lock()
			defer trees.mu.Unlock()
			trees.trees[1] = append(trees.trees[1], node(99, ptr(int64(3)), ptr(int64(102)), "1"))
		}, StatusStale},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.change()
			st, err := s.GenerationStatus(ctx, 1)
			if err != nil {
				t.Fatal(err)
			}
			if st.Status != tt.want {
				t.Fatalf("statut %s, attendu %s", st.Status, tt.want)
			}
			a, err := s.Assembly(ctx, 1)
			if err != nil || a.Generation.Status != tt.want {
				t.Fatalf("scène : statut %v, err %v", a.Generation, err)
			}
		})
	}
	if g := store.generations[1]; g.Status != StatusDone {
		t.Fatalf("l'état stocké a changé : %+v", g)
	}
	// Une relance après « stale » est acceptée et ne refait que la pièce nouvelle (ici aucune : même pièce).
	if _, err := s.StartGeneration(ctx, 1); err != nil {
		t.Fatal(err)
	}
	if st, _ := s.GenerationStatus(ctx, 1); st.Status != StatusDone {
		t.Fatalf("après relance : %+v", st)
	}
}

// Un chercheur qui échoue sur une pièce (erreur réseau…) fait échouer la génération, nommément ;
// ErrNoModel sur une pièce n'arrête pas les autres.
func TestGenerationFinderErrors(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus Status
		wantFailed int
	}{
		{"pas de modèle pour une pièce : signalée, génération finie", ErrNoModel, StatusDone, 2},
		{"erreur du chercheur : génération en échec", errors.New("sketchfab: 503"), StatusFailed, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newMemStore()
			s, _ := newTestService(store, &memBlobs{}, &countingFinder{inner: GeneratedFinder{}, failOn: 102, err: tt.err})
			if _, err := s.StartGeneration(t.Context(), 1); err != nil {
				t.Fatal(err)
			}
			st, err := s.GenerationStatus(t.Context(), 1)
			if err != nil {
				t.Fatal(err)
			}
			if st.Status != tt.wantStatus {
				t.Fatalf("statut %s (%s), attendu %s", st.Status, st.Error, tt.wantStatus)
			}
			if st.Result != nil && len(st.Result.Failed) != tt.wantFailed {
				t.Fatalf("échecs = %+v, attendu %d", st.Result.Failed, tt.wantFailed)
			}
			if tt.wantStatus == StatusFailed && !strings.Contains(st.Error, "sketchfab") {
				t.Fatalf("erreur non nommée : %q", st.Error)
			}
		})
	}
}

// glb fabrique un GLB minimal : en-tête de 12 octets (magic, version, longueur) + charge.
func glb(magic, version uint32, payload []byte, declaredLen int) []byte {
	b := make([]byte, 12, 12+len(payload))
	binary.LittleEndian.PutUint32(b[0:4], magic)
	binary.LittleEndian.PutUint32(b[4:8], version)
	binary.LittleEndian.PutUint32(b[8:12], uint32(declaredLen)) //nolint:gosec // longueurs de test
	return append(b, payload...)
}

const glTF = 0x46546C67

func TestUploadModel(t *testing.T) {
	payload := []byte(`{"asset":{"version":"2.0"}}`)
	valid := glb(glTF, 2, payload, 12+len(payload))
	tests := []struct {
		name     string
		filename string
		data     []byte
		match    MatchType
		conf     *decimal.Decimal
		wantErr  error
		// wantConf : confiance attendue sur l'association (défaut du niveau si non fournie).
		wantConf string
	}{
		{"GLB valide, exact : confiance 1 par défaut", "jante.glb", valid, MatchExact, nil, nil, "1.00"},
		{"GLB valide, similar avec confiance fournie", "Jante.GLB", valid, MatchSimilar, ptr(dec("0.45")), nil, "0.45"},
		{"extension refusée", "jante.gltf", valid, MatchExact, nil, ErrInvalidModel, ""},
		{"magic absent (zip renommé)", "jante.glb", glb(0x04034b50, 2, payload, 12+len(payload)), MatchExact, nil, ErrInvalidModel, ""},
		{"glTF version 1", "jante.glb", glb(glTF, 1, payload, 12+len(payload)), MatchExact, nil, ErrInvalidModel, ""},
		{"longueur annoncée fausse (fichier tronqué)", "jante.glb", glb(glTF, 2, payload, 12+len(payload)+40), MatchExact, nil, ErrInvalidModel, ""},
		{"fichier plus court qu'un en-tête", "jante.glb", []byte("glTF"), MatchExact, nil, ErrInvalidModel, ""},
		{"niveau de correspondance inconnu", "jante.glb", valid, MatchType("perfect"), nil, ErrInvalidModel, ""},
		{"confiance hors 0–1", "jante.glb", valid, MatchExact, ptr(dec("1.5")), ErrInvalidModel, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newMemStore()
			blobs := &memBlobs{}
			s, _ := newTestService(store, blobs, GeneratedFinder{})
			v, err := s.UploadModel(t.Context(), 102, Upload{Filename: tt.filename, Match: tt.match, Confidence: tt.conf,
				License: "CC-BY", Data: bytes.NewReader(tt.data)})
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, attendu %v", err, tt.wantErr)
			}
			if tt.wantErr != nil {
				if len(blobs.files) != 0 || len(store.models) != 0 {
					t.Fatalf("un fichier ou un modèle a été enregistré malgré le refus")
				}
				return
			}
			if v.Model == nil || v.Model.Format != FormatGLB || v.Model.License != "CC-BY" || v.Model.URL != fmt.Sprintf("/m/%d", v.Model.ID) ||
				v.Match != tt.match || v.Confidence != tt.wantConf {
				t.Fatalf("vue = %+v (modèle %+v)", v, v.Model)
			}
			if len(blobs.files) != 1 {
				t.Fatalf("%d fichiers rangés, attendu 1", len(blobs.files))
			}
			for key := range blobs.files {
				if !ValidBlobKey(key) || !strings.HasSuffix(key, ".glb") {
					t.Fatalf("clé %q", key)
				}
			}
			// Le fichier se relit tel quel.
			m, rc, size, err := s.ModelFile(t.Context(), v.Model.ID)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = rc.Close() }()
			got, _ := io.ReadAll(rc)
			if m.ID != v.Model.ID || size != int64(len(tt.data)) || !bytes.Equal(got, tt.data) {
				t.Fatalf("fichier relu : %d octets", size)
			}
		})
	}

	t.Run("fichier trop gros : refusé sans être rangé", func(t *testing.T) {
		store := newMemStore()
		blobs := &memBlobs{}
		s, _ := newTestService(store, blobs, GeneratedFinder{})
		big := glb(glTF, 2, bytes.Repeat([]byte{0}, MaxModelBytes), 12+MaxModelBytes)
		_, err := s.UploadModel(t.Context(), 102, Upload{Filename: "x.glb", Match: MatchExact, Data: bytes.NewReader(big)})
		if !errors.Is(err, ErrInvalidModel) || len(blobs.files) != 0 {
			t.Fatalf("err = %v, %d fichiers", err, len(blobs.files))
		}
	})

	t.Run("deux téléversements identiques partagent le fichier, pas le modèle", func(t *testing.T) {
		store := newMemStore()
		blobs := &memBlobs{}
		s, _ := newTestService(store, blobs, GeneratedFinder{})
		for _, part := range []int64{102, 103} {
			if _, err := s.UploadModel(t.Context(), part, Upload{Filename: "x.glb", Match: MatchSimilar, Data: bytes.NewReader(valid)}); err != nil {
				t.Fatal(err)
			}
		}
		if len(blobs.files) != 1 || len(store.models) != 2 || len(store.partModels) != 2 {
			t.Fatalf("%d fichiers, %d modèles, %d associations", len(blobs.files), len(store.models), len(store.partModels))
		}
	})

	t.Run("un modèle exact téléversé remplace la primitive dans la scène, avec son URL", func(t *testing.T) {
		store := newMemStore()
		s, _ := newTestService(store, &memBlobs{}, GeneratedFinder{})
		ctx := t.Context()
		if _, err := s.StartGeneration(ctx, 1); err != nil {
			t.Fatal(err)
		}
		v, err := s.UploadModel(ctx, 102, Upload{Filename: "pneu.glb", Match: MatchExact, Data: bytes.NewReader(valid)})
		if err != nil {
			t.Fatal(err)
		}
		a, err := s.Assembly(ctx, 1)
		if err != nil {
			t.Fatal(err)
		}
		p := find(t, a, 12) // pneu ×4
		if p.Match != MatchExact || p.Confidence != "1.00" || p.ModelID == nil || *p.ModelID != v.Model.ID ||
			p.ModelURL == nil || *p.ModelURL != v.Model.URL {
			t.Fatalf("placement du pneu : %+v", p)
		}
		if q := find(t, a, 13); q.Match != MatchGenerated || q.ModelURL != nil {
			t.Fatalf("la jante a changé : %+v", q)
		}
		// Associer à nouveau le même modèle en « similar » remplace l'association.
		if _, err := s.AssignModel(ctx, 103, Association{ModelID: v.Model.ID, Match: MatchSimilar}); err != nil {
			t.Fatal(err)
		}
		pm, err := s.PartModel(ctx, 103)
		if err != nil || pm.Match != MatchSimilar || pm.Confidence != "0.60" || pm.Model.ID != v.Model.ID {
			t.Fatalf("association = %+v, %v", pm, err)
		}
		if _, err := s.AssignModel(ctx, 103, Association{ModelID: 999, Match: MatchExact}); !errors.Is(err, ErrModelNotFound) {
			t.Fatalf("modèle inconnu : err = %v", err)
		}
		if _, err := s.PartModel(ctx, 105); !errors.Is(err, catalog.ErrPartNotFound) {
			t.Fatalf("pièce inconnue : err = %v", err)
		}
	})
}

func TestValidBlobKey(t *testing.T) {
	tests := []struct {
		key  string
		want bool
	}{
		{"models/ab/abcdef.glb", true},
		{"a.glb", true},
		{"", false},
		{"/etc/passwd", false},
		{"models/../secret", false},
		{"models/./x", false},
		{"models//x", false},
		{"Models/X.GLB", false},
		{"models/x.glb/", false},
		{"models/fi le.glb", false},
		{"models/é.glb", false},
	}
	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			if got := ValidBlobKey(tt.key); got != tt.want {
				t.Fatalf("ValidBlobKey(%q) = %v", tt.key, got)
			}
		})
	}
}
