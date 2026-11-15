package viz3d

import (
	"context"
	"errors"
	"io"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"bomstudio/internal/catalog"
)

var (
	// ErrModelNotFound : aucun modèle 3D avec cet identifiant.
	ErrModelNotFound = errors.New("3d model not found")
	// ErrNoModel : la pièce n'a pas de modèle (ou le chercheur n'en a trouvé aucun).
	ErrNoModel = errors.New("part has no 3d model")
	// ErrInvalidModel : fichier ou attributs de modèle refusés (pas un GLB, trop gros, confiance hors 0–1…).
	ErrInvalidModel = errors.New("invalid 3d model")
	// ErrGenerationRunning : une génération est déjà en attente ou en cours pour cette BOM.
	ErrGenerationRunning = errors.New("3d generation already running")
	// ErrNoGeneration : aucune génération n'a été lancée pour cette BOM.
	ErrNoGeneration = errors.New("no 3d generation for this bom")
	// ErrModelsUnavailable : le service tourne sans stockage de modèles (tranche 1 seule).
	ErrModelsUnavailable = errors.New("3d models are not configured")
	// ErrBlobNotFound : aucun fichier sous cette clé dans le BlobStore.
	ErrBlobNotFound = errors.New("model file not found")
)

// Formats d'un modèle : un fichier GLB, ou une primitive recalculée depuis la catégorie (pas de fichier).
const (
	FormatGLB       = "glb"
	FormatPrimitive = "primitive"
)

// Sources d'un modèle : généré par le chercheur V1, ou téléversé par un utilisateur.
const (
	SourceUpload = "upload"
)

// MaxModelBytes : taille maximale d'un GLB téléversé (32 Mio) : un modèle de pièce, pas une scène entière.
const MaxModelBytes = 32 << 20

// Dimensions d'un modèle en mètres (longueurs : décimales en base, jamais un montant).
type Dimensions struct {
	X decimal.Decimal `json:"x"`
	Y decimal.Decimal `json:"y"`
	Z decimal.Decimal `json:"z"`
}

// Model est un modèle 3D persisté. StoragePath est la clé du fichier dans le BlobStore ("" pour une
// primitive) ; le fichier lui-même n'est jamais en base.
type Model struct {
	ID          int64
	Name        string
	Source      string
	SourceURL   string
	StoragePath string
	Format      string
	License     string
	Dimensions  *Dimensions
	// Confidence : 0–1, ce que la source elle-même vaut (le chercheur ou l'utilisateur).
	Confidence decimal.Decimal
	CreatedAt  time.Time
}

// PartModel est le modèle retenu pour une pièce : un seul par pièce, partagé par toutes les BOM.
type PartModel struct {
	PartID     int64
	ModelID    int64
	Match      MatchType
	Confidence decimal.Decimal
	CreatedAt  time.Time
}

// Resolved : une pièce et son modèle, tels que lus ensemble.
type Resolved struct {
	PartModel PartModel
	Model     Model
}

// Status d'une génération persistée.
type Status string

const (
	StatusQueued  Status = "queued"
	StatusRunning Status = "running"
	StatusDone    Status = "done"
	StatusFailed  Status = "failed"
	// StatusStale n'est jamais stocké : la génération est finie mais l'arbre a changé depuis (empreinte).
	StatusStale Status = "stale"
	// StatusNone n'est jamais stocké : aucune génération n'a été lancée.
	StatusNone Status = "none"
)

// FailedPart : pièce pour laquelle aucun modèle n'a pu être résolu, et pourquoi.
type FailedPart struct {
	PartID int64  `json:"part_id"`
	Reason string `json:"reason"`
}

// GenerationResult est le bilan enregistré à la fin d'une génération (JSON en base).
type GenerationResult struct {
	Finder    string       `json:"finder"`
	Exact     int          `json:"exact"`
	Similar   int          `json:"similar"`
	Generated int          `json:"generated"`
	Failed    []FailedPart `json:"failed"`
}

