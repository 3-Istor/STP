package catalog

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"bomstudio/internal/money"
)

// Service valide le catalogue sans dépendance au stockage, au réseau ou à une horloge implicite.
// clock est fourni par le câblage ; il sert uniquement à dater l'archivage.
type Service struct {
	repo  Repository
	clock func() time.Time
}

func NewService(repo Repository, clock func() time.Time) *Service {
	return &Service{repo: repo, clock: clock}
}

var _ Catalog = (*Service)(nil)

// Part garde la lecture des pièces archivées pour les BOM existantes.
func (s *Service) Part(ctx context.Context, id int64) (Part, error) {
	if id <= 0 {
		return Part{}, invalid("part id must be positive")
	}
	p, err := s.repo.Part(ctx, id)
	if err != nil {
		return Part{}, fmt.Errorf("get part %d: %w", id, err)
	}
	return p, nil
}

// Offer garde la lecture des offres archivées pour les BOM existantes.
func (s *Service) Offer(ctx context.Context, id int64) (Offer, error) {
	if id <= 0 {
		return Offer{}, invalid("offer id must be positive")
	}
	o, err := s.repo.Offer(ctx, id)
	if err != nil {
		return Offer{}, fmt.Errorf("get offer %d: %w", id, err)
	}
	return o, nil
}

func (s *Service) ListParts(ctx context.Context, filters Filters) ([]PartWithOffers, error) {
	if filters.SupplierID != nil && *filters.SupplierID <= 0 {
		return nil, invalid("supplier id must be positive")
	}
	parts, err := s.repo.ListParts(ctx, filters)
	if err != nil {
		return nil, fmt.Errorf("list parts: %w", err)
	}
	if parts == nil {
		parts = []PartWithOffers{}
	}
	for i := range parts {
		if parts[i].Offers == nil {
			parts[i].Offers = []Offer{}
		}
	}
	return parts, nil
}

func (s *Service) ListSupplierOffers(ctx context.Context, supplierID int64) ([]Offer, error) {
	if supplierID <= 0 {
		return nil, invalid("supplier id must be positive")
	}
	offers, err := s.repo.ListSupplierOffers(ctx, supplierID)
	if err != nil {
		return nil, fmt.Errorf("list offers of supplier %d: %w", supplierID, err)
	}
	if offers == nil {
		offers = []Offer{}
	}
	return offers, nil
}

func (s *Service) CreatePart(ctx context.Context, in PartInput) (Part, error) {
	p, err := partFromInput(in)
	if err != nil {
		return Part{}, fmt.Errorf("create part: %w", err)
	}
	err = s.repo.WithinTx(ctx, func(tx TxRepository) error {
		var insertErr error
		p, insertErr = tx.InsertPart(ctx, p)
		return insertErr
	})
	if err != nil {
		return Part{}, fmt.Errorf("create part: %w", err)
	}
	return p, nil
}

func (s *Service) UpdatePart(ctx context.Context, id int64, patch PartPatch) (Part, error) {
	if id <= 0 {
		return Part{}, invalid("part id must be positive")
	}
	var result Part
	err := s.repo.WithinTx(ctx, func(tx TxRepository) error {
		p, err := tx.PartForUpdate(ctx, id)
		if err != nil {
			return err
		}
		p, err = patchPart(p, patch)
		if err != nil {
			return err
		}
		result, err = tx.ReplacePart(ctx, p)
		return err
	})
	if err != nil {
		return Part{}, fmt.Errorf("update part %d: %w", id, err)
	}
	return result, nil
}

func (s *Service) CreateOffer(ctx context.Context, partID int64, in OfferInput) (Offer, error) {
	if partID <= 0 {
		return Offer{}, invalid("part id must be positive")
	}
	o, err := offerFromInput(partID, in)
	if err != nil {
		return Offer{}, fmt.Errorf("create offer for part %d: %w", partID, err)
	}
	err = s.repo.WithinTx(ctx, func(tx TxRepository) error {
		p, err := tx.PartForUpdate(ctx, partID)
		if err != nil {
			return err
		}
		if p.ArchivedAt != nil {
			return ErrArchived
		}
		o, err = tx.InsertOffer(ctx, o)
		return err
	})
	if err != nil {
		return Offer{}, fmt.Errorf("create offer for part %d: %w", partID, err)
	}
	return o, nil
}

func (s *Service) UpdateOffer(ctx context.Context, id int64, patch OfferPatch) (Offer, error) {
	if id <= 0 {
		return Offer{}, invalid("offer id must be positive")
	}
	var result Offer
	err := s.repo.WithinTx(ctx, func(tx TxRepository) error {
		o, err := lockOffer(ctx, tx, id)
		if err != nil {
			return err
		}
		updated, err := patchOffer(o, patch)
		if err != nil {
			return err
		}
		if updated.Currency != o.Currency || updated.SupplierID != o.SupplierID {
			hasQuotes, err := tx.OfferHasQuotes(ctx, id)
			if err != nil {
				return err
			}
			if hasQuotes {
				return ErrQuotedOfferIdentityChange
			}
		}
		result, err = tx.ReplaceOffer(ctx, updated)
		return err
	})
	if err != nil {
		return Offer{}, fmt.Errorf("update offer %d: %w", id, err)
	}
	return result, nil
}

