package bom

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"bomstudio/internal/catalog"
	"bomstudio/internal/money"
	"bomstudio/internal/pricing"
)

var (
	// ErrInvalidInput : une entrée ne respecte pas les règles de la BOM (nom vide, quantité <= 0,
	// devise inconnue…). Le message dit laquelle.
	ErrInvalidInput = errors.New("invalid bom input")
	// ErrOfferNotForPart : l'offre choisie vend une autre pièce que celle du nœud.
	ErrOfferNotForPart = errors.New("offer does not sell this part")
)

// Service porte les règles métier de la BOM. Il ne connaît la base, le catalogue et les prix qu'à travers
// leurs interfaces : ni pgx, ni réseau, ni fournisseur de prix (AGENTS §2).
type Service struct {
	store   Store
	catalog catalog.Catalog
	pricing pricing.Pricing
}

// NewService câble le service sur un store de brouillons, un catalogue et une source de prix.
func NewService(s Store, c catalog.Catalog, p pricing.Pricing) *Service {
	return &Service{store: s, catalog: c, pricing: p}
}

// PricedOffer est une offre accompagnée de la cotation qui donne son prix.
type PricedOffer struct {
	Offer catalog.Offer
	Quote pricing.Quote
}

// PricedOffer renvoie l'offre et sa dernière cotation. Une offre sans cotation est une erreur
// (pricing.ErrNoQuote), jamais un prix à zéro.
func (s *Service) PricedOffer(ctx context.Context, offerID int64) (PricedOffer, error) {
	o, err := s.catalog.Offer(ctx, offerID)
	if err != nil {
		return PricedOffer{}, fmt.Errorf("priced offer %d: %w", offerID, err)
	}
	q, err := s.pricing.OfferPrice(ctx, offerID)
	if err != nil {
		return PricedOffer{}, fmt.Errorf("priced offer %d: %w", offerID, err)
	}
	return PricedOffer{Offer: o, Quote: q}, nil
}

// --- BOM ---------------------------------------------------------------------------------------------

// CreateBOM crée une BOM et sa racine, après validation du nom, de la devise et du volume.
func (s *Service) CreateBOM(ctx context.Context, b NewBOM) (BOM, error) {
	b.Name = strings.TrimSpace(b.Name)
	if b.Name == "" {
		return BOM{}, fmt.Errorf("create bom: empty name: %w", ErrInvalidInput)
	}
	if err := validateSettings(b.Currency, b.Volume); err != nil {
		return BOM{}, fmt.Errorf("create bom: %w", err)
	}
	out, err := s.store.CreateBOM(ctx, b)
	if err != nil {
		return BOM{}, fmt.Errorf("create bom: %w", err)
	}
	return out, nil
}

// ListBOMs renvoie toutes les BOM, triées par id.
func (s *Service) ListBOMs(ctx context.Context) ([]BOM, error) {
	out, err := s.store.ListBOMs(ctx)
	if err != nil {
		return nil, fmt.Errorf("list boms: %w", err)
	}
	return out, nil
}

// GetBOM renvoie la BOM, ou ErrBOMNotFound.
func (s *Service) GetBOM(ctx context.Context, id int64) (BOM, error) {
	out, err := s.store.GetBOM(ctx, id)
	if err != nil {
		return BOM{}, fmt.Errorf("get bom %d: %w", id, err)
	}
	return out, nil
}

// Tree renvoie la BOM et tous ses nœuds (une requête pour l'arbre), triés par id.
func (s *Service) Tree(ctx context.Context, id int64) (BOM, []Node, error) {
	b, err := s.store.GetBOM(ctx, id)
	if err != nil {
		return BOM{}, nil, fmt.Errorf("tree of bom %d: %w", id, err)
	}
	nodes, err := s.store.Tree(ctx, id)
	if err != nil {
		return BOM{}, nil, fmt.Errorf("tree of bom %d: %w", id, err)
	}
	return b, nodes, nil
}

// RenameBOM renomme la BOM ; un nom vide est refusé.
func (s *Service) RenameBOM(ctx context.Context, id int64, name string) (BOM, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return BOM{}, fmt.Errorf("rename bom %d: empty name: %w", id, ErrInvalidInput)
	}
	out, err := s.store.RenameBOM(ctx, id, name)
	if err != nil {
		return BOM{}, fmt.Errorf("rename bom %d: %w", id, err)
	}
	return out, nil
}

