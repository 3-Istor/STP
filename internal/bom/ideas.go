package bom

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/shopspring/decimal"

	"bomstudio/internal/money"
)

// Pistes de réduction (vision V2 §2) : gain estimé par voiture, responsable, statut et brouillon qui la chiffre.
// Fermer l'écart à la cible, c'est la somme des pistes ouvertes face à l'écart coût − budget global.

// IdeaStatus : idea | in_progress | to_validate | done | dropped.
type IdeaStatus string

const (
	IdeaNew        IdeaStatus = "idea"
	IdeaInProgress IdeaStatus = "in_progress"
	IdeaToValidate IdeaStatus = "to_validate"
	IdeaDone       IdeaStatus = "done"
	IdeaDropped    IdeaStatus = "dropped"
)

// Open dit si la piste compte encore pour fermer l'écart (ni réalisée, ni abandonnée).
func (s IdeaStatus) Open() bool { return s == IdeaNew || s == IdeaInProgress || s == IdeaToValidate }

func (s IdeaStatus) valid() bool { return s.Open() || s == IdeaDone || s == IdeaDropped }

var (
	// ErrIdeaNotFound : aucune piste avec cet identifiant pour cette BOM.
	ErrIdeaNotFound = errors.New("reduction idea not found")
	// ErrIdeasUnavailable : le service a été câblé sans stockage des pistes.
	ErrIdeasUnavailable = errors.New("reduction ideas unavailable: no store")
)

// Idea est une piste de réduction d'une BOM.
type Idea struct {
	ID, BOMID  int64
	Title      string
	Owner      string // texte libre, peut être vide
	Gain       money.Money
	Status     IdeaStatus
	DraftBOMID *int64
	CreatedAt  time.Time
}

// IdeaInput : contenu complet d'une piste (création, ou état après modification).
type IdeaInput struct {
	Title, Owner string
	Gain         money.Money
	Status       IdeaStatus
	DraftBOMID   *int64
}

// IdeaPatch : champs modifiés seulement ; ClearDraft détache le brouillon.
type IdeaPatch struct {
	Title, Owner *string
	Gain         *money.Money
	Status       *IdeaStatus
	DraftBOMID   *int64
	ClearDraft   bool
}

// IdeaStore : stockage des pistes ; ErrIdeaNotFound si la piste n'est pas dans la BOM.
type IdeaStore interface {
	ListIdeas(ctx context.Context, bomID int64) ([]Idea, error)
	GetIdea(ctx context.Context, bomID, id int64) (Idea, error)
	CreateIdea(ctx context.Context, bomID int64, in IdeaInput) (Idea, error)
	UpdateIdea(ctx context.Context, bomID, id int64, in IdeaInput) (Idea, error)
	DeleteIdea(ctx context.Context, bomID, id int64) error
}

// WithIdeas branche le stockage des pistes de réduction.
func (s *Service) WithIdeas(st IdeaStore) *Service {
	s.ideas = st
	return s
}

// DraftFamily dit si draftID est une version de la même famille que bomID (BOM d'origine, brouillons issus de ses
// snapshots, ADR 0032) ; *snapshot.Service la satisfait (le lien snapshot → BOM vit dans ce domaine). ErrBOMNotFound
// si bomID n'existe pas.
type DraftFamily interface {
	InSameFamily(ctx context.Context, bomID, draftID int64) (bool, error)
}

// WithDraftFamily branche la vérification de famille du brouillon lié à une piste. Sans, lier un brouillon à une
// piste est refusé (ErrIdeasUnavailable) : la règle n'est jamais sautée en silence.
func (s *Service) WithDraftFamily(f DraftFamily) *Service {
	s.family = f
	return s
}

// IdeasSummary : les pistes, le total des pistes ouvertes, l'écart à la cible (coût − budget global, nil sans
// budget global ou sans coût) et le reste à trouver (écart − pistes ouvertes ; négatif = écart couvert).
type IdeasSummary struct {
	Ideas       []Idea
	OpenTotal   money.Money
	GapToTarget *money.Money
	Remaining   *money.Money
	// GapError : pourquoi l'écart n'est pas calculable (pas de budget global, coût incomplet) ; vide sinon.
	GapError string
}

// SummarizeIdeas : pur. Toutes les pistes et le budget sont dans la devise de la BOM (vérifié à la saisie).
func SummarizeIdeas(currency money.Currency, ideas []Idea, rootCost *money.Money, rootBudget *TargetBudget) (IdeasSummary, error) {
	out := IdeasSummary{Ideas: ideas}
	total := decimal.Zero
	for _, i := range ideas {
		if i.Gain.Currency() != currency {
			return IdeasSummary{}, fmt.Errorf("idea %d in %s, bom in %s: %w", i.ID, i.Gain.Currency(), currency, money.ErrCurrencyMismatch)
		}
		if i.Status.Open() {
			total = total.Add(i.Gain.Amount())
		}
	}
	var err error
	if out.OpenTotal, err = money.New(total, string(currency)); err != nil {
		return IdeasSummary{}, err
	}
	switch {
	case rootBudget == nil:
		out.GapError = "pas de budget global"
	case rootBudget.Currency != string(currency):
		out.GapError = fmt.Sprintf("budget global en %s, BOM en %s", rootBudget.Currency, currency)
	case rootCost == nil:
		out.GapError = "coût incomplet"
	default:
		gap, err := money.New(rootCost.Amount().Sub(rootBudget.Amount), string(currency))
		if err != nil {
			return IdeasSummary{}, err
		}
		rest, err := money.New(gap.Amount().Sub(total), string(currency))
		if err != nil {
			return IdeasSummary{}, err
		}
		out.GapToTarget, out.Remaining = &gap, &rest
	}
	return out, nil
}

