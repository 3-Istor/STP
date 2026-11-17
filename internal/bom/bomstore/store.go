// Package bomstore implémente bom.Store sur Postgres (pgx + requêtes sqlc de bom/bomdb).
// Il est séparé du package bom pour que le domaine ne dépende pas de pgx (AGENTS §2).
package bomstore

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"

	"bomstudio/internal/bom"
	"bomstudio/internal/bom/bomdb"
)

// DB est ce dont le store a besoin : exécuter des requêtes et ouvrir une transaction.
// *pgxpool.Pool et pgx.Tx (transaction imbriquée = savepoint) la satisfont tous deux.
type DB interface {
	bomdb.DBTX
	Begin(ctx context.Context) (pgx.Tx, error)
}

// Store est l'implémentation Postgres de bom.Store.
type Store struct {
	db DB
	q  *bomdb.Queries
}

var _ bom.Store = (*Store)(nil)

// New câble le store sur une base.
func New(db DB) *Store {
	return &Store{db: db, q: bomdb.New(db)}
}

// Noms des contraintes (migrations 00001 et 00002) traduites en erreurs métier.
const (
	fkSnapshotBOM = "bom_snapshots_bom_id_fkey"
	fkNodeParent  = "bom_nodes_bom_id_parent_id_fkey"
	checkNodeKind = "bom_nodes_kind"
	fkStockBOM    = "bom_part_stock_bom_id_fkey"
	fkScheduleBOM = "bom_schedule_bom_id_fkey"
)

func (s *Store) inTx(ctx context.Context, fn func(q *bomdb.Queries) error) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	// Sans effet après Commit ; annule tout si fn échoue.
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()
	if err := fn(s.q.WithTx(tx)); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

// CreateBOM crée la BOM et sa racine dans une même transaction.
func (s *Store) CreateBOM(ctx context.Context, b bom.NewBOM) (bom.BOM, error) {
	var out bom.BOM
	err := s.inTx(ctx, func(q *bomdb.Queries) error {
		row, err := q.CreateBOM(ctx, bomdb.CreateBOMParams{
			Name: b.Name, Currency: b.Currency, Volume: b.Volume, SourceSnapshotID: optInt8(b.SourceSnapshotID),
			AnnualHours: annualHours(b.AnnualHours), DestinationCountry: optText(b.DestinationCountry),
		})
		if err != nil {
			return err
		}
		if _, err := q.CreateRootNode(ctx, bomdb.CreateRootNodeParams{
			BomID: row.ID, Name: pgtype.Text{String: b.Name, Valid: true},
		}); err != nil {
			return fmt.Errorf("root node: %w", err)
		}
		out = toBOM(row)
		return nil
	})
	if err != nil {
		return bom.BOM{}, fmt.Errorf("create bom %q: %w", b.Name, err)
	}
	return out, nil
}

// ListBOMs renvoie toutes les BOM, triées par id.
func (s *Store) ListBOMs(ctx context.Context) ([]bom.BOM, error) {
	rows, err := s.q.ListBOMs(ctx)
	if err != nil {
		return nil, fmt.Errorf("list boms: %w", err)
	}
	out := make([]bom.BOM, len(rows))
	for i, r := range rows {
		out[i] = toBOM(r)
	}
	return out, nil
}

// GetBOM renvoie la BOM, ou bom.ErrBOMNotFound.
func (s *Store) GetBOM(ctx context.Context, id int64) (bom.BOM, error) {
	row, err := s.q.GetBOM(ctx, id)
	return bomOrErr(row, err, fmt.Sprintf("get bom %d", id))
}

// RenameBOM renomme la BOM (sa racine garde son nom).
func (s *Store) RenameBOM(ctx context.Context, id int64, name string) (bom.BOM, error) {
	row, err := s.q.RenameBOM(ctx, bomdb.RenameBOMParams{ID: id, Name: name})
	return bomOrErr(row, err, fmt.Sprintf("rename bom %d", id))
}

// UpdateSettings change la devise de référence et le volume.
func (s *Store) UpdateSettings(ctx context.Context, id int64, currency string, volume int32) (bom.BOM, error) {
	row, err := s.q.UpdateBOMSettings(ctx, bomdb.UpdateBOMSettingsParams{
		ID: id, Currency: currency, Volume: volume,
	})
	return bomOrErr(row, err, fmt.Sprintf("update settings of bom %d", id))
}

