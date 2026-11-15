package viz3d

import (
	"context"
	"errors"
	"math"
	"reflect"
	"slices"
	"testing"

	"github.com/shopspring/decimal"

	"bomstudio/internal/bom"
	"bomstudio/internal/catalog"
)

func ptr[T any](v T) *T { return &v }

func dec(s string) decimal.Decimal { return decimal.RequireFromString(s) }

func part(id int64, name, category, zone string, mass *decimal.Decimal) catalog.Part {
	return catalog.Part{ID: id, Name: name, Category: category, Zone: zone, MassKG: mass}
}

func node(id int64, parent *int64, partID *int64, qty string) bom.Node {
	return bom.Node{ID: id, BOMID: 1, ParentID: parent, PartID: partID, Quantity: dec(qty)}
}

// Une petite voiture : racine, deux sous-ensembles, pièces de plusieurs zones, 4 pneus, une pièce sans zone
// connue, une sans masse, une absente du catalogue.
func fixture() ([]bom.Node, map[int64]catalog.Part, map[int64]string) {
	nodes := []bom.Node{
		node(1, nil, nil, "1"),
		{ID: 2, BOMID: 1, ParentID: ptr(int64(1)), Name: "Groupe motopropulseur", Quantity: dec("1")},
		{ID: 3, BOMID: 1, ParentID: ptr(int64(1)), Name: "Liaison au sol", Quantity: dec("1")},
		node(10, ptr(int64(2)), ptr(int64(100)), "1"),  // bloc moteur, 120 kg
		node(11, ptr(int64(2)), ptr(int64(101)), "1"),  // boîte, masse inconnue
		node(12, ptr(int64(3)), ptr(int64(102)), "4"),  // pneu ×4
		node(13, ptr(int64(3)), ptr(int64(103)), "2"),  // jante arrière ×2
		node(14, ptr(int64(3)), ptr(int64(104)), "40"), // vis, zone « Toutes zones »
		node(15, ptr(int64(3)), ptr(int64(105)), "1"),  // pièce absente du catalogue
		node(16, ptr(int64(1)), ptr(int64(106)), "2"),  // portière, carrosserie
		node(17, ptr(int64(3)), ptr(int64(107)), "6"),  // disque de frein ×6 (plus que de coins)
		{ID: 4, BOMID: 1, ParentID: ptr(int64(3)), Name: "Roue avant gauche", Quantity: dec("1")},
		node(18, ptr(int64(4)), ptr(int64(108)), "1"), // jante alu ×1 dans « Roue avant gauche »
		{ID: 5, BOMID: 1, ParentID: ptr(int64(1)), Name: "Porte avant droite", Quantity: dec("1")},
		node(19, ptr(int64(5)), ptr(int64(106)), "1"), // portière dans « Porte avant droite »
		{ID: 6, BOMID: 1, ParentID: ptr(int64(2)), Name: "Point de fixation 001", Quantity: dec("1")},
		node(20, ptr(int64(6)), ptr(int64(104)), "8"), // vis sous un point de fixation du GMP
	}
	parts := map[int64]catalog.Part{
		100: part(100, "Bloc moteur", "Moteur", "Compartiment moteur", ptr(dec("120"))),
		101: part(101, "Boîte de vitesses", "Transmission", "Compartiment moteur", nil),
		102: part(102, "Pneu 205/55", "Roues", "Liaison au sol", ptr(dec("9"))),
		103: part(103, "Jante AR alu", "Roues", "Liaison au sol", ptr(dec("8"))),
		104: part(104, "Vis M8", "Visserie", "Toutes zones", ptr(dec("0.01"))),
		106: part(106, "Portière avant", "Ouvrants", "Carrosserie", ptr(dec("30"))),
		107: part(107, "Disque de frein", "Freinage", "Liaison au sol", ptr(dec("7"))),
		108: part(108, "Jante alu 16\"", "Roues", "Liaison au sol", ptr(dec("8"))),
	}
	return nodes, parts, map[int64]string{105: "pièce absente du catalogue"}
}

func find(t *testing.T, a Assembly, nodeID int64) Placement {
	t.Helper()
	i := slices.IndexFunc(a.Placements, func(p Placement) bool { return p.NodeID == nodeID })
	if i < 0 {
		t.Fatalf("placement du nœud %d absent", nodeID)
	}
	return a.Placements[i]
}

