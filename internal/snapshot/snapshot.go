package snapshot

import (
	"container/heap"
	"context"
	"errors"
	"fmt"
	"math"
	"slices"
	"time"

	"github.com/shopspring/decimal"

	"bomstudio/internal/bom"
	"bomstudio/internal/catalog"
	"bomstudio/internal/money"
	"bomstudio/internal/pricing"
)

// ErrSnapshotNotFound : aucun snapshot de BOM avec cet identifiant.
var ErrSnapshotNotFound = errors.New("bom snapshot not found")

// Snapshot est l'en-tête d'une version publiée, immuable, d'une BOM (ADR 0006).
type Snapshot struct {
	ID    int64
	BOMID int64 // BOM d'où il a été publié
	// PriceSnapshotID : prix et taux figés utilisés par ce snapshot (toujours présent).
	PriceSnapshotID int64
	Name            string
	Currency        string // devise de référence, ISO 4217
	Volume          int32
	// AnnualHours : base horaire annuelle d'un ETP figée (ADR 0024) ; nil pour un snapshot publié avant.
	AnnualHours *decimal.Decimal
	// Schedule : date de départ et date cible figées (ADR 0023) ; nil si le brouillon n'en avait pas.
	Schedule *bom.Schedule
	// DestinationCountry : pays de destination figé (ADR 0025) ; nil si non renseigné.
	DestinationCountry *string
	PublishedAt        time.Time
}

// Node est un nœud figé. Key / ParentKey reprennent les ids des nœuds du brouillon à la publication.
type Node struct {
	Key       int64
	ParentKey *int64 // nil : racine
	Name      string // sous-ensemble seulement
	Quantity  decimal.Decimal
	// ProductionDays : pièce → production de la pièce copiée du catalogue ; sous-ensemble → la sienne (ADR 0007).
	ProductionDays int32
	// Part : attributs figés d'une pièce ; nil pour un sous-ensemble.
	Part *Part
	// Strategy : stratégie attachée au nœud à la publication (ADR 0028) ; nil = héritée ou aucune.
	Strategy *bom.Strategy
}

// Part porte ce qu'une pièce publiée a copié du catalogue : ils ne suivent plus le catalogue ensuite.
type Part struct {
	PartID       int64
	OfferID      int64
	CO2Kg        decimal.Decimal
	MassKg       decimal.Decimal
	LeadTimeDays int32 // délai de livraison de l'offre
	// LaborHours : heures de main-d'œuvre par unité copiées du catalogue ; nil si absentes (ADR 0024).
	LaborHours *decimal.Decimal
}

// Store est l'accès aux snapshots de BOM. Il n'a aucune méthode de modification ni de suppression
// (la base les refuse aussi, ADR 0006). L'implémentation Postgres vit dans snapshot/snapshotstore.
type Store interface {
	// Create insère l'en-tête, tous les nœuds et le stock figé dans une même transaction ; s.ID est ignoré.
	Create(ctx context.Context, s Snapshot, nodes []Node, stock []bom.PartStock) (Snapshot, error)
	// Get renvoie l'en-tête, ou ErrSnapshotNotFound.
	Get(ctx context.Context, id int64) (Snapshot, error)
	// Nodes renvoie tous les nœuds du snapshot en une requête, triés par Key.
	Nodes(ctx context.Context, id int64) ([]Node, error)
	// Stock renvoie le stock figé, trié par pièce ; vide pour un snapshot publié avant ADR 0022.
	Stock(ctx context.Context, id int64) ([]bom.PartStock, error)
}

// Service publie et lit les snapshots de BOM. Il lit et crée les brouillons via bom.Service
// (snapshot dépend de bom, jamais l'inverse) et n'écrit que ses propres tables.
type Service struct {
	store   Store
	boms    *bom.Service
	catalog catalog.Catalog
	pricing pricing.Pricing
}

