// Package demo fournit un catalogue et des prix de démonstration, réservés au développement.
//
// Feature toggle BOM_DEMO=1 (AGENTS §4), désactivé par défaut :
//   - propriétaire : Arthur.presle ;
//   - raison : tester l'UI de la BOM (NLPF-69) avant les implémentations Postgres du catalogue (NLPF-67)
//     et des prix (NLPF-68) ;
//   - suppression : à la livraison de NLPF-67 et NLPF-68 (supprimer ce paquet, son câblage dans
//     cmd/server et l'appel à /api/v1/demo/catalog dans web/src/catalog.ts).
//
// Une seule source de données (ce fichier) sert à deux choses : Seed insère fournisseurs, pièces, offres
// et cotations en base (les FK de bom_nodes l'exigent), et Catalog / NewPricing construisent les stubs
// en mémoire avec les mêmes identifiants. Ce n'est pas le jeu de données `make seed` de NLPF-67.
package demo

import (
	"time"
)

// Les identifiants commencent à 9 000 001 : loin des séquences IDENTITY de la base de dev (quelques
// centaines), pour qu'une pièce créée normalement ne prenne jamais l'id d'une pièce de démo.
// Seed vérifie quand même qu'un id déjà présent correspond bien à la même donnée.

// QuotedAt est l'horodatage (fixe, déterministe) des cotations de démo ; Source leur provenance.
var QuotedAt = time.Date(2026, 1, 5, 8, 0, 0, 0, time.UTC)

// Source est la provenance des cotations de démo (price_quotes.source, fx_quotes.source).
const Source = "demo"

type supplierRow struct {
	ID   int64
	Name string
}

type partRow struct {
	ID             int64
	Name           string
	Category       string
	Zone           string
	CO2Kg, MassKg  string // décimaux en chaîne, jamais float
	ProductionDays int
}

type offerRow struct {
	ID, PartID, SupplierID int64
	Currency               string
	PriceMin, PriceMax     string
	LeadTimeDays           int
	OriginCountry          string
	// Price : cotation de démo ; vide = aucune cotation (montre une erreur « no_quote » dans l'UI).
	Price string
}

type rateRow struct {
	From, To, Rate string
}

var suppliers = []supplierRow{
	{9_000_001, "Démo Roues & Pneus"},
	{9_000_002, "Démo Freinage"},
	{9_000_003, "Démo Electrics Inc."},
	{9_000_004, "Démo Intérieurs"},
}

var parts = []partRow{
	{9_000_101, "Jante alu 17\"", "Liaison au sol", "Roues", "45.5", "9.8", 2},
	{9_000_102, "Pneu 225/45 R17", "Liaison au sol", "Roues", "30.2", "8.9", 1},
	{9_000_103, "Écrou de roue", "Visserie", "Roues", "0.12", "0.05", 0},
	{9_000_104, "Disque de frein", "Freinage", "Roues", "18.0", "6.5", 1},
	{9_000_105, "Jeu de plaquettes", "Freinage", "Roues", "2.4", "0.9", 0},
	{9_000_106, "Batterie 12 V", "Électricité", "Compartiment moteur", "60.0", "15.0", 3},
	{9_000_107, "Phare LED", "Éclairage", "Avant", "12.5", "2.1", 2},
	{9_000_108, "Siège avant", "Habitacle", "Habitacle", "85.0", "22.0", 5},
	{9_000_109, "Faisceau électrique", "Électricité", "Habitacle", "25.0", "4.0", 4},
	{9_000_110, "Capteur ABS", "Freinage", "Roues", "1.5", "0.2", 1},
}

var offers = []offerRow{
	{9_000_201, 9_000_101, 9_000_001, "EUR", "90", "130", 10, "IT", "100.00"},
	{9_000_202, 9_000_101, 9_000_003, "USD", "95", "140", 25, "US", "108.50"},
	{9_000_203, 9_000_102, 9_000_001, "EUR", "70", "110", 7, "FR", "84.90"},
	{9_000_204, 9_000_103, 9_000_001, "EUR", "0.5", "2", 3, "DE", "1.00"},
	{9_000_205, 9_000_103, 9_000_002, "EUR", "0.4", "1.5", 12, "CN", "0.62"},
	{9_000_206, 9_000_104, 9_000_002, "EUR", "35", "60", 5, "DE", "42.00"},
	{9_000_207, 9_000_105, 9_000_002, "EUR", "20", "45", 5, "DE", "31.40"},
	{9_000_208, 9_000_106, 9_000_003, "USD", "90", "160", 30, "US", "120.00"},
	{9_000_209, 9_000_107, 9_000_003, "USD", "150", "260", 21, "US", "199.99"},
	{9_000_210, 9_000_108, 9_000_004, "EUR", "250", "420", 14, "PT", "310.00"},
	{9_000_211, 9_000_109, 9_000_003, "USD", "200", "340", 28, "MX", "265.00"},
	{9_000_212, 9_000_110, 9_000_003, "USD", "8", "20", 15, "US", "12.75"},
	{9_000_213, 9_000_110, 9_000_002, "EUR", "8", "20", 6, "DE", ""}, // sans cotation
}

// Paires exactes (pas d'inversion, voir pricing.Rate) : une BOM en EUR ou en USD se calcule entièrement.
var rates = []rateRow{
	{"USD", "EUR", "0.92"},
	{"EUR", "USD", "1.087"},
}