func TestBuild(t *testing.T) {
	nodes, parts, missing := fixture()
	a := Build(1, nodes, parts, missing)

	tests := []struct {
		name  string
		check func(t *testing.T)
	}{
		{"QUAND l'arbre a des pièces ALORS chaque nœud pièce lu a un placement, les sous-ensembles aucun", func(t *testing.T) {
			var ids []int64
			for _, p := range a.Placements {
				ids = append(ids, p.NodeID)
			}
			slices.Sort(ids)
			want := []int64{10, 11, 12, 13, 14, 16, 17, 18, 19, 20}
			if !reflect.DeepEqual(ids, want) {
				t.Fatalf("nœuds placés %v, attendu %v", ids, want)
			}
		}},
		{"SI une pièce est absente du catalogue ALORS elle est signalée dans missing, pas cachée", func(t *testing.T) {
			want := []MissingModel{{NodeID: 15, PartID: 105, Reason: "pièce absente du catalogue"}}
			if !reflect.DeepEqual(a.Missing, want) {
				t.Fatalf("missing = %+v, attendu %+v", a.Missing, want)
			}
		}},
		{"QUAND un pneu est en 4 exemplaires ALORS il a 4 instances aux 4 coins, symétriques", func(t *testing.T) {
			p := find(t, a, 12)
			if p.Shape.Kind != "wheel" || len(p.Instances) != 4 {
				t.Fatalf("pneu : kind %q, %d instances", p.Shape.Kind, len(p.Instances))
			}
			var xs, zs []float64
			for _, in := range p.Instances {
				xs = append(xs, in.Position[0])
				zs = append(zs, in.Position[2])
			}
			if xs[0] != -xs[2] || zs[0] != -zs[1] || xs[0] <= 0 {
				t.Fatalf("coins non symétriques : x %v z %v", xs, zs)
			}
		}},
		{"QUAND une jante « AR » est en 2 exemplaires ALORS elle va aux coins arrière", func(t *testing.T) {
			p := find(t, a, 13)
			if len(p.Instances) != 2 || p.Instances[0].Position[0] >= 0 || p.Instances[1].Position[0] >= 0 {
				t.Fatalf("jante AR : %+v", p.Instances)
			}
		}},
		{"SI une pièce de coin a plus de 4 exemplaires ALORS 4 sont montrés et une note le dit", func(t *testing.T) {
			p := find(t, a, 17)
			if len(p.Instances) != 4 || !slices.Contains(p.Notes, "6 exemplaires, 4 représenté(s) aux roues") {
				t.Fatalf("disques : %d instances, notes %v", len(p.Instances), p.Notes)
			}
			if p.Quantity != "6" {
				t.Fatalf("quantité %q", p.Quantity)
			}
		}},
		{"QUAND la zone d'une pièce est inconnue ALORS elle hérite de la zone majoritaire de son sous-ensemble", func(t *testing.T) {
			screw := find(t, a, 20) // vis « Toutes zones » sous un point de fixation du groupe motopropulseur
			if screw.Zone != "Compartiment moteur" || screw.Confidence != "0.10" {
				t.Fatalf("vis : zone %q confiance %q", screw.Zone, screw.Confidence)
			}
			if !slices.Contains(screw.Notes, "zone « Toutes zones » : placée avec « Groupe motopropulseur » (Compartiment moteur)") {
				t.Fatalf("notes %v", screw.Notes)
			}
			if x := screw.Instances[0].Position[0]; x < 0.7 || x > 2.0 {
				t.Fatalf("vis hors du compartiment moteur : %v", screw.Instances[0].Position)
			}
			if slices.ContainsFunc(a.Zones, func(z ZoneSummary) bool { return z.Zone == "Toutes zones" }) {
				t.Fatalf("la zone du catalogue ne doit pas apparaître comme zone de la scène : %+v", a.Zones)
			}
		}},
		{"QUAND une roue est seule dans « Roue avant gauche » ALORS elle va au coin avant gauche", func(t *testing.T) {
			p := find(t, a, 18)
			if len(p.Instances) != 1 || p.Instances[0].Position[0] <= 0 || p.Instances[0].Position[2] >= 0 {
				t.Fatalf("jante : %+v", p.Instances)
			}
			if !slices.Contains(p.Notes, "côté d'après « Roue avant gauche »") {
				t.Fatalf("notes %v", p.Notes)
			}
		}},
		{"QUAND une portière est dans « Porte avant droite » ALORS elle est à l'avant droit de la carrosserie", func(t *testing.T) {
			p := find(t, a, 19)
			pos := p.Instances[0].Position
			if pos[0] <= 0 || pos[2] <= 0 {
				t.Fatalf("portière avant droite placée en %v", pos)
			}
			left := find(t, a, 16).Instances[0].Position // portière sans côté dit : au centre de la région
			if left[2] != 0 {
				t.Fatalf("portière sans côté : z = %v, attendu 0", left[2])
			}
		}},
		{"SI la masse est inconnue ALORS taille par défaut, note et confiance réduite", func(t *testing.T) {
			p := find(t, a, 11)
			if !slices.Contains(p.Notes, "masse inconnue : taille par défaut") || p.Confidence != "0.15" {
				t.Fatalf("boîte : notes %v confiance %q", p.Notes, p.Confidence)
			}
			if p.Shape.Size[0] != defaultSideM {
				t.Fatalf("taille %v, attendu côté %v", p.Shape.Size, defaultSideM)
			}
		}},
		{"QUAND la masse est connue ALORS la taille en découle, bornée et dans la région", func(t *testing.T) {
			engine := find(t, a, 10)
			gearbox := find(t, a, 11)
			if engine.Shape.Size[0] <= gearbox.Shape.Size[0] {
				t.Fatalf("le bloc moteur (120 kg) devrait être plus grand que la taille par défaut : %v", engine.Shape.Size)
			}
			if engine.Instances[0].Position[0] < 0.7 || engine.Instances[0].Position[0] > 2.0 {
				t.Fatalf("bloc moteur hors du compartiment moteur : %v", engine.Instances[0].Position)
			}
		}},
		{"QUAND un modèle est généré ALORS il n'est jamais présenté comme exact", func(t *testing.T) {
			for _, p := range a.Placements {
				if p.Match != MatchGenerated || !p.Estimated {
					t.Fatalf("nœud %d : match %q estimated %v", p.NodeID, p.Match, p.Estimated)
				}
			}
			if a.Source != SourceGenerated {
				t.Fatalf("source %q", a.Source)
			}
		}},
		{"QUAND l'éclatement est calculé ALORS chaque vecteur est fini, non nul, et une portière s'écarte latéralement", func(t *testing.T) {
			for _, p := range a.Placements {
				n := norm(p.Explode)
				if math.IsNaN(n) || math.IsInf(n, 0) || n < 0.1 {
					t.Fatalf("nœud %d : éclatement %v", p.NodeID, p.Explode)
				}
			}
			door := find(t, a, 16)
			if norm(door.Explode) < explodeDistance*1.5 { // région carrosserie : facteur 1,6
				t.Fatalf("portière : éclatement %v trop court", door.Explode)
			}
		}},
		{"QUAND deux pièces sont dans le même sous-ensemble ALORS leurs éclatements vont dans un sens proche", func(t *testing.T) {
			e, g := find(t, a, 10).Explode, find(t, a, 11).Explode
			if e[0]*g[0]+e[1]*g[1]+e[2]*g[2] <= 0 {
				t.Fatalf("moteur %v et boîte %v partent dans des sens opposés", e, g)
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, tt.check)
	}
}

func TestBuildIsDeterministic(t *testing.T) {
	nodes, parts, missing := fixture()
	first := Build(1, nodes, parts, missing)
	shuffled := slices.Clone(nodes)
	slices.Reverse(shuffled)
	second := Build(1, shuffled, parts, missing)
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("même entrée, ordre différent : scènes différentes\n%+v\n%+v", first, second)
	}
}

func TestBuildUnknownZoneShelf(t *testing.T) {
	nodes := []bom.Node{node(1, nil, nil, "1"), node(2, ptr(int64(1)), ptr(int64(50)), "3")}
	parts := map[int64]catalog.Part{50: part(50, "Kit", "Divers", "Toutes zones", nil)}
	a := Build(1, nodes, parts, nil)
	p := find(t, a, 2)
	if p.Instances[0].Position[0] > -2.5 {
		t.Fatalf("SI aucune zone n'est connue ALORS la pièce est posée à côté du véhicule : %v", p.Instances[0].Position)
	}
	if p.Confidence != "0.05" || p.Zone != "Toutes zones" {
		t.Fatalf("confiance %q zone %q", p.Confidence, p.Zone)
	}
	if len(a.Zones) != 1 || a.Zones[0].Known {
		t.Fatalf("résumé de zone : %+v", a.Zones)
	}
}

func TestBuildEmptyTree(t *testing.T) {
	a := Build(9, []bom.Node{node(1, nil, nil, "1")}, nil, nil)
	if len(a.Placements) != 0 || len(a.Missing) != 0 || len(a.Zones) != 0 {
		t.Fatalf("arbre sans pièce : %+v", a)
	}
	if a.Placements == nil || a.Zones == nil || a.Missing == nil {
		t.Fatal("les listes vides doivent être des tableaux JSON, pas null")
	}
}

func TestShapeFor(t *testing.T) {
	tests := []struct {
		category, name string
		wantKind       string
		fixed, corner  bool
	}{
		{"Roues", "Pneumatique 195/60 R15", "wheel", true, true},
		{"Roues", "Jante tôle 6J×15", "wheel", true, true},
		{"Roues", "Roue de secours galette", "wheel", true, true},
		{"Roues", "Vis de roue M12×1,25", "cylinder", false, true}, // pas une roue entière
		{"Roues", "Enjoliveur 15\"", "cylinder", false, true},
		{"Freinage", "Disque de frein", "cylinder", false, true},
		{"Moteur", "Bloc moteur", "box", false, false},
		{"Visserie", "Vis H M6", "sphere", false, false},
		{"Divers", "Chose", "box", false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shapeFor(tt.category, tt.name)
			if got.kind != tt.wantKind || got.fixed != tt.fixed || got.corner != tt.corner {
				t.Fatalf("shapeFor(%q, %q) = %+v", tt.category, tt.name, got)
			}
		})
	}
}