// NewService câble le service. pricing doit être la même source que celle de boms.
func NewService(s Store, boms *bom.Service, c catalog.Catalog, p pricing.Pricing) *Service {
	return &Service{store: s, boms: boms, catalog: c, pricing: p}
}

// Published est un snapshot de BOM, son arbre figé et ses totaux.
type Published struct {
	Snapshot Snapshot
	Nodes    []Node // triés par Key
	Stock    []bom.PartStock
	Totals   bom.Totals
	Planning bom.Planning
}

// Publish fige le brouillon bomID (R10) : structure, quantités, offres, attributs des pièces (CO2, masse,
// production) et délai de livraison des offres sont copiés ; les prix et taux sont ceux du snapshot de prix
// du brouillon s'il est figé, sinon un snapshot de prix est créé à l'instant at (fourni par l'appelant,
// pas d'horloge dans le domaine). S'il manque un prix ou un taux, rien n'est publié : l'erreur liste tout
// ce qui manque (pricing.ErrNoQuote / ErrNoRate), pour ne pas graver un trou dans un snapshot immuable.
func (s *Service) Publish(ctx context.Context, bomID int64, at time.Time) (Snapshot, error) {
	b, draft, err := s.boms.Tree(ctx, bomID)
	if err != nil {
		return Snapshot{}, fmt.Errorf("publish bom %d: %w", bomID, err)
	}
	sch, err := s.boms.Schedule(ctx, bomID)
	if err != nil {
		return Snapshot{}, fmt.Errorf("publish bom %d: %w", bomID, err)
	}
	nodes, err := s.freezeNodes(ctx, draft)
	if err != nil {
		return Snapshot{}, fmt.Errorf("publish bom %d: %w", bomID, err)
	}
	stock, err := s.boms.Stock(ctx, bomID)
	if err != nil {
		return Snapshot{}, fmt.Errorf("publish bom %d: %w", bomID, err)
	}
	currency := money.Currency(b.Currency)
	offers := offerIDs(nodes)

	var priceSnapID int64
	if b.PriceSnapshotID != nil {
		priceSnapID = *b.PriceSnapshotID
		// Le brouillon a pu changer d'offre ou de devise après le gel : le snapshot de prix réutilisé
		// doit couvrir toutes les offres et tous les taux, comme FreezePrices l'exige.
		if err := s.checkCovered(ctx, priceSnapID, offers, currency); err != nil {
			return Snapshot{}, fmt.Errorf("publish bom %d with price snapshot %d: %w", bomID, priceSnapID, err)
		}
	} else if priceSnapID, err = s.pricing.FreezePrices(ctx, bomID, offers, []money.Currency{currency}, at); err != nil {
		return Snapshot{}, fmt.Errorf("publish bom %d: %w", bomID, err)
	}

	out, err := s.store.Create(ctx, Snapshot{
		BOMID: b.ID, PriceSnapshotID: priceSnapID, Name: b.Name, Currency: b.Currency, Volume: b.Volume,
		AnnualHours: &b.AnnualHours, Schedule: sch, DestinationCountry: b.DestinationCountry, PublishedAt: at,
	}, nodes, stock)
	if err != nil {
		return Snapshot{}, fmt.Errorf("publish bom %d: %w", bomID, err)
	}
	return out, nil
}

