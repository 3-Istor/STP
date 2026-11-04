package supplier

import (
	"context"
	"fmt"
	"time"

	"github.com/shopspring/decimal"

	"bomstudio/internal/money"
)

// EvaluateOffer applique les contrats liés à l'offre ; le prix et la quantité viennent de l'appelant.
// Il ne calcule ni l'arbre, ni le volume, ni la conversion de devises (voir ADR 0007).
func (s *Service) EvaluateOffer(ctx context.Context, in EvaluationInput) (Evaluation, error) {
	if in.OfferID <= 0 {
		return Evaluation{}, invalid("offer id must be positive")
	}
	contracts, err := s.repo.ListOfferContracts(ctx, in.OfferID)
	if err != nil {
		return Evaluation{}, fmt.Errorf("contracts for offer %d: %w", in.OfferID, err)
	}
	return EvaluateContracts(contracts, in)
}

// EvaluateContracts est un calcul pur, explicable, sans arrondi intermédiaire ni heure courante.
func EvaluateContracts(contracts []Contract, in EvaluationInput) (Evaluation, error) {
	quantity, err := decimal.NewFromString(in.Quantity)
	if err != nil || !quantity.IsPositive() {
		return Evaluation{}, invalid("quantity must be a positive decimal string")
	}
	if in.OfferID <= 0 || in.AsOf.IsZero() || in.AsOf.UTC().Year() < 1 || in.AsOf.UTC().Year() > 9999 {
		return Evaluation{}, invalid("offer id and as_of are required")
	}
	if _, err := money.ParseCurrency(string(in.UnitPrice.Currency())); err != nil || !in.UnitPrice.Amount().IsPositive() {
		return Evaluation{}, invalid("unit price must be a positive amount with a supported currency")
	}
	dateText := in.AsOf.UTC().Format(time.DateOnly)
	result := Evaluation{OfferID: in.OfferID, SelectedContractID: in.SelectedContractID, AsOfDate: dateText, Quantity: quantity.String(), UnitPrice: in.UnitPrice, Total: in.UnitPrice.Mul(quantity), DiscountPercent: "0", Reason: "no_active_contract"}
	var selected *Contract
	var active []Contract
	for _, c := range contracts {
		// Ne pas rendre une liste de contrats étrangers applicable au seul motif de sa validité.
		covers := false
		for _, id := range c.OfferIDs {
			if id == in.OfferID {
				covers = true
				break
			}
		}
		if !covers {
			continue
		}
		validated, err := validateStoredContract(c)
		if err != nil {
			return Evaluation{}, err
		}
		status, err := ContractStatus(validated, in.AsOf)
		if err != nil {
			return Evaluation{}, err
		}
		if in.SelectedContractID != nil && c.ID == *in.SelectedContractID {
			copyContract := validated
			selected = &copyContract
		}
		if status == "active" {
			active = append(active, validated)
		}
	}
	if in.SelectedContractID != nil {
		if selected == nil {
			return Evaluation{}, invalid("selected contract does not cover the offer")
		}
		status, err := ContractStatus(*selected, in.AsOf)
		if err != nil {
			return Evaluation{}, err
		}
		if status == "not_started" {
			result.Reason = "contract_not_started"
			return result, nil
		}
		if status == "expired" {
			result.Reason = "contract_expired"
			return result, nil
		}
	} else {
		if len(active) > 1 {
			return Evaluation{}, ErrSelectionRequired
		}
		if len(active) == 0 {
			return result, nil
		}
		selected = &active[0]
	}
	id := selected.ID
	result.AppliedContractID = &id
	var reached *Tier
	for _, tier := range selected.Tiers {
		if tier.MinQuantity.LessThanOrEqual(quantity) && (reached == nil || tier.MinQuantity.GreaterThan(reached.MinQuantity)) {
			copyTier := tier
			reached = &copyTier
		}
	}
	if reached == nil {
		result.Reason = "below_first_tier"
		return result, nil
	}
	result.Tier = reached
	result.DiscountPercent = reached.DiscountPercent.String()
	result.Reason = "discount_applied"
	// Diviser par 100 est ici un déplacement décimal exact, jamais decimal.Div à précision implicite.
	factor := decimal.NewFromInt(1).Sub(reached.DiscountPercent.Shift(-2))
	result.Total = result.Total.Mul(factor)
	return result, nil
}

// ContractStatus applique les dates calendaires inclusives à l'instant métier fourni, en UTC.
func ContractStatus(c Contract, asOf time.Time) (string, error) {
	start, err := parseDate("starts_on", c.StartsOn)
	if err != nil {
		return "", err
	}
	end, err := parseDate("ends_on", c.EndsOn)
	if err != nil {
		return "", err
	}
	if start.After(end) {
		return "", invalid("starts_on must not exceed ends_on")
	}
	if asOf.IsZero() || asOf.UTC().Year() < 1 || asOf.UTC().Year() > 9999 {
		return "", invalid("as_of must be an explicit valid instant")
	}
	date := asOf.UTC().Format(time.DateOnly)
	if date < c.StartsOn {
		return "not_started", nil
	}
	if date > c.EndsOn {
		return "expired", nil
	}
	return "active", nil
}

func validateStoredContract(c Contract) (Contract, error) {
	tiers := make([]TierInput, 0, len(c.Tiers))
	for _, t := range c.Tiers {
		tiers = append(tiers, TierInput{MinQuantity: t.MinQuantity.String(), DiscountPercent: t.DiscountPercent.String()})
	}
	return patchContract(c, ContractPatch{Tiers: &tiers})
}
