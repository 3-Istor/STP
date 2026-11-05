package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"bomstudio/internal/money"
)

func ptr[T any](v T) *T { return &v }

func testMoney(t *testing.T, amount, currency string) money.Money {
	t.Helper()
	m, err := money.Parse(amount, currency)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func completePart(t *testing.T) PartInput {
	t.Helper()
	return PartInput{
		Name: "Moteur", Category: "Motorisation", Zone: "moteur",
		MassKG: "9007199254740993.000000000000000001", CO2KGPerUnit: "0.12345678901234567890",
		RecycledPercent: "12.34567890123456789", RecyclabilityPercent: "100",
		ManufacturingCost: testMoney(t, "0.005", "EUR"), ProductionDays: ptr(0),
		LaborHours: "0.333333333333333333", HourlyRate: testMoney(t, "12.34567890123456789", "USD"),
	}
}

func completeOffer() OfferInput {
	return OfferInput{SupplierID: 7, Currency: "USD", PriceMin: "0.005", PriceMax: "100.1234567890123456789",
		LeadTimeDays: ptr(0), QualityScore: "5", OriginCountry: "US"}
}

func testClock() time.Time { return time.Date(2026, 11, 4, 11, 22, 33, 123456789, time.UTC) }

func TestCreatePartValidatesCompleteInput(t *testing.T) {
	tests := []struct {
		name   string
		change func(*PartInput)
		valid  bool
	}{
		{"complete", func(*PartInput) {}, true},
		{"name absent", func(p *PartInput) { p.Name = "  " }, false},
		{"category absent", func(p *PartInput) { p.Category = "" }, false},
		{"zone absent", func(p *PartInput) { p.Zone = "" }, false},
		{"mass absent", func(p *PartInput) { p.MassKG = "" }, false},
		{"negative mass", func(p *PartInput) { p.MassKG = "-0.001" }, false},
		{"mass NaN", func(p *PartInput) { p.MassKG = "NaN" }, false},
		{"infinite CO2", func(p *PartInput) { p.CO2KGPerUnit = "Infinity" }, false},
		{"negative CO2", func(p *PartInput) { p.CO2KGPerUnit = "-0.001" }, false},
		{"recycled above hundred", func(p *PartInput) { p.RecycledPercent = "100.0000000001" }, false},
		{"negative recyclability", func(p *PartInput) { p.RecyclabilityPercent = "-0.001" }, false},
		{"production days absent", func(p *PartInput) { p.ProductionDays = nil }, false},
		{"negative production days", func(p *PartInput) { p.ProductionDays = ptr(-1) }, false},
		{"labor absent", func(p *PartInput) { p.LaborHours = "" }, false},
		{"negative labor", func(p *PartInput) { p.LaborHours = "-1" }, false},
		{"manufacturing currency absent", func(p *PartInput) { p.ManufacturingCost = money.Money{} }, false},
		{"negative manufacturing cost", func(p *PartInput) { p.ManufacturingCost = testMoney(t, "-0.001", "EUR") }, false},
		{"hourly currency absent", func(p *PartInput) { p.HourlyRate = money.Money{} }, false},
		{"negative hourly rate", func(p *PartInput) { p.HourlyRate = testMoney(t, "-0.001", "USD") }, false},
		{"inclusive zero bounds", func(p *PartInput) {
			p.MassKG, p.CO2KGPerUnit, p.RecycledPercent, p.LaborHours = "0", "0", "0", "0"
			p.ManufacturingCost, p.HourlyRate = testMoney(t, "0", "EUR"), testMoney(t, "0", "JPY")
		}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newMemoryRepository()
			in := completePart(t)
			tt.change(&in)
			got, err := NewService(repo, testClock).CreatePart(t.Context(), in)
			if !tt.valid {
				if !errors.Is(err, ErrInvalidInput) {
					t.Fatalf("err = %v, want ErrInvalidInput", err)
				}
				if repo.transactions != 0 || len(repo.parts) != 0 {
					t.Fatal("invalid input reached persistence")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got.ID == 0 || got.Name != in.Name || got.ArchivedAt != nil {
				t.Fatalf("created part = %+v", got)
			}
			encoded, err := json.Marshal(got)
			if err != nil {
				t.Fatal(err)
			}
			var object map[string]json.RawMessage
			if err := json.Unmarshal(encoded, &object); err != nil {
				t.Fatal(err)
			}
			for _, field := range []string{"mass_kg", "co2_kg_per_unit", "recycled_percent", "recyclability_percent", "labor_hours"} {
				var decimalText string
				if err := json.Unmarshal(object[field], &decimalText); err != nil || decimalText == "" {
					t.Errorf("%s is not a decimal string: %s (%v)", field, object[field], err)
				}
			}
			if got.MassKG.String() != in.MassKG || got.ManufacturingCost.Currency() != in.ManufacturingCost.Currency() ||
				!got.ManufacturingCost.Amount().Equal(in.ManufacturingCost.Amount()) || got.HourlyRate.Currency() != in.HourlyRate.Currency() ||
				!got.HourlyRate.Amount().Equal(in.HourlyRate.Amount()) || *got.ProductionDays != 0 {
				t.Fatalf("precision, independent currencies or zero-day value lost: %+v", got)
			}
		})
	}
}

func TestUpdatePartPreservesAbsentLegacyFields(t *testing.T) {
	tests := []struct {
		name  string
		patch PartPatch
	}{
		{"empty patch", PartPatch{}},
		{"rename", PartPatch{Name: ptr("Nouveau nom")}},
		{"fill only mass", PartPatch{MassKG: ptr("0.0000000000000000001")}},
		{"fill zero days", PartPatch{ProductionDays: ptr(0)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newMemoryRepository()
			repo.parts[1] = Part{ID: 1, Name: "Ancienne pièce", Category: "", Zone: ""}
			got, err := NewService(repo, testClock).UpdatePart(t.Context(), 1, tt.patch)
			if err != nil {
				t.Fatal(err)
			}
			if got.CO2KGPerUnit != nil || got.ManufacturingCost != nil || got.HourlyRate != nil || got.RecycledPercent != nil || got.Category != "" {
				t.Fatalf("patch fabricated legacy data: %+v", got)
			}
			if (got.MassKG != nil) != (tt.patch.MassKG != nil) || (got.ProductionDays != nil) != (tt.patch.ProductionDays != nil) {
				t.Fatalf("absent field changed: %+v", got)
			}
			if tt.patch.Name != nil && got.Name != *tt.patch.Name {
				t.Fatalf("name = %q", got.Name)
			}
			if !reflect.DeepEqual(repo.events, []string{"part-lock:1"}) {
				t.Fatalf("patch did not lock part: %v", repo.events)
			}
		})
	}
}

func TestCreateOfferValidatesInput(t *testing.T) {
	tests := []struct {
		name   string
		change func(*OfferInput)
		valid  bool
	}{
		{"complete", func(*OfferInput) {}, true},
		{"supplier invalid", func(o *OfferInput) { o.SupplierID = 0 }, false},
		{"unknown currency", func(o *OfferInput) { o.Currency = "ZZZ" }, false},
		{"lowercase currency", func(o *OfferInput) { o.Currency = "usd" }, false},
		{"min exceeds max", func(o *OfferInput) { o.PriceMin, o.PriceMax = "2", "1" }, false},
		{"min absent", func(o *OfferInput) { o.PriceMin = "" }, false},
		{"zero min", func(o *OfferInput) { o.PriceMin = "0" }, false},
		{"negative max", func(o *OfferInput) { o.PriceMax = "-1" }, false},
		{"infinite max", func(o *OfferInput) { o.PriceMax = "Infinity" }, false},
		{"days absent", func(o *OfferInput) { o.LeadTimeDays = nil }, false},
		{"negative days", func(o *OfferInput) { o.LeadTimeDays = ptr(-1) }, false},
		{"quality absent", func(o *OfferInput) { o.QualityScore = "" }, false},
		{"quality above five", func(o *OfferInput) { o.QualityScore = "5.00001" }, false},
		{"negative quality", func(o *OfferInput) { o.QualityScore = "-0.00001" }, false},
		{"quality zero", func(o *OfferInput) { o.QualityScore = "0" }, true},
		{"unknown country", func(o *OfferInput) { o.OriginCountry = "ZZ" }, false},
		{"lowercase country", func(o *OfferInput) { o.OriginCountry = "fr" }, false},
		{"ISO territory", func(o *OfferInput) { o.OriginCountry = "AX" }, true},
		{"equal bounds", func(o *OfferInput) { o.PriceMin = o.PriceMax }, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newMemoryRepository()
			repo.parts[1] = Part{ID: 1, Name: "Pièce"}
			in := completeOffer()
			tt.change(&in)
			got, err := NewService(repo, testClock).CreateOffer(t.Context(), 1, in)
			if !tt.valid {
				if !errors.Is(err, ErrInvalidInput) {
					t.Fatalf("err = %v, want ErrInvalidInput", err)
				}
				if len(repo.offers) != 0 {
					t.Fatal("invalid offer was persisted")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got.PartID != 1 || got.SupplierID != in.SupplierID || string(got.Currency) != in.Currency || got.PriceMin.Amount().String() != in.PriceMin ||
				got.PriceMax.Amount().String() != in.PriceMax || got.QualityScore == nil || !got.QualityScore.Equal(decimal.RequireFromString(in.QualityScore)) || got.OriginCountry != in.OriginCountry {
				t.Fatalf("offer attributes changed: %+v", got)
			}
			if !reflect.DeepEqual(repo.events, []string{"part-lock:1"}) {
				t.Fatalf("parent was not locked: %v", repo.events)
			}
		})
	}
}

func TestUpdateOfferPreservesQuotedIdentity(t *testing.T) {
	tests := []struct {
		name    string
		quoted  bool
		patch   OfferPatch
		wantErr error
	}{
		{"quoted supplier change", true, OfferPatch{SupplierID: ptr(int64(8))}, ErrQuotedOfferIdentityChange},
		{"quoted currency change", true, OfferPatch{Currency: ptr("EUR")}, ErrQuotedOfferIdentityChange},
		{"quoted same supplier", true, OfferPatch{SupplierID: ptr(int64(7))}, nil},
		{"quoted same currency", true, OfferPatch{Currency: ptr("USD")}, nil},
		{"quoted other attribute", true, OfferPatch{LeadTimeDays: ptr(6)}, nil},
		{"unquoted supplier change", false, OfferPatch{SupplierID: ptr(int64(8))}, nil},
		{"unquoted currency change", false, OfferPatch{Currency: ptr("EUR")}, nil},
		{"partial lower bound invalid", false, OfferPatch{PriceMin: ptr("101")}, ErrInvalidInput},
		{"partial upper bound invalid", false, OfferPatch{PriceMax: ptr("0.001")}, ErrInvalidInput},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := repositoryWithOffer(t)
			repo.quoted = tt.quoted
			before := repo.offers[10]
			got, err := NewService(repo, testClock).UpdateOffer(t.Context(), 10, tt.patch)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if err != nil {
				if !reflect.DeepEqual(repo.offers[10], before) {
					t.Fatal("rejected patch changed offer")
				}
			} else {
				if tt.patch.SupplierID != nil && got.SupplierID != *tt.patch.SupplierID {
					t.Fatalf("supplier = %d", got.SupplierID)
				}
				if tt.patch.Currency != nil && (string(got.Currency) != *tt.patch.Currency || got.PriceMin.Currency() != got.Currency || got.PriceMax.Currency() != got.Currency) {
					t.Fatalf("inconsistent bounds/currency: %+v", got)
				}
				if tt.patch.LeadTimeDays != nil && got.LeadTimeDays != *tt.patch.LeadTimeDays {
					t.Fatalf("days = %d", got.LeadTimeDays)
				}
			}
			if !reflect.DeepEqual(repo.events, []string{"offer-read:10", "part-lock:1", "offer-lock:10"}) {
				t.Fatalf("wrong lock order: %v", repo.events)
			}
		})
	}
}

func TestNewAssociationsRejectArchivedParts(t *testing.T) {
	tests := []struct {
		name string
		run  func(*Service, context.Context) error
	}{
		{"new offer", func(s *Service, ctx context.Context) error {
			_, err := s.CreateOffer(ctx, 1, completeOffer())
			return err
		}},
		{"new alternative", func(s *Service, ctx context.Context) error { return s.AddAlternative(ctx, 2, 1) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newMemoryRepository()
			repo.parts[1], repo.parts[2] = Part{ID: 1, ArchivedAt: ptr(testClock())}, Part{ID: 2}
			if err := tt.run(NewService(repo, testClock), t.Context()); !errors.Is(err, ErrArchived) {
				t.Fatalf("err = %v, want ErrArchived", err)
			}
			if len(repo.offers) != 0 || len(repo.links) != 0 {
				t.Fatal("archived item received association")
			}
		})
	}
}

func TestAlternativeSymmetryAndCanonicalLockOrder(t *testing.T) {
	tests := []struct {
		name          string
		first, second int64
	}{
		{"forward then reverse", 1, 2}, {"reverse then forward", 2, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newMemoryRepository()
			repo.parts[1], repo.parts[2] = Part{ID: 1}, Part{ID: 2}
			svc := NewService(repo, testClock)
			if err := svc.AddAlternative(t.Context(), tt.first, tt.second); err != nil {
				t.Fatal(err)
			}
			if err := svc.AddAlternative(t.Context(), tt.second, tt.first); err != nil {
				t.Fatal(err)
			}
			if len(repo.links) != 1 || !repo.links[[2]int64{1, 2}] {
				t.Fatalf("noncanonical or duplicate links: %v", repo.links)
			}
			for id, other := range map[int64]int64{1: 2, 2: 1} {
				got, err := svc.Alternatives(t.Context(), id)
				if err != nil || len(got) != 1 || got[0].ID != other {
					t.Fatalf("alternatives %d = %+v, %v", id, got, err)
				}
			}
			if !reflect.DeepEqual(repo.events, []string{"part-lock:1", "part-lock:2", "part-lock:1", "part-lock:2"}) {
				t.Fatalf("canonical lock order lost: %v", repo.events)
			}
		})
	}
}

