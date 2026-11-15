package viz3d

import (
	"cmp"
	"fmt"
	"math"
	"slices"
	"strings"

	"bomstudio/internal/bom"
	"bomstudio/internal/catalog"
)

// Gabarit d'une berline : 4,4 m de long (x, avant vers +x), 1,5 m de haut (y, sol à 0), 1,8 m de large
// (z, droite vers +z).
var (
	defaultVehicle = Vehicle{Length: 4.4, Height: 1.5, Width: 1.8}
	vehicleCenter  = Vec3{0, 0.6, 0}
	// Quatre coins de roue : avant gauche, avant droit, arrière gauche, arrière droit.
	wheelCorners = [4]Vec3{{1.45, 0.33, -0.95}, {1.45, 0.33, 0.95}, {-1.45, 0.33, -0.95}, {-1.45, 0.33, 0.95}}
)

// box est un pavé de la scène : centre et dimensions complètes.
type box struct{ center, extent Vec3 }

// region est la boîte du gabarit où les pièces d'une zone sont réparties ; explode module l'éclatement.
type region struct {
	box
	explode float64
}

// zoneKeys : ordre fixe des régions (jamais un parcours de map), la dernière accueille les zones inconnues.
var zoneKeys = []string{"moteur", "habitacle", "carrosserie", "caisse", "soubassement", "liaison", ""}

var regions = map[string]region{
	"moteur":       {box{Vec3{1.35, 0.75, 0}, Vec3{1.3, 0.8, 1.4}}, 1.0},
	"habitacle":    {box{Vec3{-0.3, 0.95, 0}, Vec3{1.9, 0.9, 1.4}}, 1.0},
	"carrosserie":  {box{Vec3{0, 1.05, 0}, Vec3{4.2, 1.0, 1.9}}, 1.6},
	"caisse":       {box{Vec3{0, 0.55, 0}, Vec3{3.8, 0.45, 1.6}}, 0.8},
	"soubassement": {box{Vec3{0, 0.22, 0}, Vec3{3.8, 0.25, 1.6}}, 1.0},
	"liaison":      {box{Vec3{0, 0.38, 0}, Vec3{3.9, 0.5, 1.9}}, 1.2},
	// Étagère à côté du véhicule : une pièce sans zone connue n'est pas cachée, elle est posée à part.
	"": {box{Vec3{-3.6, 0.5, 0}, Vec3{1.2, 1.0, 2.2}}, 0.6},
}

// zoneKey rattache une zone du catalogue à une région du gabarit ; "" si aucune ne correspond.
func zoneKey(zone string) string {
	z := normalize(zone)
	switch {
	case strings.Contains(z, "moteur"):
		return "moteur"
	case strings.Contains(z, "habitacle"), strings.Contains(z, "interieur"):
		return "habitacle"
	case strings.Contains(z, "carrosserie"):
		return "carrosserie"
	case strings.Contains(z, "caisse"):
		return "caisse"
	case strings.Contains(z, "soubassement"):
		return "soubassement"
	case strings.Contains(z, "liaison"), strings.Contains(z, "chassis"), strings.Contains(z, "train"):
		return "liaison"
	}
	return ""
}

var accents = strings.NewReplacer("é", "e", "è", "e", "ê", "e", "ë", "e", "à", "a", "â", "a", "î", "i", "ï", "i",
	"ô", "o", "ö", "o", "û", "u", "ù", "u", "ü", "u", "ç", "c", "É", "e", "È", "e", "Ê", "e", "À", "a", "Â", "a",
	"Î", "i", "Ô", "o", "Û", "u", "Ç", "c")

func normalize(s string) string { return strings.ToLower(accents.Replace(s)) }

// tokens découpe un libellé en mots minuscules sans accent.
func tokens(s string) []string {
	return strings.FieldsFunc(normalize(s), func(r rune) bool { return (r < 'a' || r > 'z') && (r < '0' || r > '9') })
}

// hint : côté déduit des libellés (sous-ensembles ancêtres et nom de la pièce) ; x : +1 avant, −1 arrière ;
// z : −1 gauche, +1 droite ; 0 : rien de dit. from : le libellé qui l'a donné.
type hint struct {
	x, z int
	from string
}