// Stock renvoie le stock de la BOM, trié par pièce (ADR 0022).
func (s *Store) Stock(ctx context.Context, bomID int64) ([]bom.PartStock, error) {
	rows, err := s.q.ListPartStock(ctx, bomID)
	if err != nil {
		return nil, fmt.Errorf("stock of bom %d: %w", bomID, err)
	}
	out := make([]bom.PartStock, len(rows))
	for i, r := range rows {
		out[i] = bom.PartStock{PartID: r.PartID, Quantity: r.Quantity}
	}
	return out, nil
}

// SetStock enregistre le stock d'une pièce ; une BOM absente (FK) devient bom.ErrBOMNotFound.
func (s *Store) SetStock(ctx context.Context, bomID int64, st bom.PartStock) (bom.PartStock, error) {
	row, err := s.q.UpsertPartStock(ctx, bomdb.UpsertPartStockParams{BomID: bomID, PartID: st.PartID, Quantity: st.Quantity})
	if isConstraint(err, fkStockBOM) {
		err = bom.ErrBOMNotFound
	}
	if err != nil {
		return bom.PartStock{}, fmt.Errorf("set stock of part %d in bom %d: %w", st.PartID, bomID, err)
	}
	return bom.PartStock{PartID: row.PartID, Quantity: row.Quantity}, nil
}

// DeleteStock retire le stock d'une pièce.
func (s *Store) DeleteStock(ctx context.Context, bomID, partID int64) error {
	if _, err := s.q.DeletePartStock(ctx, bomdb.DeletePartStockParams{BomID: bomID, PartID: partID}); err != nil {
		return fmt.Errorf("delete stock of part %d in bom %d: %w", partID, bomID, err)
	}
	return nil
}

// Schedule renvoie le planning de la BOM, ou nil s'il n'y en a pas (ADR 0023).
func (s *Store) Schedule(ctx context.Context, bomID int64) (*bom.Schedule, error) {
	row, err := s.q.GetSchedule(ctx, bomID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("schedule of bom %d: %w", bomID, err)
	}
	out := toSchedule(row.StartOn, row.TargetOn)
	return &out, nil
}

// SetSchedule enregistre le planning ; une BOM absente (FK) devient bom.ErrBOMNotFound.
func (s *Store) SetSchedule(ctx context.Context, bomID int64, sch bom.Schedule) (bom.Schedule, error) {
	row, err := s.q.UpsertSchedule(ctx, bomdb.UpsertScheduleParams{
		BomID: bomID, StartOn: optDate(&sch.StartOn), TargetOn: optDate(sch.TargetOn),
	})
	if isConstraint(err, fkScheduleBOM) {
		err = bom.ErrBOMNotFound
	}
	if err != nil {
		return bom.Schedule{}, fmt.Errorf("set schedule of bom %d: %w", bomID, err)
	}
	return toSchedule(row.StartOn, row.TargetOn), nil
}

// DeleteSchedule retire le planning de la BOM.
func (s *Store) DeleteSchedule(ctx context.Context, bomID int64) error {
	if _, err := s.q.DeleteSchedule(ctx, bomID); err != nil {
		return fmt.Errorf("delete schedule of bom %d: %w", bomID, err)
	}
	return nil
}

// optDate convertit une date calendaire (nil → NULL).
func optDate(t *time.Time) pgtype.Date {
	if t == nil {
		return pgtype.Date{}
	}
	return pgtype.Date{Time: *t, Valid: true}
}

// ptrDate relit une date (NULL → nil) en minuit UTC.
func ptrDate(d pgtype.Date) *time.Time {
	if !d.Valid {
		return nil
	}
	t := time.Date(d.Time.Year(), d.Time.Month(), d.Time.Day(), 0, 0, 0, 0, time.UTC)
	return &t
}

func toSchedule(start, target pgtype.Date) bom.Schedule {
	return bom.Schedule{StartOn: *ptrDate(start), TargetOn: ptrDate(target)}
}

// SetDestination fixe (ou efface) le pays de destination (ADR 0025).
func (s *Store) SetDestination(ctx context.Context, id int64, country *string) (bom.BOM, error) {
	var c pgtype.Text
	if country != nil {
		c = pgtype.Text{String: *country, Valid: true}
	}
	row, err := s.q.SetBOMDestination(ctx, bomdb.SetBOMDestinationParams{ID: id, DestinationCountry: c})
	return bomOrErr(row, err, fmt.Sprintf("set destination of bom %d", id))
}

func optText(s *string) pgtype.Text {
	if s == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: *s, Valid: true}
}

func ptrText(t pgtype.Text) *string {
	if !t.Valid {
		return nil
	}
	return &t.String
}

