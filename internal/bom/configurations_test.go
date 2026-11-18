package bom

import (
	"bomstudio/internal/money"
	"errors"
	"reflect"
	"testing"
)

func TestConfigurationsReuseCostStrategies(t *testing.T) {
	for _, method := range []CostMethod{DefaultCostMethod(), {Basis: BasisLanded, FX: FXBudget}, full(FXLatest)} {
		t.Run(string(method.Basis)+"/"+string(method.FX), func(t *testing.T) {
			in := methodInput(t, method, activeDay, nil)
			req := ConfigurationRequest{RootID: 1, Guards: []OccurrenceGuard{{ID: 1}, {ID: 2, ParentID: id(1)}}, Scenarios: []ConfigurationDefinition{
				{Key: "hybrid"}, {Key: "thermal", ExcludedRoots: []int64{2}}, {Key: "electric"},
			}}
			got, err := ComputeConfigurations(in, req)
			if err != nil {
				t.Fatal(err)
			}
			filtered := in
			filtered.Nodes = []CalcNode{in.Nodes[0], in.Nodes[3], in.Nodes[4]}
			want, err := ComputeTotals(filtered)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got[1].Totals, want) {
				t.Fatal("scenario bypasses active cost strategy, quantities or volume discounts")
			}
		})
	}
}

func TestServiceConfigurationsPreserveReference(t *testing.T) {
	f := newFixture(t)
	bomID, rootID := f.newBOM(t)
	thermal := f.addPart(t, bomID, rootID, 1, 11, "2") // USD quote and FX must be captured once for all modes.
	electric := f.addPart(t, bomID, rootID, 2, 20, "4")
	before, err := f.store.Draft(t.Context(), bomID)
	if err != nil {
		t.Fatal(err)
	}
	counted := &countingPricing{Pricing: f.prices}
	f.svc.pricing = counted
	req := ConfigurationRequest{RootID: rootID, Guards: []OccurrenceGuard{{ID: rootID}, {ID: thermal, ParentID: &rootID, PartID: id(1)}, {ID: electric, ParentID: &rootID, PartID: id(2)}}, Scenarios: []ConfigurationDefinition{
		{Key: "hybrid"}, {Key: "thermal", ExcludedRoots: []int64{electric}}, {Key: "electric", ExcludedRoots: []int64{thermal}},
	}}
	result, err := f.svc.SimulateConfigurations(t.Context(), bomID, req)
	if err != nil {
		t.Fatal(err)
	}
	for i, want := range []string{"188.00 EUR", "184.00 EUR", "4.00 EUR"} {
		if costOf(t, result.Scenarios[i].Tree, rootID) != want {
			t.Fatalf("scenario %d: %s", i, costOf(t, result.Scenarios[i].Tree, rootID))
		}
	}
	after, err := f.store.Draft(t.Context(), bomID)
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("reference modified", err)
	}
	if result.PriceAsOf == nil {
		t.Fatal("missing input quote date")
	}
	if counted.single != 0 || counted.batch != 2 {
		t.Fatalf("configuration inputs read %d single / %d batch times; want 0 / 2 (quotes, FX) for all three scenarios", counted.single, counted.batch)
	}
}

