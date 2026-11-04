package pricing

import (
	"fmt"
	"math/big"
	"sort"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"bomstudio/internal/money"
)

// AnnualWindowStart retire douze mois calendaires en UTC et rabat le 29 février
// au 28 février. AddDate seul normaliserait ce cas vers mars (voir ADR 0008).
func AnnualWindowStart(asOf time.Time) time.Time {
	at := asOf.UTC()
	year, month, day := at.Date()
	lastDay := time.Date(year-1, month+1, 0, 0, 0, 0, 0, time.UTC).Day()
	if day > lastDay {
		day = lastDay
	}
	return time.Date(year-1, month, day, at.Hour(), at.Minute(), at.Second(), at.Nanosecond(), time.UTC)
}

// CalculateStatistics est pure : aucune horloge, base ou précision décimale globale.
// Elle ne modifie pas l'entrée et reste déterministe indépendamment de son ordre.
func CalculateStatistics(offerID int64, asOf time.Time, quotes []HistoricalQuote) (Statistics, error) {
	if offerID <= 0 || asOf.IsZero() {
		return Statistics{}, fmt.Errorf("positive offer id and explicit as_of are required: %w", ErrInvalidHistoryRequest)
	}
	at := asOf.UTC()
	start := AnnualWindowStart(at)
	result := Statistics{OfferID: offerID, AsOf: at, WindowStart: start, History: []HistoricalQuote{}}
	eligible := make([]HistoricalQuote, 0, len(quotes))
	seen := make(map[int64]struct{}, len(quotes))
	var currency money.Currency
	for _, quote := range quotes {
		if quote.QuotedAt.After(at) {
			continue
		}
		if err := validateHistoricalQuote(offerID, quote, currency); err != nil {
			return result, err
		}
		if _, duplicate := seen[quote.ID]; duplicate {
			return result, fmt.Errorf("duplicate quote id %d: %w", quote.ID, ErrInvalidHistoryData)
		}
		seen[quote.ID] = struct{}{}
		currency = quote.Price.Currency()
		quote.QuotedAt = quote.QuotedAt.UTC()
		eligible = append(eligible, quote)
	}
	if len(eligible) == 0 {
		return result, ErrNoQuote
	}
	sort.Slice(eligible, func(i, j int) bool {
		if eligible[i].QuotedAt.Equal(eligible[j].QuotedAt) {
			return eligible[i].ID < eligible[j].ID
		}
		return eligible[i].QuotedAt.Before(eligible[j].QuotedAt)
	})
	result.CurrentQuote = &eligible[len(eligible)-1]
	end := result.CurrentQuote.QuotedAt
	result.PeriodEnd = &end
	for i := range eligible {
		quote := &eligible[i]
		if quote.QuotedAt.Before(start) {
			result.ReferenceQuote = quote
			continue
		}
		result.History = append(result.History, *quote)
		if result.Minimum == nil || quote.Price.Amount().LessThan(result.Minimum.Amount()) {
			price := quote.Price
			result.Minimum = &price
		}
		if result.Maximum == nil || quote.Price.Amount().GreaterThan(result.Maximum.Amount()) {
			price := quote.Price
			result.Maximum = &price
		}
	}
	if len(result.History) == 0 {
		// Le prix courant connu reste disponible ; il ne devient pas un extrême annuel.
		return result, ErrNoWindow
	}
	if result.ReferenceQuote == nil {
		first := result.History[0]
		if !first.QuotedAt.After(start) {
			return result, ErrNoReference
		}
		// Historique récent : première date connue, id le plus élevé à cette date.
		for _, quote := range result.History[1:] {
			if !quote.QuotedAt.Equal(first.QuotedAt) {
				break
			}
			first = quote
		}
		result.ReferenceQuote = &first
	}
	begin := result.ReferenceQuote.QuotedAt
	result.PeriodStart = &begin
	difference := result.CurrentQuote.Price.Amount().Sub(result.ReferenceQuote.Price.Amount())
	trend := "stable"
	if difference.IsPositive() {
		trend = "up"
	} else if difference.IsNegative() {
		trend = "down"
	}
	variation := roundedVariation(difference, result.ReferenceQuote.Price.Amount())
	result.VariationPercent = &variation
	result.Trend = &trend
	return result, nil
}

func validateHistoricalQuote(offerID int64, quote HistoricalQuote, currency money.Currency) error {
	if quote.ID <= 0 || quote.OfferID != offerID || quote.QuotedAt.IsZero() || strings.TrimSpace(quote.Source) == "" {
		return fmt.Errorf("quote %d has invalid identity, date or source: %w", quote.ID, ErrInvalidHistoryData)
	}
	if _, err := money.ParseCurrency(string(quote.Price.Currency())); err != nil {
		return fmt.Errorf("quote %d currency: %w: %w", quote.ID, ErrInvalidHistoryData, err)
	}
	if !quote.Price.Amount().IsPositive() {
		return fmt.Errorf("quote %d must have a positive price: %w", quote.ID, ErrInvalidHistoryData)
	}
	if currency != "" && currency != quote.Price.Currency() {
		return fmt.Errorf("quote %d changes currency inside one offer history: %w", quote.ID, ErrInvalidHistoryData)
	}
	return nil
}

// roundedVariation calcule difference/reference×100 puis arrondit directement à 0,1 %.
// Le quotient et le reste entiers évitent tout Div et tout arrondi intermédiaire implicite.
func roundedVariation(difference, reference decimal.Decimal) string {
	ratio := new(big.Rat).Quo(difference.Rat(), reference.Rat())
	numerator := new(big.Int).Mul(ratio.Num(), big.NewInt(1000))
	negative := numerator.Sign() < 0
	numerator.Abs(numerator)
	quotient, remainder := new(big.Int), new(big.Int)
	quotient.QuoRem(numerator, ratio.Denom(), remainder)
	twiceRemainder := new(big.Int).Lsh(remainder, 1)
	comparison := twiceRemainder.Cmp(ratio.Denom())
	if comparison > 0 || (comparison == 0 && quotient.Bit(0) == 1) {
		quotient.Add(quotient, big.NewInt(1))
	}
	digits := quotient.String()
	if len(digits) < 2 {
		digits = "0" + digits
	}
	formatted := digits[:len(digits)-1] + "." + digits[len(digits)-1:]
	if negative && quotient.Sign() != 0 {
		formatted = "-" + formatted
	}
	return formatted
}