// UpdateSettings change la devise de référence et le volume du brouillon.
func (s *Service) UpdateSettings(ctx context.Context, id int64, currency string, volume int32) (BOM, error) {
	if err := validateSettings(currency, volume); err != nil {
		return BOM{}, fmt.Errorf("update settings of bom %d: %w", id, err)
	}
	out, err := s.store.UpdateSettings(ctx, id, currency, volume)
	if err != nil {
		return BOM{}, fmt.Errorf("update settings of bom %d: %w", id, err)
	}
	return out, nil
}

// DeleteBOM supprime la BOM et son arbre ; ErrHasSnapshots si un snapshot a été publié (R2).
func (s *Service) DeleteBOM(ctx context.Context, id int64) error {
	if err := s.store.DeleteBOM(ctx, id); err != nil {
		return fmt.Errorf("delete bom %d: %w", id, err)
	}
	return nil
}

func validateSettings(currency string, volume int32) error {
	if _, err := money.ParseCurrency(currency); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidInput, err)
	}
	if volume <= 0 {
		return fmt.Errorf("volume %d must be > 0: %w", volume, ErrInvalidInput)
	}
	return nil
}

// --- Arbre -------------------------------------------------------------------------------------------

// AddNode ajoute un sous-ensemble (Name) ou une pièce (PartID + OfferID, l'offre choisie par l'utilisateur)
// sous n.ParentID. Une pièce peut se placer sous n'importe quel nœud, pièce comprise (ADR 0008).
// Les jours de production d'une pièce viennent du catalogue : NewNode.ProductionDays est réservé aux
// sous-ensembles.
func (s *Service) AddNode(ctx context.Context, bomID int64, n NewNode) (Node, error) {
	n.Name = strings.TrimSpace(n.Name)
	if err := s.validateNewNode(ctx, n); err != nil {
		return Node{}, fmt.Errorf("add node to bom %d: %w", bomID, err)
	}
	out, err := s.store.AddNode(ctx, bomID, n)
	if err != nil {
		return Node{}, fmt.Errorf("add node to bom %d: %w", bomID, err)
	}
	return out, nil
}

func (s *Service) validateNewNode(ctx context.Context, n NewNode) error {
	if err := validateQuantity(n.Quantity); err != nil {
		return err
	}
	isPart := n.PartID != nil || n.OfferID != nil
	switch {
	case isPart && (n.PartID == nil || n.OfferID == nil):
		return fmt.Errorf("a part needs both part_id and offer_id: %w", ErrInvalidInput)
	case isPart && n.Name != "":
		return fmt.Errorf("a part takes its name from the catalog, not %q: %w", n.Name, ErrInvalidInput)
	case isPart && n.ProductionDays != 0:
		return fmt.Errorf("a part's production days come from the catalog: %w", ErrInvalidInput)
	case !isPart && n.Name == "":
		return fmt.Errorf("a sub-assembly needs a name: %w", ErrInvalidInput)
	case !isPart && n.ProductionDays < 0:
		return fmt.Errorf("production days %d must be >= 0: %w", n.ProductionDays, ErrInvalidInput)
	}
	if isPart {
		return s.checkOfferSellsPart(ctx, *n.OfferID, *n.PartID)
	}
	return nil
}

func validateQuantity(q decimal.Decimal) error {
	if !q.IsPositive() {
		return fmt.Errorf("quantity %s must be > 0: %w", q, ErrInvalidInput)
	}
	return nil
}

// checkOfferSellsPart vérifie dans le catalogue que l'offre existe et vend la pièce.
func (s *Service) checkOfferSellsPart(ctx context.Context, offerID, partID int64) error {
	o, err := s.catalog.Offer(ctx, offerID)
	if err != nil {
		return fmt.Errorf("offer %d: %w", offerID, err)
	}
	if o.PartID != partID {
		return fmt.Errorf("offer %d sells part %d, not part %d: %w", offerID, o.PartID, partID, ErrOfferNotForPart)
	}
	return nil
}

