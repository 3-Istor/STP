package pricing

import (
	"context"
	"errors"
	"fmt"
	"time"

	"bomstudio/internal/money"
)

var (
	ErrNoReference           = errors.New("no price quote before annual reference boundary")
	ErrNoWindow              = errors.New("no price quote inside annual window")
	ErrInvalidHistoryRequest = errors.New("invalid price history request")
	ErrInvalidHistoryData    = errors.New("invalid price history data")
)

// HistoricalQuote identifie exactement la cotation immuable utilisée (ADR 0011).
// HistoryWindow : fenêtre [From, To] des cotations à lire pour une offre (lecture par lot, ReadWindows).
type HistoryWindow struct {
	OfferID  int64
	From, To time.Time
}

type HistoricalQuote struct {
	ID       int64       `json:"id"`
	OfferID  int64       `json:"offer_id"`
	Price    money.Money `json:"price"`
	Source   string      `json:"source"`
	QuotedAt time.Time   `json:"quoted_at"`
}

// Statistics garde les données disponibles même si une référence ou une fenêtre manque.
// Les pointeurs nil s'encodent en null ; aucun montant ou pourcentage manquant ne devient zéro.
type Statistics struct {
	OfferID          int64             `json:"offer_id"`
	AsOf             time.Time         `json:"as_of"`
	WindowStart      time.Time         `json:"window_start"`
	CurrentQuote     *HistoricalQuote  `json:"current_quote"`
	ReferenceQuote   *HistoricalQuote  `json:"reference_quote"`
	PeriodStart      *time.Time        `json:"period_start"`
	PeriodEnd        *time.Time        `json:"period_end"`
	Minimum          *money.Money      `json:"minimum"`
	Maximum          *money.Money      `json:"maximum"`
	VariationPercent *string           `json:"variation_percent"`
	Trend            *string           `json:"trend"`
	History          []HistoricalQuote `json:"history"`
}

// History est indépendante de Pricing : ses consommateurs existants ne changent pas.
// ReadHistory restitue les cotations de [from,to] et la dernière strictement avant from,
// lues dans le même instantané de requête. CurrentAsOf observe la date maximale persistée.
type History interface {
	CurrentAsOf(context.Context) (time.Time, error)
	ReadHistory(context.Context, int64, time.Time, time.Time) ([]HistoricalQuote, error)
}

type HistoryService struct {
	history History
}

func NewHistoryService(history History) *HistoryService {
	return &HistoryService{history: history}
}

// CurrentAsOf ne lit aucune horloge réelle : il expose la date des cotations connues.
func (s *HistoryService) CurrentAsOf(ctx context.Context) (time.Time, error) {
	at, err := s.history.CurrentAsOf(ctx)
	if err != nil {
		return time.Time{}, fmt.Errorf("read current quote observation time: %w", err)
	}
	if at.IsZero() {
		return time.Time{}, fmt.Errorf("zero quote observation time: %w", ErrInvalidHistoryData)
	}
	return at.UTC(), nil
}

func (s *HistoryService) Statistics(ctx context.Context, offerID int64, asOf time.Time) (Statistics, error) {
	if offerID <= 0 || asOf.IsZero() {
		return Statistics{}, fmt.Errorf("positive offer id and explicit as_of are required: %w", ErrInvalidHistoryRequest)
	}
	quotes, err := s.history.ReadHistory(ctx, offerID, AnnualWindowStart(asOf), asOf.UTC())
	if err != nil {
		return Statistics{}, fmt.Errorf("read history for offer %d: %w", offerID, err)
	}
	result, err := CalculateStatistics(offerID, asOf, quotes)
	if err != nil {
		return result, fmt.Errorf("calculate statistics for offer %d: %w", offerID, err)
	}
	return result, nil
}

// StatisticsForQuote conserve la cotation choisie par un snapshot, même si un autre prix
// a été inséré plus tard avec la même date. Le snapshot ne se reconstruit jamais par Latest.
func (s *HistoryService) StatisticsForQuote(ctx context.Context, offerID int64, asOf time.Time, current HistoricalQuote) (Statistics, error) {
	if offerID <= 0 || current.OfferID != offerID || asOf.IsZero() || current.QuotedAt.After(asOf) {
		return Statistics{}, ErrInvalidHistoryRequest
	}
	quotes, err := s.history.ReadHistory(ctx, offerID, AnnualWindowStart(asOf), asOf.UTC())
	if err != nil {
		return Statistics{}, err
	}
	eligible := make([]HistoricalQuote, 0, len(quotes)+1)
	for _, q := range quotes {
		if q.QuotedAt.Before(current.QuotedAt) {
			eligible = append(eligible, q)
		}
	}
	eligible = append(eligible, current)
	return CalculateStatistics(offerID, asOf, eligible)
}
