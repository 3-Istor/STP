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

// Gabarit d'une berline : 4,4 m de long (x), 1,5 m de haut (y, sol à 0), 1,8 m de large (z).
var (
	defaultVehicle = Vehicle{Length: 4.4, Height: 1.5, Width: 1.8}
	vehicleCenter  = Vec3{0, 0.6, 0}
	// Quatre coins de roue : avant gauche, avant droit, arrière gauche, arrière droit.
	wheelCorners = [4]Vec3{{1.45, 0.33, -0.95}, {1.45, 0.33, 0.95}, {-1.45, 0.33, -0.95}, {-1.45, 0.33, 0.95}}
)

// region est la boîte du gabarit où les pièces d'une zone sont réparties ; explode module l'éclatement.
type region struct {
	center, extent Vec3
	explode        float64
}

// zoneKeys : ordre fixe des régions (jamais un parcours de map), la dernière accueille les zones inconnues.
var zoneKeys = []string{"moteur", "habitacle", "carrosserie", "caisse", "soubassement", "liaison", ""}

var regions = map[string]region{
	"moteur":       {center: Vec3{1.35, 0.75, 0}, extent: Vec3{1.3, 0.8, 1.4}, explode: 1.0},
	"habitacle":    {center: Vec3{-0.3, 0.95, 0}, extent: Vec3{1.9, 0.9, 1.4}, explode: 1.0},
	"carrosserie":  {center: Vec3{0, 1.05, 0}, extent: Vec3{4.2, 1.0, 1.9}, explode: 1.6},
	"caisse":       {center: Vec3{0, 0.55, 0}, extent: Vec3{3.8, 0.45, 1.6}, explode: 0.8},
	"soubassement": {center: Vec3{0, 0.22, 0}, extent: Vec3{3.8, 0.25, 1.6}, explode: 1.0},
	"liaison":      {center: Vec3{0, 0.38, 0}, extent: Vec3{3.9, 0.5, 1.9}, explode: 1.2},
	// Étagère à côté du véhicule : une pièce sans zone connue n'est pas cachée, elle est posée à part.
	"": {center: Vec3{-3.6, 0.5, 0}, extent: Vec3{1.2, 1.0, 2.2}, explode: 0.6},
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

// shapeSpec : primitive et proportions (composante max = 1) d'une catégorie ; fixed = taille indépendante
// de la masse ; corner = pièce montée aux roues quand elle est en plusieurs exemplaires.
type shapeSpec struct {
	kind         string
	aspect       Vec3
	fixed        bool
	rotation     Vec3
	corner       bool
	cornerOffset Vec3 // décalage depuis le coin, z vers l'intérieur du véhicule (signe appliqué au placement)
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
	case strings.Contains(c, "roue"):
		return shapeSpec{kind: "wheel", aspect: Vec3{0.65, 0.22, 0.65}, fixed: true, rotation: Vec3{halfPi, 0, 0}, corner: true}
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

// item : un nœud pièce à placer, avec ce que le catalogue en dit.
type item struct {
	node bom.Node
	part catalog.Part
	spec shapeSpec
	side float64
	// count : exemplaires à représenter aux coins (0 : pas une pièce de coin ou un seul exemplaire).
	count int
	notes []string
	conf  float64
}

// Build construit la scène : fonction pure, sans horloge ni aléa. parts : pièces lues ; missing : pièces
// illisibles et la raison. Les nœuds sont traités par id croissant, quel que soit l'ordre reçu.
func Build(bomID int64, nodes []bom.Node, parts map[int64]catalog.Part, missing map[int64]string) Assembly {
	out := Assembly{BOMID: bomID, Source: SourceGenerated, Vehicle: defaultVehicle,
		Zones: []ZoneSummary{}, Placements: []Placement{}, Missing: []MissingModel{}}
	sorted := slices.Clone(nodes)
	slices.SortFunc(sorted, func(a, b bom.Node) int { return cmp.Compare(a.ID, b.ID) })

	byZone := make(map[string][]item)
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
		it := item{node: n, part: p, spec: shapeFor(p.Category, p.Name), conf: baseConfidence}
		var known bool
		it.side, known = sideFromMass(p)
		if !known {
			it.notes = append(it.notes, "masse inconnue : taille par défaut")
			it.conf -= 0.05
		}
		key := zoneKey(p.Zone)
		if key == "" {
			it.notes = append(it.notes, fmt.Sprintf("zone « %s » inconnue du gabarit : pièce posée à côté du véhicule", p.Zone))
			it.conf = 0.05
		}
		if it.spec.corner && key == "liaison" {
			q := n.Quantity.Ceil().IntPart()
			if q >= 2 {
				it.count = int(min(q, 4))
				if q > 4 {
					it.notes = append(it.notes, fmt.Sprintf("%s exemplaires, 4 représentés aux roues", n.Quantity))
				}
			}
		}
		byZone[key] = append(byZone[key], it)
	}

	for _, key := range zoneKeys {
		items := byZone[key]
		if len(items) == 0 {
			continue
		}
		out.Placements = append(out.Placements, placeRegion(regions[key], items)...)
	}
	addExplosion(out.Placements, sorted, regionOf(parts))
	out.Zones = summarizeZones(out.Placements)
	return out
}

// regionOf renvoie, par id de pièce, le facteur d'éclatement de sa région.
func regionOf(parts map[int64]catalog.Part) func(partID int64) float64 {
	return func(partID int64) float64 { return regions[zoneKey(parts[partID].Zone)].explode }
}

// placeRegion répartit les pièces d'une région sur une grille (x, z) au niveau du centre, dans l'ordre
// catégorie puis id ; les pièces de coin en plusieurs exemplaires vont aux roues.
func placeRegion(r region, items []item) []Placement {
	var grid, corners []item
	for _, it := range items {
		if it.count > 0 {
			corners = append(corners, it)
		} else {
			grid = append(grid, it)
		}
	}
	slices.SortStableFunc(grid, func(a, b item) int {
		if c := strings.Compare(normalize(a.part.Category), normalize(b.part.Category)); c != 0 {
			return c
		}
		return cmp.Compare(a.node.ID, b.node.ID)
	})
	n := len(grid)
	cols := max(1, int(math.Ceil(math.Sqrt(float64(n)*r.extent[0]/r.extent[2]))))
	rows := max(1, int(math.Ceil(float64(n)/float64(cols))))
	cell := Vec3{r.extent[0] / float64(cols), r.extent[1], r.extent[2] / float64(rows)}
	out := make([]Placement, 0, len(items))
	for i, it := range grid {
		col, row := i%cols, i/cols
		pos := Vec3{
			r.center[0] - r.extent[0]/2 + (float64(col)+0.5)*cell[0],
			r.center[1],
			r.center[2] - r.extent[2]/2 + (float64(row)+0.5)*cell[2],
		}
		out = append(out, placement(it, fitCell(it, cell), []Instance{{Position: pos, Rotation: it.spec.rotation}}))
	}
	for _, it := range corners {
		inst := make([]Instance, 0, it.count)
		for _, c := range cornerSlots(it) {
			pos := wheelCorners[c]
			pos[1] += it.spec.cornerOffset[1]
			pos[2] -= math.Copysign(it.spec.cornerOffset[2], pos[2]) // vers l'intérieur
			inst = append(inst, Instance{Position: pos, Rotation: it.spec.rotation})
		}
		out = append(out, placement(it, shapeSize(it), inst))
	}
	return out
}

// cornerSlots : indices de coins d'une pièce de roue. Deux exemplaires nommés « arrière » vont à l'arrière,
// « avant » à l'avant ; sinon les premiers coins.
func cornerSlots(it item) []int {
	if it.count == 2 {
		if hasToken(it.part.Name, "ar", "arriere") {
			return []int{2, 3}
		}
		return []int{0, 1}
	}
	slots := make([]int, 0, it.count)
	for i := range it.count {
		slots = append(slots, i)
	}
	return slots
}

func hasToken(name string, tokens ...string) bool {
	fields := strings.FieldsFunc(normalize(name), func(r rune) bool { return r < 'a' || r > 'z' })
	for _, f := range fields {
		if slices.Contains(tokens, f) {
			return true
		}
	}
	return false
}

// shapeSize : encombrement de la primitive = proportions × côté (fixe pour une roue).
func shapeSize(it item) Vec3 {
	if it.spec.fixed {
		return it.spec.aspect
	}
	return Vec3{it.spec.aspect[0] * it.side, it.spec.aspect[1] * it.side, it.spec.aspect[2] * it.side}
}

// fitCell réduit uniformément une primitive qui déborderait de sa case (jamais agrandie).
func fitCell(it item, cell Vec3) Vec3 {
	s := shapeSize(it)
	k := 1.0
	for i := range 3 {
		if s[i] > cell[i] && cell[i] > 0 {
			k = math.Min(k, cell[i]/s[i])
		}
	}
	return Vec3{s[0] * k, s[1] * k, s[2] * k}
}

func placement(it item, size Vec3, inst []Instance) Placement {
	notes := it.notes
	if notes == nil {
		notes = []string{}
	}
	return Placement{
		NodeID: it.node.ID, ParentID: it.node.ParentID, PartID: it.part.ID, Zone: it.part.Zone, Category: it.part.Category,
		Quantity: it.node.Quantity.String(), Match: MatchGenerated, Confidence: fmt.Sprintf("%.2f", it.conf),
		Estimated: true, Notes: notes, Shape: Shape{Kind: it.spec.kind, Size: size}, Instances: inst,
	}
}

// addExplosion calcule le vecteur d'éclatement : moitié direction du groupe (sous-ensemble de premier
// niveau, pour que ses pièces partent ensemble), moitié direction propre depuis le centre du véhicule.
func addExplosion(placements []Placement, nodes []bom.Node, factor func(partID int64) float64) {
	parent := make(map[int64]*int64, len(nodes))
	for _, n := range nodes {
		parent[n.ID] = n.ParentID
	}
	top := func(id int64) int64 { // ancêtre juste sous la racine (ou le nœud lui-même)
		for p := parent[id]; p != nil && parent[*p] != nil; p = parent[id] {
			id = *p
		}
		return id
	}
	sum := make(map[int64]Vec3)
	count := make(map[int64]int)
	for _, p := range placements {
		g := top(p.NodeID)
		sum[g] = add(sum[g], p.Instances[0].Position)
		count[g]++
	}
	for i := range placements {
		p := &placements[i]
		g := top(p.NodeID)
		centroid := scale(sum[g], 1/float64(count[g]))
		dir := add(scale(unit(sub(centroid, vehicleCenter)), 0.5), scale(unit(sub(p.Instances[0].Position, vehicleCenter)), 0.5))
		if norm(dir) < 1e-9 {
			dir = Vec3{0, 1, 0}
		}
		p.Explode = scale(unit(dir), explodeDistance*factor(p.PartID))
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