func TestZoneKey(t *testing.T) {
	tests := []struct{ zone, want string }{
		{"Compartiment moteur", "moteur"}, {"Habitacle", "habitacle"}, {"Carrosserie", "carrosserie"},
		{"Caisse", "caisse"}, {"Soubassement", "soubassement"}, {"Liaison au sol", "liaison"}, {"Châssis", "liaison"},
		{"Toutes zones", ""}, {"", ""}, {"Intérieur", "habitacle"},
	}
	for _, tt := range tests {
		t.Run(tt.zone, func(t *testing.T) {
			if got := zoneKey(tt.zone); got != tt.want {
				t.Fatalf("zoneKey(%q) = %q, attendu %q", tt.zone, got, tt.want)
			}
		})
	}
}

// --- Service ------------------------------------------------------------------------------------------

type fakeTrees struct {
	nodes []bom.Node
	err   error
}

func (f fakeTrees) Tree(context.Context, int64) (bom.BOM, []bom.Node, error) {
	return bom.BOM{ID: 1}, f.nodes, f.err
}

type fakeParts struct {
	parts map[int64]catalog.Part
	err   error
}

func (f fakeParts) Part(_ context.Context, id int64) (catalog.Part, error) {
	if f.err != nil {
		return catalog.Part{}, f.err
	}
	p, ok := f.parts[id]
	if !ok {
		return catalog.Part{}, catalog.ErrPartNotFound
	}
	return p, nil
}

