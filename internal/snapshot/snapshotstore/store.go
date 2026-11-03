// Package snapshotstore implémente snapshot.Store sur Postgres (pgx + requêtes sqlc de snapshot/snapshotdb).
// Il est séparé du package snapshot pour que le domaine ne dépende pas de pgx (AGENTS §2), comme bom/bomstore.
package snapshotstore

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"

	"bomstudio/internal/snapshot"
	"bomstudio/internal/snapshot/snapshotdb"
)

// DB est ce dont le store a besoin : exécuter des requêtes, COPY et ouvrir une transaction.
// *pgxpool.Pool et pgx.Tx (transaction imbriquée = savepoint) la satisfont tous deux.
type DB interface {
	snapshotdb.DBTX
	Begin(ctx context.Context) (pgx.Tx, error)
}

// Store est l'implémentation Postgres de snapshot.Store. Il n'écrit que bom_snapshots et bom_snapshot_nodes.
type Store struct {
	db DB
	q  *snapshotdb.Queries
}

var _ snapshot.Store = (*Store)(nil)

// New câble le store sur une base.
func New(db DB) *Store {
	return &Store{db: db, q: snapshotdb.New(db)}
}

// Create insère l'en-tête puis tous les nœuds (un COPY) dans une même transaction : un snapshot est
// publié en entier ou pas du tout. Les contraintes différées (parent présent) sont vérifiées au commit.
func (s *Store) Create(ctx context.Context, h snapshot.Snapshot, nodes []snapshot.Node) (snapshot.Snapshot, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return snapshot.Snapshot{}, fmt.Errorf("create snapshot of bom %d: begin: %w", h.BOMID, err)
	}
	// Sans effet après Commit ; annule tout si une étape échoue.
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()
	q := s.q.WithTx(tx)

	row, err := q.CreateSnapshot(ctx, snapshotdb.CreateSnapshotParams{
		BomID: h.BOMID, PriceSnapshotID: h.PriceSnapshotID, Name: h.Name,
		Currency: h.Currency, Volume: h.Volume, PublishedAt: h.PublishedAt,
	})
	if err != nil {
		return snapshot.Snapshot{}, fmt.Errorf("create snapshot of bom %d: %w", h.BOMID, err)
	}
	params := make([]snapshotdb.InsertSnapshotNodesParams, len(nodes))
	for i, n := range nodes {
		params[i] = toParams(row.ID, n)
	}
	if _, err := q.InsertSnapshotNodes(ctx, params); err != nil {
		return snapshot.Snapshot{}, fmt.Errorf("create snapshot of bom %d: nodes: %w", h.BOMID, err)
	}
	if err := tx.Commit(ctx); err != nil {
		return snapshot.Snapshot{}, fmt.Errorf("create snapshot of bom %d: commit: %w", h.BOMID, err)
	}
	return toSnapshot(row), nil
}

// Get renvoie l'en-tête, ou snapshot.ErrSnapshotNotFound.
func (s *Store) Get(ctx context.Context, id int64) (snapshot.Snapshot, error) {
	row, err := s.q.GetSnapshot(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		err = snapshot.ErrSnapshotNotFound
	}
	if err != nil {
		return snapshot.Snapshot{}, fmt.Errorf("get snapshot %d: %w", id, err)
	}
	return toSnapshot(row), nil
}

// Nodes renvoie tous les nœuds du snapshot en une requête, triés par clé ; ErrSnapshotNotFound si aucun
// (tout snapshot publié a au moins sa racine).
func (s *Store) Nodes(ctx context.Context, id int64) ([]snapshot.Node, error) {
	rows, err := s.q.ListSnapshotNodes(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("nodes of snapshot %d: %w", id, err)
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("nodes of snapshot %d: %w", id, snapshot.ErrSnapshotNotFound)
	}
	out := make([]snapshot.Node, len(rows))
	for i, r := range rows {
		n, err := toNode(r)
		if err != nil {
			return nil, fmt.Errorf("nodes of snapshot %d: %w", id, err)
		}
		out[i] = n
	}
	return out, nil
}

func toParams(snapID int64, n snapshot.Node) snapshotdb.InsertSnapshotNodesParams {
	p := snapshotdb.InsertSnapshotNodesParams{
		SnapshotID:     snapID,
		NodeKey:        n.Key,
		ParentKey:      optInt8(n.ParentKey),
		Name:           pgtype.Text{String: n.Name, Valid: n.Name != ""},
		Quantity:       n.Quantity,
		ProductionDays: n.ProductionDays,
	}
	if part := n.Part; part != nil {
		p.PartID = pgtype.Int8{Int64: part.PartID, Valid: true}
		p.OfferID = pgtype.Int8{Int64: part.OfferID, Valid: true}
		p.Co2Kg = decimal.NullDecimal{Decimal: part.CO2Kg, Valid: true}
		p.MassKg = decimal.NullDecimal{Decimal: part.MassKg, Valid: true}
		p.LeadTimeDays = pgtype.Int4{Int32: part.LeadTimeDays, Valid: true}
	}
	return p
}

// toNode relit un nœud figé. La contrainte bom_snapshot_nodes_kind garantit qu'une pièce a tous ses
// attributs ; on le revérifie plutôt que de lire un NULL comme un zéro (AGENTS §3).
func toNode(r snapshotdb.BomSnapshotNode) (snapshot.Node, error) {
	n := snapshot.Node{
		Key:            r.NodeKey,
		Name:           r.Name.String,
		Quantity:       r.Quantity,
		ProductionDays: r.ProductionDays,
	}
	if r.ParentKey.Valid {
		n.ParentKey = &r.ParentKey.Int64
	}
	if !r.PartID.Valid {
		return n, nil
	}
	if !r.OfferID.Valid || !r.Co2Kg.Valid || !r.MassKg.Valid || !r.LeadTimeDays.Valid {
		return snapshot.Node{}, fmt.Errorf("part node %d: missing frozen attribute", r.NodeKey)
	}
	n.Part = &snapshot.Part{
		PartID: r.PartID.Int64, OfferID: r.OfferID.Int64,
		CO2Kg: r.Co2Kg.Decimal, MassKg: r.MassKg.Decimal, LeadTimeDays: r.LeadTimeDays.Int32,
	}
	return n, nil
}

func toSnapshot(r snapshotdb.BomSnapshot) snapshot.Snapshot {
	return snapshot.Snapshot{
		ID: r.ID, BOMID: r.BomID, PriceSnapshotID: r.PriceSnapshotID, Name: r.Name,
		Currency: r.Currency, Volume: r.Volume, PublishedAt: r.PublishedAt,
	}
}

func optInt8(v *int64) pgtype.Int8 {
	if v == nil {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: *v, Valid: true}
}
