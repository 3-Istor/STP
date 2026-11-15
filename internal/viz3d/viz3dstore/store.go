// Package viz3dstore implémente viz3d.Store sur Postgres (pgx + requêtes sqlc de viz3d/viz3ddb).
// Il est séparé du package viz3d pour que le domaine ne dépende pas de pgx (AGENTS §2), comme bom/bomstore.
package viz3dstore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/shopspring/decimal"

	"bomstudio/internal/catalog"
	"bomstudio/internal/viz3d"
	"bomstudio/internal/viz3d/viz3ddb"
)

// Store est l'implémentation Postgres de viz3d.Store (tables models_3d, part_models, bom_assemblies).
type Store struct {
	q *viz3ddb.Queries
}

var _ viz3d.Store = (*Store)(nil)

// New câble le store sur une base (*pgxpool.Pool ou pgx.Tx).
func New(db viz3ddb.DBTX) *Store { return &Store{q: viz3ddb.New(db)} }

// Noms des contraintes (migration 00012) traduites en erreurs métier.
const (
	fkPartModelPart  = "part_models_part_id_fkey"
	fkPartModelModel = "part_models_model_id_fkey"
)

// CreateModel insère un modèle.
func (s *Store) CreateModel(ctx context.Context, m viz3d.Model) (viz3d.Model, error) {
	var dx, dy, dz decimal.NullDecimal
	if m.Dimensions != nil {
		dx = decimal.NullDecimal{Decimal: m.Dimensions.X, Valid: true}
		dy = decimal.NullDecimal{Decimal: m.Dimensions.Y, Valid: true}
		dz = decimal.NullDecimal{Decimal: m.Dimensions.Z, Valid: true}
	}
	row, err := s.q.CreateModel(ctx, viz3ddb.CreateModelParams{
		Name: m.Name, Source: m.Source, SourceUrl: m.SourceURL, StoragePath: m.StoragePath, Format: m.Format,
		License: m.License, DimX: dx, DimY: dy, DimZ: dz, Confidence: m.Confidence,
	})
	if err != nil {
		return viz3d.Model{}, fmt.Errorf("create model %q: %w", m.Name, err)
	}
	return toModel(row), nil
}

// Model renvoie le modèle, ou viz3d.ErrModelNotFound.
func (s *Store) Model(ctx context.Context, id int64) (viz3d.Model, error) {
	row, err := s.q.GetModel(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return viz3d.Model{}, fmt.Errorf("get model %d: %w", id, viz3d.ErrModelNotFound)
	}
	if err != nil {
		return viz3d.Model{}, fmt.Errorf("get model %d: %w", id, err)
	}
	return toModel(row), nil
}

// SetPartModel enregistre ou remplace le modèle d'une pièce.
func (s *Store) SetPartModel(ctx context.Context, pm viz3d.PartModel) (viz3d.PartModel, error) {
	row, err := s.q.SetPartModel(ctx, viz3ddb.SetPartModelParams{
		PartID: pm.PartID, ModelID: pm.ModelID, MatchType: string(pm.Match), Confidence: pm.Confidence,
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			switch pgErr.ConstraintName {
			case fkPartModelPart:
				return viz3d.PartModel{}, fmt.Errorf("set model of part %d: %w", pm.PartID, catalog.ErrPartNotFound)
			case fkPartModelModel:
				return viz3d.PartModel{}, fmt.Errorf("set model of part %d: model %d: %w", pm.PartID, pm.ModelID, viz3d.ErrModelNotFound)
			}
		}
		return viz3d.PartModel{}, fmt.Errorf("set model of part %d: %w", pm.PartID, err)
	}
	return toPartModel(row), nil
}

// PartModel renvoie le modèle d'une pièce, ou viz3d.ErrNoModel.
func (s *Store) PartModel(ctx context.Context, partID int64) (viz3d.Resolved, error) {
	row, err := s.q.GetPartModel(ctx, partID)
	if errors.Is(err, pgx.ErrNoRows) {
		return viz3d.Resolved{}, fmt.Errorf("model of part %d: %w", partID, viz3d.ErrNoModel)
	}
	if err != nil {
		return viz3d.Resolved{}, fmt.Errorf("model of part %d: %w", partID, err)
	}
	return viz3d.Resolved{PartModel: toPartModel(row.PartModel), Model: toModel(row.Models3d)}, nil
}

// PartModels renvoie les modèles des pièces données, indexés par pièce.
func (s *Store) PartModels(ctx context.Context, partIDs []int64) (map[int64]viz3d.Resolved, error) {
	out := make(map[int64]viz3d.Resolved, len(partIDs))
	if len(partIDs) == 0 {
		return out, nil
	}
	rows, err := s.q.ListPartModels(ctx, partIDs)
	if err != nil {
		return nil, fmt.Errorf("models of %d parts: %w", len(partIDs), err)
	}
	for _, r := range rows {
		out[r.PartModel.PartID] = viz3d.Resolved{PartModel: toPartModel(r.PartModel), Model: toModel(r.Models3d)}
	}
	return out, nil
}