func TestServiceAssembly(t *testing.T) {
	nodes, parts, _ := fixture()
	dbDown := errors.New("db down")
	tests := []struct {
		name    string
		trees   fakeTrees
		parts   fakeParts
		wantErr error
		check   func(t *testing.T, a Assembly)
	}{
		{"SI la BOM n'existe pas ALORS ErrBOMNotFound", fakeTrees{err: bom.ErrBOMNotFound}, fakeParts{}, bom.ErrBOMNotFound, nil},
		{"SI le catalogue est en panne ALORS l'erreur remonte, pas une scène partielle", fakeTrees{nodes: nodes}, fakeParts{err: dbDown}, dbDown, nil},
		{"QUAND une pièce manque au catalogue ALORS la scène est construite et la pièce signalée", fakeTrees{nodes: nodes}, fakeParts{parts: parts}, nil,
			func(t *testing.T, a Assembly) {
				if len(a.Placements) != 10 || len(a.Missing) != 1 || a.Missing[0].NodeID != 15 {
					t.Fatalf("placements %d, missing %+v", len(a.Placements), a.Missing)
				}
			}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a, err := NewService(tt.trees, tt.parts).Assembly(context.Background(), 1)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, attendu %v", err, tt.wantErr)
			}
			if tt.check != nil {
				tt.check(t, a)
			}
		})
	}
}
