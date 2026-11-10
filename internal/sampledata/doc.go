// Package sampledata génère le jeu de démonstration sample-dataset (docs/features.md:279–298) :
// fournisseurs, catalogue, offres, contrats à paliers, alternatives et deux BOM complètes
// (Fiat Multipla en EUR, Aston Martin Valhalla en GBP). Voir ADR 0012 (chargement) et ADR 0021 (contenu).
//
// Generate est pure : aucune base, aucune horloge, aucun math/rand. Chaque valeur dérive de
// SHA-256(graine, clé logique, champ) : même graine = même jeu, quel que soit l'ordre d'itération.
// Les noms et valeurs sont plausibles mais fictifs : aucune donnée réelle de constructeur.
// Le chargement en base (idempotent, transactionnel) est dans le sous-paquet seedstore.
package sampledata
