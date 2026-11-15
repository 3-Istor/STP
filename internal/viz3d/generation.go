package viz3d

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"bomstudio/internal/bom"
	"bomstudio/internal/catalog"
)

// GenerationView est l'état d'une génération tel que l'API le rend : le statut stocké, ou « stale » si
// l'arbre a changé depuis, ou « none » si rien n'a été lancé.
type GenerationView struct {
	BOMID     int64             `json:"bom_id"`
	Status    Status            `json:"status"`
	Done      int               `json:"done"`
	Total     int               `json:"total"`
	Error     string            `json:"error"`
	Result    *GenerationResult `json:"result"`
	UpdatedAt *time.Time        `json:"updated_at"`
}

// view calcule la vue d'une génération face à l'empreinte courante de l'arbre.
func view(g Generation, currentFingerprint string) GenerationView {
	v := GenerationView{BOMID: g.BOMID, Status: g.Status, Done: g.Done, Total: g.Total, Error: g.Error, Result: g.Result}
	if !g.UpdatedAt.IsZero() {
		t := g.UpdatedAt
		v.UpdatedAt = &t
	}
	if g.Status == StatusDone && g.Fingerprint != currentFingerprint {
		v.Status = StatusStale
	}
	return v
}

// GenerationStatus renvoie l'état de la génération d'une BOM ; bom.ErrBOMNotFound si la BOM n'existe pas,
// statut « none » si rien n'a été lancé.
func (s *Service) GenerationStatus(ctx context.Context, bomID int64) (GenerationView, error) {
	if s.store == nil {
		return GenerationView{}, ErrModelsUnavailable
	}
	_, nodes, err := s.trees.Tree(ctx, bomID)
	if err != nil {
		return GenerationView{}, fmt.Errorf("generation status of bom %d: %w", bomID, err)
	}
	return s.status(ctx, bomID, nodes)
}

func (s *Service) status(ctx context.Context, bomID int64, nodes []bom.Node) (GenerationView, error) {
	g, err := s.store.Generation(ctx, bomID)
	switch {
	case errors.Is(err, ErrNoGeneration):
		return GenerationView{BOMID: bomID, Status: StatusNone}, nil
	case err != nil:
		return GenerationView{}, fmt.Errorf("generation status of bom %d: %w", bomID, err)
	}
	return view(g, Fingerprint(nodes)), nil
}

// StartGeneration prend la génération d'une BOM (ErrGenerationRunning si elle est déjà en attente ou en
// cours) et la lance en tâche de fond. L'état rendu est celui du lancement (« queued »).
func (s *Service) StartGeneration(ctx context.Context, bomID int64) (GenerationView, error) {
	if s.store == nil || s.finder == nil {
		return GenerationView{}, ErrModelsUnavailable
	}
	_, nodes, err := s.trees.Tree(ctx, bomID)
	if err != nil {
		return GenerationView{}, fmt.Errorf("start generation of bom %d: %w", bomID, err)
	}
	fp := Fingerprint(nodes)
	g, err := s.store.ClaimGeneration(ctx, bomID, len(distinctPartIDs(nodes)), fp)
	if err != nil {
		return GenerationView{}, fmt.Errorf("start generation of bom %d: %w", bomID, err)
	}
	s.launch(func(ctx context.Context) { s.runGeneration(ctx, bomID) })
	return view(g, fp), nil
}

// ResumeGenerations relance, au démarrage, les générations laissées en attente ou en cours par un arrêt.
// La reprise saute les pièces déjà résolues (part_models) : rien n'est refait.
func (s *Service) ResumeGenerations(ctx context.Context) error {
	if s.store == nil || s.finder == nil {
		return nil
	}
	pending, err := s.store.PendingGenerations(ctx)
	if err != nil {
		return fmt.Errorf("resume generations: %w", err)
	}
	for _, g := range pending {
		slog.InfoContext(ctx, "resuming 3d generation", "bom_id", g.BOMID, "done", g.Done, "total", g.Total)
		s.launch(func(ctx context.Context) { s.runGeneration(ctx, g.BOMID) })
	}
	return nil
}

// runGeneration résout un modèle par pièce distincte de la BOM, en persistant chaque association au fil
// de l'eau. Une annulation du contexte (arrêt du serveur) laisse l'état « running » : la reprise au
// démarrage reprend où elle en était. Toute autre erreur termine la génération en « failed », nommément.
func (s *Service) runGeneration(ctx context.Context, bomID int64) {
	if err := s.generate(ctx, bomID); err != nil {
		if ctx.Err() != nil {
			slog.WarnContext(ctx, "3d generation interrupted; will resume at startup", "bom_id", bomID, "err", err)
			return
		}
		slog.ErrorContext(ctx, "3d generation failed", "bom_id", bomID, "err", err)
		if _, ferr := s.store.FinishGeneration(context.WithoutCancel(ctx), bomID, StatusFailed, 0, err.Error(), nil); ferr != nil {
			slog.ErrorContext(ctx, "record failed 3d generation", "bom_id", bomID, "err", ferr)
		}
	}
}

