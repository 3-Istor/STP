package bom

import (
	"cmp"
	"fmt"
	"slices"
	"time"
)

// Schedule : date de départ et date cible d'une BOM (ADR 0023). Dates calendaires en UTC, à minuit.
type Schedule struct {
	StartOn  time.Time
	TargetOn *time.Time // nil : pas de date cible
}

// LatePart : nœud (pièce ou sous-ensemble) qui a un délai propre et dont la chaîne de délais (de la racine à
// ses descendants) dépasse la date cible.
type LatePart struct {
	NodeID      int64
	ChainDays   int // délais propres des ancêtres + délai total du nœud
	OverrunDays int // ChainDays − jours disponibles, > 0
}

// Planning : disponibilité de la voiture et pièces qui la mettent en danger (ADR 0023).
type Planning struct {
	Schedule *Schedule // nil : pas de date de départ, donc pas de date (délais en jours seulement)
	// LeadTimeDays : délai de la racine (jours calendaires) ; AvailableOn = StartOn + LeadTimeDays.
	LeadTimeDays int
	AvailableOn  *time.Time
	// Critical : nœuds à délai propre non nul dont la chaîne égale le délai de la racine, par id (égalités
	// conservées). Un sous-ensemble compte par sa production ; une pièce en stock (délai propre nul) non.
	Critical []int64
	// Avec une date cible : jours disponibles, cible tenue ou non, et nœuds responsables (dépassement décroissant).
	AllowedDays *int
	TargetMet   *bool
	Late        []LatePart
}

// PlanSchedule calcule le planning à partir des totaux déjà calculés (délais propres et totaux par nœud).
// Pur : ni horloge ni base ; même arbre, mêmes totaux, même planning.
func PlanSchedule(nodes []CalcNode, t Totals, s *Schedule) (Planning, error) {
	parent := make(map[int64]*int64, len(nodes))
	var root *int64
	for _, n := range nodes {
		parent[n.ID] = n.ParentID
		if n.ParentID == nil {
			id := n.ID
			root = &id
		}
	}
	if root == nil {
		return Planning{}, fmt.Errorf("plan schedule: no root: %w", ErrInvalidTree)
	}
	rt, ok := t.Node(*root)
	if !ok {
		return Planning{}, fmt.Errorf("plan schedule: root %d has no totals: %w", *root, ErrInvalidTree)
	}
	out := Planning{Schedule: s, LeadTimeDays: rt.LeadTimeDays}

	// above[id] : somme des délais propres des ancêtres stricts. Remontée mémorisée, sans récursion.
	above := map[int64]int{*root: 0}
	aboveOf := func(id int64) (int, error) {
		var chain []int64
		cur := id
		for {
			if _, done := above[cur]; done {
				break
			}
			chain = append(chain, cur)
			p := parent[cur]
			if p == nil || len(chain) > len(nodes) {
				return 0, fmt.Errorf("plan schedule: node %d not under root: %w", id, ErrInvalidTree)
			}
			cur = *p
		}
		for k := len(chain) - 1; k >= 0; k-- {
			p := *parent[chain[k]]
			pt, _ := t.Node(p)
			above[chain[k]] = above[p] + pt.OwnLeadTimeDays
		}
		return above[id], nil
	}

	var allowed int
	if s != nil {
		at := s.StartOn.AddDate(0, 0, rt.LeadTimeDays)
		out.AvailableOn = &at
		if s.TargetOn != nil {
			allowed = daysBetween(s.StartOn, *s.TargetOn)
			met := rt.LeadTimeDays <= allowed
			out.AllowedDays, out.TargetMet = &allowed, &met
		}
	}
	// Seuls les nœuds qui ajoutent du délai (délai propre > 0) sont responsables : une pièce en stock ou un
	// sous-ensemble sans production ne fait que porter le délai de ses enfants.
	for _, n := range nodes {
		nt, _ := t.Node(n.ID)
		if nt.OwnLeadTimeDays == 0 {
			continue
		}
		a, err := aboveOf(n.ID)
		if err != nil {
			return Planning{}, err
		}
		chain := a + nt.LeadTimeDays
		if chain == rt.LeadTimeDays && rt.LeadTimeDays > 0 {
			out.Critical = append(out.Critical, n.ID)
		}
		if out.AllowedDays != nil && chain > allowed {
			out.Late = append(out.Late, LatePart{NodeID: n.ID, ChainDays: chain, OverrunDays: chain - allowed})
		}
	}
	slices.Sort(out.Critical)
	slices.SortFunc(out.Late, func(a, b LatePart) int {
		return cmp.Or(cmp.Compare(b.OverrunDays, a.OverrunDays), cmp.Compare(a.NodeID, b.NodeID))
	})
	return out, nil
}

// daysBetween : jours calendaires entre deux dates à minuit UTC, sans time.Duration (qui sature à ~292 ans).
func daysBetween(from, to time.Time) int {
	return int((to.Unix() - from.Unix()) / 86400)
}