func hintOf(label string) hint {
	var h hint
	for _, t := range tokens(label) {
		switch t {
		case "avant", "av":
			h.x = 1
		case "arriere", "ar":
			h.x = -1
		case "gauche", "g":
			h.z = -1
		case "droite", "droit", "d":
			h.z = 1
		}
	}
	if h.x != 0 || h.z != 0 {
		h.from = label
	}
	return h
}

// shapeSpec : primitive et proportions (composante max = 1) d'une catégorie ; fixed = taille indépendante
// de la masse ; corner = pièce montée aux roues.
type shapeSpec struct {
	kind         string
	aspect       Vec3
	fixed        bool
	rotation     Vec3
	corner       bool
	cornerOffset Vec3 // décalage depuis le coin : y vers le haut, z vers l'intérieur du véhicule
}

const halfPi = math.Pi / 2

// shapeFor choisit la primitive d'une pièce depuis sa catégorie puis son nom (le pneu et la jante sont
// dans la même catégorie « Roues »).
func shapeFor(category, name string) shapeSpec {
	c, n := normalize(category), normalize(name)
	switch {
	case strings.Contains(n, "pneu"):
		return shapeSpec{kind: "wheel", aspect: Vec3{0.65, 0.22, 0.65}, fixed: true, rotation: Vec3{halfPi, 0, 0}, corner: true}
	case strings.Contains(n, "jante"):
		return shapeSpec{kind: "wheel", aspect: Vec3{0.45, 0.2, 0.45}, fixed: true, rotation: Vec3{halfPi, 0, 0}, corner: true}
	case strings.HasPrefix(n, "roue"):
		return shapeSpec{kind: "wheel", aspect: Vec3{0.65, 0.22, 0.65}, fixed: true, rotation: Vec3{halfPi, 0, 0}, corner: true}
	case strings.Contains(c, "roue"): // valve, vis de roue, enjoliveur… : petite pièce montée côté extérieur de la roue
		return shapeSpec{kind: "cylinder", aspect: Vec3{1, 0.3, 1}, rotation: Vec3{halfPi, 0, 0}, corner: true,
			cornerOffset: Vec3{0, 0, -0.14}}
	case strings.Contains(c, "frein"):
		return shapeSpec{kind: "cylinder", aspect: Vec3{1, 0.12, 1}, rotation: Vec3{halfPi, 0, 0}, corner: true,
			cornerOffset: Vec3{0, 0, 0.16}}
	case strings.Contains(c, "suspension"), strings.Contains(c, "amortisseur"):
		return shapeSpec{kind: "cylinder", aspect: Vec3{0.25, 1, 0.25}, corner: true, cornerOffset: Vec3{0, 0.35, 0.25}}
	case strings.Contains(c, "moteur"), strings.Contains(c, "transmission"), strings.Contains(c, "hybride"):
		return shapeSpec{kind: "box", aspect: Vec3{1, 0.8, 0.7}}
	case strings.Contains(c, "vitrage"):
		return shapeSpec{kind: "box", aspect: Vec3{1, 0.6, 0.04}}
	case strings.Contains(c, "ouvrant"), strings.Contains(c, "aerodynamique"), strings.Contains(c, "composite"):
		return shapeSpec{kind: "box", aspect: Vec3{1, 0.7, 0.06}}
	case strings.Contains(c, "echappement"), strings.Contains(c, "cablage"), strings.Contains(c, "direction"):
		return shapeSpec{kind: "cylinder", aspect: Vec3{0.08, 1, 0.08}, rotation: Vec3{0, 0, halfPi}}
	case strings.Contains(c, "visserie"), strings.Contains(c, "fluide"):
		return shapeSpec{kind: "sphere", aspect: Vec3{1, 1, 1}}
	}
	return shapeSpec{kind: "box", aspect: Vec3{1, 0.7, 0.7}}
}

// Taille d'une pièce : côté = racine cubique de masse ÷ 1 500 kg/m³ (acier et plastique mêlés), borné.
const (
	densityKGPerM3  = 1500.0
	minSideM        = 0.06
	maxSideM        = 0.9
	defaultSideM    = 0.15
	explodeDistance = 1.2 // mètres à éclatement complet, avant le facteur de la région
	baseConfidence  = 0.20
)