// Ideas renvoie les pistes de la BOM, leur total ouvert et le reste à trouver face au budget global.
func (s *Service) Ideas(ctx context.Context, bomID int64) (IdeasSummary, error) {
	if s.ideas == nil {
		return IdeasSummary{}, ErrIdeasUnavailable
	}
	dc, err := s.DraftCost(ctx, bomID)
	if err != nil {
		return IdeasSummary{}, fmt.Errorf("ideas of bom %d: %w", bomID, err)
	}
	ideas, err := s.ideas.ListIdeas(ctx, bomID)
	if err != nil {
		return IdeasSummary{}, fmt.Errorf("ideas of bom %d: %w", bomID, err)
	}
	root := RootID(dc.Nodes)
	budget := RootBudgetOf(dc.Budgets, root) // même lecture que le coût
	var cost *money.Money
	if root, ok := dc.Totals.Node(root); ok {
		cost = root.Cost
	}
	out, err := SummarizeIdeas(money.Currency(dc.BOM.Currency), ideas, cost, budget)
	if err != nil {
		return IdeasSummary{}, fmt.Errorf("ideas of bom %d: %w", bomID, err)
	}
	return out, nil
}

const maxIdeaText = 200

// maxIdeaGain : borne haute du gain estimé par voiture (exclue) ; au-delà, une erreur de saisie, pas une piste.
var maxIdeaGain = decimal.New(1, 9)

// validateIdea : titre non vide, textes bornés, 0 ≤ gain < 1e9 dans la devise de la BOM, statut connu ; brouillon
// lié existant, de la même famille et différent de la BOM (sinon le gain, déjà dans son coût, compterait deux fois).
func (s *Service) validateIdea(ctx context.Context, b BOM, in *IdeaInput) error {
	in.Title, in.Owner = strings.TrimSpace(in.Title), strings.TrimSpace(in.Owner)
	switch {
	case in.Title == "" || utf8.RuneCountInString(in.Title) > maxIdeaText:
		return fmt.Errorf("title must have 1 to %d characters: %w", maxIdeaText, ErrInvalidInput)
	case utf8.RuneCountInString(in.Owner) > maxIdeaText:
		return fmt.Errorf("owner has more than %d characters: %w", maxIdeaText, ErrInvalidInput)
	case !in.Status.valid():
		return fmt.Errorf("status %q must be idea, in_progress, to_validate, done or dropped: %w", in.Status, ErrInvalidInput)
	case in.Gain.Currency() == "":
		return fmt.Errorf("estimated gain is required: %w", ErrInvalidInput)
	case string(in.Gain.Currency()) != b.Currency:
		return fmt.Errorf("estimated gain in %s, bom in %s: %w", in.Gain.Currency(), b.Currency, ErrInvalidInput)
	case in.Gain.Amount().IsNegative() || in.Gain.Amount().GreaterThanOrEqual(maxIdeaGain):
		return fmt.Errorf("estimated gain %s must be >= 0 and < %s: %w", in.Gain, maxIdeaGain, ErrInvalidInput)
	}
	if in.DraftBOMID == nil {
		return nil
	}
	draft := *in.DraftBOMID
	if draft == b.ID {
		return fmt.Errorf("draft bom %d is the idea's own bom: link another version of the family: %w", draft, ErrInvalidInput)
	}
	if _, err := s.store.GetBOM(ctx, draft); err != nil {
		if errors.Is(err, ErrBOMNotFound) {
			return fmt.Errorf("draft bom %d does not exist: %w", draft, ErrInvalidInput)
		}
		return err
	}
	if s.family == nil {
		return fmt.Errorf("draft family check: %w", ErrIdeasUnavailable)
	}
	same, err := s.family.InSameFamily(ctx, b.ID, draft)
	if err != nil {
		return err
	}
	if !same {
		return fmt.Errorf("draft bom %d is not a version of bom %d's family: %w", draft, b.ID, ErrInvalidInput)
	}
	return nil
}

// CreateIdea ajoute une piste à la BOM ; statut par défaut : idea.
func (s *Service) CreateIdea(ctx context.Context, bomID int64, in IdeaInput) (Idea, error) {
	if s.ideas == nil {
		return Idea{}, ErrIdeasUnavailable
	}
	if in.Status == "" {
		in.Status = IdeaNew
	}
	b, err := s.store.GetBOM(ctx, bomID)
	if err != nil {
		return Idea{}, fmt.Errorf("create idea in bom %d: %w", bomID, err)
	}
	if err := s.validateIdea(ctx, b, &in); err != nil {
		return Idea{}, fmt.Errorf("create idea in bom %d: %w", bomID, err)
	}
	out, err := s.ideas.CreateIdea(ctx, bomID, in)
	if err != nil {
		return Idea{}, fmt.Errorf("create idea in bom %d: %w", bomID, err)
	}
	return out, nil
}