// freezeNodes copie l'arbre du brouillon avec les attributs courants du catalogue ; chaque pièce et
// chaque offre distincte est lue une fois.
func (s *Service) freezeNodes(ctx context.Context, draft []bom.Node) ([]Node, error) {
	parts := map[int64]catalog.Part{}
	offers := map[int64]catalog.Offer{}
	out := make([]Node, len(draft))
	for i, n := range draft {
		out[i] = Node{
			Key: n.ID, ParentKey: n.ParentID, Name: n.Name, Quantity: n.Quantity, ProductionDays: n.ProductionDays,
			Strategy: n.Strategy,
		}
		if !n.IsPart() {
			continue
		}
		p, ok := parts[*n.PartID]
		if !ok {
			var err error
			if p, err = s.catalog.Part(ctx, *n.PartID); err != nil {
				return nil, fmt.Errorf("node %d: %w", n.ID, err)
			}
			parts[p.ID] = p
		}
		o, ok := offers[*n.OfferID]
		if !ok {
			var err error
			if o, err = s.catalog.Offer(ctx, *n.OfferID); err != nil {
				return nil, fmt.Errorf("node %d: %w", n.ID, err)
			}
			offers[o.ID] = o
		}
		attrs, err := p.BOMAttributes() // attribut NULL : la publication est refusée, rien n'est figé à zéro
		if err != nil {
			return nil, fmt.Errorf("node %d: %w", n.ID, err)
		}
		prod, err := days(attrs.ProductionDays)
		if err != nil {
			return nil, fmt.Errorf("node %d: production days of part %d: %w", n.ID, p.ID, err)
		}
		lead, err := days(o.LeadTimeDays)
		if err != nil {
			return nil, fmt.Errorf("node %d: lead time of offer %d: %w", n.ID, o.ID, err)
		}
		out[i].Name = ""
		out[i].ProductionDays = prod
		out[i].Part = &Part{
			PartID: p.ID, OfferID: o.ID, CO2Kg: attrs.CO2KGPerUnit, MassKg: attrs.MassKG, LeadTimeDays: lead,
			LaborHours: p.LaborHours,
		}
	}
	return out, nil
}

// days convertit un nombre de jours du catalogue en INTEGER de la base ; négatif ou hors bornes : erreur.
func days(v int) (int32, error) {
	if v < 0 || v > math.MaxInt32 {
		return 0, fmt.Errorf("%d days out of range", v)
	}
	return int32(v), nil
}

// checkCovered vérifie que le snapshot de prix donne un prix à chaque offre et un taux à chaque devise
// d'offre vers currency ; sinon l'erreur liste tout ce qui manque.
func (s *Service) checkCovered(ctx context.Context, priceSnapID int64, offers []int64, currency money.Currency) error {
	prices, rates, err := bom.LoadPrices(ctx, s.pricing, &priceSnapID, offers, currency)
	if err != nil {
		return err
	}
	var errs []error
	for _, id := range offers {
		if e := prices[id].Err; e != nil {
			errs = append(errs, e)
		}
	}
	froms := make([]money.Currency, 0, len(rates))
	for c := range rates {
		froms = append(froms, c)
	}
	slices.Sort(froms) // message déterministe
	for _, c := range froms {
		if e := rates[c].Err; e != nil {
			errs = append(errs, e)
		}
	}
	return errors.Join(errs...)
}

// Get lit un snapshot publié et calcule ses totaux avec ses propres attributs figés et les prix de son
// snapshot de prix : le catalogue et les cotations arrivées depuis n'y changent rien (R10).
func (s *Service) Get(ctx context.Context, id int64) (Published, error) {
	h, err := s.store.Get(ctx, id)
	if err != nil {
		return Published{}, fmt.Errorf("get snapshot %d: %w", id, err)
	}
	nodes, err := s.store.Nodes(ctx, id)
	if err != nil {
		return Published{}, fmt.Errorf("get snapshot %d: %w", id, err)
	}
	stock, err := s.store.Stock(ctx, id)
	if err != nil {
		return Published{}, fmt.Errorf("get snapshot %d: %w", id, err)
	}
	currency := money.Currency(h.Currency)
	prices, rates, err := bom.LoadPrices(ctx, s.pricing, &h.PriceSnapshotID, offerIDs(nodes), currency)
	if err != nil {
		return Published{}, fmt.Errorf("get snapshot %d: %w", id, err)
	}
	calc := make([]bom.CalcNode, len(nodes))
	for i, n := range nodes {
		calc[i] = bom.CalcNode{ID: n.Key, ParentID: n.ParentKey, Quantity: n.Quantity, ProductionDays: int(n.ProductionDays)}
		if p := n.Part; p != nil {
			calc[i].Part = &bom.PartFacts{
				PartID: p.PartID, OfferID: p.OfferID, LeadTimeDays: int(p.LeadTimeDays), CO2Kg: p.CO2Kg, MassKg: p.MassKg, LaborHours: p.LaborHours,
			}
		}
	}
	t, err := bom.ComputeTotals(bom.TotalsInput{
		Nodes: calc, Currency: currency, Prices: prices, Rates: rates, PriceSnapshotID: &h.PriceSnapshotID,
		Volume: h.Volume, AnnualHours: h.AnnualHours, Stock: bom.StockMap(stock),
		LaborNotFrozen: h.AnnualHours == nil, // publié avant ADR 0024 : heures non figées
	})
	if err != nil {
		return Published{}, fmt.Errorf("get snapshot %d: %w", id, err)
	}
	plan, err := bom.PlanSchedule(calc, t, h.Schedule)
	if err != nil {
		return Published{}, fmt.Errorf("get snapshot %d: %w", id, err)
	}
	return Published{Snapshot: h, Nodes: nodes, Stock: stock, Totals: t, Planning: plan}, nil
}

