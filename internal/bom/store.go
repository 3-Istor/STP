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
	// AnnualHours : base horaire annuelle d'un ETP (ADR 0024), > 0.
	AnnualHours decimal.Decimal
	// DestinationCountry : pays de l'usine d'assemblage (ISO 3166-1 alpha-2, ADR 0025) ; nil si non renseigné.
	DestinationCountry *string
	// PriceSnapshotID : nil = prix en direct ; sinon les prix du brouillon sont figés (R9).
	PriceSnapshotID *int64
	// SourceSnapshotID : snapshot dont ce brouillon a été créé (D3), nil sinon.
	SourceSnapshotID *int64
	// CostMethod : méthode de calcul du coût (ADR 0031) ; zéro = DefaultCostMethod.
	CostMethod CostMethod
	CreatedAt  time.Time
}

// TargetBudget : budget cible par voiture d'un nœud (ADR 0018, 0031) ; celui de la racine est le budget global.
type TargetBudget struct {
	NodeID   int64
	Amount   decimal.Decimal
	Currency string
}

// NewBOM décrit une BOM à créer. Sa racine porte le nom de la BOM.
type NewBOM struct {
	Name             string
	Currency         string
	Volume           int32
	SourceSnapshotID *int64
	// AnnualHours : nil → DefaultAnnualHours (ADR 0024).
	AnnualHours *decimal.Decimal
	// DestinationCountry : recopié d'un snapshot (ADR 0025) ; nil sinon.
	DestinationCountry *string
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
	// Strategy : stratégie attachée à ce nœud (ADR 0028) ; nil = celle de l'ancêtre le plus proche.
	Strategy *Strategy
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

// PartStock est la quantité d'une pièce en stock pour une BOM, propre à cette BOM (ADR 0022).
type PartStock struct {
	PartID   int64
	Quantity decimal.Decimal // > 0 ; un stock nul n'est pas enregistré
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
	SetAnnualHours(ctx context.Context, id int64, hours decimal.Decimal) (BOM, error)
	SetDestination(ctx context.Context, id int64, country *string) (BOM, error)
	// SetPriceSnapshot fige les prix (snapshotID non nil) ou les remet en direct (nil).
	SetPriceSnapshot(ctx context.Context, id int64, snapshotID *int64) (BOM, error)
	// DeleteBOM supprime la BOM et son arbre ; ErrHasSnapshots si un snapshot existe.
	DeleteBOM(ctx context.Context, id int64) error

	// Draft lit la BOM, son arbre, son stock, son planning et ses budgets cibles dans une même lecture
	// cohérente (une transaction en lecture seule) ; ErrBOMNotFound si elle n'existe pas.
	Draft(ctx context.Context, bomID int64) (Draft, error)
	// Tree renvoie tous les nœuds de la BOM en une requête, triés par id ; ErrBOMNotFound si elle n'existe pas.
	Tree(ctx context.Context, bomID int64) ([]Node, error)
	// AddNode ajoute un nœud ; ErrNodeNotFound si le parent n'est pas dans cette BOM.
	AddNode(ctx context.Context, bomID int64, n NewNode) (Node, error)
	// SetNodeOffer change l'offre d'une pièce ; ErrNotAPart sur un sous-ensemble.
	SetNodeOffer(ctx context.Context, bomID, nodeID, offerID int64) (Node, error)
	SetNodeQuantity(ctx context.Context, bomID, nodeID int64, qty decimal.Decimal) (Node, error)
	SetNodeProductionDays(ctx context.Context, bomID, nodeID int64, days int32) (Node, error)
	// SetNodeStrategy attache (ou retire, nil) une stratégie au nœud.
	SetNodeStrategy(ctx context.Context, bomID, nodeID int64, s *Strategy) (Node, error)
	// SetNodePart remplace la pièce et l'offre d'un nœud pièce (id, quantité et enfants gardés) ; ErrNotAPart
	// sur un sous-ensemble.
	SetNodePart(ctx context.Context, bomID, nodeID, partID, offerID int64) (Node, error)
	// MoveNode rattache le nœud à newParentID ; ErrCycle si newParentID est le nœud ou un descendant. check (non
	// nil) juge l'arbre et les budgets après le déplacement (ADR 0042). Contrôles et mise à jour dans la même
	// transaction.
	MoveNode(ctx context.Context, bomID, nodeID, newParentID int64, check BudgetCheck) (Node, error)
	// DeleteNode supprime le nœud et son sous-arbre ; ErrRootNode pour la racine.
	DeleteNode(ctx context.Context, bomID, nodeID int64) error

	// Stock renvoie le stock de la BOM, trié par pièce (ADR 0022).
	Stock(ctx context.Context, bomID int64) ([]PartStock, error)
	// SetStock enregistre (ou remplace) le stock d'une pièce ; ErrBOMNotFound si la BOM n'existe pas.
	SetStock(ctx context.Context, bomID int64, st PartStock) (PartStock, error)
	// DeleteStock retire le stock d'une pièce (sans effet s'il n'y en a pas).
	DeleteStock(ctx context.Context, bomID, partID int64) error
	// Schedule renvoie la date de départ et la date cible, ou nil si la BOM n'en a pas (ADR 0023).
	Schedule(ctx context.Context, bomID int64) (*Schedule, error)
	// SetSchedule enregistre (ou remplace) le planning ; ErrBOMNotFound si la BOM n'existe pas.
	SetSchedule(ctx context.Context, bomID int64, s Schedule) (Schedule, error)
	DeleteSchedule(ctx context.Context, bomID int64) error
	// SetCostMethod enregistre la méthode de calcul du coût (ADR 0031) ; ErrBOMNotFound si la BOM n'existe pas.
	SetCostMethod(ctx context.Context, id int64, m CostMethod) (BOM, error)
	// TargetBudgets renvoie les budgets cibles de la BOM, triés par nœud.
	TargetBudgets(ctx context.Context, bomID int64) ([]TargetBudget, error)
	// SetTargetBudget enregistre (ou remplace) le budget cible d'un nœud ; ErrNodeNotFound s'il n'est pas dans la BOM.
	SetTargetBudget(ctx context.Context, bomID int64, b TargetBudget) error
	// EditTargetBudget pose (b non nil) ou retire (b nil) le budget du nœud nodeID, BOM verrouillée : check juge
	// l'arbre et les budgets résultants avant l'écriture ; s'il échoue, rien n'est écrit (ADR 0042).
	// ErrBOMNotFound / ErrNodeNotFound si la BOM ou le nœud n'existe pas.
	EditTargetBudget(ctx context.Context, bomID, nodeID int64, b *TargetBudget, check BudgetCheck) error
}
