package sampledata

import (
	"crypto/sha256"
	"encoding/binary"

	"github.com/shopspring/decimal"
)

// fraction renvoie une valeur déterministe de [0, 1) pour (graine, clé, champ), à 9 décimales.
func fraction(seed, key, field string) decimal.Decimal {
	h := sha256.Sum256([]byte(seed + "\x00" + key + "\x00" + field))
	n := binary.BigEndian.Uint64(h[:8]) % 1_000_000_000
	return decimal.New(int64(n), -9) // #nosec G115 -- n < 1e9
}

// between renvoie lo + (hi - lo) × fraction, arrondi à places décimales.
func between(seed, key, field string, lo, hi decimal.Decimal, places int32) decimal.Decimal {
	return lo.Add(hi.Sub(lo).Mul(fraction(seed, key, field))).Round(places)
}

// intBetween renvoie un entier de [lo, hi].
func intBetween(seed, key, field string, lo, hi int) int {
	h := sha256.Sum256([]byte(seed + "\x00" + key + "\x00" + field))
	span := uint64(hi - lo + 1)                          // #nosec G115 -- hi >= lo, petites bornes écrites dans le code
	return lo + int(binary.BigEndian.Uint64(h[:8])%span) // #nosec G115 -- résultat < span
}

func dec(s string) decimal.Decimal { return decimal.RequireFromString(s) }

// material porte les facteurs physiques d'une famille de matière (valeurs d'ordre de grandeur, fictives).
type material struct {
	co2PerKg      string // kg CO2e par kg de pièce
	recycled      [2]int // % de matière recyclée
	recyclability [2]int // % recyclable en fin de vie
	laborPerKg    string // heures de main-d'œuvre par kg
}

type matKind int

// name : matière telle qu'elle est écrite dans le catalogue (ADR 0039).
func (k matKind) name() string { return matNames[k] }

var matNames = map[matKind]string{
	steel: "acier", castIron: "fonte", alu: "aluminium", plastic: "plastique", rubber: "caoutchouc", glass: "verre",
	copper: "cuivre", electronic: "électronique", carbon: "carbone", textile: "textile", foam: "mousse", leather: "cuir",
	fluid: "fluide", battery: "batterie", magnet: "aimant", titanium: "titane", ceramic: "céramique", inox: "inox",
}

const (
	steel matKind = iota
	castIron
	alu
	plastic
	rubber
	glass
	copper
	electronic
	carbon
	textile
	foam
	leather
	fluid
	battery
	magnet
	titanium
	ceramic
	inox
)

var materials = map[matKind]material{
	steel:      {"2.3", [2]int{20, 45}, [2]int{85, 95}, "0.08"},
	castIron:   {"1.9", [2]int{40, 70}, [2]int{90, 98}, "0.05"},
	alu:        {"9.5", [2]int{25, 60}, [2]int{85, 95}, "0.12"},
	plastic:    {"3.4", [2]int{5, 30}, [2]int{20, 60}, "0.30"},
	rubber:     {"3.1", [2]int{2, 15}, [2]int{10, 40}, "0.25"},
	glass:      {"1.3", [2]int{10, 30}, [2]int{30, 70}, "0.20"},
	copper:     {"4.6", [2]int{30, 60}, [2]int{80, 95}, "0.60"},
	electronic: {"45", [2]int{0, 10}, [2]int{10, 35}, "2.50"},
	carbon:     {"28", [2]int{0, 15}, [2]int{5, 25}, "1.80"},
	textile:    {"6.5", [2]int{5, 40}, [2]int{10, 35}, "0.70"},
	foam:       {"4.8", [2]int{0, 20}, [2]int{5, 30}, "0.40"},
	leather:    {"17", [2]int{0, 5}, [2]int{5, 20}, "1.50"},
	fluid:      {"1.6", [2]int{0, 30}, [2]int{20, 60}, "0.02"},
	battery:    {"14", [2]int{5, 20}, [2]int{50, 85}, "0.90"},
	magnet:     {"32", [2]int{0, 10}, [2]int{20, 50}, "1.20"},
	titanium:   {"38", [2]int{10, 40}, [2]int{80, 95}, "0.90"},
	ceramic:    {"12", [2]int{0, 5}, [2]int{5, 15}, "0.80"},
	inox:       {"4.2", [2]int{40, 70}, [2]int{90, 98}, "0.15"},
}

// eurTo convertit (approximativement, pour fixer des bornes de cotation) un montant de référence en EUR
// vers la devise d'une offre. Ce ne sont pas les taux de calcul : ceux-là viennent de la source de prix.
var eurTo = map[string]string{"EUR": "1", "USD": "1.09", "GBP": "0.855", "JPY": "161"}

// minorPlaces : décimales des bornes de cotation par devise (ISO 4217).
var minorPlaces = map[string]int32{"EUR": 2, "USD": 2, "GBP": 2, "JPY": 0}