func (s *Service) Alternatives(ctx context.Context, id int64) ([]Part, error) {
	if _, err := s.Part(ctx, id); err != nil {
		return nil, err
	}
	parts, err := s.repo.Alternatives(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("list alternatives of part %d: %w", id, err)
	}
	if parts == nil {
		parts = []Part{}
	}
	return parts, nil
}

func (s *Service) AddAlternative(ctx context.Context, id, otherID int64) error {
	if id <= 0 || otherID <= 0 || id == otherID {
		return invalid("alternatives require two distinct positive part ids")
	}
	low, high := id, otherID
	if low > high {
		low, high = high, low
	}
	err := s.repo.WithinTx(ctx, func(tx TxRepository) error {
		for _, partID := range []int64{low, high} {
			p, err := tx.PartForUpdate(ctx, partID)
			if err != nil {
				return err
			}
			if p.ArchivedAt != nil {
				return ErrArchived
			}
		}
		return tx.InsertAlternative(ctx, low, high)
	})
	if err != nil {
		return fmt.Errorf("link alternatives %d/%d: %w", id, otherID, err)
	}
	return nil
}

func (s *Service) DeletePart(ctx context.Context, id int64) (DeleteResult, error) {
	if id <= 0 {
		return DeleteResult{}, invalid("part id must be positive")
	}
	var result DeleteResult
	// Une FK concurrente ne laisse aucune commande exploitable dans la transaction avortée.
	// Le second passage ouvre une nouvelle transaction et privilégie l'archivage (ADR 0006).
	for attempt := 0; attempt < 2; attempt++ {
		err := s.repo.WithinTx(ctx, func(tx TxRepository) error {
			p, err := tx.PartForUpdate(ctx, id)
			if err != nil {
				return err
			}
			if _, err := tx.OffersForPartForUpdate(ctx, id); err != nil {
				return err
			}
			referenced, err := tx.PartHasReferences(ctx, id)
			if err != nil {
				return err
			}
			result = DeleteResult{ID: id, Archived: p.ArchivedAt != nil || referenced || attempt > 0}
			if result.Archived {
				if p.ArchivedAt != nil {
					return nil
				}
				_, err = tx.ArchivePart(ctx, id, s.clock().UTC())
				return err
			}
			return tx.DeletePart(ctx, id)
		})
		if err == nil {
			return result, nil
		}
		if !errors.Is(err, ErrReferenceConflict) || attempt > 0 {
			return DeleteResult{}, fmt.Errorf("delete part %d: %w", id, err)
		}
	}
	return DeleteResult{}, fmt.Errorf("delete part %d: %w", id, ErrReferenceConflict)
}

func (s *Service) DeleteOffer(ctx context.Context, id int64) (DeleteResult, error) {
	if id <= 0 {
		return DeleteResult{}, invalid("offer id must be positive")
	}
	var result DeleteResult
	for attempt := 0; attempt < 2; attempt++ {
		err := s.repo.WithinTx(ctx, func(tx TxRepository) error {
			o, err := lockOffer(ctx, tx, id)
			if err != nil {
				return err
			}
			referenced, err := tx.OfferHasReferences(ctx, id)
			if err != nil {
				return err
			}
			result = DeleteResult{ID: id, Archived: o.ArchivedAt != nil || referenced || attempt > 0}
			if result.Archived {
				if o.ArchivedAt != nil {
					return nil
				}
				_, err = tx.ArchiveOffer(ctx, id, s.clock().UTC())
				return err
			}
			return tx.DeleteOffer(ctx, id)
		})
		if err == nil {
			return result, nil
		}
		if !errors.Is(err, ErrReferenceConflict) || attempt > 0 {
			return DeleteResult{}, fmt.Errorf("delete offer %d: %w", id, err)
		}
	}
	return DeleteResult{}, fmt.Errorf("delete offer %d: %w", id, ErrReferenceConflict)
}

// lockOffer découvre le parent sans verrou puis acquiert les verrous dans l'ordre commun pièce/offre.
// part_id est immuable après création ; toutes les écritures de l'offre verrouillent ce parent.
func lockOffer(ctx context.Context, tx TxRepository, id int64) (Offer, error) {
	o, err := tx.Offer(ctx, id)
	if err != nil {
		return Offer{}, err
	}
	if _, err = tx.PartForUpdate(ctx, o.PartID); err != nil {
		return Offer{}, err
	}
	return tx.OfferForUpdate(ctx, id)
}

func invalid(message string) error { return fmt.Errorf("%s: %w", message, ErrInvalidInput) }

func validText(field, value string) error {
	if strings.TrimSpace(value) == "" {
		return invalid(field + " is required")
	}
	return nil
}

func parseNonnegative(field, value string, maximum *decimal.Decimal) (*decimal.Decimal, error) {
	d, err := decimal.NewFromString(value)
	if err != nil {
		return nil, fmt.Errorf("%s must be a decimal string: %w", field, ErrInvalidInput)
	}
	if d.IsNegative() || (maximum != nil && d.GreaterThan(*maximum)) {
		return nil, invalid(field + " is outside its allowed range")
	}
	return &d, nil
}

func validMoney(field string, m money.Money) error {
	if _, err := money.ParseCurrency(string(m.Currency())); err != nil {
		return fmt.Errorf("%s currency: %w: %w", field, ErrInvalidInput, err)
	}
	if m.Amount().IsNegative() {
		return invalid(field + " must be nonnegative")
	}
	return nil
}