// node renvoie un nœud du brouillon, ou ErrNodeNotFound.
func (s *Service) node(ctx context.Context, bomID, nodeID int64) (Node, error) {
	nodes, err := s.store.Tree(ctx, bomID)
	if err != nil {
		return Node{}, err
	}
	for _, n := range nodes {
		if n.ID == nodeID {
			return n, nil
		}
	}
	return Node{}, fmt.Errorf("node %d: %w", nodeID, ErrNodeNotFound)
}

// SetNodeOffer remplace l'offre d'une pièce par une autre offre de la même pièce (un geste, R3).
func (s *Service) SetNodeOffer(ctx context.Context, bomID, nodeID, offerID int64) (Node, error) {
	n, err := s.node(ctx, bomID, nodeID)
	if err != nil {
		return Node{}, fmt.Errorf("set offer of node %d in bom %d: %w", nodeID, bomID, err)
	}
	if !n.IsPart() {
		return Node{}, fmt.Errorf("set offer of node %d in bom %d: %w", nodeID, bomID, ErrNotAPart)
	}
	if err := s.checkOfferSellsPart(ctx, offerID, *n.PartID); err != nil {
		return Node{}, fmt.Errorf("set offer of node %d in bom %d: %w", nodeID, bomID, err)
	}
	out, err := s.store.SetNodeOffer(ctx, bomID, nodeID, offerID)
	if err != nil {
		return Node{}, fmt.Errorf("set offer of node %d in bom %d: %w", nodeID, bomID, err)
	}
	return out, nil
}

// SetNodeQuantity change la quantité du nœud par unité de son parent ; elle doit être > 0.
func (s *Service) SetNodeQuantity(ctx context.Context, bomID, nodeID int64, qty decimal.Decimal) (Node, error) {
	if err := validateQuantity(qty); err != nil {
		return Node{}, fmt.Errorf("set quantity of node %d in bom %d: %w", nodeID, bomID, err)
	}
	out, err := s.store.SetNodeQuantity(ctx, bomID, nodeID, qty)
	if err != nil {
		return Node{}, fmt.Errorf("set quantity of node %d in bom %d: %w", nodeID, bomID, err)
	}
	return out, nil
}

// SetNodeProductionDays change les jours de production d'un sous-ensemble (ceux d'une pièce viennent du
// catalogue : refus, plutôt qu'une valeur stockée et ignorée par le calcul).
func (s *Service) SetNodeProductionDays(ctx context.Context, bomID, nodeID int64, days int32) (Node, error) {
	if days < 0 {
		return Node{}, fmt.Errorf("set production days of node %d in bom %d: %d must be >= 0: %w", nodeID, bomID, days, ErrInvalidInput)
	}
	n, err := s.node(ctx, bomID, nodeID)
	if err != nil {
		return Node{}, fmt.Errorf("set production days of node %d in bom %d: %w", nodeID, bomID, err)
	}
	if n.IsPart() {
		return Node{}, fmt.Errorf("set production days of node %d in bom %d: a part's production days come from the catalog: %w",
			nodeID, bomID, ErrInvalidInput)
	}
	out, err := s.store.SetNodeProductionDays(ctx, bomID, nodeID, days)
	if err != nil {
		return Node{}, fmt.Errorf("set production days of node %d in bom %d: %w", nodeID, bomID, err)
	}
	return out, nil
}

// MoveNode rattache le nœud à un autre parent ; ErrCycle si c'est le nœud ou l'un de ses descendants
// (contrôle fait par le store, dans la transaction de la mise à jour).
func (s *Service) MoveNode(ctx context.Context, bomID, nodeID, newParentID int64) (Node, error) {
	out, err := s.store.MoveNode(ctx, bomID, nodeID, newParentID)
	if err != nil {
		return Node{}, fmt.Errorf("move node %d in bom %d: %w", nodeID, bomID, err)
	}
	return out, nil
}

// DeleteNode supprime le nœud et son sous-arbre ; ErrRootNode pour la racine.
func (s *Service) DeleteNode(ctx context.Context, bomID, nodeID int64) error {
	if err := s.store.DeleteNode(ctx, bomID, nodeID); err != nil {
		return fmt.Errorf("delete node %d in bom %d: %w", nodeID, bomID, err)
	}
	return nil
}

