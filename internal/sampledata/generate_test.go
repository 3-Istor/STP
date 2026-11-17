package sampledata

import (
	"errors"
	"reflect"
	"testing"

	"github.com/shopspring/decimal"
)

func TestGenerateIsDeterministic(t *testing.T) {
	a, err := Generate("42")
	if err != nil {
		t.Fatal(err)
	}
	b, err := Generate("42")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(a, b) || a.Fingerprint() != b.Fingerprint() {
		t.Fatal("same seed produced different datasets")
	}
	c, err := Generate("43")
	if err != nil {
		t.Fatal(err)
	}
	if c.Fingerprint() == a.Fingerprint() {
		t.Fatal("another seed produced the same dataset")
	}
}

func TestDatasetInvariants(t *testing.T) {
	d, err := Generate("42")
	if err != nil {
		t.Fatal(err)
	}
	parts := map[string]Part{}
	for _, p := range d.Parts {
		parts[p.Key] = p
	}
	offers := map[string]Offer{}
	offersPerPart := map[string]int{}
	currencies := map[string]bool{}
	for _, o := range d.Offers {
		offers[o.Key] = o
		offersPerPart[o.PartKey]++
		currencies[o.Currency] = true
	}
	nonNegative := func(s string) bool {
		v, err := decimal.NewFromString(s)
		return err == nil && !v.IsNegative()
	}

	tests := []struct {
		name string
		ok   func() bool
	}{
		{"every part attribute is filled", func() bool {
			for _, p := range d.Parts {
				for _, v := range []string{p.MassKG, p.CO2KGPerUnit, p.RecycledPercent, p.RecyclabilityPercent,
					p.ManufacturingAmount, p.LaborHours, p.HourlyAmount} {
					if !nonNegative(v) {
						t.Logf("part %s: %q", p.Key, v)
						return false
					}
				}
				if p.Name == "" || p.Category == "" || p.Zone == "" || p.ManufacturingCurrency == "" || p.HourlyCurrency == "" {
					return false
				}
			}
			return true
		}},
		{"every part has several offers", func() bool {
			for _, p := range d.Parts {
				if offersPerPart[p.Key] < 2 {
					t.Logf("part %s: %d offers", p.Key, offersPerPart[p.Key])
					return false
				}
			}
			return true
		}},
		{"offer bounds are positive and ordered", func() bool {
			for _, o := range d.Offers {
				lo, hi := decimal.RequireFromString(o.PriceMin), decimal.RequireFromString(o.PriceMax)
				if !lo.IsPositive() || lo.GreaterThan(hi) {
					t.Logf("offer %s: %s..%s", o.Key, o.PriceMin, o.PriceMax)
					return false
				}
			}
			return true
		}},
		{"four currencies are offered", func() bool {
			return reflect.DeepEqual(currencies, map[string]bool{"EUR": true, "USD": true, "GBP": true, "JPY": true})
		}},
		{"contracts have three increasing tiers", func() bool {
			for _, c := range d.Contracts {
				if len(c.Tiers) != 3 || len(c.OfferKeys) == 0 {
					return false
				}
				for i := 1; i < len(c.Tiers); i++ {
					if !decimal.RequireFromString(c.Tiers[i].MinQuantity).GreaterThan(decimal.RequireFromString(c.Tiers[i-1].MinQuantity)) ||
						!decimal.RequireFromString(c.Tiers[i].DiscountPercent).GreaterThan(decimal.RequireFromString(c.Tiers[i-1].DiscountPercent)) {
						return false
					}
				}
			}
			return true
		}},
		{"alternatives link two distinct known parts", func() bool {
			for _, a := range d.Alternatives {
				if _, ok := parts[a.PartKey]; !ok || a.PartKey == a.OtherKey {
					return false
				}
				if _, ok := parts[a.OtherKey]; !ok {
					return false
				}
			}
			return len(d.Alternatives) > 0
		}},
		{"BOM trees: one root, parent before child, offer sells the part", func() bool {
			for _, b := range d.BOMs {
				seen := map[string]bool{}
				for i, n := range b.Nodes {
					if (i == 0) != (n.ParentKey == "") || (i > 0 && !seen[n.ParentKey]) || seen[n.Key] {
						t.Logf("bom %s node %s", b.Key, n.Key)
						return false
					}
					seen[n.Key] = true
					if n.PartKey != "" && offers[n.OfferKey].PartKey != n.PartKey {
						t.Logf("bom %s node %s: offer %s", b.Key, n.Key, n.OfferKey)
						return false
					}
					if (n.PartKey == "") == (n.Name == "") || !decimal.RequireFromString(n.Quantity).IsPositive() {
						return false
					}
				}
			}
			return true
		}},
		{"every offer has packaging and declares tooling (0 when none)", func() bool {
			for _, o := range d.Offers {
				if !nonNegative(o.PackagingAmount) || o.PackagingCurrency != o.Currency {
					return false
				}
				if o.ToolingAmount == "" || o.ToolingCurrency == "" || !nonNegative(o.ToolingAmount) {
					t.Logf("offer %s: tooling %q", o.Key, o.ToolingAmount)
					return false
				}
				if o.PartKey == "mpl.porte-av" && (o.ToolingAmount != "84000" || o.ToolingCurrency != "EUR") {
					return false
				}
			}
			return true
		}},
		{"customs and freight cover every supplier country towards both destinations", func() bool {
			lanes := map[string]bool{}
			for _, c := range d.CustomsDuties {
				lanes["c/"+c.Origin+"/"+c.Destination] = nonNegative(c.Percent)
			}
			for _, f := range d.FreightRates {
				lanes["f/"+f.Origin+"/"+f.Destination] = nonNegative(f.AmountPerKg)
			}
			for _, s := range d.Suppliers {
				for _, dest := range []string{"IT", "GB"} {
					if !lanes["c/"+s.Country+"/"+dest] || !lanes["f/"+s.Country+"/"+dest] {
						t.Logf("lane %s→%s missing", s.Country, dest)
						return false
					}
				}
			}
			return true
		}},
		{"2026 budget rates to EUR: GBP 1.1500, JPY 0.0062, USD 0.9100", func() bool {
			want := map[string]string{"GBP": "1.1500", "JPY": "0.0062", "USD": "0.9100"}
			for _, r := range d.BudgetRates {
				if r.Year == 2026 && r.To == "EUR" && want[r.From] == r.Rate {
					delete(want, r.From)
				}
			}
			return len(want) == 0
		}},
		{"Multipla: full + budget, 60 000, IT, 10 budgets, schedule; Arden contract expired on 2026-03-31", func() bool {
			var m BOM
			for _, b := range d.BOMs {
				if b.Key == "bom.multipla" {
					m = b
				}
			}
			arden := false
			for _, c := range d.Contracts {
				arden = arden || (c.SupplierKey == "sup.arden" && c.EndsOn == "2026-03-31")
			}
			return m.CostBasis == "full" && m.FX == "budget" && m.ProgramVolume == 60000 && m.Destination == "IT" &&
				len(m.Budgets) == 10 && m.Budgets[0].NodeKey == m.Nodes[0].Key && m.Budgets[0].Amount == "26500" &&
				m.StartOn == "2026-01-01" && m.TargetOn == "2026-03-02" && arden
		}},
		{"Multipla ~30 000 nodes, Valhalla ~15 000 nodes", func() bool {
			sizes := map[string][2]int{"bom.multipla": {25_000, 32_000}, "bom.valhalla": {12_000, 17_000}}
			for _, b := range d.BOMs {
				r := sizes[b.Key]
				if len(b.Nodes) < r[0] || len(b.Nodes) > r[1] {
					t.Logf("bom %s: %d nodes", b.Key, len(b.Nodes))
					return false
				}
			}
			return len(d.BOMs) == 2
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !tt.ok() {
				t.Fail()
			}
		})
	}
}

func TestInvalidSpecIsRejected(t *testing.T) {
	tests := []struct {
		name string
		run  func() error
	}{
		{"unknown part in a tree", func() error {
			_, err := makeBOM("42", bomSpec{key: "bom.x", name: "X", currency: "EUR", volume: 1,
				tree: []item{asm("A", pc("nope", "1"))}}, map[string][]string{})
			return err
		}},
		{"category with a single supplier", func() error {
			_, err := makeOffers("42", "Vitrage", partSpec{key: "p", price: "1", mass: "1"},
				[]Supplier{{Key: "sup.a", categories: []string{"Vitrage"}, Currency: "EUR", QualityScore: "4.0"}})
			return err
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.run(); !errors.Is(err, ErrInvalidSpec) {
				t.Fatalf("err = %v, want ErrInvalidSpec", err)
			}
		})
	}
}
