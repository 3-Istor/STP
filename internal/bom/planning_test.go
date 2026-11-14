package bom

import (
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/shopspring/decimal"
)

func day(s string) time.Time {
	t, err := time.Parse(time.DateOnly, s)
	if err != nil {
		panic(err)
	}
	return t
}

func dayPtr(s string) *time.Time { t := day(s); return &t }

// TestPlanSchedule : disponibilité, chemin critique et pièces responsables (lead-time-planning, ADR 0023).
func TestPlanSchedule(t *testing.T) {
	// Voiture (2 j d'assemblage) > Moteur (5 j) > Bloc (60 j livraison) ; Voiture > Roue (10 j) ;
	// Voiture > Caisse (40 j) > Vis (27 j). Chaînes : Bloc 2+5+60 = 67 ; Roue 12 ; Caisse 2+40+27 = 69 ; Vis 69.
	nodes := []CalcNode{
		sub(1, nil, "1", 2), sub(2, id(1), "1", 5), part(3, id(2), "1", 10, 60, "0"), part(4, id(1), "4", 11, 10, "0"),
		part(5, id(1), "1", 12, 40, "0"), part(6, id(5), "2", 13, 27, "0"),
	}
	tests := []struct {
		name         string
		schedule     *Schedule
		wantLead     int
		wantAvail    string // "" : pas de date
		wantCritical []int64
		wantMet      *bool
		wantLate     string
	}{
		{
			name:     "QUAND la pièce la plus longue a un délai de 60 jours ALORS la disponibilité est d'au moins 60 jours",
			schedule: &Schedule{StartOn: day("2027-01-01")}, wantLead: 69, wantAvail: "2027-03-11",
			wantCritical: []int64{1, 5, 6}, wantLate: "[]",
		},
		{
			name:     "SI la date cible est impossible ALORS les pièces responsables sont listées, plus gros dépassement d'abord",
			schedule: &Schedule{StartOn: day("2027-01-01"), TargetOn: dayPtr("2027-03-05")}, // 63 jours
			wantLead: 69, wantAvail: "2027-03-11", wantCritical: []int64{1, 5, 6}, wantMet: ptrBool(false),
			wantLate: "[{1 69 6} {5 69 6} {6 69 6} {2 67 4} {3 67 4}]",
		},
		{
			name:     "une date cible tenable ne liste aucune pièce",
			schedule: &Schedule{StartOn: day("2027-01-01"), TargetOn: dayPtr("2027-03-11")},
			wantLead: 69, wantAvail: "2027-03-11", wantCritical: []int64{1, 5, 6}, wantMet: ptrBool(true), wantLate: "[]",
		},
		{
			name: "SANS date de départ ALORS délais en jours, ni date ni cible", wantLead: 69,
			wantCritical: []int64{1, 5, 6}, wantLate: "[]",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			totals, err := ComputeTotals(TotalsInput{Nodes: nodes, Currency: "EUR"})
			if err != nil {
				t.Fatal(err)
			}
			got, err := PlanSchedule(nodes, totals, tt.schedule)
			if err != nil {
				t.Fatal(err)
			}
			if got.LeadTimeDays != tt.wantLead {
				t.Errorf("lead %d, want %d", got.LeadTimeDays, tt.wantLead)
			}
			if avail := got.AvailableOn; (avail == nil) != (tt.wantAvail == "") ||
				(avail != nil && avail.Format(time.DateOnly) != tt.wantAvail) {
				t.Errorf("available on %v, want %q", avail, tt.wantAvail)
			}
			if !slices.Equal(got.Critical, tt.wantCritical) {
				t.Errorf("critical %v, want %v", got.Critical, tt.wantCritical)
			}
			if (got.TargetMet == nil) != (tt.wantMet == nil) || (got.TargetMet != nil && *got.TargetMet != *tt.wantMet) {
				t.Errorf("target met %v, want %v", got.TargetMet, tt.wantMet)
			}
			late := make([]string, len(got.Late))
			for i, l := range got.Late {
				late[i] = fmt.Sprintf("{%d %d %d}", l.NodeID, l.ChainDays, l.OverrunDays)
			}
			if g := fmt.Sprint(late); g != tt.wantLate {
				t.Errorf("late %s, want %s", g, tt.wantLate)
			}
		})
	}
}

