package bom

import (
	"context"
	"errors"
	"time"

	"github.com/shopspring/decimal"
)

var (
	// ErrBOMNotFound : aucune BOM avec cet identifiant.
	ErrBOMNotFound = errors.New("bom not found")
	// ErrNodeNotFound : aucun nœud avec cet identifiant dans cette BOM.
	ErrNodeNotFound = errors.New("bom node not found")
	// ErrCycle : le nouveau parent est le nœud lui-même ou l'un de ses descendants (AGENTS §3).
	ErrCycle = errors.New("move would create a cycle")
	// ErrRootNode : la racine d'une BOM ne se supprime pas.
	ErrRootNode = errors.New("root node cannot be deleted")
	// ErrHasSnapshots : la BOM a au moins un snapshot publié, elle ne se supprime pas (R2, ADR 0006).
	ErrHasSnapshots = errors.New("bom has published snapshots")
	// ErrNotAPart : opération réservée aux nœuds pièce (offre), appliquée à un sous-ensemble.
	ErrNotAPart = errors.New("node is not a part")
)

// BOM est un brouillon : réglages globaux et lien éventuel vers un snapshot de prix.
type BOM struct {
	ID       int64
	Name     string
	Currency string // devise de référence, ISO 4217
	Volume   int32  // volume de production
	// PriceSnapshotID : nil = prix en direct ; sinon les prix du brouillon sont figés (R9).
	PriceSnapshotID *int64
	// SourceSnapshotID : snapshot dont ce brouillon a été créé (D3), nil sinon.
	SourceSnapshotID *int64
	CreatedAt        time.Time
}

// NewBOM décrit une BOM à créer. Sa racine porte le nom de la BOM.
type NewBOM struct {
	Name             string
	Currency         string
	Volume           int32
	SourceSnapshotID *int64
}

// Node est un nœud de l'arbre : un sous-ensemble (Name, PartID et OfferID nil) ou une pièce
// (PartID et OfferID renseignés, Name vide).
type Node struct {
	ID       int64
	BOMID    int64
	ParentID *int64 // nil : racine
	Name     string
	PartID   *int64
	OfferID  *int64
	// Quantity : quantité par unité du parent (décimal, jamais float).
	Quantity decimal.Decimal
	// ProductionDays : jours de production propres au nœud (ADR 0007).
	ProductionDays int32
}

// IsPart dit si le nœud est une pièce (et non un sous-ensemble).
func (n Node) IsPart() bool { return n.PartID != nil }

// NewNode décrit un nœud à ajouter sous ParentID. Sous-ensemble : Name ; pièce : PartID et OfferID.
type NewNode struct {
	ParentID       int64
	Name           string
	PartID         *int64
	OfferID        *int64
	Quantity       decimal.Decimal
	ProductionDays int32
}

// Store est l'accès aux brouillons de BOM. Le domaine ne connaît que cette interface ;
// l'implémentation Postgres vit dans bom/bomstore (le domaine reste sans pgx, AGENTS §2).
type Store interface {
	// CreateBOM crée la BOM et sa racine dans une même transaction.
	CreateBOM(ctx context.Context, b NewBOM) (BOM, error)
	// ListBOMs renvoie toutes les BOM, triées par id.
	ListBOMs(ctx context.Context) ([]BOM, error)
	// GetBOM renvoie la BOM, ou ErrBOMNotFound.
	GetBOM(ctx context.Context, id int64) (BOM, error)
	RenameBOM(ctx context.Context, id int64, name string) (BOM, error)
	UpdateSettings(ctx context.Context, id int64, currency string, volume int32) (BOM, error)
	// SetPriceSnapshot fige les prix (snapshotID non nil) ou les remet en direct (nil).
	SetPriceSnapshot(ctx context.Context, id int64, snapshotID *int64) (BOM, error)
	// DeleteBOM supprime la BOM et son arbre ; ErrHasSnapshots si un snapshot existe.
	DeleteBOM(ctx context.Context, id int64) error

	// Tree renvoie tous les nœuds de la BOM en une requête, triés par id ; ErrBOMNotFound si elle n'existe pas.
	Tree(ctx context.Context, bomID int64) ([]Node, error)
	// AddNode ajoute un nœud ; ErrNodeNotFound si le parent n'est pas dans cette BOM.
	AddNode(ctx context.Context, bomID int64, n NewNode) (Node, error)
	// SetNodeOffer change l'offre d'une pièce ; ErrNotAPart sur un sous-ensemble.
	SetNodeOffer(ctx context.Context, bomID, nodeID, offerID int64) (Node, error)
	SetNodeQuantity(ctx context.Context, bomID, nodeID int64, qty decimal.Decimal) (Node, error)
	SetNodeProductionDays(ctx context.Context, bomID, nodeID int64, days int32) (Node, error)
	// MoveNode rattache le nœud à newParentID ; ErrCycle si newParentID est le nœud ou un descendant.
	// Le contrôle et la mise à jour se font dans la même transaction.
	MoveNode(ctx context.Context, bomID, nodeID, newParentID int64) (Node, error)
	// DeleteNode supprime le nœud et son sous-arbre ; ErrRootNode pour la racine.
	DeleteNode(ctx context.Context, bomID, nodeID int64) error
}
