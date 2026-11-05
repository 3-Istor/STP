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
	published map[int64]bool // BOM qui ont un snapshot publié
	nextID    int64
}

var _ Store = (*memStore)(nil)

func newMemStore() *memStore {
	return &memStore{boms: map[int64]BOM{}, nodes: map[int64]Node{}, published: map[int64]bool{}}
}

func (m *memStore) id() int64 { m.nextID++; return m.nextID }

func (m *memStore) CreateBOM(_ context.Context, b NewBOM) (BOM, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := BOM{ID: m.id(), Name: b.Name, Currency: b.Currency, Volume: b.Volume, SourceSnapshotID: b.SourceSnapshotID}
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

func (m *memStore) UpdateSettings(_ context.Context, id int64, currency string, volume int32) (BOM, error) {
	return m.updateBOM(id, func(b *BOM) { b.Currency, b.Volume = currency, volume })
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

func (m *memStore) Tree(_ context.Context, bomID int64) ([]Node, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, err := m.getBOM(bomID); err != nil {
		return nil, err
	}
	var out []Node
	for _, n := range m.nodes {
		if n.BOMID == bomID {
			out = append(out, n)
		}
	}
	slices.SortFunc(out, func(a, b Node) int { return cmp.Compare(a.ID, b.ID) })
	return out, nil
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

func (m *memStore) MoveNode(_ context.Context, bomID, nodeID, newParentID int64) (Node, error) {
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
	n.ParentID = &newParentID
	m.nodes[nodeID] = n
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