func (s *Service) generate(ctx context.Context, bomID int64) error {
	_, nodes, err := s.trees.Tree(ctx, bomID)
	if err != nil {
		return fmt.Errorf("generate bom %d: %w", bomID, err)
	}
	ids := distinctPartIDs(nodes)
	resolved, err := s.store.PartModels(ctx, ids)
	if err != nil {
		return fmt.Errorf("generate bom %d: %w", bomID, err)
	}
	result := &GenerationResult{Finder: s.finder.Name(), Failed: []FailedPart{}}
	done := 0
	for _, r := range resolved {
		result.count(r.PartModel.Match)
		done++
	}
	if _, err := s.store.SetGenerationProgress(ctx, bomID, done); err != nil {
		return fmt.Errorf("generate bom %d: %w", bomID, err)
	}
	for _, id := range ids {
		if _, ok := resolved[id]; ok {
			continue // reprise : pièce déjà résolue par une génération précédente (ou une autre BOM)
		}
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("generate bom %d: %w", bomID, err)
		}
		match, err := s.resolvePart(ctx, id)
		switch {
		case err == nil:
			result.count(match)
		case errors.Is(err, ErrNoModel), errors.Is(err, catalog.ErrPartNotFound):
			result.Failed = append(result.Failed, FailedPart{PartID: id, Reason: err.Error()})
		default:
			return fmt.Errorf("generate bom %d: part %d: %w", bomID, id, err)
		}
		done++
		if _, err := s.store.SetGenerationProgress(ctx, bomID, done); err != nil {
			return fmt.Errorf("generate bom %d: %w", bomID, err)
		}
	}
	if _, err := s.store.FinishGeneration(ctx, bomID, StatusDone, done, "", result); err != nil {
		return fmt.Errorf("generate bom %d: %w", bomID, err)
	}
	return nil
}

// resolvePart cherche, crée et associe le modèle d'une pièce ; renvoie le niveau de correspondance.
func (s *Service) resolvePart(ctx context.Context, partID int64) (MatchType, error) {
	p, err := s.parts.Part(ctx, partID)
	if err != nil {
		return "", err
	}
	c, err := s.finder.Find(ctx, p)
	if err != nil {
		return "", err
	}
	if !ValidMatch(c.Match) || !ValidConfidence(c.Confidence) {
		return "", fmt.Errorf("finder %s returned match %q confidence %s: %w", s.finder.Name(), c.Match, c.Confidence, ErrInvalidModel)
	}
	m, err := s.store.CreateModel(ctx, c.Model)
	if err != nil {
		return "", err
	}
	if _, err := s.store.SetPartModel(ctx, PartModel{PartID: partID, ModelID: m.ID, Match: c.Match, Confidence: c.Confidence}); err != nil {
		return "", err
	}
	return c.Match, nil
}

func (r *GenerationResult) count(m MatchType) {
	switch m {
	case MatchExact:
		r.Exact++
	case MatchSimilar:
		r.Similar++
	case MatchGenerated:
		r.Generated++
	}
}

// enrich applique les modèles persistés à la scène générée : un modèle exact ou similaire remplace le
// niveau, la confiance et donne l'URL du fichier ; une primitive persistée garde la forme et la confiance
// calculées (même information, mieux expliquée par le placement). L'état de génération est joint.
func (s *Service) enrich(ctx context.Context, a *Assembly, nodes []bom.Node) error {
	resolved, err := s.store.PartModels(ctx, distinctPartIDs(nodes))
	if err != nil {
		return err
	}
	for i := range a.Placements {
		p := &a.Placements[i]
		r, ok := resolved[p.PartID]
		if !ok {
			continue
		}
		id := r.Model.ID
		p.ModelID = &id
		if r.PartModel.Match == MatchGenerated {
			continue
		}
		p.Match = r.PartModel.Match
		p.Confidence = r.PartModel.Confidence.StringFixed(2)
		if r.Model.Format == FormatGLB && s.modelURL != nil {
			u := s.modelURL(r.Model.ID)
			p.ModelURL = &u
		}
	}
	g, err := s.status(ctx, a.BOMID, nodes)
	if err != nil {
		return err
	}
	a.Generation = &g
	return nil
}
