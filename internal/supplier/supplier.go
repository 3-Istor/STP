package supplier

import (
	"errors"
	"time"

	"github.com/shopspring/decimal"

	"bomstudio/internal/money"
)

var (
	ErrInvalidInput      = errors.New("invalid supplier input")
	ErrSupplierNotFound  = errors.New("supplier not found")
	ErrContractNotFound  = errors.New("contract not found")
	ErrOfferNotFound     = errors.New("offer not found")
	ErrArchivedOffer     = errors.New("offer or its part is archived")
	ErrSupplierMismatch  = errors.New("contract and offer must have the same supplier")
	ErrSelectionRequired = errors.New("multiple active contracts require a selection")
	ErrReferenceConflict = errors.New("supplier reference conflict")
)

// Supplier garde les notes absentes des anciennes lignes sous forme NULL.
type Supplier struct {
	ID           int64            `json:"id"`
	Name         string           `json:"name"`
	QualityScore *decimal.Decimal `json:"quality_score"`
	TrustScore   *decimal.Decimal `json:"trust_score"`
}

type Tier struct {
	MinQuantity     decimal.Decimal `json:"min_quantity"`
	DiscountPercent decimal.Decimal `json:"discount_percent"`
}

// Contract garde les dates calendaires ISO ; ni l'évaluation ni la lecture ne consultent l'heure réelle.
type Contract struct {
	ID         int64   `json:"id"`
	SupplierID int64   `json:"supplier_id"`
	StartsOn   string  `json:"starts_on"`
	EndsOn     string  `json:"ends_on"`
	Conditions string  `json:"conditions"`
	OfferIDs   []int64 `json:"offer_ids"`
	Tiers      []Tier  `json:"tiers"`
}

type SupplierInput struct {
	Name         string `json:"name"`
	QualityScore string `json:"quality_score"`
	TrustScore   string `json:"trust_score"`
}

type SupplierPatch struct {
	Name         *string `json:"name"`
	QualityScore *string `json:"quality_score"`
	TrustScore   *string `json:"trust_score"`
}

type TierInput struct {
	MinQuantity     string `json:"min_quantity"`
	DiscountPercent string `json:"discount_percent"`
}

type ContractInput struct {
	StartsOn   string      `json:"starts_on"`
	EndsOn     string      `json:"ends_on"`
	Conditions string      `json:"conditions"`
	OfferIDs   []int64     `json:"offer_ids"`
	Tiers      []TierInput `json:"tiers"`
}

// ContractPatch conserve les champs absents ; null explicite est rejeté par l'adapter HTTP.
type ContractPatch struct {
	StartsOn   *string      `json:"starts_on"`
	EndsOn     *string      `json:"ends_on"`
	Conditions *string      `json:"conditions"`
	OfferIDs   *[]int64     `json:"offer_ids"`
	Tiers      *[]TierInput `json:"tiers"`
}

// EvaluationInput est fourni par le calcul BOM ; quantité déjà multipliée par le volume/les parents.
type EvaluationInput struct {
	OfferID            int64
	SelectedContractID *int64
	AsOf               time.Time
	UnitPrice          money.Money
	Quantity           string
}

type Evaluation struct {
	OfferID            int64       `json:"offer_id"`
	SelectedContractID *int64      `json:"selected_contract_id"`
	AppliedContractID  *int64      `json:"applied_contract_id"`
	AsOfDate           string      `json:"as_of_date"`
	Quantity           string      `json:"quantity"`
	UnitPrice          money.Money `json:"unit_price"`
	Tier               *Tier       `json:"tier"`
	DiscountPercent    string      `json:"discount_percent"`
	Total              money.Money `json:"total"`
	Reason             string      `json:"reason"`
}
