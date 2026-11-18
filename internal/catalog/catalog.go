package catalog

import (
	"context"
	"errors"
	"time"

	"github.com/shopspring/decimal"

	"bomstudio/internal/money"
)

var (
	// ErrPartNotFound : aucune pièce avec cet identifiant.
	ErrPartNotFound = errors.New("part not found")
	// ErrOfferNotFound : aucune offre avec cet identifiant.
	ErrOfferNotFound = errors.New("offer not found")
	// ErrInvalidInput : un attribut requis est absent ou invalide.
	ErrInvalidInput = errors.New("invalid catalog input")
	// ErrArchived : une pièce/offre archivée ne peut pas recevoir de nouvelles associations.
	ErrArchived = errors.New("catalog item is archived")
	// ErrSupplierNotFound : le fournisseur demandé n'existe pas.
	ErrSupplierNotFound = errors.New("supplier not found")
	// ErrReferenceConflict : une référence concurrente empêche une suppression physique.
	ErrReferenceConflict = errors.New("catalog reference conflict")
	// ErrQuotedOfferIdentityChange : créer une autre offre pour changer cette identité.
	ErrQuotedOfferIdentityChange = errors.New("quoted offer currency or supplier cannot change")
)

// Part est une pièce du catalogue, décrite indépendamment de qui la vend.
// Les pointeurs NULL conservent explicitement les attributs absents des anciennes lignes (ADR 0009).
type Part struct {
	ID                   int64            `json:"id"`
	Name                 string           `json:"name"`
	Category             string           `json:"category"`
	Zone                 string           `json:"zone"`
	MassKG               *decimal.Decimal `json:"mass_kg"`
	CO2KGPerUnit         *decimal.Decimal `json:"co2_kg_per_unit"`
	RecycledPercent      *decimal.Decimal `json:"recycled_percent"`
	RecyclabilityPercent *decimal.Decimal `json:"recyclability_percent"`
	ManufacturingCost    *money.Money     `json:"manufacturing_cost"`
	ProductionDays       *int             `json:"production_days"`
	LaborHours           *decimal.Decimal `json:"labor_hours"`
	HourlyRate           *money.Money     `json:"hourly_rate"`
	ArchivedAt           *time.Time       `json:"archived_at"`
	// Material : matière de la pièce (famille de comparaison des prix au kg, ADR 0041) ; nil = non renseignée.
	Material *string `json:"material"`
}

// Offer est une pièce vendue par un fournisseur. PriceMin et PriceMax sont les bornes de référence
// des cotations, dans la devise de l'offre ; ce ne sont pas des prix d'achat (le prix est la dernière
// cotation, fournie par pricing).
type Offer struct {
	ID            int64            `json:"id"`
	PartID        int64            `json:"part_id"`
	SupplierID    int64            `json:"supplier_id"`
	Currency      money.Currency   `json:"currency"`
	PriceMin      money.Money      `json:"price_min"`
	PriceMax      money.Money      `json:"price_max"`
	LeadTimeDays  int              `json:"lead_time_days"`
	OriginCountry string           `json:"origin_country"` // ISO 3166-1 alpha-2
	QualityScore  *decimal.Decimal `json:"quality_score"`
	ArchivedAt    *time.Time       `json:"archived_at"`
}

// PartWithOffers est une pièce unique et toutes ses offres actives, même chez plusieurs fournisseurs.
type PartWithOffers struct {
	Part
	Offers []Offer `json:"offers"`
}

// Catalog donne accès aux pièces et aux offres. Implémentations : Stub (en mémoire), puis la base.
type Catalog interface {
	// Part renvoie la pièce, ou ErrPartNotFound.
	Part(ctx context.Context, id int64) (Part, error)
	// Offer renvoie l'offre, ou ErrOfferNotFound.
	Offer(ctx context.Context, id int64) (Offer, error)
}