// UpdateIdea applique les champs fournis puis revalide la piste entière.
func (s *Service) UpdateIdea(ctx context.Context, bomID, id int64, p IdeaPatch) (Idea, error) {
	if s.ideas == nil {
		return Idea{}, ErrIdeasUnavailable
	}
	b, err := s.store.GetBOM(ctx, bomID)
	if err != nil {
		return Idea{}, fmt.Errorf("update idea %d in bom %d: %w", id, bomID, err)
	}
	cur, err := s.ideas.GetIdea(ctx, bomID, id)
	if err != nil {
		return Idea{}, fmt.Errorf("update idea %d in bom %d: %w", id, bomID, err)
	}
	in := IdeaInput{Title: cur.Title, Owner: cur.Owner, Gain: cur.Gain, Status: cur.Status, DraftBOMID: cur.DraftBOMID}
	if p.Title != nil {
		in.Title = *p.Title
	}
	if p.Owner != nil {
		in.Owner = *p.Owner
	}
	if p.Gain != nil {
		in.Gain = *p.Gain
	}
	if p.Status != nil {
		in.Status = *p.Status
	}
	if p.DraftBOMID != nil {
		in.DraftBOMID = p.DraftBOMID
	}
	if p.ClearDraft {
		in.DraftBOMID = nil
	}
	if err := s.validateIdea(ctx, b, &in); err != nil {
		return Idea{}, fmt.Errorf("update idea %d in bom %d: %w", id, bomID, err)
	}
	out, err := s.ideas.UpdateIdea(ctx, bomID, id, in)
	if err != nil {
		return Idea{}, fmt.Errorf("update idea %d in bom %d: %w", id, bomID, err)
	}
	return out, nil
}

// DeleteIdea retire la piste.
func (s *Service) DeleteIdea(ctx context.Context, bomID, id int64) error {
	if s.ideas == nil {
		return ErrIdeasUnavailable
	}
	if err := s.ideas.DeleteIdea(ctx, bomID, id); err != nil {
		return fmt.Errorf("delete idea %d in bom %d: %w", id, bomID, err)
	}
	return nil
}

// TargetBudgetList renvoie les budgets cibles bruts de la BOM (sans calcul de coût).
func (s *Service) TargetBudgetList(ctx context.Context, bomID int64) ([]TargetBudget, error) {
	out, err := s.store.TargetBudgets(ctx, bomID)
	if err != nil {
		return nil, fmt.Errorf("target budgets of bom %d: %w", bomID, err)
	}
	return out, nil
}

// CheckFrozenBudgets juge les budgets figés d'un snapshot (nœuds et budgets sous la même clé) selon la règle de
// l'ADR 0042 : aucun ensemble budgété n'est dépassé par ses sous-budgets. Les taux sont ceux du snapshot de prix
// de b (b.PriceSnapshotID), pas ceux du jour : le même snapshot donne le même verdict (ADR 0044). Erreurs
// nommées : ErrBudgetExceedsParent, pricing.ErrNoRate.
func (s *Service) CheckFrozenBudgets(ctx context.Context, b BOM, nodes []Node, budgets []TargetBudget) error {
	fx, err := s.budgetFX(ctx, b, budgets)
	if err != nil {
		return err
	}
	touched := make([]int64, len(budgets))
	for i, x := range budgets {
		touched[i] = x.NodeID
	}
	return CheckBudgetTree(nodes, budgets, touched, fx)
}

// SetTargetBudget enregistre le budget cible d'un nœud (copie d'un brouillon depuis un snapshot).
func (s *Service) SetTargetBudget(ctx context.Context, bomID int64, b TargetBudget) error {
	if b.Amount.IsNegative() {
		return fmt.Errorf("budget of node %d: %s must be >= 0: %w", b.NodeID, b.Amount, ErrInvalidInput)
	}
	if _, err := money.ParseCurrency(b.Currency); err != nil {
		return fmt.Errorf("budget of node %d: %w: %w", b.NodeID, err, ErrInvalidInput)
	}
	if err := s.store.SetTargetBudget(ctx, bomID, b); err != nil {
		return fmt.Errorf("set budget of node %d in bom %d: %w", b.NodeID, bomID, err)
	}
	return nil
}

// IdeaList renvoie les pistes de la BOM, sans le calcul de l'écart (lecture légère).
func (s *Service) IdeaList(ctx context.Context, bomID int64) ([]Idea, error) {
	if s.ideas == nil {
		return nil, ErrIdeasUnavailable
	}
	out, err := s.ideas.ListIdeas(ctx, bomID)
	if err != nil {
		return nil, fmt.Errorf("ideas of bom %d: %w", bomID, err)
	}
	return out, nil
}
