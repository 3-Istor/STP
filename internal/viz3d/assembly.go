package viz3d

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"bomstudio/internal/bom"
	"bomstudio/internal/catalog"
)

// MatchType dit ce que vaut le modèle d'une pièce : exact, similaire, ou généré depuis ses attributs.
type MatchType string

const (
	// MatchExact : modèle de la référence recherchée.
	MatchExact MatchType = "exact"
	// MatchSimilar : modèle d'une pièce comparable, représentation visuelle seulement.
	MatchSimilar MatchType = "similar"
	// MatchGenerated : primitive construite depuis la catégorie et la masse ; jamais une représentation exacte.
	MatchGenerated MatchType = "generated"
)

// Source dit d'où vient l'assemblage. V1 : toujours généré à la lecture, rien n'est stocké.
const SourceGenerated = "generated"

// Vec3 : (x, y, z) en mètres ; x = avant du véhicule, y = vers le haut, z = côté droit.
type Vec3 [3]float64

// Shape est la primitive d'un modèle généré. Size : encombrement (x, y, z) en mètres ; pour un cylindre
// ou une roue, x et z sont le diamètre et y la hauteur (axe de la roue : z).
type Shape struct {
	Kind string `json:"kind"` // box | cylinder | wheel | sphere
	Size Vec3   `json:"size"`
}

// Instance est une occurrence d'un modèle dans la scène (position du centre, rotation d'Euler en radians).
type Instance struct {
	Position Vec3 `json:"position"`
	Rotation Vec3 `json:"rotation"`
}

// Placement est une pièce de la BOM placée dans la scène. Un nœud pièce = un placement.
type Placement struct {
	NodeID   int64  `json:"node_id"`
	ParentID *int64 `json:"parent_id"`
	PartID   int64  `json:"part_id"`
	Zone     string `json:"zone"`
	Category string `json:"category"`
	// Quantity : quantité par parent, telle quelle (chaîne décimale) ; les instances n'en montrent qu'une partie.
	Quantity string    `json:"quantity"`
	Match    MatchType `json:"match_type"`
	// Confidence : 0–1 en chaîne décimale ; un modèle généré reste sous 0.25.
	Confidence string `json:"confidence"`
	// Estimated : la position est déduite de la zone et de la catégorie, pas d'une géométrie connue.
	Estimated bool `json:"estimated"`
	// Notes : chaque approximation faite (zone inconnue, masse absente…), pour l'afficher.
	Notes     []string   `json:"notes"`
	Shape     Shape      `json:"shape"`
	Instances []Instance `json:"instances"`
	// Explode : déplacement à éclatement complet (mètres) ; le front applique position + explode × niveau.
	Explode Vec3 `json:"explode"`
}

// MissingModel : pièce de la BOM qui n'a pas de modèle, gardée dans la BOM et signalée dans la vue 3D.
type MissingModel struct {
	NodeID int64  `json:"node_id"`
	PartID int64  `json:"part_id"`
	Reason string `json:"reason"`
}

// ZoneSummary compte les pièces par zone ; Known = false quand la zone n'a pas de région dans le gabarit.
type ZoneSummary struct {
	Zone  string `json:"zone"`
	Count int    `json:"count"`
	Known bool   `json:"known"`
}

// Vehicle est le gabarit : longueur (x), hauteur (y), largeur (z), en mètres.
type Vehicle struct {
	Length float64 `json:"length"`
	Height float64 `json:"height"`
	Width  float64 `json:"width"`
}

// Assembly est la scène complète d'une BOM.
type Assembly struct {
	BOMID      int64          `json:"bom_id"`
	Source     string         `json:"source"`
	Vehicle    Vehicle        `json:"vehicle"`
	Zones      []ZoneSummary  `json:"zones"`
	Placements []Placement    `json:"placements"`
	Missing    []MissingModel `json:"missing"`
}

// Trees lit l'arbre d'une BOM ; *bom.Service la satisfait.
type Trees interface {
	Tree(ctx context.Context, bomID int64) (bom.BOM, []bom.Node, error)
}

// Parts lit une pièce du catalogue ; catalog.Catalog la satisfait.
type Parts interface {
	Part(ctx context.Context, id int64) (catalog.Part, error)
}

// Service assemble une BOM. Il ne connaît la BOM et le catalogue qu'à travers leurs interfaces (AGENTS §2).
type Service struct {
	trees Trees
	parts Parts
}

// NewService câble le service sur l'arbre des BOM et le catalogue.
func NewService(t Trees, p Parts) *Service { return &Service{trees: t, parts: p} }

// Assembly lit l'arbre et les pièces, puis construit la scène. Une pièce illisible au catalogue n'est pas
// une erreur : elle figure dans Missing. ErrBOMNotFound si la BOM n'existe pas.
func (s *Service) Assembly(ctx context.Context, bomID int64) (Assembly, error) {
	_, nodes, err := s.trees.Tree(ctx, bomID)
	if err != nil {
		return Assembly{}, fmt.Errorf("assembly of bom %d: %w", bomID, err)
	}
	parts := make(map[int64]catalog.Part)
	missing := make(map[int64]string)
	for _, id := range distinctPartIDs(nodes) {
		p, err := s.parts.Part(ctx, id)
		switch {
		case err == nil:
			parts[id] = p
		case errors.Is(err, catalog.ErrPartNotFound):
			missing[id] = "pièce absente du catalogue"
		default:
			return Assembly{}, fmt.Errorf("assembly of bom %d: part %d: %w", bomID, id, err)
		}
	}
	return Build(bomID, nodes, parts, missing), nil
}

// distinctPartIDs : identifiants de pièce de l'arbre, sans doublon, triés (ordre de lecture stable).
func distinctPartIDs(nodes []bom.Node) []int64 {
	seen := make(map[int64]struct{})
	var ids []int64
	for _, n := range nodes {
		if n.PartID == nil {
			continue
		}
		if _, ok := seen[*n.PartID]; ok {
			continue
		}
		seen[*n.PartID] = struct{}{}
		ids = append(ids, *n.PartID)
	}
	slices.Sort(ids)
	return ids
}
