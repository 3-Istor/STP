package supplier

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

// Service possède les validations métier ; ni pgx, ni réseau, ni horloge implicite ici.
type Service struct{ repo Repository }

func NewService(repo Repository) *Service { return &Service{repo: repo} }

func (s *Service) Supplier(ctx context.Context, id int64) (Supplier, error) {
	if id <= 0 {
		return Supplier{}, invalid("supplier id must be positive")
	}
	p, err := s.repo.Supplier(ctx, id)
	if err != nil {
		return Supplier{}, fmt.Errorf("get supplier %d: %w", id, err)
	}
	return p, nil
}
func (s *Service) ListSuppliers(ctx context.Context) ([]Supplier, error) {
	p, err := s.repo.ListSuppliers(ctx)
	if err != nil {
		return nil, fmt.Errorf("list suppliers: %w", err)
	}
	if p == nil {
		p = []Supplier{}
	}
	return p, nil
}
func (s *Service) CreateSupplier(ctx context.Context, in SupplierInput) (Supplier, error) {
	p, err := patchSupplier(Supplier{}, SupplierPatch{Name: &in.Name, QualityScore: &in.QualityScore, TrustScore: &in.TrustScore})
	if err != nil {
		return Supplier{}, fmt.Errorf("create supplier: %w", err)
	}
	err = s.repo.WithinTx(ctx, func(tx TxRepository) error { var e error; p, e = tx.InsertSupplier(ctx, p); return e })
	if err != nil {
		return Supplier{}, fmt.Errorf("create supplier: %w", err)
	}
	return p, nil
}
func (s *Service) UpdateSupplier(ctx context.Context, id int64, patch SupplierPatch) (Supplier, error) {
	if id <= 0 {
		return Supplier{}, invalid("supplier id must be positive")
	}
	var result Supplier
	err := s.repo.WithinTx(ctx, func(tx TxRepository) error {
		p, err := tx.SupplierForUpdate(ctx, id)
		if err != nil {
			return err
		}
		p, err = patchSupplier(p, patch)
		if err != nil {
			return err
		}
		result, err = tx.ReplaceSupplier(ctx, p)
		return err
	})
	if err != nil {
		return Supplier{}, fmt.Errorf("update supplier %d: %w", id, err)
	}
	return result, nil
}
func (s *Service) Contract(ctx context.Context, id int64) (Contract, error) {
	if id <= 0 {
		return Contract{}, invalid("contract id must be positive")
	}
	c, err := s.repo.Contract(ctx, id)
	if err != nil {
		return Contract{}, fmt.Errorf("get contract %d: %w", id, err)
	}
	return c, nil
}
func (s *Service) ListContracts(ctx context.Context, supplierID int64) ([]Contract, error) {
	if _, err := s.Supplier(ctx, supplierID); err != nil {
		return nil, err
	}
	c, err := s.repo.ListContracts(ctx, supplierID)
	if err != nil {
		return nil, fmt.Errorf("list contracts of supplier %d: %w", supplierID, err)
	}
	if c == nil {
		c = []Contract{}
	}
	return c, nil
}
func (s *Service) CreateContract(ctx context.Context, supplierID int64, in ContractInput) (Contract, error) {
	if supplierID <= 0 {
		return Contract{}, invalid("supplier id must be positive")
	}
	c, err := contractFromInput(supplierID, in)
	if err != nil {
		return Contract{}, fmt.Errorf("create contract: %w", err)
	}
	err = s.repo.WithinTx(ctx, func(tx TxRepository) error {
		if _, err := tx.Supplier(ctx, supplierID); err != nil {
			return err
		}
		if err := tx.CheckOffers(ctx, supplierID, c.OfferIDs); err != nil {
			return err
		}
		var e error
		c, e = tx.InsertContract(ctx, c)
		return e
	})
	if err != nil {
		return Contract{}, fmt.Errorf("create contract for supplier %d: %w", supplierID, err)
	}
	return c, nil
}
func (s *Service) UpdateContract(ctx context.Context, id int64, patch ContractPatch) (Contract, error) {
	if id <= 0 {
		return Contract{}, invalid("contract id must be positive")
	}
	var result Contract
	err := s.repo.WithinTx(ctx, func(tx TxRepository) error {
		c, err := tx.ContractForUpdate(ctx, id)
		if err != nil {
			return err
		}
		oldOfferIDs := c.OfferIDs
		c, err = patchContract(c, patch)
		if err != nil {
			return err
		}
		if patch.OfferIDs != nil {
			oldIDs := make(map[int64]bool, len(oldOfferIDs))
			for _, offerID := range oldOfferIDs {
				oldIDs[offerID] = true
			}
			var additions []int64
			for _, offerID := range c.OfferIDs {
				if !oldIDs[offerID] {
					additions = append(additions, offerID)
				}
			}
			if err := tx.CheckOffers(ctx, c.SupplierID, additions); err != nil {
				return err
			}
		}
		result, err = tx.ReplaceContract(ctx, c)
		return err
	})
	if err != nil {
		return Contract{}, fmt.Errorf("update contract %d: %w", id, err)
	}
	return result, nil
}