// SetAnnualHours change la base horaire annuelle d'un ETP (ADR 0024).
func (s *Store) SetAnnualHours(ctx context.Context, id int64, hours decimal.Decimal) (bom.BOM, error) {
	row, err := s.q.SetBOMAnnualHours(ctx, bomdb.SetBOMAnnualHoursParams{ID: id, AnnualHours: hours})
	return bomOrErr(row, err, fmt.Sprintf("set annual hours of bom %d", id))
}

// annualHours : le service fixe toujours la base ; nil (appel direct du store) → défaut du domaine.
func annualHours(h *decimal.Decimal) decimal.Decimal {
	if h == nil {
		return bom.DefaultAnnualHours
	}
	return *h
}

// SetPriceSnapshot fige les prix (snapshotID non nil) ou les remet en direct (nil).
func (s *Store) SetPriceSnapshot(ctx context.Context, id int64, snapshotID *int64) (bom.BOM, error) {
	row, err := s.q.SetBOMPriceSnapshot(ctx, bomdb.SetBOMPriceSnapshotParams{
		ID: id, PriceSnapshotID: optInt8(snapshotID),
	})
	return bomOrErr(row, err, fmt.Sprintf("set price snapshot of bom %d", id))
}

// DeleteBOM supprime la BOM et son arbre (cascade). La FK sans cascade de bom_snapshots
// fait refuser la suppression par la base dès qu'un snapshot existe : bom.ErrHasSnapshots.
func (s *Store) DeleteBOM(ctx context.Context, id int64) error {
	n, err := s.q.DeleteBOM(ctx, id)
	switch {
	case isConstraint(err, fkSnapshotBOM):
		return fmt.Errorf("delete bom %d: %w", id, bom.ErrHasSnapshots)
	case err != nil:
		return fmt.Errorf("delete bom %d: %w", id, err)
	case n == 0:
		return fmt.Errorf("delete bom %d: %w", id, bom.ErrBOMNotFound)
	}
	return nil
}

// Tree renvoie tous les nœuds de la BOM en une requête, triés par id.
func (s *Store) Tree(ctx context.Context, bomID int64) ([]bom.Node, error) {
	rows, err := s.q.ListNodes(ctx, bomID)
	if err != nil {
		return nil, fmt.Errorf("tree of bom %d: %w", bomID, err)
	}
	if len(rows) == 0 { // toute BOM existante a une racine
		return nil, fmt.Errorf("tree of bom %d: %w", bomID, bom.ErrBOMNotFound)
	}
	out := make([]bom.Node, len(rows))
	for i, r := range rows {
		out[i] = toNode(r)
	}
	return out, nil
}

// AddNode ajoute un nœud sous n.ParentID ; bom.ErrNodeNotFound si le parent n'est pas dans cette BOM.
func (s *Store) AddNode(ctx context.Context, bomID int64, n bom.NewNode) (bom.Node, error) {
	name := pgtype.Text{String: n.Name, Valid: n.Name != ""}
	row, err := s.q.AddNode(ctx, bomdb.AddNodeParams{
		BomID:          bomID,
		ParentID:       pgtype.Int8{Int64: n.ParentID, Valid: true},
		Name:           name,
		PartID:         optInt8(n.PartID),
		OfferID:        optInt8(n.OfferID),
		Quantity:       n.Quantity,
		ProductionDays: n.ProductionDays,
	})
	if isConstraint(err, fkNodeParent) {
		err = bom.ErrNodeNotFound
	}
	if err != nil {
		return bom.Node{}, fmt.Errorf("add node under %d in bom %d: %w", n.ParentID, bomID, err)
	}
	return toNode(row), nil
}

// SetNodeOffer change l'offre d'une pièce ; bom.ErrNotAPart sur un sous-ensemble.
func (s *Store) SetNodeOffer(ctx context.Context, bomID, nodeID, offerID int64) (bom.Node, error) {
	row, err := s.q.SetNodeOffer(ctx, bomdb.SetNodeOfferParams{
		BomID: bomID, ID: nodeID, OfferID: pgtype.Int8{Int64: offerID, Valid: true},
	})
	if isConstraint(err, checkNodeKind) {
		err = bom.ErrNotAPart
	}
	return nodeOrErr(row, err, fmt.Sprintf("set offer of node %d in bom %d", nodeID, bomID))
}

// SetNodeQuantity change la quantité du nœud par unité de son parent.
func (s *Store) SetNodeQuantity(ctx context.Context, bomID, nodeID int64, qty decimal.Decimal) (bom.Node, error) {
	row, err := s.q.SetNodeQuantity(ctx, bomdb.SetNodeQuantityParams{BomID: bomID, ID: nodeID, Quantity: qty})
	return nodeOrErr(row, err, fmt.Sprintf("set quantity of node %d in bom %d", nodeID, bomID))
}