// --- Prix figés ou en direct -------------------------------------------------------------------------

// FreezePrices fige les prix du brouillon (R9) : un snapshot de prix est créé pour toutes ses offres et
// pour les taux vers sa devise, puis attaché à la BOM. at est l'instant du gel, fourni par l'appelant
// (pas d'horloge dans le domaine). S'il manque un prix ou un taux, rien n'est figé (pricing.FreezePrices
// refuse) et la BOM reste comme elle était. Figer une BOM déjà figée la fige à nouveau sur les cotations
// courantes.
func (s *Service) FreezePrices(ctx context.Context, bomID int64, at time.Time) (BOM, error) {
	b, err := s.store.GetBOM(ctx, bomID)
	if err != nil {
		return BOM{}, fmt.Errorf("freeze prices of bom %d: %w", bomID, err)
	}
	nodes, err := s.store.Tree(ctx, bomID)
	if err != nil {
		return BOM{}, fmt.Errorf("freeze prices of bom %d: %w", bomID, err)
	}
	snapID, err := s.pricing.FreezePrices(ctx, offerIDs(nodes), []money.Currency{money.Currency(b.Currency)}, at)
	if err != nil {
		return BOM{}, fmt.Errorf("freeze prices of bom %d: %w", bomID, err)
	}
	out, err := s.store.SetPriceSnapshot(ctx, bomID, &snapID)
	if err != nil {
		return BOM{}, fmt.Errorf("freeze prices of bom %d: %w", bomID, err)
	}
	return out, nil
}

// UnfreezePrices remet le brouillon en prix en direct. Le snapshot de prix détaché reste en base (immuable).
func (s *Service) UnfreezePrices(ctx context.Context, bomID int64) (BOM, error) {
	out, err := s.store.SetPriceSnapshot(ctx, bomID, nil)
	if err != nil {
		return BOM{}, fmt.Errorf("unfreeze prices of bom %d: %w", bomID, err)
	}
	return out, nil
}

// offerIDs renvoie les offres distinctes de l'arbre, triées (entrée déterministe).
func offerIDs(nodes []Node) []int64 {
	var ids []int64
	for _, n := range nodes {
		if n.OfferID != nil {
			ids = append(ids, *n.OfferID)
		}
	}
	slices.Sort(ids)
	return slices.Compact(ids)
}

// --- Totaux ------------------------------------------------------------------------------------------

// TreeTotals est l'arbre d'un brouillon et ses totaux par nœud.
type TreeTotals struct {
	BOM    BOM
	Nodes  []Node // triés par id
	Totals Totals
}

// Totals calcule les totaux du brouillon (ADR 0007, ADR 0008), avec les prix en direct ou ceux de son
// snapshot de prix. Chaque pièce, offre, prix et taux distinct est lu une seule fois. Un prix ou un taux
// absent (pricing.ErrNoQuote / ErrNoRate) devient une erreur nommée sur le nœud ; toute autre erreur
// (catalogue incohérent, snapshot de prix introuvable, panne) fait échouer l'appel : rien n'est caché.
func (s *Service) Totals(ctx context.Context, bomID int64) (TreeTotals, error) {
	b, err := s.store.GetBOM(ctx, bomID)
	if err != nil {
		return TreeTotals{}, fmt.Errorf("totals of bom %d: %w", bomID, err)
	}
	nodes, err := s.store.Tree(ctx, bomID)
	if err != nil {
		return TreeTotals{}, fmt.Errorf("totals of bom %d: %w", bomID, err)
	}
	in, err := s.totalsInput(ctx, b, nodes)
	if err != nil {
		return TreeTotals{}, fmt.Errorf("totals of bom %d: %w", bomID, err)
	}
	t, err := ComputeTotals(in)
	if err != nil {
		return TreeTotals{}, fmt.Errorf("totals of bom %d: %w", bomID, err)
	}
	return TreeTotals{BOM: b, Nodes: nodes, Totals: t}, nil
}