// Une pièce en stock (délai nul, ADR 0022) sort du chemin critique et de la liste des responsables.
func TestPlanScheduleWithStock(t *testing.T) {
	nodes := []CalcNode{sub(1, nil, "1", 0), stockPart(2, id(1), "1", 100, 10, 60, 0), stockPart(3, id(1), "1", 101, 11, 20, 0)}
	totals, err := ComputeTotals(TotalsInput{Nodes: nodes, Currency: "EUR", Volume: 1, Stock: map[int64]decimal.Decimal{100: dec("1")}})
	if err != nil {
		t.Fatal(err)
	}
	got, err := PlanSchedule(nodes, totals, &Schedule{StartOn: day("2027-01-01"), TargetOn: dayPtr("2027-01-11")})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got.Critical, []int64{3}) || len(got.Late) != 1 || got.Late[0].NodeID != 3 {
		t.Fatalf("critical %v late %+v, want only node 3", got.Critical, got.Late)
	}
}

// Revue ADR 0023 : un sous-ensemble dont la production rend la cible impossible est listé (jamais une liste
// vide quand la cible est manquée) ; les jours se comptent sans time.Duration (dates très éloignées).
func TestPlanScheduleEdgeCases(t *testing.T) {
	t.Run("sous-ensemble sans pièce responsable du retard", func(t *testing.T) {
		nodes := []CalcNode{sub(1, nil, "1", 0), sub(2, id(1), "1", 30), part(3, id(1), "1", 10, 5, "0")}
		totals, err := ComputeTotals(TotalsInput{Nodes: nodes, Currency: "EUR"})
		if err != nil {
			t.Fatal(err)
		}
		got, err := PlanSchedule(nodes, totals, &Schedule{StartOn: day("2027-01-01"), TargetOn: dayPtr("2027-01-21")})
		if err != nil {
			t.Fatal(err)
		}
		if got.TargetMet == nil || *got.TargetMet || len(got.Late) != 1 || got.Late[0].NodeID != 2 || got.Late[0].OverrunDays != 10 {
			t.Fatalf("met %v late %+v, want node 2 late by 10 days", got.TargetMet, got.Late)
		}
		if !slices.Equal(got.Critical, []int64{2}) {
			t.Fatalf("critical %v, want [2]", got.Critical)
		}
	})
	t.Run("dates éloignées de plus de 292 ans", func(t *testing.T) {
		nodes := []CalcNode{sub(1, nil, "1", 1)}
		totals, err := ComputeTotals(TotalsInput{Nodes: nodes, Currency: "EUR"})
		if err != nil {
			t.Fatal(err)
		}
		got, err := PlanSchedule(nodes, totals, &Schedule{StartOn: day("0001-01-01"), TargetOn: dayPtr("9999-12-31")})
		if err != nil {
			t.Fatal(err)
		}
		if got.AllowedDays == nil || *got.AllowedDays != 3652058 {
			t.Fatalf("allowed days %v, want 3652058", got.AllowedDays)
		}
	})
}

func TestSetSchedule(t *testing.T) {
	tests := []struct {
		name    string
		sch     Schedule
		wantErr error
	}{
		{"départ et cible valides", Schedule{StartOn: day("2027-01-01"), TargetOn: dayPtr("2027-06-30")}, nil},
		{"cible le jour du départ", Schedule{StartOn: day("2027-01-01"), TargetOn: dayPtr("2027-01-01")}, nil},
		{"départ sans cible", Schedule{StartOn: day("2027-01-01")}, nil},
		{"cible avant le départ refusée", Schedule{StartOn: day("2027-01-02"), TargetOn: dayPtr("2027-01-01")}, ErrInvalidInput},
		{"heure dans la date refusée", Schedule{StartOn: day("2027-01-01").Add(time.Hour)}, ErrInvalidInput},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := NewService(newMemStore(), nil, nil)
			b, err := svc.CreateBOM(t.Context(), NewBOM{Name: "B", Currency: "EUR", Volume: 1})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := svc.SetSchedule(t.Context(), b.ID, tt.sch); !errors.Is(err, tt.wantErr) {
				t.Fatalf("err %v, want %v", err, tt.wantErr)
			}
			got, err := svc.Schedule(t.Context(), b.ID)
			if err != nil {
				t.Fatal(err)
			}
			if (got != nil) != (tt.wantErr == nil) {
				t.Errorf("schedule stored = %v, want stored only when valid", got)
			}
		})
	}
}

func ptrBool(b bool) *bool { return &b }