// NewDraft crée un nouveau brouillon depuis le snapshot (D3, R11) : une nouvelle BOM liée au snapshot par
// SourceSnapshotID, avec les mêmes pièces, offres, quantités, jours de production des sous-ensembles,
// devise et volume. Le brouillon part en prix en direct. Il est créé via bom.Service (mêmes validations
// qu'une saisie) ; si une étape échoue, le brouillon partiel est supprimé.
func (s *Service) NewDraft(ctx context.Context, id int64) (bom.BOM, error) {
	h, err := s.store.Get(ctx, id)
	if err != nil {
		return bom.BOM{}, fmt.Errorf("new draft from snapshot %d: %w", id, err)
	}
	nodes, err := s.store.Nodes(ctx, id)
	if err != nil {
		return bom.BOM{}, fmt.Errorf("new draft from snapshot %d: %w", id, err)
	}
	stock, err := s.store.Stock(ctx, id)
	if err != nil {
		return bom.BOM{}, fmt.Errorf("new draft from snapshot %d: %w", id, err)
	}
	b, err := s.boms.CreateBOM(ctx, bom.NewBOM{
		Name: h.Name, Currency: h.Currency, Volume: h.Volume, SourceSnapshotID: &h.ID, AnnualHours: h.AnnualHours,
		DestinationCountry: h.DestinationCountry,
	})
	if err != nil {
		return bom.BOM{}, fmt.Errorf("new draft from snapshot %d: %w", id, err)
	}
	if err := s.copyDraft(ctx, b.ID, nodes, stock, h.Schedule); err != nil {
		if delErr := s.boms.DeleteBOM(ctx, b.ID); delErr != nil {
			err = errors.Join(err, fmt.Errorf("remove partial draft %d: %w", b.ID, delErr))
		}
		return bom.BOM{}, fmt.Errorf("new draft from snapshot %d: %w", id, err)
	}
	return b, nil
}

// copyDraft recopie l'arbre, le stock (ADR 0022) et le planning (ADR 0023) figés dans le brouillon bomID.
func (s *Service) copyDraft(ctx context.Context, bomID int64, nodes []Node, stock []bom.PartStock, sch *bom.Schedule) error {
	if err := s.copyTree(ctx, bomID, nodes); err != nil {
		return err
	}
	for _, st := range stock {
		if _, err := s.boms.SetPartStock(ctx, bomID, st.PartID, st.Quantity); err != nil {
			return fmt.Errorf("copy stock of part %d: %w", st.PartID, err)
		}
	}
	if sch != nil {
		if _, err := s.boms.SetSchedule(ctx, bomID, *sch); err != nil {
			return fmt.Errorf("copy schedule: %w", err)
		}
	}
	return nil
}