func (s *Service) totalsInput(ctx context.Context, b BOM, nodes []Node) (TotalsInput, error) {
	currency := money.Currency(b.Currency)
	parts := map[int64]catalog.Part{}
	offers := map[int64]catalog.Offer{}
	calc := make([]CalcNode, len(nodes))
	for i, n := range nodes {
		calc[i] = CalcNode{ID: n.ID, ParentID: n.ParentID, Quantity: n.Quantity, ProductionDays: int(n.ProductionDays)}
		if !n.IsPart() {
			continue
		}
		p, ok := parts[*n.PartID]
		if !ok {
			var err error
			if p, err = s.catalog.Part(ctx, *n.PartID); err != nil {
				return TotalsInput{}, fmt.Errorf("node %d: %w", n.ID, err)
			}
			parts[p.ID] = p
		}
		o, ok := offers[*n.OfferID]
		if !ok {
			var err error
			if o, err = s.catalog.Offer(ctx, *n.OfferID); err != nil {
				return TotalsInput{}, fmt.Errorf("node %d: %w", n.ID, err)
			}
			offers[o.ID] = o
		}
		calc[i].ProductionDays = p.ProductionDays // ADR 0007 règle 4 : production de la pièce (catalogue)
		calc[i].Part = &PartFacts{OfferID: o.ID, LeadTimeDays: o.LeadTimeDays, CO2Kg: p.CO2Kg, MassKg: p.MassKg}
	}

	prices, rates, err := LoadPrices(ctx, s.pricing, b.PriceSnapshotID, offerIDs(nodes), currency)
	if err != nil {
		return TotalsInput{}, err
	}
	return TotalsInput{Nodes: calc, Currency: currency, Prices: prices, Rates: rates, PriceSnapshotID: b.PriceSnapshotID}, nil
}

// LoadPrices lit, une seule fois chacun, le prix de chaque offre de offerIDs et le taux de chaque devise
// d'offre vers currency : en direct si priceSnapshotID est nil, sinon dans ce snapshot de prix. Un prix ou un
// taux absent (pricing.ErrNoQuote / ErrNoRate) est rendu dans le résultat, pour devenir une erreur nommée sur
// le nœud ; toute autre erreur (snapshot de prix introuvable, panne) est renvoyée. Partagé par les totaux d'un
// brouillon et ceux d'un snapshot de BOM (domaine snapshot) : même lecture des prix, même calcul.
func LoadPrices(ctx context.Context, p pricing.Pricing, priceSnapshotID *int64, offerIDs []int64, currency money.Currency) (
	map[int64]PriceResult, map[money.Currency]RateResult, error,
) {
	prices := make(map[int64]PriceResult, len(offerIDs))
	var fromCurrencies []money.Currency
	for _, id := range offerIDs {
		q, err := loadOfferPrice(ctx, p, priceSnapshotID, id)
		switch {
		case errors.Is(err, pricing.ErrNoQuote):
			prices[id] = PriceResult{Err: err}
		case err != nil:
			return nil, nil, err
		default:
			prices[id] = PriceResult{Quote: q}
			if c := q.Price.Currency(); c != currency {
				fromCurrencies = append(fromCurrencies, c)
			}
		}
	}
	slices.Sort(fromCurrencies)
	rates := make(map[money.Currency]RateResult)
	for _, from := range slices.Compact(fromCurrencies) {
		r, err := loadRate(ctx, p, priceSnapshotID, from, currency)
		switch {
		case errors.Is(err, pricing.ErrNoRate):
			rates[from] = RateResult{Err: err}
		case err != nil:
			return nil, nil, err
		default:
			rates[from] = RateResult{Rate: r}
		}
	}
	return prices, rates, nil
}

func loadOfferPrice(ctx context.Context, p pricing.Pricing, snapID *int64, offerID int64) (pricing.Quote, error) {
	if snapID == nil {
		return p.OfferPrice(ctx, offerID)
	}
	return p.SnapshotOfferPrice(ctx, *snapID, offerID)
}

func loadRate(ctx context.Context, p pricing.Pricing, snapID *int64, from, to money.Currency) (pricing.Rate, error) {
	if snapID == nil {
		return p.Rate(ctx, from, to)
	}
	return p.SnapshotRate(ctx, *snapID, from, to)
}
