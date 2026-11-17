package bom

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"sync"

	"github.com/shopspring/decimal"
)

// memStore est un Store en mémoire pour les tests du service : mêmes erreurs métier que bomstore
// (ErrBOMNotFound, ErrNodeNotFound, ErrCycle, ErrRootNode, ErrHasSnapshots, ErrNotAPart).
// Le comportement Postgres réel est couvert par bomstore/store_integration_test.go.
type memStore struct {
	mu        sync.Mutex
	boms      map[int64]BOM
	nodes     map[int64]Node
	published map[int64]bool               // BOM qui ont un snapshot publié
	stock     map[[2]int64]decimal.Decimal // (bom, pièce) → quantité
	schedules map[int64]Schedule
	budgets   map[int64][]TargetBudget
	nextID    int64
}

var _ Store = (*memStore)(nil)

func newMemStore() *memStore {
	return &memStore{boms: map[int64]BOM{}, nodes: map[int64]Node{}, published: map[int64]bool{}, stock: map[[2]int64]decimal.Decimal{},
		schedules: map[int64]Schedule{}}
}

func (m *memStore) id() int64 { m.nextID++; return m.nextID }

func (m *memStore) CreateBOM(_ context.Context, b NewBOM) (BOM, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := BOM{ID: m.id(), Name: b.Name, Currency: b.Currency, Volume: b.Volume, SourceSnapshotID: b.SourceSnapshotID}
	if b.AnnualHours != nil {
		out.AnnualHours = *b.AnnualHours
	}
	out.DestinationCountry = b.DestinationCountry
	m.boms[out.ID] = out
	root := Node{ID: m.id(), BOMID: out.ID, Name: b.Name, Quantity: decimal.NewFromInt(1)}
	m.nodes[root.ID] = root
	return out, nil
}

func (m *memStore) ListBOMs(context.Context) ([]BOM, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]BOM, 0, len(m.boms))
	for _, b := range m.boms {
		out = append(out, b)
	}
	slices.SortFunc(out, func(a, b BOM) int { return cmp.Compare(a.ID, b.ID) })
	return out, nil
}

func (m *memStore) GetBOM(_ context.Context, id int64) (BOM, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.getBOM(id)
}

func (m *memStore) getBOM(id int64) (BOM, error) {
	b, ok := m.boms[id]
	if !ok {
		return BOM{}, fmt.Errorf("bom %d: %w", id, ErrBOMNotFound)
	}
	return b, nil
}

func (m *memStore) updateBOM(id int64, fn func(*BOM)) (BOM, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	b, err := m.getBOM(id)
	if err != nil {
		return BOM{}, err
	}
	fn(&b)
	m.boms[id] = b
	return b, nil
}

func (m *memStore) RenameBOM(_ context.Context, id int64, name string) (BOM, error) {
	return m.updateBOM(id, func(b *BOM) { b.Name = name })
}

func (m *memStore) UpdateSettings(_ context.Context, id int64, currency string, volume int32, check SettingsCheck) (BOM, error) {
	if check != nil {
		m.mu.Lock()
		b, err := m.getBOM(id)
		var global *TargetBudget
		if err == nil {
			root := RootID(m.treeLocked(id))
			for _, x := range m.budgets[id] {
				if x.NodeID == root {
					x := x
					global = &x
				}
			}
		}
		m.mu.Unlock()
		if err != nil {
			return BOM{}, err
		}
		if err := check(b.Currency, global); err != nil {
			return BOM{}, err
		}
	}
	return m.updateBOM(id, func(b *BOM) { b.Currency, b.Volume = currency, volume })
}

func (m *memStore) Stock(_ context.Context, bomID int64) ([]PartStock, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []PartStock
	for k, q := range m.stock {
		if k[0] == bomID {
			out = append(out, PartStock{PartID: k[1], Quantity: q})
		}
	}
	slices.SortFunc(out, func(a, b PartStock) int { return cmp.Compare(a.PartID, b.PartID) })
	return out, nil
}

func (m *memStore) SetStock(_ context.Context, bomID int64, st PartStock) (PartStock, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, err := m.getBOM(bomID); err != nil {
		return PartStock{}, err
	}
	m.stock[[2]int64{bomID, st.PartID}] = st.Quantity
	return st, nil
}

func (m *memStore) DeleteStock(_ context.Context, bomID, partID int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.stock, [2]int64{bomID, partID})
	return nil
}