// sideFromMass renvoie le côté caractéristique et vrai si la masse est connue.
func sideFromMass(p catalog.Part) (float64, bool) {
	if p.MassKG == nil || !p.MassKG.IsPositive() {
		return defaultSideM, false
	}
	m, _ := p.MassKG.Float64() // longueur de scène, pas un montant : le float est acceptable ici
	return math.Min(maxSideM, math.Max(minSideM, math.Cbrt(m/densityKGPerM3)*1.3)), true
}

// item : un nœud pièce à placer, avec ce que l'arbre et le catalogue en disent.
type item struct {
	node  bom.Node
	part  catalog.Part
	spec  shapeSpec
	side  float64
	zone  string // zone retenue : celle du catalogue, ou héritée du sous-ensemble
	key   string // région de cette zone
	group int64  // sous-ensemble de regroupement (ensemble de deuxième niveau) : grille et éclatement communs
	hint  hint
	// corners : coins de roue occupés (vide : pièce placée sur la grille).
	corners []int
	notes   []string
	conf    float64
}

// tree : index de l'arbre, calculé une fois par scène.
type tree struct {
	byID     map[int64]bom.Node
	parent   map[int64]*int64
	depth    map[int64]int
	children map[int64][]int64
	// zoneCounts : par sous-ensemble, nombre de pièces descendantes par zone connue.
	zoneCounts map[int64]map[string]int
}

func indexTree(nodes []bom.Node, parts map[int64]catalog.Part) tree {
	t := tree{byID: map[int64]bom.Node{}, parent: map[int64]*int64{}, depth: map[int64]int{},
		children: map[int64][]int64{}, zoneCounts: map[int64]map[string]int{}}
	for _, n := range nodes {
		t.byID[n.ID] = n
		t.parent[n.ID] = n.ParentID
		if n.ParentID != nil {
			t.children[*n.ParentID] = append(t.children[*n.ParentID], n.ID)
		}
	}
	for _, n := range nodes { // nodes est trié par id : un parent a toujours un id inférieur ? Non garanti → calcul explicite.
		d := 0
		for p := n.ParentID; p != nil && d < len(nodes); p = t.parent[*p] {
			d++
		}
		t.depth[n.ID] = d
		if n.PartID == nil {
			continue
		}
		if p, ok := parts[*n.PartID]; ok && zoneKey(p.Zone) != "" {
			for anc := n.ParentID; anc != nil; anc = t.parent[*anc] {
				if t.zoneCounts[*anc] == nil {
					t.zoneCounts[*anc] = map[string]int{}
				}
				t.zoneCounts[*anc][p.Zone]++
			}
		}
	}
	return t
}

// label d'un nœud : son nom (sous-ensemble) ou celui de sa pièce.
func (t tree) label(id int64, parts map[int64]catalog.Part) string {
	n := t.byID[id]
	if n.PartID != nil {
		return parts[*n.PartID].Name
	}
	return n.Name
}

// inheritedZone : zone majoritaire des pièces du plus proche ancêtre qui en connaît une (égalité : ordre
// alphabétique) et le nom de cet ancêtre ; "" si aucun.
func (t tree) inheritedZone(id int64) (zone, from string) {
	for anc := t.parent[id]; anc != nil; anc = t.parent[*anc] {
		counts := t.zoneCounts[*anc]
		if len(counts) == 0 {
			continue
		}
		zones := make([]string, 0, len(counts))
		for z := range counts {
			zones = append(zones, z)
		}
		slices.SortFunc(zones, func(a, b string) int {
			if c := cmp.Compare(counts[b], counts[a]); c != 0 {
				return c
			}
			return strings.Compare(a, b)
		})
		return zones[0], t.byID[*anc].Name
	}
	return "", ""
}

// hintFor : côté donné par le libellé le plus proche (la pièce, puis ses ancêtres en remontant).
func (t tree) hintFor(id int64, parts map[int64]catalog.Part) hint {
	var h hint
	for cur := &id; cur != nil; cur = t.parent[*cur] {
		got := hintOf(t.label(*cur, parts))
		if h.x == 0 && got.x != 0 {
			h.x, h.from = got.x, got.from
		}
		if h.z == 0 && got.z != 0 {
			h.z, h.from = got.z, got.from
		}
		if h.x != 0 && h.z != 0 {
			break
		}
	}
	return h
}

