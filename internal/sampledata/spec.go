package sampledata

import (
	"fmt"
	"strconv"
)

// Données de référence écrites à la main. Les valeurs nominales (masse, prix de référence) sont ensuite
// variées de façon déterministe par Generate.

// section regroupe des pièces de même catégorie et de même zone.
type section struct {
	category, zone string
	parts          []partSpec
}

// partSpec : clé logique, nom, masse nominale (kg), prix de référence (EUR), matière, jours de production.
type partSpec struct {
	key, name   string
	mass, price string
	mat         matKind
	days        int
}

// supplierSpec : fournisseur fictif et catégories qu'il fournit.
type supplierSpec struct {
	key, name, country, currency string
	leadDays                     int
	categories                   []string
	smallSeries                  bool
}

// item est un nœud de l'arbre de référence : un sous-ensemble (name), une pièce (part) ou un groupe
// (ni l'un ni l'autre) dont les enfants sont insérés directement dans le parent.
type item struct {
	name     string
	part     string
	qty      string
	children []item
}

// asm décrit un sous-ensemble.
func asm(name string, children ...item) item { return item{name: name, children: children} }

// pc décrit une pièce et sa quantité par unité du parent (décimal en chaîne).
func pc(part, qty string) item { return item{part: part, qty: qty} }

// group insère plusieurs nœuds dans le parent, sans sous-ensemble intermédiaire.
func group(children ...item) item { return item{children: children} }

// each crée un sous-ensemble par libellé (par ex. une roue par position), tous de même contenu.
func each(format string, labels []string, children ...item) item {
	out := make([]item, len(labels))
	for i, l := range labels {
		out[i] = asm(fmt.Sprintf(format, l), children...)
	}
	return group(out...)
}

// repeat crée n sous-ensembles numérotés de même contenu.
func repeat(n int, format string, children ...item) item {
	out := make([]item, n)
	for i := range out {
		out[i] = asm(fmt.Sprintf(format, i+1), children...)
	}
	return group(out...)
}

// fix décrit les n points de fixation d'un sous-ensemble : vis, écrou et rondelle de la taille donnée.
func fix(n int, size string) item {
	return asm("Fixations", repeat(n, "Point de fixation %03d",
		pc("com.vis-"+size, "1"), pc("com.ecrou-"+size, "1"), pc("com.rondelle-"+size, "1")))
}

// clips décrit n agrafes de garnissage, chacune à sa position.
func clips(n int) item {
	return asm("Agrafage", repeat(n, "Agrafe %03d", pc("com.agrafe", "1")))
}

// harness décrit un faisceau : protections, n connecteurs de ways voies (boîtier et joints), un circuit
// par paire de voies (fil de meters mètres et ses deux cosses), épissures et points d'attache.
func harness(name string, n, ways int, meters string) item {
	w := strconv.Itoa(ways)
	return asm(name,
		pc("com.gaine", strconv.Itoa(n/4+1)), pc("com.ruban", strconv.Itoa(n/2+1)),
		repeat(n, "Connecteur C%03d", pc("com.connecteur-"+w+"v", "1"), pc("com.joint-fil", w)),
		repeat(n*ways/2, "Circuit %04d", pc("com.fil-05", meters), pc("com.cosse", "2")),
		repeat(n/2, "Épissure S%03d", pc("com.epissure", "1")),
		asm("Attaches", repeat(n/2, "Attache %03d", pc("com.serre-cable", "1"))),
	)
}

var positions4 = []string{"avant gauche", "avant droite", "arrière gauche", "arrière droite"}
var sides = []string{"gauche", "droite"}