func (m *memStore) Schedule(_ context.Context, bomID int64) (*Schedule, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s, ok := m.schedules[bomID]; ok {
		return &s, nil
	}
	return nil, nil
}

func (m *memStore) SetSchedule(_ context.Context, bomID int64, s Schedule) (Schedule, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, err := m.getBOM(bomID); err != nil {
		return Schedule{}, err
	}
	m.schedules[bomID] = s
	return s, nil
}

func (m *memStore) DeleteSchedule(_ context.Context, bomID int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.schedules, bomID)
	return nil
}

func (m *memStore) SetDestination(_ context.Context, id int64, c *string) (BOM, error) {
	return m.updateBOM(id, func(b *BOM) { b.DestinationCountry = c })
}

func (m *memStore) SetAnnualHours(_ context.Context, id int64, hours decimal.Decimal) (BOM, error) {
	return m.updateBOM(id, func(b *BOM) { b.AnnualHours = hours })
}

func (m *memStore) SetCostMethod(_ context.Context, id int64, cm CostMethod) (BOM, error) {
	return m.updateBOM(id, func(b *BOM) { b.CostMethod = cm })
}

func (m *memStore) TargetBudgets(_ context.Context, bomID int64) ([]TargetBudget, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, err := m.getBOM(bomID); err != nil {
		return nil, err
	}
	return m.budgets[bomID], nil
}

func (m *memStore) SetTargetBudget(_ context.Context, bomID int64, b TargetBudget) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, err := m.getBOM(bomID); err != nil {
		return err
	}
	if m.budgets == nil {
		m.budgets = map[int64][]TargetBudget{}
	}
	m.budgets[bomID] = append(m.budgets[bomID], b)
	return nil
}

func (m *memStore) EditTargetBudget(_ context.Context, bomID, nodeID int64, b *TargetBudget, check BudgetCheck) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, err := m.getNode(bomID, nodeID); err != nil {
		return err
	}
	next := slices.DeleteFunc(slices.Clone(m.budgets[bomID]), func(x TargetBudget) bool { return x.NodeID == nodeID })
	if b != nil {
		next = append(next, *b)
	}
	if check != nil {
		cur, _ := m.getBOM(bomID)
		if err := check(BudgetState{Currency: cur.Currency, Nodes: m.treeLocked(bomID), Budgets: next}); err != nil {
			return err
		}
	}
	if m.budgets == nil {
		m.budgets = map[int64][]TargetBudget{}
	}
	m.budgets[bomID] = next
	return nil
}

func (m *memStore) SetPriceSnapshot(_ context.Context, id int64, snapshotID *int64) (BOM, error) {
	return m.updateBOM(id, func(b *BOM) { b.PriceSnapshotID = snapshotID })
}

func (m *memStore) DeleteBOM(_ context.Context, id int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, err := m.getBOM(id); err != nil {
		return err
	}
	if m.published[id] {
		return ErrHasSnapshots
	}
	delete(m.boms, id)
	for nid, n := range m.nodes {
		if n.BOMID == id {
			delete(m.nodes, nid)
		}
	}
	return nil
}

// Draft : en mémoire, chaque lecture est déjà cohérente (un seul verrou).
func (m *memStore) Draft(ctx context.Context, bomID int64) (Draft, error) {
	b, err := m.GetBOM(ctx, bomID)
	if err != nil {
		return Draft{}, err
	}
	d := Draft{BOM: b}
	if d.Nodes, err = m.Tree(ctx, bomID); err != nil {
		return Draft{}, err
	}
	if d.Stock, err = m.Stock(ctx, bomID); err != nil {
		return Draft{}, err
	}
	if d.Schedule, err = m.Schedule(ctx, bomID); err != nil {
		return Draft{}, err
	}
	if d.Budgets, err = m.TargetBudgets(ctx, bomID); err != nil {
		return Draft{}, err
	}
	return d, nil
}

func (m *memStore) Tree(_ context.Context, bomID int64) ([]Node, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, err := m.getBOM(bomID); err != nil {
		return nil, err
	}
	return m.treeLocked(bomID), nil
}

// treeLocked : nœuds de la BOM triés par id ; m.mu tenu par l'appelant.
func (m *memStore) treeLocked(bomID int64) []Node {
	var out []Node
	for _, n := range m.nodes {
		if n.BOMID == bomID {
			out = append(out, n)
		}
	}
	slices.SortFunc(out, func(a, b Node) int { return cmp.Compare(a.ID, b.ID) })
	return out
}

