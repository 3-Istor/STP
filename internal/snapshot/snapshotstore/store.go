// Package snapshotstore implémente snapshot.Store sur Postgres (pgx + requêtes sqlc de snapshot/snapshotdb).
// Il est séparé du package snapshot pour que le domaine ne dépende pas de pgx (AGENTS §2), comme bom/bomstore.
package snapshotstore

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"

	"bomstudio/internal/bom"
	"bomstudio/internal/money"
	"bomstudio/internal/snapshot"
	"bomstudio/internal/snapshot/snapshotdb"
)

// DB est ce dont le store a besoin : exécuter des requêtes, COPY et ouvrir une transaction.
// *pgxpool.Pool et pgx.Tx (transaction imbriquée = savepoint) la satisfont tous deux.
type DB interface {
	snapshotdb.DBTX
	Begin(ctx context.Context) (pgx.Tx, error)
}

// Store est l'implémentation Postgres de snapshot.Store. Il n'écrit que bom_snapshots, bom_snapshot_nodes et
// bom_snapshot_part_stock.
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
func (s *Store) Create(ctx context.Context, h snapshot.Snapshot, nodes []snapshot.Node, stock []bom.PartStock, cost snapshot.FrozenCost) (snapshot.Snapshot, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return snapshot.Snapshot{}, fmt.Errorf("create snapshot of bom %d: begin: %w", h.BOMID, err)
	}
	// Sans effet après Commit ; annule tout si une étape échoue.
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()
	q := s.q.WithTx(tx)

	row, err := q.CreateSnapshot(ctx, snapshotdb.CreateSnapshotParams{
		BomID: h.BOMID, PriceSnapshotID: h.PriceSnapshotID, Name: h.Name,
		Currency: h.Currency, Volume: h.Volume, PublishedAt: h.PublishedAt, AnnualHours: optDecimal(h.AnnualHours),
		StartOn: startOn(h.Schedule), TargetOn: targetOn(h.Schedule), DestinationCountry: optText(h.DestinationCountry),
		CostBasis: methodText(h.CostMethod, true), FxPolicy: methodText(h.CostMethod, false), ProgramVolume: programVolume(h.CostMethod),
		CostAsOf: optTime(h.CostAsOf),
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
	if len(stock) > 0 {
		rows := make([]snapshotdb.InsertSnapshotStockParams, len(stock))
		for i, st := range stock {
			rows[i] = snapshotdb.InsertSnapshotStockParams{SnapshotID: row.ID, PartID: st.PartID, Quantity: st.Quantity}
		}
		if _, err := q.InsertSnapshotStock(ctx, rows); err != nil {
			return snapshot.Snapshot{}, fmt.Errorf("create snapshot of bom %d: stock: %w", h.BOMID, err)
		}
	}
	if err := insertCost(ctx, q, row.ID, cost); err != nil {
		return snapshot.Snapshot{}, fmt.Errorf("create snapshot of bom %d: %w", h.BOMID, err)
	}
	if err := insertProfile(ctx, q, row.ID, h.Profile); err != nil {
		return snapshot.Snapshot{}, fmt.Errorf("create snapshot of bom %d: %w", h.BOMID, err)
	}
	if err := tx.Commit(ctx); err != nil {
		return snapshot.Snapshot{}, fmt.Errorf("create snapshot of bom %d: commit: %w", h.BOMID, err)
	}
	out := toSnapshot(row)
	out.Profile = h.Profile
	return out, nil
}

// insertCost copie les taux budget et les faits de coût par offre (ADR 0031), chacun en un COPY.
func insertCost(ctx context.Context, q *snapshotdb.Queries, id int64, cost snapshot.FrozenCost) error {
	if len(cost.BudgetRates) > 0 {
		rows := make([]snapshotdb.InsertSnapshotBudgetRatesParams, len(cost.BudgetRates))
		for i, r := range cost.BudgetRates {
			rows[i] = snapshotdb.InsertSnapshotBudgetRatesParams{SnapshotID: id, Year: r.Year, FromCurrency: string(r.From),
				ToCurrency: string(r.To), Rate: r.Rate}
		}
		if _, err := q.InsertSnapshotBudgetRates(ctx, rows); err != nil {
			return fmt.Errorf("budget rates: %w", err)
		}
	}
	if len(cost.Offers) == 0 {
		return nil
	}
	rows := make([]snapshotdb.InsertSnapshotOfferCostsParams, len(cost.Offers))
	for i, o := range cost.Offers {
		r := snapshotdb.InsertSnapshotOfferCostsParams{SnapshotID: id, OfferID: o.OfferID, OriginCountry: o.Origin,
			CustomsPercent: optDecimal(o.CustomsPercent)}
		r.FreightAmountPerKg, r.FreightCurrency = optMoney(o.FreightPerKg)
		r.PackagingAmount, r.PackagingCurrency = optMoney(o.Packaging)
		r.ToolingAmount, r.ToolingCurrency = optMoney(o.Tooling)
		if d := o.Discount; d != nil {
			r.DiscountPercent = decimal.NullDecimal{Decimal: d.Percent, Valid: true}
			r.DiscountReason = pgtype.Text{String: d.Reason, Valid: true}
			r.DiscountContractID = optInt8(d.ContractID)
			r.DiscountTierMin = optDecimal(d.TierMin)
		}
		rows[i] = r
	}
	if _, err := q.InsertSnapshotOfferCosts(ctx, rows); err != nil {
		return fmt.Errorf("offer costs: %w", err)
	}
	return nil
}

// CostFacts relit les faits de coût figés du snapshot (ADR 0031).
func (s *Store) CostFacts(ctx context.Context, id int64) (snapshot.FrozenCost, error) {
	var out snapshot.FrozenCost
	rates, err := s.q.ListSnapshotBudgetRates(ctx, id)
	if err != nil {
		return out, fmt.Errorf("cost facts of snapshot %d: %w", id, err)
	}
	for _, r := range rates {
		out.BudgetRates = append(out.BudgetRates, snapshot.BudgetRate{Year: r.Year, From: money.Currency(r.FromCurrency),
			To: money.Currency(r.ToCurrency), Rate: r.Rate})
	}
	offers, err := s.q.ListSnapshotOfferCosts(ctx, id)
	if err != nil {
		return out, fmt.Errorf("cost facts of snapshot %d: %w", id, err)
	}
	for _, r := range offers {
		o := snapshot.FrozenOffer{OfferID: r.OfferID, OfferCost: bom.OfferCost{Origin: r.OriginCountry, CustomsPercent: ptrDecimal(r.CustomsPercent)}}
		if o.FreightPerKg, err = ptrMoney(r.FreightAmountPerKg, r.FreightCurrency); err != nil {
			return out, fmt.Errorf("offer %d of snapshot %d: %w", r.OfferID, id, err)
		}
		if o.Packaging, err = ptrMoney(r.PackagingAmount, r.PackagingCurrency); err != nil {
			return out, fmt.Errorf("offer %d of snapshot %d: %w", r.OfferID, id, err)
		}
		if o.Tooling, err = ptrMoney(r.ToolingAmount, r.ToolingCurrency); err != nil {
			return out, fmt.Errorf("offer %d of snapshot %d: %w", r.OfferID, id, err)
		}
		if r.DiscountPercent.Valid {
			d := bom.Discount{Percent: r.DiscountPercent.Decimal, Reason: r.DiscountReason.String, TierMin: ptrDecimal(r.DiscountTierMin)}
			if r.DiscountContractID.Valid {
				d.ContractID = &r.DiscountContractID.Int64
			}
			o.Discount = &d
		}
		out.Offers = append(out.Offers, o)
	}
	return out, nil
}

func optMoney(m *money.Money) (decimal.NullDecimal, pgtype.Text) {
	if m == nil {
		return decimal.NullDecimal{}, pgtype.Text{}
	}
	return decimal.NullDecimal{Decimal: m.Amount(), Valid: true}, pgtype.Text{String: string(m.Currency()), Valid: true}
}

func ptrMoney(amount decimal.NullDecimal, currency pgtype.Text) (*money.Money, error) {
	if !amount.Valid || !currency.Valid {
		return nil, nil //nolint:nilnil // NULL : composante non figée
	}
	m, err := money.New(amount.Decimal, currency.String)
	if err != nil {
		return nil, err
	}
	return &m, nil
}

func methodText(m *bom.CostMethod, basis bool) pgtype.Text {
	if m == nil {
		return pgtype.Text{}
	}
	if basis {
		return pgtype.Text{String: string(m.Basis), Valid: true}
	}
	return pgtype.Text{String: string(m.FX), Valid: true}
}

func programVolume(m *bom.CostMethod) pgtype.Int4 {
	if m == nil || m.ProgramVolume == nil {
		return pgtype.Int4{}
	}
	return pgtype.Int4{Int32: *m.ProgramVolume, Valid: true}
}

func optTime(t *time.Time) pgtype.Timestamptz {
	if t == nil {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: *t, Valid: true}
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
	out := toSnapshot(row)
	if out.Profile, err = s.profile(ctx, id); err != nil {
		return snapshot.Snapshot{}, fmt.Errorf("get snapshot %d: %w", id, err)
	}
	return out, nil
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

// Stock renvoie le stock figé du snapshot, trié par pièce (ADR 0022).
func (s *Store) Stock(ctx context.Context, id int64) ([]bom.PartStock, error) {
	rows, err := s.q.ListSnapshotStock(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("stock of snapshot %d: %w", id, err)
	}
	out := make([]bom.PartStock, len(rows))
	for i, r := range rows {
		out[i] = bom.PartStock{PartID: r.PartID, Quantity: r.Quantity}
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
	if n.Strategy != nil {
		p.Strategy = pgtype.Text{String: string(*n.Strategy), Valid: true}
	}
	if part := n.Part; part != nil {
		p.PartID = pgtype.Int8{Int64: part.PartID, Valid: true}
		p.OfferID = pgtype.Int8{Int64: part.OfferID, Valid: true}
		p.Co2Kg = decimal.NullDecimal{Decimal: part.CO2Kg, Valid: true}
		p.MassKg = decimal.NullDecimal{Decimal: part.MassKg, Valid: true}
		p.LeadTimeDays = pgtype.Int4{Int32: part.LeadTimeDays, Valid: true}
		p.LaborHours = optDecimal(part.LaborHours)
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
	if r.Strategy.Valid {
		st := bom.Strategy(r.Strategy.String)
		n.Strategy = &st
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
		LaborHours: ptrDecimal(r.LaborHours),
	}
	return n, nil
}

func toSnapshot(r snapshotdb.BomSnapshot) snapshot.Snapshot {
	return snapshot.Snapshot{
		ID: r.ID, BOMID: r.BomID, PriceSnapshotID: r.PriceSnapshotID, Name: r.Name,
		Currency: r.Currency, Volume: r.Volume, PublishedAt: r.PublishedAt, AnnualHours: ptrDecimal(r.AnnualHours),
		Schedule: toSchedule(r.StartOn, r.TargetOn), DestinationCountry: ptrText(r.DestinationCountry),
		CostMethod: toMethod(r), CostAsOf: ptrTime(r.CostAsOf),
	}
}

func toMethod(r snapshotdb.BomSnapshot) *bom.CostMethod {
	if !r.CostBasis.Valid || !r.FxPolicy.Valid {
		return nil
	}
	m := bom.CostMethod{Basis: bom.CostBasis(r.CostBasis.String), FX: bom.FXPolicy(r.FxPolicy.String)}
	if r.ProgramVolume.Valid {
		m.ProgramVolume = &r.ProgramVolume.Int32
	}
	return &m
}

func ptrTime(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	v := t.Time.UTC()
	return &v
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

func startOn(s *bom.Schedule) pgtype.Date {
	if s == nil {
		return pgtype.Date{}
	}
	return pgtype.Date{Time: s.StartOn, Valid: true}
}

func targetOn(s *bom.Schedule) pgtype.Date {
	if s == nil || s.TargetOn == nil {
		return pgtype.Date{}
	}
	return pgtype.Date{Time: *s.TargetOn, Valid: true}
}

// toSchedule relit le planning figé (dates à minuit UTC) ; pas de date de départ → pas de planning.
func toSchedule(start, target pgtype.Date) *bom.Schedule {
	if !start.Valid {
		return nil
	}
	day := func(t time.Time) time.Time { return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC) }
	s := &bom.Schedule{StartOn: day(start.Time)}
	if target.Valid {
		t := day(target.Time)
		s.TargetOn = &t
	}
	return s
}

func optDecimal(v *decimal.Decimal) decimal.NullDecimal {
	if v == nil {
		return decimal.NullDecimal{}
	}
	return decimal.NullDecimal{Decimal: *v, Valid: true}
}

// ptrDecimal : NULL reste nil (donnée absente), jamais un zéro (AGENTS §3).
func ptrDecimal(v decimal.NullDecimal) *decimal.Decimal {
	if !v.Valid {
		return nil
	}
	return &v.Decimal
}

func optInt8(v *int64) pgtype.Int8 {
	if v == nil {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: *v, Valid: true}
}
