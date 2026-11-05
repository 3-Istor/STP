package pricing

import (
	"errors"
	"strings"
	"testing"
)

func TestCoverageErrorsNameEveryGap(t *testing.T) {
	tests := []struct {
		name     string
		gaps     []CoverageGap
		want     []error
		notWant  []error
		mentions []string
	}{
		{"prix manquants", []CoverageGap{{Kind: "price", OfferID: 3}, {Kind: "price", OfferID: 7}}, []error{ErrNoQuote, ErrIncompletePriceSnapshot}, []error{ErrNoRate}, []string{"[3 7]"}},
		{"taux manquant", []CoverageGap{{Kind: "fx", Base: "USD", Quote: "JPY"}}, []error{ErrNoRate}, []error{ErrNoQuote}, []string{"USD->JPY"}},
		{"les deux", []CoverageGap{{Kind: "price", OfferID: 3}, {Kind: "fx", Base: "USD", Quote: "JPY"}}, []error{ErrNoQuote, ErrNoRate}, nil, []string{"[3]", "USD->JPY"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := coverageErrors(&CoverageError{Missing: tt.gaps})
			for _, w := range tt.want {
				if !errors.Is(err, w) {
					t.Errorf("err = %v, want %v", err, w)
				}
			}
			for _, w := range tt.notWant {
				if errors.Is(err, w) {
					t.Errorf("err = %v, must not wrap %v", err, w)
				}
			}
			for _, m := range tt.mentions {
				if !strings.Contains(err.Error(), m) {
					t.Errorf("err = %v, want %q in message", err, m)
				}
			}
		})
	}
}
