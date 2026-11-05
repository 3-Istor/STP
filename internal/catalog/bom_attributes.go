package catalog

import (
	"errors"
	"fmt"
	"strings"

	"github.com/shopspring/decimal"
)

// ErrMissingAttribute : un attribut nécessaire au calcul est NULL dans le catalogue. Jamais remplacé
// par zéro (AGENTS §3) : la pièce est signalée.
var ErrMissingAttribute = errors.New("missing part attribute")

// BOMAttributes sont les attributs d'une pièce dont dépendent les totaux et les snapshots de BOM
// (ADR 0007, ADR 0006).
type BOMAttributes struct {
	CO2KGPerUnit   decimal.Decimal
	MassKG         decimal.Decimal
	ProductionDays int
}

// BOMAttributes renvoie les attributs de calcul de la pièce, ou ErrMissingAttribute en listant ceux qui
// sont absents (anciennes lignes du catalogue, ADR 0009).
func (p Part) BOMAttributes() (BOMAttributes, error) {
	var missing []string
	if p.CO2KGPerUnit == nil {
		missing = append(missing, "co2_kg_per_unit")
	}
	if p.MassKG == nil {
		missing = append(missing, "mass_kg")
	}
	if p.ProductionDays == nil {
		missing = append(missing, "production_days")
	}
	if len(missing) > 0 {
		return BOMAttributes{}, fmt.Errorf("part %d: %s: %w", p.ID, strings.Join(missing, ", "), ErrMissingAttribute)
	}
	return BOMAttributes{CO2KGPerUnit: *p.CO2KGPerUnit, MassKG: *p.MassKG, ProductionDays: *p.ProductionDays}, nil
}
