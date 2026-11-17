package snapshot

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"bomstudio/internal/bom"
	"bomstudio/internal/money"
)

// Versions d'une BOM (vision V2, maquettes I4-0 et I4-7) : la famille d'une BOM est sa BOM d'origine, les
// snapshots publiés depuis chacun de ses brouillons, et les brouillons créés depuis ces snapshots (lien
// source_snapshot_id, transitif). Snapshots nommés V1, V2… par ordre de publication ; brouillons nommés
// « Brouillon A », « Brouillon B »… par ordre de création, la BOM d'origine étant A.

// Version : un brouillon ou un snapshot de la famille, avec son coût selon sa méthode.
type Version struct {
	Kind  string // draft | snapshot
	ID    int64  // id de BOM (draft) ou de snapshot
	BOMID int64  // BOM du brouillon, ou BOM d'où le snapshot a été publié
	Label string
	Name  string
	// At : création du brouillon ou publication du snapshot.
	At               time.Time
	SourceSnapshotID *int64
	Method           bom.CostMethod
	Cost             *money.Money
	CostError        string
	// BudgetPercent : coût ÷ budget global × 100 (bom.BudgetPercent, exact, arrondi une fois en sortie) ; nil sans
	// budget, sans coût ou budget dans une autre devise.
	// Snapshot : budget global figé à la publication (budget actuel de sa BOM s'il a été publié avant 00015).
	BudgetPercent    *decimal.Decimal
	PriceSnapshotID  *int64 // nil : prix en direct (brouillon)
	ProposedBy       *string
	ValidatedBy      *string
	Note             *string
	PartCount        int
	NodeCount        int
	TargetBudgetRoot *money.Money
}

// family : versions d'une famille, sans coût (lecture légère, sert aussi aux libellés de la comparaison).
type family struct {
	drafts []bom.BOM         // par id
	snaps  []Snapshot        // par id
	labels map[string]string // "bom:<id>" / "snapshot:<id>" → libellé
}

func letter(i int) string {
	if i < 26 {
		return "ABCDEFGHIJKLMNOPQRSTUVWXYZ"[i : i+1]
	}
	return fmt.Sprintf("%d", i+1)
}

// familyOf lit toutes les BOM et tous les en-têtes de snapshot et garde la famille de la version ref.
func (s *Service) familyOf(ctx context.Context, ref Ref) (family, error) {
	boms, err := s.boms.ListBOMs(ctx)
	if err != nil {
		return family{}, err
	}
	snaps, err := s.store.List(ctx)
	if err != nil {
		return family{}, err
	}
	bomOf := make(map[int64]int64, len(snaps)) // snapshot → BOM
	for _, sn := range snaps {
		bomOf[sn.ID] = sn.BOMID
	}
	byID := make(map[int64]bom.BOM, len(boms))
	for _, b := range boms {
		byID[b.ID] = b
	}
	origin := func(id int64) int64 {
		seen := map[int64]bool{}
		for !seen[id] {
			seen[id] = true
			b, ok := byID[id]
			if !ok || b.SourceSnapshotID == nil {
				return id
			}
			parent, ok := bomOf[*b.SourceSnapshotID]
			if !ok {
				return id
			}
			id = parent
		}
		return id
	}
	start := ref.ID
	if ref.Kind == RefSnapshot {
		b, ok := bomOf[ref.ID]
		if !ok {
			return family{}, fmt.Errorf("snapshot %d: %w", ref.ID, ErrSnapshotNotFound)
		}
		start = b
	} else if _, ok := byID[ref.ID]; !ok {
		return family{}, fmt.Errorf("bom %d: %w", ref.ID, bom.ErrBOMNotFound)
	}
	root := origin(start)
	f := family{labels: map[string]string{}}
	members := map[int64]bool{}
	for _, b := range boms { // ListBOMs : par id
		if origin(b.ID) == root {
			members[b.ID] = true
			f.labels[fmt.Sprintf("bom:%d", b.ID)] = "Brouillon " + letter(len(f.drafts))
			f.drafts = append(f.drafts, b)
		}
	}
	for _, sn := range snaps { // par id
		if members[sn.BOMID] {
			f.snaps = append(f.snaps, sn)
			f.labels[fmt.Sprintf("snapshot:%d", sn.ID)] = fmt.Sprintf("V%d", len(f.snaps))
		}
	}
	return f, nil
}