func (m *memStore) getNode(bomID, nodeID int64) (Node, error) {
	n, ok := m.nodes[nodeID]
	if !ok || n.BOMID != bomID {
		return Node{}, fmt.Errorf("node %d: %w", nodeID, ErrNodeNotFound)
	}
	return n, nil
}

func (m *memStore) AddNode(_ context.Context, bomID int64, nn NewNode) (Node, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, err := m.getNode(bomID, nn.ParentID); err != nil {
		return Node{}, err
	}
	parent := nn.ParentID
	n := Node{ID: m.id(), BOMID: bomID, ParentID: &parent, Name: nn.Name, PartID: nn.PartID, OfferID: nn.OfferID,
		Quantity: nn.Quantity, ProductionDays: nn.ProductionDays}
	m.nodes[n.ID] = n
	return n, nil
}

func (m *memStore) updateNode(bomID, nodeID int64, fn func(*Node) error) (Node, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	n, err := m.getNode(bomID, nodeID)
	if err != nil {
		return Node{}, err
	}
	if err := fn(&n); err != nil {
		return Node{}, err
	}
	m.nodes[nodeID] = n
	return n, nil
}

func (m *memStore) SetNodeOffer(_ context.Context, bomID, nodeID, offerID int64) (Node, error) {
	return m.updateNode(bomID, nodeID, func(n *Node) error {
		if !n.IsPart() {
			return ErrNotAPart
		}
		n.OfferID = &offerID
		return nil
	})
}

func (m *memStore) SetNodeQuantity(_ context.Context, bomID, nodeID int64, qty decimal.Decimal) (Node, error) {
	return m.updateNode(bomID, nodeID, func(n *Node) error { n.Quantity = qty; return nil })
}

func (m *memStore) SetNodeProductionDays(_ context.Context, bomID, nodeID int64, days int32) (Node, error) {
	return m.updateNode(bomID, nodeID, func(n *Node) error { n.ProductionDays = days; return nil })
}

func (m *memStore) SetNodeStrategy(_ context.Context, bomID, nodeID int64, s *Strategy) (Node, error) {
	return m.updateNode(bomID, nodeID, func(n *Node) error { n.Strategy = s; return nil })
}

func (m *memStore) SetNodePart(_ context.Context, bomID, nodeID, partID, offerID int64) (Node, error) {
	return m.updateNode(bomID, nodeID, func(n *Node) error {
		if !n.IsPart() {
			return ErrNotAPart
		}
		n.PartID, n.OfferID = &partID, &offerID
		return nil
	})
}

func (m *memStore) MoveNode(_ context.Context, bomID, nodeID, newParentID int64, check BudgetCheck) (Node, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	n, err := m.getNode(bomID, nodeID)
	if err != nil {
		return Node{}, err
	}
	if _, err := m.getNode(bomID, newParentID); err != nil {
		return Node{}, err
	}
	for cur := &newParentID; cur != nil; cur = m.nodes[*cur].ParentID { // remonte du nouveau parent à la racine
		if *cur == nodeID {
			return Node{}, ErrCycle
		}
	}
	old := n.ParentID
	n.ParentID = &newParentID
	m.nodes[nodeID] = n
	if check != nil {
		cur, _ := m.getBOM(bomID)
		if err := check(BudgetState{Currency: cur.Currency, Nodes: m.treeLocked(bomID), Budgets: m.budgets[bomID]}); err != nil {
			n.ParentID = old
			m.nodes[nodeID] = n
			return Node{}, err
		}
	}
	return n, nil
}

func (m *memStore) DeleteNode(_ context.Context, bomID, nodeID int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	n, err := m.getNode(bomID, nodeID)
	if err != nil {
		return err
	}
	if n.ParentID == nil {
		return ErrRootNode
	}
	doomed := map[int64]bool{nodeID: true}
	for changed := true; changed; { // ferme le sous-arbre (taille de test : quadratique acceptable)
		changed = false
		for id, c := range m.nodes {
			if c.ParentID != nil && doomed[*c.ParentID] && !doomed[id] {
				doomed[id], changed = true, true
			}
		}
	}
	for id := range doomed {
		delete(m.nodes, id)
	}
	return nil
}
