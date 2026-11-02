package catalog

import (
	"errors"
	"testing"

	"github.com/shopspring/decimal"
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
		{"valid attributes", []Part{{ID: 1, CO2Kg: decimal.RequireFromString("2.5"), MassKg: decimal.RequireFromString("0.8"), ProductionDays: 2}}, nil, false, nil},
		{"negative CO2", []Part{{ID: 1, CO2Kg: decimal.RequireFromString("-0.1")}}, nil, true, nil},
		{"negative mass", []Part{{ID: 1, MassKg: decimal.RequireFromString("-1")}}, nil, true, nil},
		{"negative production days", []Part{{ID: 1, ProductionDays: -1}}, nil, true, nil},
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
	stub, err := NewStub(
		[]Part{{ID: 1, Name: "Écrou", CO2Kg: decimal.RequireFromString("0.05"), ProductionDays: 3}},
		[]Offer{{ID: 10, PartID: 1, SupplierID: 7}},
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Run("attributes are returned unchanged", func(t *testing.T) {
		p, err := stub.Part(t.Context(), 1)
		if err != nil {
			t.Fatal(err)
		}
		if !p.CO2Kg.Equal(decimal.RequireFromString("0.05")) || p.ProductionDays != 3 {
			t.Fatalf("part = %+v, want CO2 0.05 kg and 3 production days", p)
		}
	})
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