// SetNodeProductionDays change les jours de production propres au nœud.
func (s *Store) SetNodeProductionDays(ctx context.Context, bomID, nodeID int64, days int32) (bom.Node, error) {
	row, err := s.q.SetNodeProductionDays(ctx, bomdb.SetNodeProductionDaysParams{
		BomID: bomID, ID: nodeID, ProductionDays: days,
	})
	return nodeOrErr(row, err, fmt.Sprintf("set production days of node %d in bom %d", nodeID, bomID))
}

// MoveNode rattache le nœud à newParentID. Dans une seule transaction : verrou de la BOM (deux
// déplacements concurrents ne peuvent pas chacun passer le contrôle), contrôle de cycle par CTE
// récursive, puis UPDATE. Déplacer la racine est toujours un cycle (tout nœud en descend).
func (s *Store) MoveNode(ctx context.Context, bomID, nodeID, newParentID int64) (bom.Node, error) {
	var out bom.Node
	err := s.inTx(ctx, func(q *bomdb.Queries) error {
		if _, err := q.LockBOM(ctx, bomID); err != nil {
			return notFound(err, bom.ErrBOMNotFound)
		}
		for _, id := range []int64{nodeID, newParentID} {
			if _, err := q.GetNode(ctx, bomdb.GetNodeParams{BomID: bomID, ID: id}); err != nil {
				return fmt.Errorf("node %d: %w", id, notFound(err, bom.ErrNodeNotFound))
			}
		}
		inSubtree, err := q.IsInSubtree(ctx, bomdb.IsInSubtreeParams{
			BomID: bomID, NodeID: nodeID, CandidateID: newParentID,
		})
		if err != nil {
			return fmt.Errorf("cycle check: %w", err)
		}
		if inSubtree {
			return bom.ErrCycle
		}
		row, err := q.SetNodeParent(ctx, bomdb.SetNodeParentParams{
			BomID: bomID, ID: nodeID, ParentID: pgtype.Int8{Int64: newParentID, Valid: true},
		})
		if err != nil {
			return err
		}
		out = toNode(row)
		return nil
	})
	if err != nil {
		return bom.Node{}, fmt.Errorf("move node %d under %d in bom %d: %w", nodeID, newParentID, bomID, err)
	}
	return out, nil
}

// DeleteNode supprime le nœud et son sous-arbre (FK ON DELETE CASCADE) ; bom.ErrRootNode pour la racine.
func (s *Store) DeleteNode(ctx context.Context, bomID, nodeID int64) error {
	n, err := s.q.DeleteNode(ctx, bomdb.DeleteNodeParams{BomID: bomID, ID: nodeID})
	if err != nil {
		return fmt.Errorf("delete node %d in bom %d: %w", nodeID, bomID, err)
	}
	if n > 0 {
		return nil
	}
	// Rien supprimé : soit le nœud n'existe pas, soit c'est la racine.
	if _, err := s.q.GetNode(ctx, bomdb.GetNodeParams{BomID: bomID, ID: nodeID}); err != nil {
		return fmt.Errorf("delete node %d in bom %d: %w", nodeID, bomID, notFound(err, bom.ErrNodeNotFound))
	}
	return fmt.Errorf("delete node %d in bom %d: %w", nodeID, bomID, bom.ErrRootNode)
}

// bomOrErr convertit le résultat d'une requête sur une BOM : pas de ligne → bom.ErrBOMNotFound.
func bomOrErr(row bomdb.Bom, err error, op string) (bom.BOM, error) {
	if err != nil {
		return bom.BOM{}, fmt.Errorf("%s: %w", op, notFound(err, bom.ErrBOMNotFound))
	}
	return toBOM(row), nil
}

// nodeOrErr convertit le résultat d'une requête sur un nœud : pas de ligne → bom.ErrNodeNotFound.
func nodeOrErr(row bomdb.BomNode, err error, op string) (bom.Node, error) {
	if err != nil {
		return bom.Node{}, fmt.Errorf("%s: %w", op, notFound(err, bom.ErrNodeNotFound))
	}
	return toNode(row), nil
}

// notFound remplace pgx.ErrNoRows par l'erreur métier donnée.
func notFound(err, sentinel error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return sentinel
	}
	return err
}

func isConstraint(err error, name string) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.ConstraintName == name
}

func optInt8(v *int64) pgtype.Int8 {
	if v == nil {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: *v, Valid: true}
}

func ptrInt64(v pgtype.Int8) *int64 {
	if !v.Valid {
		return nil
	}
	return &v.Int64
}