// copyTree recopie l'arbre figé sous la racine du brouillon bomID, parents avant enfants, par Key croissante
// parmi les nœuds prêts (ordre déterministe des nouveaux ids).
func (s *Service) copyTree(ctx context.Context, bomID int64, nodes []Node) error {
	_, draft, err := s.boms.Tree(ctx, bomID)
	if err != nil {
		return err
	}
	if len(draft) != 1 {
		return fmt.Errorf("new bom %d has %d nodes, want only its root", bomID, len(draft))
	}
	draftRoot := draft[0]

	children := map[int64][]Node{}
	var root *Node
	for i := range nodes {
		n := nodes[i]
		if n.ParentKey == nil {
			root = &nodes[i]
			continue
		}
		children[*n.ParentKey] = append(children[*n.ParentKey], n)
	}
	if root == nil {
		return errors.New("snapshot has no root")
	}
	if !root.Quantity.Equal(draftRoot.Quantity) {
		if _, err := s.boms.SetNodeQuantity(ctx, bomID, draftRoot.ID, root.Quantity); err != nil {
			return fmt.Errorf("root: %w", err)
		}
	}
	if root.Part == nil && root.ProductionDays != draftRoot.ProductionDays {
		if _, err := s.boms.SetNodeProductionDays(ctx, bomID, draftRoot.ID, root.ProductionDays); err != nil {
			return fmt.Errorf("root: %w", err)
		}
	}
	if root.Strategy != nil {
		if _, err := s.boms.SetNodeStrategy(ctx, bomID, draftRoot.ID, root.Strategy); err != nil {
			return fmt.Errorf("root: %w", err)
		}
	}

	// Les nœuds sont créés par clé croissante parmi ceux dont le parent existe déjà (tas) : les nouveaux ids
	// gardent l'ordre des clés figées dès que chaque parent a une clé plus petite que ses enfants, donc la même
	// répartition du stock, qui suit l'id de nœud (ADR 0022).
	ready := &readyNodes{}
	push := func(parentKey, newParent int64) {
		for _, c := range children[parentKey] {
			heap.Push(ready, readyNode{node: c, newParent: newParent})
		}
	}
	push(root.Key, draftRoot.ID)
	for ready.Len() > 0 {
		it := heap.Pop(ready).(readyNode)
		c := it.node
		nn := bom.NewNode{ParentID: it.newParent, Quantity: c.Quantity}
		if c.Part != nil {
			nn.PartID, nn.OfferID = &c.Part.PartID, &c.Part.OfferID
		} else {
			nn.Name, nn.ProductionDays = c.Name, c.ProductionDays
		}
		added, err := s.boms.AddNode(ctx, bomID, nn)
		if err != nil {
			return fmt.Errorf("copy node %d: %w", c.Key, err)
		}
		if c.Strategy != nil {
			if _, err := s.boms.SetNodeStrategy(ctx, bomID, added.ID, c.Strategy); err != nil {
				return fmt.Errorf("copy strategy of node %d: %w", c.Key, err)
			}
		}
		push(c.Key, added.ID)
	}
	return nil
}

// readyNode : nœud figé dont le parent est déjà recréé sous newParent.
type readyNode struct {
	node      Node
	newParent int64
}

// readyNodes : tas min par clé figée (container/heap).
type readyNodes []readyNode

func (h readyNodes) Len() int           { return len(h) }
func (h readyNodes) Less(i, j int) bool { return h[i].node.Key < h[j].node.Key }
func (h readyNodes) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *readyNodes) Push(x any)        { *h = append(*h, x.(readyNode)) }
func (h *readyNodes) Pop() any {
	old := *h
	x := old[len(old)-1]
	*h = old[:len(old)-1]
	return x
}

// offerIDs renvoie les offres distinctes de l'arbre figé, triées (entrée déterministe).
func offerIDs(nodes []Node) []int64 {
	var ids []int64
	for _, n := range nodes {
		if n.Part != nil {
			ids = append(ids, n.Part.OfferID)
		}
	}
	slices.Sort(ids)
	return slices.Compact(ids)
}