func configurationInput(t *testing.T) TotalsInput {
	t.Helper()
	in := TotalsInput{Currency: "EUR", Nodes: []CalcNode{
		sub(1, nil, "1", 0), part(2, id(1), "2", 1, 4, "1"),
		sub(10, id(1), "2", 0), part(11, id(10), "3", 2, 8, "2"), part(12, id(11), "1", 4, 2, "1"),
		sub(20, id(1), "1", 0), part(21, id(20), "4", 3, 6, "3"),
	}, Prices: map[int64]PriceResult{1: price(t, 1, "10", "EUR"), 2: price(t, 2, "20", "USD"), 3: price(t, 3, "30", "EUR"), 4: price(t, 4, "5", "EUR")},
		Rates: map[money.Currency]RateResult{"USD": rate("USD", "EUR", "0.5")}}
	for i := range in.Nodes {
		if in.Nodes[i].Part != nil {
			in.Nodes[i].Part.PartID = in.Nodes[i].Part.OfferID
		}
	}
	return in
}
func configurationRequest() ConfigurationRequest {
	return ConfigurationRequest{RootID: 1, Guards: []OccurrenceGuard{{ID: 1}, {ID: 10, ParentID: id(1)}, {ID: 20, ParentID: id(1)}}, Scenarios: []ConfigurationDefinition{
		{Key: "hybrid"}, {Key: "thermal", ExcludedRoots: []int64{20}}, {Key: "electric", ExcludedRoots: []int64{10}},
	}}
}
func TestConfigurationsRecomputeBOM(t *testing.T) {
	in := configurationInput(t)
	got, err := ComputeConfigurations(in, configurationRequest())
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		key, cost string
		count     int
		delta     string
	}{{"hybrid", "230", 7, "0"}, {"thermal", "110", 5, "-120"}, {"electric", "140", 4, "-90"}} {
		t.Run(tt.key, func(t *testing.T) {
			var found *ConfigurationResult
			for i := range got {
				if got[i].Key == tt.key {
					found = &got[i]
				}
			}
			if found == nil {
				t.Fatal("scenario absent")
			}
			root, _ := found.Totals.Node(1)
			if root.Cost == nil || !root.Cost.Amount().Equal(dec(tt.cost)) {
				t.Fatalf("cost = %v, want %s", root.Cost, tt.cost)
			}
			if len(found.ActiveIDs) != tt.count {
				t.Fatalf("active count %d", len(found.ActiveIDs))
			}
			if found.Delta == nil || !found.Delta.Amount().Equal(dec(tt.delta)) {
				t.Fatalf("delta = %v", found.Delta)
			}
			common, _ := found.Totals.Node(2)
			if common.OwnCost == nil || !common.OwnCost.Amount().Equal(dec("20")) {
				t.Fatal("common contribution changed")
			}
		})
	}
	if !reflect.DeepEqual(in, configurationInput(t)) {
		t.Fatal("reference input mutated")
	}
	again, err := ComputeConfigurations(in, configurationRequest())
	if err != nil || !reflect.DeepEqual(got, again) {
		t.Fatal("non deterministic result", err)
	}
}
func TestConfigurationsFollowQuotesAndRates(t *testing.T) {
	for _, tt := range []struct {
		name                      string
		change                    func(*TotalsInput)
		hybrid, thermal, electric string
	}{
		{"supplier quote", func(in *TotalsInput) { in.Prices[1] = price(t, 1, "20", "EUR") }, "250", "130", "160"},
		{"exchange rate", func(in *TotalsInput) { in.Rates["USD"] = rate("USD", "EUR", "0.6") }, "242", "122", "140"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			in := configurationInput(t)
			tt.change(&in)
			got, err := ComputeConfigurations(in, configurationRequest())
			if err != nil {
				t.Fatal(err)
			}
			for i, want := range []string{tt.hybrid, tt.thermal, tt.electric} {
				root, _ := got[i].Totals.Node(1)
				if root.Cost == nil || !root.Cost.Amount().Equal(dec(want)) {
					t.Fatalf("%s = %v, want %s", got[i].Key, root.Cost, want)
				}
			}
		})
	}
}
func TestConfigurationsMissingPrice(t *testing.T) {
	in := configurationInput(t)
	delete(in.Prices, 2)
	got, err := ComputeConfigurations(in, configurationRequest())
	if err != nil {
		t.Fatal(err)
	}
	for i := range got {
		root, _ := got[i].Totals.Node(1)
		if got[i].Key == "electric" {
			if root.Cost == nil || !root.Cost.Amount().Equal(dec("140")) {
				t.Fatal("excluded missing price contaminates scenario")
			}
		} else if root.Cost != nil || len(root.MissingCost) == 0 {
			t.Fatal("missing price hidden")
		}
		if got[i].Delta != nil || got[i].DeltaPercent != nil {
			t.Fatal("comparison with incomplete reference must be unavailable")
		}
	}
}
func TestConfigurationsValidateAndDeduplicate(t *testing.T) {
	for _, tt := range []struct {
		name string
		edit func(*ConfigurationRequest)
	}{
		{"wrong root", func(r *ConfigurationRequest) { r.RootID = 99 }},
		{"wrong parent", func(r *ConfigurationRequest) { r.Guards[1].ParentID = id(20) }},
		{"wrong part", func(r *ConfigurationRequest) { r.Guards[1].PartID = id(99) }},
		{"unknown cut", func(r *ConfigurationRequest) { r.Scenarios[1].ExcludedRoots = []int64{999} }},
		{"cut root", func(r *ConfigurationRequest) { r.Scenarios[1].ExcludedRoots = []int64{1} }},
		{"hybrid changed", func(r *ConfigurationRequest) { r.Scenarios[0].ExcludedRoots = []int64{10} }},
		{"duplicate mode", func(r *ConfigurationRequest) { r.Scenarios[1].Key = "hybrid" }},
		{"unguarded cut", func(r *ConfigurationRequest) { r.Guards = r.Guards[:2] }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			r := configurationRequest()
			tt.edit(&r)
			_, err := ComputeConfigurations(configurationInput(t), r)
			if !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("error %v", err)
			}
		})
	}
	r := configurationRequest()
	r.Guards = append(r.Guards, OccurrenceGuard{ID: 11, ParentID: id(10), PartID: id(2)})
	r.Scenarios[2].ExcludedRoots = []int64{10, 11, 10}
	got, err := ComputeConfigurations(configurationInput(t), r)
	if err != nil {
		t.Fatal(err)
	}
	root, _ := got[2].Totals.Node(1)
	if root.Cost == nil || !root.Cost.Amount().Equal(dec("140")) || len(got[2].ExcludedIDs) != 3 {
		t.Fatal("overlapping subtrees counted twice")
	}
}