func toBOM(r bomdb.Bom) bom.BOM {
	return bom.BOM{
		ID:                 r.ID,
		Name:               r.Name,
		Currency:           r.Currency,
		Volume:             r.Volume,
		AnnualHours:        r.AnnualHours,
		DestinationCountry: ptrText(r.DestinationCountry),
		PriceSnapshotID:    ptrInt64(r.PriceSnapshotID),
		SourceSnapshotID:   ptrInt64(r.SourceSnapshotID),
		CostMethod: bom.CostMethod{
			Basis: bom.CostBasis(r.CostBasis), FX: bom.FXPolicy(r.FxPolicy), ProgramVolume: ptrInt32(r.ProgramVolume),
		},
		CreatedAt: r.CreatedAt,
	}
}

func ptrInt32(v pgtype.Int4) *int32 {
	if !v.Valid {
		return nil
	}
	return &v.Int32
}

// SetCostMethod enregistre la méthode de calcul du coût (ADR 0031).
func (s *Store) SetCostMethod(ctx context.Context, id int64, m bom.CostMethod) (bom.BOM, error) {
	var pv pgtype.Int4
	if m.ProgramVolume != nil {
		pv = pgtype.Int4{Int32: *m.ProgramVolume, Valid: true}
	}
	row, err := s.q.SetBOMCostMethod(ctx, bomdb.SetBOMCostMethodParams{
		ID: id, CostBasis: string(m.Basis), FxPolicy: string(m.FX), ProgramVolume: pv,
	})
	return bomOrErr(row, err, fmt.Sprintf("set cost method of bom %d", id))
}

// TargetBudgets renvoie les budgets cibles de la BOM, triés par nœud (ADR 0018, 0031).
func (s *Store) TargetBudgets(ctx context.Context, bomID int64) ([]bom.TargetBudget, error) {
	rows, err := s.q.ListTargetBudgets(ctx, bomID)
	if err != nil {
		return nil, fmt.Errorf("target budgets of bom %d: %w", bomID, err)
	}
	out := make([]bom.TargetBudget, len(rows))
	for i, r := range rows {
		out[i] = bom.TargetBudget{NodeID: r.NodeID, Amount: r.Amount, Currency: r.Currency}
	}
	return out, nil
}

func toNode(r bomdb.BomNode) bom.Node {
	return bom.Node{
		ID:             r.ID,
		BOMID:          r.BomID,
		ParentID:       ptrInt64(r.ParentID),
		Name:           r.Name.String,
		PartID:         ptrInt64(r.PartID),
		OfferID:        ptrInt64(r.OfferID),
		Quantity:       r.Quantity,
		ProductionDays: r.ProductionDays,
		Strategy:       ptrStrategy(r.Strategy),
	}
}

func ptrStrategy(t pgtype.Text) *bom.Strategy {
	if !t.Valid {
		return nil
	}
	s := bom.Strategy(t.String)
	return &s
}

// SetNodeStrategy attache (ou retire) la stratégie du nœud (ADR 0028).
func (s *Store) SetNodeStrategy(ctx context.Context, bomID, nodeID int64, st *bom.Strategy) (bom.Node, error) {
	var v pgtype.Text
	if st != nil {
		v = pgtype.Text{String: string(*st), Valid: true}
	}
	row, err := s.q.SetNodeStrategy(ctx, bomdb.SetNodeStrategyParams{BomID: bomID, ID: nodeID, Strategy: v})
	return nodeOrErr(row, err, fmt.Sprintf("set strategy of node %d in bom %d", nodeID, bomID))
}

// SetNodePart remplace la pièce et l'offre d'un nœud pièce ; un sous-ensemble (aucune ligne) → ErrNotAPart
// s'il existe, ErrNodeNotFound sinon.
func (s *Store) SetNodePart(ctx context.Context, bomID, nodeID, partID, offerID int64) (bom.Node, error) {
	row, err := s.q.SetNodePart(ctx, bomdb.SetNodePartParams{BomID: bomID, ID: nodeID, PartID: pgtype.Int8{Int64: partID, Valid: true},
		OfferID: pgtype.Int8{Int64: offerID, Valid: true}})
	if errors.Is(err, pgx.ErrNoRows) {
		if _, getErr := s.q.GetNode(ctx, bomdb.GetNodeParams{BomID: bomID, ID: nodeID}); getErr == nil {
			err = bom.ErrNotAPart
		}
	}
	return nodeOrErr(row, err, fmt.Sprintf("set part of node %d in bom %d", nodeID, bomID))
}