// InSameFamily dit si le brouillon draftID est une version de la famille de bomID (bom.DraftFamily, pistes de
// réduction) ; ErrBOMNotFound si bomID n'existe pas.
func (s *Service) InSameFamily(ctx context.Context, bomID, draftID int64) (bool, error) {
	f, err := s.familyOf(ctx, Ref{Kind: RefBOM, ID: bomID})
	if err != nil {
		return false, fmt.Errorf("family of bom %d: %w", bomID, err)
	}
	return slices.ContainsFunc(f.drafts, func(b bom.BOM) bool { return b.ID == draftID }), nil
}

// Versions renvoie les brouillons et snapshots de la famille de la BOM bomID, par date puis par id, chacun
// chiffré selon sa méthode. Un coût indisponible est dit (CostError), il ne fait pas échouer la liste.
func (s *Service) Versions(ctx context.Context, bomID int64) ([]Version, error) {
	f, err := s.familyOf(ctx, Ref{Kind: RefBOM, ID: bomID})
	if err != nil {
		return nil, fmt.Errorf("versions of bom %d: %w", bomID, err)
	}
	var out []Version
	for _, b := range f.drafts {
		dc, err := s.boms.DraftCost(ctx, b.ID)
		if err != nil {
			return nil, fmt.Errorf("versions of bom %d: %w", bomID, err)
		}
		v := Version{Kind: "draft", ID: b.ID, BOMID: b.ID, Label: f.labels[fmt.Sprintf("bom:%d", b.ID)], Name: b.Name,
			At: b.CreatedAt, SourceSnapshotID: b.SourceSnapshotID, Method: b.CostMethod.OrDefault(),
			PriceSnapshotID: b.PriceSnapshotID, NodeCount: len(dc.Nodes)}
		for _, n := range dc.Nodes {
			if n.IsPart() {
				v.PartCount++
			}
		}
		root := bom.RootID(dc.Nodes)
		if err := s.versionCost(&v, dc.Totals, root); err != nil {
			return nil, err
		}
		setBudget(&v, bom.RootBudgetOf(dc.Budgets, root)) // budgets de la même lecture que le coût
		out = append(out, v)
	}
	for _, sn := range f.snaps {
		p, err := s.Get(ctx, sn.ID)
		if err != nil {
			return nil, fmt.Errorf("versions of bom %d: %w", bomID, err)
		}
		m := bom.DefaultCostMethod()
		if sn.CostMethod != nil {
			m = *sn.CostMethod
		}
		v := Version{Kind: "snapshot", ID: sn.ID, BOMID: sn.BOMID, Label: f.labels[fmt.Sprintf("snapshot:%d", sn.ID)], Name: sn.Name,
			At: sn.PublishedAt, Method: m, PriceSnapshotID: &sn.PriceSnapshotID, ProposedBy: sn.ProposedBy, ValidatedBy: sn.ValidatedBy,
			Note: sn.Note, NodeCount: len(p.Nodes)}
		for _, n := range p.Nodes {
			if n.Part != nil {
				v.PartCount++
			}
		}
		root := bom.CalcRootID(p.Input.Nodes)
		if err := s.versionCost(&v, p.Totals, root); err != nil {
			return nil, err
		}
		budgets, err := s.frozenBudgets(ctx, sn) // budgets figés à la publication (v2-lot-g T3)
		if err != nil {
			return nil, fmt.Errorf("versions of bom %d: %w", bomID, err)
		}
		setBudget(&v, bom.RootBudgetOf(budgets, root))
		out = append(out, v)
	}
	slices.SortStableFunc(out, func(a, b Version) int {
		return cmp.Or(a.At.Compare(b.At), strings.Compare(a.Kind, b.Kind), cmp.Compare(a.ID, b.ID))
	})
	return out, nil
}

func (s *Service) versionCost(v *Version, t bom.Totals, root int64) error {
	n, ok := t.Node(root)
	if !ok {
		return fmt.Errorf("version %s: root %d has no totals: %w", v.Label, root, bom.ErrInvalidTree)
	}
	if n.Cost == nil {
		v.CostError = n.CostErr.Error()
		return nil
	}
	v.Cost = n.Cost
	return nil
}

func setBudget(v *Version, b *bom.TargetBudget) {
	if b == nil {
		return
	}
	target, err := money.New(b.Amount, b.Currency)
	if err != nil {
		return
	}
	v.TargetBudgetRoot = &target
	if v.Cost == nil {
		return
	}
	v.BudgetPercent = bom.BudgetPercent(*v.Cost, &target) // exact : arrondi une seule fois, en sortie
}
