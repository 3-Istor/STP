package catalog

import (
	"errors"
	"testing"

	"bomstudio/internal/money"
)

func TestNewStub(t *testing.T) {
	tests := []struct {
		name    string
		parts   []Part
		offers  []Offer
		wantErr bool
		wantIs  error
	}{
		{"valid", []Part{{ID: 1}}, []Offer{{ID: 10, PartID: 1}}, false, nil},
		{"duplicate part", []Part{{ID: 1}, {ID: 1}}, nil, true, nil},
		{"duplicate offer", []Part{{ID: 1}}, []Offer{{ID: 10, PartID: 1}, {ID: 10, PartID: 1}}, true, nil},
		{"offer of unknown part", []Part{{ID: 1}}, []Offer{{ID: 10, PartID: 2}}, true, ErrPartNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewStub(tt.parts, tt.offers)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantIs != nil && !errors.Is(err, tt.wantIs) {
				t.Fatalf("err = %v, want %v", err, tt.wantIs)
			}
		})
	}
}

func TestStubLookups(t *testing.T) {
	stub, err := NewStub([]Part{{ID: 1, Name: "Écrou"}}, []Offer{{ID: 10, PartID: 1, SupplierID: 7}})
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name    string
		lookup  func() (int64, error)
		wantID  int64
		wantErr error
	}{
		{"part found", func() (int64, error) { p, err := stub.Part(t.Context(), 1); return p.ID, err }, 1, nil},
		{"part missing", func() (int64, error) { p, err := stub.Part(t.Context(), 2); return p.ID, err }, 0, ErrPartNotFound},
		{"offer found", func() (int64, error) { o, err := stub.Offer(t.Context(), 10); return o.ID, err }, 10, nil},
		{"offer missing", func() (int64, error) { o, err := stub.Offer(t.Context(), 11); return o.ID, err }, 0, ErrOfferNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id, err := tt.lookup()
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if id != tt.wantID {
				t.Fatalf("id = %d, want %d", id, tt.wantID)
			}
		})
	}
}

func TestStubPreservesPartAndOfferAttributes(t *testing.T) {
	part := Part{ID: 1, Name: "Écrou moteur", Category: "Fixation", Zone: "moteur"}
	tests := []struct {
		name     string
		offerID  int64
		supplier int64
		currency string
		min, max string
		days     int
		origin   string
	}{
		{"premier fournisseur en USD", 10, 7, "USD", "0.005", "9007199254740993.123456789", 3, "US"},
		{"même pièce chez un autre fournisseur en JPY", 11, 8, "JPY", "100.5", "200.75", 12, "JP"},
	}
	offers := make([]Offer, 0, len(tests))
	for _, tt := range tests {
		minPrice, err := money.Parse(tt.min, tt.currency)
		if err != nil {
			t.Fatal(err)
		}
		maxPrice, err := money.Parse(tt.max, tt.currency)
		if err != nil {
			t.Fatal(err)
		}
		offers = append(offers, Offer{
			ID: tt.offerID, PartID: part.ID, SupplierID: tt.supplier,
			Currency: money.Currency(tt.currency), PriceMin: minPrice, PriceMax: maxPrice,
			LeadTimeDays: tt.days, OriginCountry: tt.origin,
		})
	}
	stub, err := NewStub([]Part{part}, offers)
	if err != nil {
		t.Fatal(err)
	}
	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotPart, err := stub.Part(t.Context(), part.ID)
			if err != nil {
				t.Fatal(err)
			}
			if gotPart.ID != part.ID || gotPart.Name != part.Name || gotPart.Category != part.Category || gotPart.Zone != part.Zone {
				t.Errorf("pièce = %+v, want %+v", gotPart, part)
			}
			got, err := stub.Offer(t.Context(), tt.offerID)
			if err != nil {
				t.Fatal(err)
			}
			if got.ID != tt.offerID || got.PartID != part.ID || got.SupplierID != tt.supplier || got.Currency != money.Currency(tt.currency) {
				t.Errorf("identité/devise d'offre = %+v, want offre %d, pièce %d, fournisseur %d, devise %s", got, tt.offerID, part.ID, tt.supplier, tt.currency)
			}
			if !got.PriceMin.Amount().Equal(offers[i].PriceMin.Amount()) || !got.PriceMax.Amount().Equal(offers[i].PriceMax.Amount()) ||
				got.PriceMin.Currency() != money.Currency(tt.currency) || got.PriceMax.Currency() != money.Currency(tt.currency) {
				t.Errorf("bornes = %s / %s, want %s / %s %s sans arrondi", got.PriceMin, got.PriceMax, tt.min, tt.max, tt.currency)
			}
			if got.LeadTimeDays != tt.days || got.OriginCountry != tt.origin {
				t.Errorf("délai/origine = %d/%s, want %d/%s", got.LeadTimeDays, got.OriginCountry, tt.days, tt.origin)
			}
		})
	}
}