func TestDeleteArchivesReferencesAndRetriesWithNewTransaction(t *testing.T) {
	tests := []struct {
		name       string
		offer      bool
		referenced bool
		already    bool
		conflict   bool
	}{
		{"free part deleted", false, false, false, false},
		{"part with references archived", false, true, false, false},
		{"archived part retained", false, false, true, false},
		{"part concurrent reference archived", false, false, false, true},
		{"free offer deleted", true, false, false, false},
		{"offer with references archived", true, true, false, false},
		{"archived offer retained", true, false, true, false},
		{"offer concurrent reference archived", true, false, false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := repositoryWithOffer(t)
			if !tt.offer {
				delete(repo.offers, 10)
			}
			repo.referenced, repo.deleteConflict = tt.referenced, tt.conflict
			if tt.already {
				oldTime := testClock().Add(-24 * time.Hour)
				if tt.offer {
					o := repo.offers[10]
					o.ArchivedAt = &oldTime
					repo.offers[10] = o
				} else {
					p := repo.parts[1]
					p.ArchivedAt = &oldTime
					repo.parts[1] = p
				}
			}
			svc := NewService(repo, testClock)
			var got DeleteResult
			var err error
			if tt.offer {
				got, err = svc.DeleteOffer(t.Context(), 10)
			} else {
				got, err = svc.DeletePart(t.Context(), 1)
			}
			if err != nil {
				t.Fatal(err)
			}
			wantArchive := tt.referenced || tt.already || tt.conflict
			if got.Archived != wantArchive || got.ID == 0 {
				t.Fatalf("result = %+v, want archived=%v", got, wantArchive)
			}
			var at *time.Time
			if tt.offer {
				o, exists := repo.offers[10]
				at = o.ArchivedAt
				if exists != wantArchive {
					t.Fatal("wrong deletion outcome")
				}
			} else {
				p, exists := repo.parts[1]
				at = p.ArchivedAt
				if exists != wantArchive {
					t.Fatal("wrong deletion outcome")
				}
			}
			if wantArchive {
				wantTime := testClock()
				if tt.already {
					wantTime = wantTime.Add(-24 * time.Hour)
				}
				if at == nil || !at.Equal(wantTime) {
					t.Fatalf("archive date = %v, want %s", at, wantTime)
				}
			}
			wantTransactions := 1
			if tt.conflict {
				wantTransactions = 2
			}
			if repo.transactions != wantTransactions {
				t.Fatalf("transactions = %d, want %d", repo.transactions, wantTransactions)
			}
		})
	}
}