// ClaimGeneration prend la génération d'une BOM en une seule instruction (INSERT … ON CONFLICT … WHERE) :
// zéro ligne = déjà en attente ou en cours → viz3d.ErrGenerationRunning. Un bom_id inconnu viole la clé
// étrangère : le service a déjà lu l'arbre, c'est une course, rendue telle quelle.
func (s *Store) ClaimGeneration(ctx context.Context, bomID int64, total int, fingerprint string) (viz3d.Generation, error) {
	row, err := s.q.ClaimGeneration(ctx, viz3ddb.ClaimGenerationParams{BomID: bomID, Total: int32(total), TreeFingerprint: fingerprint}) //nolint:gosec // total : nombre de pièces, loin de 2^31
	if errors.Is(err, pgx.ErrNoRows) {
		return viz3d.Generation{}, fmt.Errorf("claim generation of bom %d: %w", bomID, viz3d.ErrGenerationRunning)
	}
	if err != nil {
		return viz3d.Generation{}, fmt.Errorf("claim generation of bom %d: %w", bomID, err)
	}
	return toGeneration(row)
}

// Generation renvoie l'état de la génération d'une BOM, ou viz3d.ErrNoGeneration.
func (s *Store) Generation(ctx context.Context, bomID int64) (viz3d.Generation, error) {
	row, err := s.q.GetGeneration(ctx, bomID)
	if errors.Is(err, pgx.ErrNoRows) {
		return viz3d.Generation{}, fmt.Errorf("generation of bom %d: %w", bomID, viz3d.ErrNoGeneration)
	}
	if err != nil {
		return viz3d.Generation{}, fmt.Errorf("generation of bom %d: %w", bomID, err)
	}
	return toGeneration(row)
}

// SetGenerationProgress passe en « running » et enregistre l'avancement.
func (s *Store) SetGenerationProgress(ctx context.Context, bomID int64, done int) (viz3d.Generation, error) {
	row, err := s.q.UpdateGenerationProgress(ctx, viz3ddb.UpdateGenerationProgressParams{BomID: bomID, Done: int32(done)}) //nolint:gosec // nombre de pièces
	if errors.Is(err, pgx.ErrNoRows) {
		return viz3d.Generation{}, fmt.Errorf("progress of bom %d: %w", bomID, viz3d.ErrNoGeneration)
	}
	if err != nil {
		return viz3d.Generation{}, fmt.Errorf("progress of bom %d: %w", bomID, err)
	}
	return toGeneration(row)
}

// FinishGeneration termine la génération avec son statut final et son bilan.
func (s *Store) FinishGeneration(ctx context.Context, bomID int64, status viz3d.Status, done int, errText string, result *viz3d.GenerationResult) (viz3d.Generation, error) {
	var raw []byte
	if result != nil {
		var err error
		if raw, err = json.Marshal(result); err != nil {
			return viz3d.Generation{}, fmt.Errorf("finish generation of bom %d: encode result: %w", bomID, err)
		}
	}
	row, err := s.q.FinishGeneration(ctx, viz3ddb.FinishGenerationParams{
		BomID: bomID, Status: string(status), Done: int32(done), Error: errText, Result: raw, //nolint:gosec // nombre de pièces
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return viz3d.Generation{}, fmt.Errorf("finish generation of bom %d: %w", bomID, viz3d.ErrNoGeneration)
	}
	if err != nil {
		return viz3d.Generation{}, fmt.Errorf("finish generation of bom %d: %w", bomID, err)
	}
	return toGeneration(row)
}

// PendingGenerations : générations en attente ou en cours, par BOM croissante.
func (s *Store) PendingGenerations(ctx context.Context) ([]viz3d.Generation, error) {
	rows, err := s.q.ListPendingGenerations(ctx)
	if err != nil {
		return nil, fmt.Errorf("pending generations: %w", err)
	}
	out := make([]viz3d.Generation, 0, len(rows))
	for _, r := range rows {
		g, err := toGeneration(r)
		if err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, nil
}

func toModel(r viz3ddb.Models3d) viz3d.Model {
	m := viz3d.Model{ID: r.ID, Name: r.Name, Source: r.Source, SourceURL: r.SourceUrl, StoragePath: r.StoragePath,
		Format: r.Format, License: r.License, Confidence: r.Confidence, CreatedAt: r.CreatedAt}
	if r.DimX.Valid && r.DimY.Valid && r.DimZ.Valid {
		m.Dimensions = &viz3d.Dimensions{X: r.DimX.Decimal, Y: r.DimY.Decimal, Z: r.DimZ.Decimal}
	}
	return m
}

func toPartModel(r viz3ddb.PartModel) viz3d.PartModel {
	return viz3d.PartModel{PartID: r.PartID, ModelID: r.ModelID, Match: viz3d.MatchType(r.MatchType), Confidence: r.Confidence, CreatedAt: r.CreatedAt}
}

func toGeneration(r viz3ddb.BomAssembly) (viz3d.Generation, error) {
	g := viz3d.Generation{BOMID: r.BomID, Status: viz3d.Status(r.Status), Done: int(r.Done), Total: int(r.Total), Error: r.Error,
		Fingerprint: r.TreeFingerprint, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt}
	if len(r.Result) > 0 {
		var res viz3d.GenerationResult
		if err := json.Unmarshal(r.Result, &res); err != nil {
			return viz3d.Generation{}, fmt.Errorf("generation of bom %d: decode result: %w", r.BomID, err)
		}
		g.Result = &res
	}
	return g, nil
}