func invalid(message string) error { return fmt.Errorf("%s: %w", message, ErrInvalidInput) }
func score(field, value string) (*decimal.Decimal, error) {
	d, err := decimal.NewFromString(value)
	if err != nil || d.IsNegative() || d.GreaterThan(decimal.NewFromInt(5)) {
		return nil, invalid(field + " must be a decimal string between 0 and 5")
	}
	return &d, nil
}
func patchSupplier(p Supplier, patch SupplierPatch) (Supplier, error) {
	if patch.Name != nil {
		if strings.TrimSpace(*patch.Name) == "" {
			return Supplier{}, invalid("name is required")
		}
		p.Name = *patch.Name
	}
	var err error
	if patch.QualityScore != nil {
		p.QualityScore, err = score("quality_score", *patch.QualityScore)
		if err != nil {
			return Supplier{}, err
		}
	}
	if patch.TrustScore != nil {
		p.TrustScore, err = score("trust_score", *patch.TrustScore)
		if err != nil {
			return Supplier{}, err
		}
	}
	return p, nil
}
func parseDate(field, value string) (time.Time, error) {
	d, err := time.Parse(time.DateOnly, value)
	if err != nil || d.Year() < 1 || d.Format(time.DateOnly) != value {
		return time.Time{}, invalid(field + " must be a real YYYY-MM-DD date")
	}
	return d, nil
}
func contractFromInput(supplierID int64, in ContractInput) (Contract, error) {
	return patchContract(Contract{SupplierID: supplierID}, ContractPatch{StartsOn: &in.StartsOn, EndsOn: &in.EndsOn, Conditions: &in.Conditions, OfferIDs: &in.OfferIDs, Tiers: &in.Tiers})
}
func patchContract(c Contract, patch ContractPatch) (Contract, error) {
	if patch.StartsOn != nil {
		c.StartsOn = *patch.StartsOn
	}
	if patch.EndsOn != nil {
		c.EndsOn = *patch.EndsOn
	}
	if patch.Conditions != nil {
		c.Conditions = *patch.Conditions
	}
	start, err := parseDate("starts_on", c.StartsOn)
	if err != nil {
		return Contract{}, err
	}
	end, err := parseDate("ends_on", c.EndsOn)
	if err != nil {
		return Contract{}, err
	}
	if start.After(end) {
		return Contract{}, invalid("starts_on must not exceed ends_on")
	}
	if patch.OfferIDs != nil {
		c.OfferIDs = append([]int64{}, (*patch.OfferIDs)...)
		sort.Slice(c.OfferIDs, func(i, j int) bool { return c.OfferIDs[i] < c.OfferIDs[j] })
		for i, id := range c.OfferIDs {
			if id <= 0 {
				return Contract{}, invalid("offer_ids must be positive")
			}
			if i > 0 && id == c.OfferIDs[i-1] {
				return Contract{}, invalid("offer_ids must be unique")
			}
		}
	}
	if patch.Tiers != nil {
		c.Tiers = make([]Tier, 0, len(*patch.Tiers))
		for _, in := range *patch.Tiers {
			q, err := decimal.NewFromString(in.MinQuantity)
			if err != nil || !q.IsPositive() {
				return Contract{}, invalid("min_quantity must be a positive decimal string")
			}
			d, err := decimal.NewFromString(in.DiscountPercent)
			if err != nil || d.IsNegative() || d.GreaterThan(decimal.NewFromInt(100)) {
				return Contract{}, invalid("discount_percent must be a decimal string between 0 and 100")
			}
			c.Tiers = append(c.Tiers, Tier{MinQuantity: q, DiscountPercent: d})
		}
		sort.Slice(c.Tiers, func(i, j int) bool { return c.Tiers[i].MinQuantity.LessThan(c.Tiers[j].MinQuantity) })
		for i := 1; i < len(c.Tiers); i++ {
			if c.Tiers[i].MinQuantity.Equal(c.Tiers[i-1].MinQuantity) {
				return Contract{}, invalid("tier quantities must be unique")
			}
		}
	}
	if c.OfferIDs == nil {
		c.OfferIDs = []int64{}
	}
	if c.Tiers == nil {
		c.Tiers = []Tier{}
	}
	return c, nil
}