// groupOf : l'ancêtre de profondeur 2 (ensemble sous un système de premier niveau), ou le plus proche
// ancêtre si la pièce est plus haut. Ses pièces partagent une case de la grille et un sens d'éclatement.
func (t tree) groupOf(id int64) int64 {
	want := min(2, t.depth[id]-1)
	cur := id
	for t.depth[cur] > want {
		cur = *t.parent[cur]
	}
	return cur
}

// Build construit la scène : fonction pure, sans horloge ni aléa. parts : pièces lues ; missing : pièces
// illisibles et la raison. Les nœuds sont traités par id croissant, quel que soit l'ordre reçu.
func Build(bomID int64, nodes []bom.Node, parts map[int64]catalog.Part, missing map[int64]string) Assembly {
	out := Assembly{BOMID: bomID, Source: SourceGenerated, Vehicle: defaultVehicle,
		Zones: []ZoneSummary{}, Placements: []Placement{}, Missing: []MissingModel{}}
	sorted := slices.Clone(nodes)
	slices.SortFunc(sorted, func(a, b bom.Node) int { return cmp.Compare(a.ID, b.ID) })
	t := indexTree(sorted, parts)

	byKey := make(map[string][]item)
	for _, n := range sorted {
		if n.PartID == nil {
			continue
		}
		p, ok := parts[*n.PartID]
		if !ok {
			reason := missing[*n.PartID]
			if reason == "" {
				reason = "pièce non lue au catalogue"
			}
			out.Missing = append(out.Missing, MissingModel{NodeID: n.ID, PartID: *n.PartID, Reason: reason})
			continue
		}
		it := item{node: n, part: p, spec: shapeFor(p.Category, p.Name), conf: baseConfidence,
			zone: p.Zone, key: zoneKey(p.Zone), group: t.groupOf(n.ID), hint: t.hintFor(n.ID, parts)}
		var known bool
		it.side, known = sideFromMass(p)
		if !known {
			it.notes = append(it.notes, "masse inconnue : taille par défaut")
			it.conf -= 0.05
		}
		if it.key == "" {
			if z, from := t.inheritedZone(n.ID); z != "" {
				it.zone, it.key = z, zoneKey(z)
				it.notes = append(it.notes, fmt.Sprintf("zone « %s » : placée avec « %s » (%s)", p.Zone, from, z))
				it.conf = 0.10
			} else {
				it.notes = append(it.notes, fmt.Sprintf("zone « %s » inconnue du gabarit : pièce posée à côté du véhicule", p.Zone))
				it.conf = 0.05
			}
		}
		if it.hint.from != "" {
			it.notes = append(it.notes, fmt.Sprintf("côté d'après « %s »", it.hint.from))
		}
		if it.spec.corner && it.key == "liaison" {
			it.corners = cornerSlots(it)
		}
		byKey[it.key] = append(byKey[it.key], it)
	}

	for _, key := range zoneKeys {
		if items := byKey[key]; len(items) > 0 {
			out.Placements = append(out.Placements, placeRegion(regions[key], items, t, parts)...)
		}
	}
	addExplosion(out.Placements, t, regionFactor(out.Placements))
	out.Zones = summarizeZones(out.Placements)
	return out
}

// cornerSlots : coins de roue d'une pièce. Côté connu (avant/arrière et gauche/droite) → ce coin ; un seul
// axe connu et au moins deux exemplaires → les deux coins de ce côté ; rien de dit → autant de coins que
// d'exemplaires (4 au plus) ; une roue seule sans côté (roue de secours) reste sur la grille.
func cornerSlots(it item) []int {
	q := it.node.Quantity.Ceil().IntPart()
	h := it.hint
	switch {
	case h.x != 0 && h.z != 0:
		return []int{cornerIndex(h.x, h.z)}
	case h.x != 0 && q >= 2:
		return []int{cornerIndex(h.x, -1), cornerIndex(h.x, 1)}
	case h.z != 0 && q >= 2:
		return []int{cornerIndex(1, h.z), cornerIndex(-1, h.z)}
	case h.x == 0 && h.z == 0 && q >= 2:
		slots := make([]int, 0, 4)
		for i := range int(min(q, 4)) {
			slots = append(slots, i)
		}
		return slots
	}
	return nil
}