func TestPartReferencesAreArchivedWithoutDeletingChildren(t *testing.T) {
	tests := []struct {
		name        string
		alternative bool
	}{
		{"offer alone is a reference", false},
		{"alternative alone is a reference", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := repositoryWithOffer(t)
			if tt.alternative {
				delete(repo.offers, 10)
				repo.parts[2] = Part{ID: 2}
				repo.links[[2]int64{1, 2}] = true
			}
			got, err := NewService(repo, testClock).DeletePart(t.Context(), 1)
			if err != nil || !got.Archived {
				t.Fatalf("delete = %+v, %v", got, err)
			}
			if tt.alternative && !repo.links[[2]int64{1, 2}] {
				t.Fatal("part delete removed alternative")
			}
			if _, ok := repo.offers[10]; !tt.alternative && !ok {
				t.Fatal("part delete removed child offer")
			}
			if !reflect.DeepEqual(repo.events, []string{"part-lock:1", "part-offers-lock:1"}) {
				t.Fatalf("wrong lock order: %v", repo.events)
			}
		})
	}
}

func TestReadPreservesArchivedLegacyRecords(t *testing.T) {
	repo := repositoryWithOffer(t)
	p := repo.parts[1]
	p.ArchivedAt = ptr(testClock())
	repo.parts[1] = p
	o := repo.offers[10]
	o.ArchivedAt = ptr(testClock())
	repo.offers[10] = o
	svc := NewService(repo, testClock)
	tests := []struct {
		name string
		read func() error
	}{
		{"part", func() error {
			got, err := svc.Part(t.Context(), 1)
			if got.ArchivedAt == nil {
				t.Error("archive not readable")
			}
			return err
		}},
		{"offer", func() error {
			got, err := svc.Offer(t.Context(), 10)
			if got.ArchivedAt == nil {
				t.Error("archive not readable")
			}
			return err
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.read(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestServicePreservesNamedErrors(t *testing.T) {
	tests := []struct {
		name string
		run  func(*Service, context.Context) error
		want error
	}{
		{"missing part", func(s *Service, ctx context.Context) error { _, err := s.Part(ctx, 99); return err }, ErrPartNotFound},
		{"missing offer", func(s *Service, ctx context.Context) error { _, err := s.Offer(ctx, 99); return err }, ErrOfferNotFound},
		{"missing patch part", func(s *Service, ctx context.Context) error { _, err := s.UpdatePart(ctx, 99, PartPatch{}); return err }, ErrPartNotFound},
		{"missing delete offer", func(s *Service, ctx context.Context) error { _, err := s.DeleteOffer(ctx, 99); return err }, ErrOfferNotFound},
		{"self alternative", func(s *Service, ctx context.Context) error { return s.AddAlternative(ctx, 1, 1) }, ErrInvalidInput},
		{"invalid filter", func(s *Service, ctx context.Context) error {
			_, err := s.ListParts(ctx, Filters{SupplierID: ptr(int64(-1))})
			return err
		}, ErrInvalidInput},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.run(NewService(newMemoryRepository(), testClock), t.Context())
			if !errors.Is(err, tt.want) || !strings.Contains(err.Error(), tt.want.Error()) {
				t.Fatalf("err = %v, want wrapped %v", err, tt.want)
			}
		})
	}
}

// memoryRepository exerce les frontières du service ; les contraintes SQL sont testées séparément.
// Les événements rendent l'ordre de verrous observable et les transactions restaurent tout changement refusé.
type memoryRepository struct {
	parts                              map[int64]Part
	offers                             map[int64]Offer
	links                              map[[2]int64]bool
	referenced, quoted, deleteConflict bool
	transactions                       int
	events                             []string
}

func newMemoryRepository() *memoryRepository {
	return &memoryRepository{parts: map[int64]Part{}, offers: map[int64]Offer{}, links: map[[2]int64]bool{}}
}
func repositoryWithOffer(t *testing.T) *memoryRepository {
	t.Helper()
	r := newMemoryRepository()
	r.parts[1] = Part{ID: 1, Name: "Pièce legacy"}
	o, err := offerFromInput(1, completeOffer())
	if err != nil {
		t.Fatal(err)
	}
	o.ID = 10
	r.offers[10] = o
	return r
}
func (r *memoryRepository) Part(_ context.Context, id int64) (Part, error) {
	p, ok := r.parts[id]
	if !ok {
		return Part{}, ErrPartNotFound
	}
	return p, nil
}
func (r *memoryRepository) Offer(_ context.Context, id int64) (Offer, error) {
	r.events = append(r.events, fmt.Sprintf("offer-read:%d", id))
	o, ok := r.offers[id]
	if !ok {
		return Offer{}, ErrOfferNotFound
	}
	return o, nil
}
func (r *memoryRepository) ListParts(context.Context, Filters) ([]PartWithOffers, error) {
	return nil, nil
}
func (r *memoryRepository) ListSupplierOffers(context.Context, int64) ([]Offer, error) {
	return nil, nil
}
func (r *memoryRepository) Alternatives(_ context.Context, id int64) ([]Part, error) {
	var parts []Part
	for pair := range r.links {
		other := int64(0)
		if pair[0] == id {
			other = pair[1]
		}
		if pair[1] == id {
			other = pair[0]
		}
		if other != 0 && r.parts[other].ArchivedAt == nil {
			parts = append(parts, r.parts[other])
		}
	}
	return parts, nil
}
func (r *memoryRepository) WithinTx(ctx context.Context, fn func(TxRepository) error) error {
	r.transactions++
	parts := make(map[int64]Part, len(r.parts))
	offers := make(map[int64]Offer, len(r.offers))
	links := make(map[[2]int64]bool, len(r.links))
	for id, p := range r.parts {
		parts[id] = p
	}
	for id, o := range r.offers {
		offers[id] = o
	}
	for pair, v := range r.links {
		links[pair] = v
	}
	if err := fn(r); err != nil {
		r.parts, r.offers, r.links = parts, offers, links
		return err
	}
	return ctx.Err()
}
func (r *memoryRepository) PartForUpdate(ctx context.Context, id int64) (Part, error) {
	r.events = append(r.events, fmt.Sprintf("part-lock:%d", id))
	return r.Part(ctx, id)
}
func (r *memoryRepository) OfferForUpdate(_ context.Context, id int64) (Offer, error) {
	r.events = append(r.events, fmt.Sprintf("offer-lock:%d", id))
	o, ok := r.offers[id]
	if !ok {
		return Offer{}, ErrOfferNotFound
	}
	return o, nil
}
func (r *memoryRepository) OffersForPartForUpdate(_ context.Context, id int64) ([]Offer, error) {
	r.events = append(r.events, fmt.Sprintf("part-offers-lock:%d", id))
	return nil, nil
}
func (r *memoryRepository) InsertPart(_ context.Context, p Part) (Part, error) {
	p.ID = 100 + int64(len(r.parts))
	r.parts[p.ID] = p
	return p, nil
}
func (r *memoryRepository) ReplacePart(_ context.Context, p Part) (Part, error) {
	r.parts[p.ID] = p
	return p, nil
}
func (r *memoryRepository) InsertOffer(_ context.Context, o Offer) (Offer, error) {
	o.ID = 100 + int64(len(r.offers))
	r.offers[o.ID] = o
	return o, nil
}
func (r *memoryRepository) ReplaceOffer(_ context.Context, o Offer) (Offer, error) {
	r.offers[o.ID] = o
	return o, nil
}
func (r *memoryRepository) InsertAlternative(_ context.Context, low, high int64) error {
	if low >= high {
		return ErrInvalidInput
	}
	r.links[[2]int64{low, high}] = true
	return nil
}
func (r *memoryRepository) PartHasReferences(_ context.Context, id int64) (bool, error) {
	if r.referenced {
		return true, nil
	}
	for _, o := range r.offers {
		if o.PartID == id {
			return true, nil
		}
	}
	for pair := range r.links {
		if pair[0] == id || pair[1] == id {
			return true, nil
		}
	}
	return false, nil
}
func (r *memoryRepository) OfferHasReferences(context.Context, int64) (bool, error) {
	return r.referenced, nil
}
func (r *memoryRepository) OfferHasQuotes(context.Context, int64) (bool, error) { return r.quoted, nil }
func (r *memoryRepository) ArchivePart(_ context.Context, id int64, at time.Time) (Part, error) {
	p := r.parts[id]
	p.ArchivedAt = &at
	r.parts[id] = p
	return p, nil
}
func (r *memoryRepository) ArchiveOffer(_ context.Context, id int64, at time.Time) (Offer, error) {
	o := r.offers[id]
	o.ArchivedAt = &at
	r.offers[id] = o
	return o, nil
}
func (r *memoryRepository) DeletePart(_ context.Context, id int64) error {
	if r.deleteConflict {
		r.deleteConflict = false
		return fmt.Errorf("concurrent part reference: %w", ErrReferenceConflict)
	}
	delete(r.parts, id)
	return nil
}
func (r *memoryRepository) DeleteOffer(_ context.Context, id int64) error {
	if r.deleteConflict {
		r.deleteConflict = false
		return fmt.Errorf("concurrent offer reference: %w", ErrReferenceConflict)
	}
	delete(r.offers, id)
	return nil
}
