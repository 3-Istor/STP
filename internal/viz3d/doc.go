// Package viz3d construit l'assemblage 3D d'une BOM : pour chaque pièce de l'arbre, un modèle (en V1 une
// primitive générée), une position estimée dans un gabarit de voiture et un vecteur d'éclatement.
// Le placement est une fonction pure de l'arbre et des attributs du catalogue (zone, catégorie, masse,
// quantité) : même BOM = même scène (AGENTS §3). Voir ADR 0031.
//
// Les coordonnées sont des float64 : ce sont des longueurs de scène en mètres, pas des montants.
package viz3d