// cornerIndex : 0 avant gauche, 1 avant droit, 2 arrière gauche, 3 arrière droit.
func cornerIndex(x, z int) int {
	i := 0
	if x < 0 {
		i += 2
	}
	if z > 0 {
		i++
	}
	return i
}

// placeRegion répartit les pièces d'une région. Les pièces de coin vont aux roues. Les autres sont
// groupées par côté (moitié avant/arrière, gauche/droite de la région quand un libellé le dit), puis par
// sous-ensemble sur une grille, puis par pièce dans la case du sous-ensemble.
func placeRegion(r region, items []item, t tree, parts map[int64]catalog.Part) []Placement {
	out := make([]Placement, 0, len(items))
	type sideKey struct{ x, z int }
	sides := make(map[sideKey]map[int64][]item)
	var sideOrder []sideKey
	for _, it := range items {
		if len(it.corners) > 0 {
			inst := make([]Instance, 0, len(it.corners))
			for _, c := range it.corners {
				pos := wheelCorners[c]
				pos[1] += it.spec.cornerOffset[1]
				pos[2] -= math.Copysign(it.spec.cornerOffset[2], pos[2]) // vers l'intérieur
				inst = append(inst, Instance{Position: pos, Rotation: it.spec.rotation})
			}
			if q := it.node.Quantity.Ceil().IntPart(); q > int64(len(it.corners)) {
				it.notes = append(it.notes, fmt.Sprintf("%s exemplaires, %d représenté(s) aux roues", it.node.Quantity, len(it.corners)))
			}
			out = append(out, placement(it, shapeSize(it), inst))
			continue
		}
		k := sideKey{it.hint.x, it.hint.z}
		if sides[k] == nil {
			sides[k] = map[int64][]item{}
			sideOrder = append(sideOrder, k)
		}
		sides[k][it.group] = append(sides[k][it.group], it)
	}
	slices.SortFunc(sideOrder, func(a, b sideKey) int { return cmp.Or(cmp.Compare(a.x, b.x), cmp.Compare(a.z, b.z)) })
	for _, k := range sideOrder {
		sub := halfBox(r.box, k.x, k.z)
		groups := sides[k]
		ids := make([]int64, 0, len(groups))
		for id := range groups {
			ids = append(ids, id)
		}
		slices.SortFunc(ids, func(a, b int64) int {
			return cmp.Or(strings.Compare(normalize(t.label(a, parts)), normalize(t.label(b, parts))), cmp.Compare(a, b))
		})
		cells := gridCells(sub, len(ids))
		for gi, id := range ids {
			members := groups[id]
			slices.SortStableFunc(members, func(a, b item) int {
				return cmp.Or(strings.Compare(normalize(a.part.Category), normalize(b.part.Category)), cmp.Compare(a.node.ID, b.node.ID))
			})
			slots := gridCells(cells[gi], len(members))
			for mi, it := range members {
				out = append(out, placement(it, fitCell(it, slots[mi].extent),
					[]Instance{{Position: slots[mi].center, Rotation: it.spec.rotation}}))
			}
		}
	}
	return out
}

// halfBox restreint une boîte à sa moitié avant/arrière (x) et gauche/droite (z) selon les indices non nuls.
func halfBox(b box, x, z int) box {
	if x != 0 {
		b.center[0] += float64(x) * b.extent[0] / 4
		b.extent[0] /= 2
	}
	if z != 0 {
		b.center[2] += float64(z) * b.extent[2] / 4
		b.extent[2] /= 2
	}
	return b
}

// gridCells découpe une boîte en n cases (x, z), proportionnées à la boîte, au niveau de son centre.
func gridCells(b box, n int) []box {
	if n <= 0 {
		return nil
	}
	cols := max(1, int(math.Ceil(math.Sqrt(float64(n)*b.extent[0]/b.extent[2]))))
	rows := max(1, int(math.Ceil(float64(n)/float64(cols))))
	cell := Vec3{b.extent[0] / float64(cols), b.extent[1], b.extent[2] / float64(rows)}
	cells := make([]box, 0, n)
	for i := range n {
		col, row := i%cols, i/cols
		cells = append(cells, box{center: Vec3{
			b.center[0] - b.extent[0]/2 + (float64(col)+0.5)*cell[0],
			b.center[1],
			b.center[2] - b.extent[2]/2 + (float64(row)+0.5)*cell[2],
		}, extent: cell})
	}
	return cells
}