// Generation est l'état persisté de la génération des modèles d'une BOM (une ligne par BOM).
type Generation struct {
	BOMID  int64
	Status Status
	// Done / Total : pièces distinctes avec un modèle / à résoudre.
	Done, Total int
	Error       string
	// Fingerprint : empreinte de l'arbre au lancement (voir Fingerprint).
	Fingerprint string
	Result      *GenerationResult
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// Store est l'accès aux modèles et aux générations. Le domaine ne connaît que cette interface ;
// l'implémentation Postgres vit dans viz3d/viz3dstore (le domaine reste sans pgx, AGENTS §2).
type Store interface {
	CreateModel(ctx context.Context, m Model) (Model, error)
	// Model renvoie le modèle, ou ErrModelNotFound.
	Model(ctx context.Context, id int64) (Model, error)
	// SetPartModel enregistre (ou remplace) le modèle d'une pièce ; catalog.ErrPartNotFound ou ErrModelNotFound
	// si l'un des deux n'existe pas.
	SetPartModel(ctx context.Context, pm PartModel) (PartModel, error)
	// PartModel renvoie le modèle d'une pièce, ou ErrNoModel.
	PartModel(ctx context.Context, partID int64) (Resolved, error)
	// PartModels renvoie les modèles des pièces données (absentes : pas d'entrée).
	PartModels(ctx context.Context, partIDs []int64) (map[int64]Resolved, error)

	// ClaimGeneration crée ou remet en file la génération d'une BOM, dans une seule instruction ;
	// ErrGenerationRunning si elle est déjà en attente ou en cours.
	ClaimGeneration(ctx context.Context, bomID int64, total int, fingerprint string) (Generation, error)
	// Generation renvoie l'état, ou ErrNoGeneration.
	Generation(ctx context.Context, bomID int64) (Generation, error)
	// SetGenerationProgress passe la génération en cours et enregistre l'avancement.
	SetGenerationProgress(ctx context.Context, bomID int64, done int) (Generation, error)
	// FinishGeneration termine la génération (done ou failed) avec son bilan.
	FinishGeneration(ctx context.Context, bomID int64, status Status, done int, errText string, result *GenerationResult) (Generation, error)
	// PendingGenerations : générations en attente ou en cours (à reprendre après un redémarrage), par BOM.
	PendingGenerations(ctx context.Context) ([]Generation, error)
}

// BlobStore range les fichiers de modèle hors base (ADR 0031, option 2b). Implémentations :
// viz3d/fsblob (système de fichiers sous MODELS_DIR) ; un adapter S3 plus tard, derrière la même interface.
type BlobStore interface {
	// Put écrit le fichier sous la clé (remplace s'il existe).
	Put(ctx context.Context, key string, r io.Reader) error
	// Get ouvre le fichier et donne sa taille, ou ErrBlobNotFound.
	Get(ctx context.Context, key string) (io.ReadCloser, int64, error)
	Exists(ctx context.Context, key string) (bool, error)
}

// ValidBlobKey : clé relative, minuscules, chiffres, '/', '.', '-', '_' ; jamais vide, absolue ni « .. ».
// Chaque implémentation du BlobStore la vérifie avant tout accès.
func ValidBlobKey(key string) bool {
	if key == "" || len(key) > 200 || strings.HasPrefix(key, "/") || strings.HasSuffix(key, "/") {
		return false
	}
	for _, seg := range strings.Split(key, "/") {
		if seg == "" || seg == "." || seg == ".." {
			return false
		}
	}
	for _, r := range key {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '/', r == '.', r == '-', r == '_':
		default:
			return false
		}
	}
	return true
}

// Candidate est ce qu'un chercheur propose pour une pièce : un modèle (sans id) et ce qu'il vaut.
type Candidate struct {
	Model      Model
	Match      MatchType
	Confidence decimal.Decimal
}

// ModelFinder cherche un modèle pour une pièce du catalogue (ADR 0031, option 3b). V1 : GeneratedFinder ;
// un adapter Sketchfab ou un LLM viendront derrière la même interface, hors du domaine.
type ModelFinder interface {
	// Name identifie le chercheur dans le bilan d'une génération (reproductibilité).
	Name() string
	// Find renvoie le candidat, ou ErrNoModel.
	Find(ctx context.Context, p catalog.Part) (Candidate, error)
}

// GeneratedFinder est le chercheur V1 : déterministe, il renvoie pour toute pièce une primitive
// (la forme est celle de layout.go, recalculée à la lecture) avec la confiance de base.
type GeneratedFinder struct{}

// Name du chercheur V1.
func (GeneratedFinder) Name() string { return "generated-v1" }

// Find renvoie toujours un candidat : une primitive n'est jamais exacte, sa confiance reste basse.
func (GeneratedFinder) Find(_ context.Context, p catalog.Part) (Candidate, error) {
	spec := shapeFor(p.Category, p.Name)
	conf := decimal.NewFromFloat(baseConfidence)
	return Candidate{
		Model: Model{Name: p.Name + " (" + spec.kind + ")", Source: SourceGenerated, Format: FormatPrimitive, Confidence: conf},
		Match: MatchGenerated, Confidence: conf,
	}, nil
}

var _ ModelFinder = GeneratedFinder{}

// ValidMatch dit si le niveau de correspondance est l'un des trois connus.
func ValidMatch(m MatchType) bool {
	return m == MatchExact || m == MatchSimilar || m == MatchGenerated
}

// ValidConfidence : 0 ≤ c ≤ 1.
func ValidConfidence(c decimal.Decimal) bool {
	return !c.IsNegative() && c.LessThanOrEqual(decimal.NewFromInt(1))
}