// shapeSize : encombrement de la primitive = proportions × côté (fixe pour une roue).
func shapeSize(it item) Vec3 {
	if it.spec.fixed {
		return it.spec.aspect
	}
	return Vec3{it.spec.aspect[0] * it.side, it.spec.aspect[1] * it.side, it.spec.aspect[2] * it.side}
}

// fitCell réduit uniformément une primitive qui déborderait de sa case (jamais agrandie), sans descendre
// sous un centimètre : une case minuscule garde un point visible.
func fitCell(it item, cell Vec3) Vec3 {
	s := shapeSize(it)
	k := 1.0
	for i := range 3 {
		if s[i] > cell[i] && cell[i] > 0 {
			k = math.Min(k, cell[i]/s[i])
		}
	}
	if s[0]*k < 0.01 {
		k = 0.01 / s[0]
	}
	return Vec3{s[0] * k, s[1] * k, s[2] * k}
}

func placement(it item, size Vec3, inst []Instance) Placement {
	notes := it.notes
	if notes == nil {
		notes = []string{}
	}
	return Placement{
		NodeID: it.node.ID, ParentID: it.node.ParentID, PartID: it.part.ID, Zone: it.zone, Category: it.part.Category,
		Quantity: it.node.Quantity.String(), Match: MatchGenerated, Confidence: fmt.Sprintf("%.2f", it.conf),
		Estimated: true, Notes: notes, Shape: Shape{Kind: it.spec.kind, Size: size}, Instances: inst,
	}
}

// regionFactor : facteur d'éclatement de la région de chaque placement (par zone retenue).
func regionFactor(placements []Placement) func(nodeID int64) float64 {
	f := make(map[int64]float64, len(placements))
	for _, p := range placements {
		f[p.NodeID] = regions[zoneKey(p.Zone)].explode
	}
	return func(nodeID int64) float64 { return f[nodeID] }
}

// addExplosion calcule le vecteur d'éclatement : moitié direction du groupe (sous-ensemble de regroupement,
// pour que ses pièces partent ensemble), moitié direction propre depuis le centre du véhicule.
func addExplosion(placements []Placement, t tree, factor func(nodeID int64) float64) {
	sum := make(map[int64]Vec3)
	count := make(map[int64]int)
	for _, p := range placements {
		g := t.groupOf(p.NodeID)
		sum[g] = add(sum[g], p.Instances[0].Position)
		count[g]++
	}
	for i := range placements {
		p := &placements[i]
		g := t.groupOf(p.NodeID)
		centroid := scale(sum[g], 1/float64(count[g]))
		dir := add(scale(unit(sub(centroid, vehicleCenter)), 0.5), scale(unit(sub(p.Instances[0].Position, vehicleCenter)), 0.5))
		if norm(dir) < 1e-9 {
			dir = Vec3{0, 1, 0}
		}
		p.Explode = scale(unit(dir), explodeDistance*factor(p.NodeID))
	}
}

func summarizeZones(placements []Placement) []ZoneSummary {
	counts := make(map[string]int)
	for _, p := range placements {
		counts[p.Zone]++
	}
	zones := make([]ZoneSummary, 0, len(counts))
	for z, c := range counts {
		zones = append(zones, ZoneSummary{Zone: z, Count: c, Known: zoneKey(z) != ""})
	}
	slices.SortFunc(zones, func(a, b ZoneSummary) int { return strings.Compare(a.Zone, b.Zone) })
	return zones
}

func add(a, b Vec3) Vec3           { return Vec3{a[0] + b[0], a[1] + b[1], a[2] + b[2]} }
func sub(a, b Vec3) Vec3           { return Vec3{a[0] - b[0], a[1] - b[1], a[2] - b[2]} }
func scale(a Vec3, k float64) Vec3 { return Vec3{a[0] * k, a[1] * k, a[2] * k} }
func norm(a Vec3) float64          { return math.Sqrt(a[0]*a[0] + a[1]*a[1] + a[2]*a[2]) }

// unit normalise un vecteur ; le vecteur nul reste nul.
func unit(a Vec3) Vec3 {
	n := norm(a)
	if n < 1e-9 {
		return Vec3{}
	}
	return scale(a, 1/n)
}
