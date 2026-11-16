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
	// listing : alternatives et offres du catalogue, pour les suggestions (ADR 0028) ; nil → indisponible.
	listing CatalogListing
	// anomalies : détecteur des anomalies de coût (ADR 0027), remplaçable par un modèle IA.
	anomalies pricing.AnomalyDetector
	// logistics : tables de douane, de transport et emballage (coût rendu, ADR 0025) ; nil → indisponible.
	logistics pricing.LogisticsStore
}

// WithLogistics branche les tables du coût total rendu (ADR 0025).
func (s *Service) WithLogistics(l pricing.LogisticsStore) *Service {
	s.logistics = l
	return s
}

// CatalogListing : ce que les suggestions lisent du catalogue. *catalog.Service la satisfait.
type CatalogListing interface {
	Alternatives(ctx context.Context, partID int64) ([]catalog.Part, error)
	ListParts(ctx context.Context, f catalog.Filters) ([]catalog.PartWithOffers, error)
}

// ErrSuggestionsUnavailable : le service a été câblé sans accès aux alternatives du catalogue.
var ErrSuggestionsUnavailable = errors.New("suggestions unavailable: no catalog listing")

// WithAnomalyDetector remplace le détecteur des anomalies de coût (par défaut : médiane, 5 pièces minimum).
func (s *Service) WithAnomalyDetector(d pricing.AnomalyDetector) *Service {
	s.anomalies = d
	return s
}

// WithCatalogListing branche la lecture des alternatives et des offres (suggestions, ADR 0028).
func (s *Service) WithCatalogListing(l CatalogListing) *Service {
	s.listing = l
	return s
}

// NewService câble le service sur un store de brouillons, un catalogue et une source de prix.
func NewService(s Store, c catalog.Catalog, p pricing.Pricing) *Service {
	return &Service{store: s, catalog: c, pricing: p, anomalies: pricing.MedianDetector{MinPeers: 5}}
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
	if b.AnnualHours == nil {
		h := DefaultAnnualHours
		b.AnnualHours = &h
	}
	if err := validateAnnualHours(*b.AnnualHours); err != nil {
		return BOM{}, fmt.Errorf("create bom: %w", err)
	}
	if b.DestinationCountry != nil {
		if err := pricing.ValidCountry(*b.DestinationCountry); err != nil {
			return BOM{}, fmt.Errorf("create bom: %w: %w", err, ErrInvalidInput)
		}
	}
	out, err := s.store.CreateBOM(ctx, b) // destination comprise, dans la même transaction
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

// SetDestinationCountry fixe le pays de destination (ISO 3166-1 alpha-2) ; nil l'efface (ADR 0025).
func (s *Service) SetDestinationCountry(ctx context.Context, id int64, country *string) (BOM, error) {
	if country != nil {
		if err := pricing.ValidCountry(*country); err != nil {
			return BOM{}, fmt.Errorf("set destination of bom %d: %w: %w", id, err, ErrInvalidInput)
		}
	}
	out, err := s.store.SetDestination(ctx, id, country)
	if err != nil {
		return BOM{}, fmt.Errorf("set destination of bom %d: %w", id, err)
	}
	return out, nil
}

// DefaultAnnualHours : base horaire annuelle d'un ETP à la création d'une BOM (ADR 0024).
var DefaultAnnualHours = decimal.NewFromInt(1607)

// SetAnnualHours change la base horaire annuelle d'un ETP ; elle doit être > 0.
func (s *Service) SetAnnualHours(ctx context.Context, id int64, hours decimal.Decimal) (BOM, error) {
	if err := validateAnnualHours(hours); err != nil {
		return BOM{}, fmt.Errorf("set annual hours of bom %d: %w", id, err)
	}
	out, err := s.store.SetAnnualHours(ctx, id, hours)
	if err != nil {
		return BOM{}, fmt.Errorf("set annual hours of bom %d: %w", id, err)
	}
	return out, nil
}

// maxAnnualHours : une année bissextile compte 8 784 heures ; au-delà, la base n'a pas de sens (et une
// valeur hors des bornes de NUMERIC ferait échouer la base au lieu d'un refus explicite).
var maxAnnualHours = decimal.NewFromInt(8784)

func validateAnnualHours(h decimal.Decimal) error {
	if !h.IsPositive() || h.GreaterThan(maxAnnualHours) {
		return fmt.Errorf("annual hours %s must be in ]0, %s]: %w", h, maxAnnualHours, ErrInvalidInput)
	}
	return nil
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

// SetNodeStrategy attache une stratégie au nœud (sous-ensemble ou pièce) ; nil la retire (ADR 0028).
func (s *Service) SetNodeStrategy(ctx context.Context, bomID, nodeID int64, st *Strategy) (Node, error) {
	if st != nil {
		if _, err := ParseStrategy(string(*st)); err != nil {
			return Node{}, fmt.Errorf("set strategy of node %d in bom %d: %w: %w", nodeID, bomID, err, ErrInvalidInput)
		}
	}
	out, err := s.store.SetNodeStrategy(ctx, bomID, nodeID, st)
	if err != nil {
		return Node{}, fmt.Errorf("set strategy of node %d in bom %d: %w", nodeID, bomID, err)
	}
	return out, nil
}

// ReplaceNodePart applique une suggestion confirmée par l'utilisateur : la pièce du nœud devient partID
// vendue par offerID (même pièce : simple changement d'offre). Id, quantité et enfants sont gardés.
func (s *Service) ReplaceNodePart(ctx context.Context, bomID, nodeID, partID, offerID int64) (Node, error) {
	n, err := s.node(ctx, bomID, nodeID)
	if err != nil {
		return Node{}, fmt.Errorf("replace part of node %d in bom %d: %w", nodeID, bomID, err)
	}
	if !n.IsPart() {
		return Node{}, fmt.Errorf("replace part of node %d in bom %d: %w", nodeID, bomID, ErrNotAPart)
	}
	if err := s.checkReplacement(ctx, *n.PartID, partID, offerID); err != nil {
		return Node{}, fmt.Errorf("replace part of node %d in bom %d: %w", nodeID, bomID, err)
	}
	out, err := s.store.SetNodePart(ctx, bomID, nodeID, partID, offerID)
	if err != nil {
		return Node{}, fmt.Errorf("replace part of node %d in bom %d: %w", nodeID, bomID, err)
	}
	return out, nil
}

// checkReplacement : l'offre vend la pièce, ni l'une ni l'autre n'est archivée (ADR 0009), et la pièce est
// celle du nœud ou l'une de ses alternatives déclarées (une suggestion, pas un remplacement arbitraire).
func (s *Service) checkReplacement(ctx context.Context, currentPart, partID, offerID int64) error {
	if err := s.checkOfferSellsPart(ctx, offerID, partID); err != nil {
		return err
	}
	o, err := s.catalog.Offer(ctx, offerID)
	if err != nil {
		return err
	}
	p, err := s.catalog.Part(ctx, partID)
	if err != nil {
		return err
	}
	if o.ArchivedAt != nil || p.ArchivedAt != nil {
		return fmt.Errorf("part %d / offer %d: %w: %w", partID, offerID, ErrArchived, ErrInvalidInput)
	}
	if partID == currentPart {
		return nil
	}
	if s.listing == nil {
		return ErrSuggestionsUnavailable
	}
	alts, err := s.listing.Alternatives(ctx, currentPart)
	if err != nil {
		return err
	}
	if !slices.ContainsFunc(alts, func(a catalog.Part) bool { return a.ID == partID }) {
		return fmt.Errorf("part %d is not a declared alternative of part %d: %w", partID, currentPart, ErrInvalidInput)
	}
	return nil
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

// --- Stock (ADR 0022) ---------------------------------------------------------------------------------

// Stock renvoie le stock de la BOM, trié par pièce ; ErrBOMNotFound si elle n'existe pas.
func (s *Service) Stock(ctx context.Context, bomID int64) ([]PartStock, error) {
	if _, err := s.store.GetBOM(ctx, bomID); err != nil {
		return nil, fmt.Errorf("stock of bom %d: %w", bomID, err)
	}
	out, err := s.store.Stock(ctx, bomID)
	if err != nil {
		return nil, fmt.Errorf("stock of bom %d: %w", bomID, err)
	}
	return out, nil
}

// SetPartStock fixe la quantité en stock d'une pièce du catalogue pour cette BOM. Zéro retire le stock ;
// une quantité négative est refusée.
func (s *Service) SetPartStock(ctx context.Context, bomID, partID int64, qty decimal.Decimal) (PartStock, error) {
	if qty.IsNegative() || qty.GreaterThanOrEqual(maxStockQuantity) || qty.Exponent() < -maxStockDecimals {
		return PartStock{}, fmt.Errorf("set stock of part %d in bom %d: quantity %s must be in [0, %s[ with at most %d decimals: %w",
			partID, bomID, qty, maxStockQuantity, maxStockDecimals, ErrInvalidInput)
	}
	if _, err := s.store.GetBOM(ctx, bomID); err != nil {
		return PartStock{}, fmt.Errorf("set stock of part %d in bom %d: %w", partID, bomID, err)
	}
	p, err := s.catalog.Part(ctx, partID)
	if err != nil {
		return PartStock{}, fmt.Errorf("set stock of part %d in bom %d: %w", partID, bomID, err)
	}
	if p.ArchivedAt != nil && qty.IsPositive() { // ADR 0009 : une pièce archivée ne reçoit pas de nouvelle association
		return PartStock{}, fmt.Errorf("set stock of part %d in bom %d: part is archived: %w", partID, bomID, ErrInvalidInput)
	}
	if qty.IsZero() {
		if err := s.store.DeleteStock(ctx, bomID, partID); err != nil {
			return PartStock{}, fmt.Errorf("set stock of part %d in bom %d: %w", partID, bomID, err)
		}
		return PartStock{PartID: partID, Quantity: decimal.Zero}, nil
	}
	out, err := s.store.SetStock(ctx, bomID, PartStock{PartID: partID, Quantity: qty})
	if err != nil {
		return PartStock{}, fmt.Errorf("set stock of part %d in bom %d: %w", partID, bomID, err)
	}
	return out, nil
}

// Bornes de saisie du stock : au-delà, la valeur n'a pas de sens physique et dépasserait NUMERIC (la base
// renverrait une erreur serveur au lieu d'un refus explicite).
var maxStockQuantity = decimal.New(1, 15)

const maxStockDecimals = 6

// StockMap indexe un stock par pièce, pour le calcul des totaux.
func StockMap(stock []PartStock) map[int64]decimal.Decimal {
	m := make(map[int64]decimal.Decimal, len(stock))
	for _, st := range stock {
		m[st.PartID] = st.Quantity
	}
	return m
}

// --- Planning (ADR 0023) -----------------------------------------------------------------------------

// SetSchedule fixe la date de départ et, optionnellement, la date cible. Les dates sont des jours
// calendaires (minuit UTC) ; une cible avant le départ est refusée.
func (s *Service) SetSchedule(ctx context.Context, bomID int64, sch Schedule) (Schedule, error) {
	if !isDate(sch.StartOn) || (sch.TargetOn != nil && !isDate(*sch.TargetOn)) {
		return Schedule{}, fmt.Errorf("set schedule of bom %d: dates must be calendar days (midnight UTC): %w", bomID, ErrInvalidInput)
	}
	if sch.TargetOn != nil && sch.TargetOn.Before(sch.StartOn) {
		return Schedule{}, fmt.Errorf("set schedule of bom %d: target %s before start %s: %w",
			bomID, sch.TargetOn.Format(time.DateOnly), sch.StartOn.Format(time.DateOnly), ErrInvalidInput)
	}
	out, err := s.store.SetSchedule(ctx, bomID, sch)
	if err != nil {
		return Schedule{}, fmt.Errorf("set schedule of bom %d: %w", bomID, err)
	}
	return out, nil
}

// Schedule renvoie le planning du brouillon, ou nil s'il n'en a pas.
func (s *Service) Schedule(ctx context.Context, bomID int64) (*Schedule, error) {
	if _, err := s.store.GetBOM(ctx, bomID); err != nil {
		return nil, fmt.Errorf("schedule of bom %d: %w", bomID, err)
	}
	out, err := s.store.Schedule(ctx, bomID)
	if err != nil {
		return nil, fmt.Errorf("schedule of bom %d: %w", bomID, err)
	}
	return out, nil
}

// ClearSchedule retire le planning : les délais restent en jours, sans date.
func (s *Service) ClearSchedule(ctx context.Context, bomID int64) error {
	if _, err := s.store.GetBOM(ctx, bomID); err != nil {
		return fmt.Errorf("clear schedule of bom %d: %w", bomID, err)
	}
	if err := s.store.DeleteSchedule(ctx, bomID); err != nil {
		return fmt.Errorf("clear schedule of bom %d: %w", bomID, err)
	}
	return nil
}

func isDate(t time.Time) bool {
	return t.Location() == time.UTC && t.Equal(time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC))
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
	snapID, err := s.pricing.FreezePrices(ctx, bomID, offerIDs(nodes), []money.Currency{money.Currency(b.Currency)}, at)
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
	BOM      BOM
	Nodes    []Node      // triés par id
	Stock    []PartStock // stock de la BOM, par pièce (ADR 0022)
	Totals   Totals
	Planning Planning // ADR 0023
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
	stock, err := s.store.Stock(ctx, bomID)
	if err != nil {
		return TreeTotals{}, fmt.Errorf("totals of bom %d: %w", bomID, err)
	}
	in, err := s.totalsInput(ctx, b, nodes, stock)
	if err != nil {
		return TreeTotals{}, fmt.Errorf("totals of bom %d: %w", bomID, err)
	}
	t, err := ComputeTotals(in)
	if err != nil {
		return TreeTotals{}, fmt.Errorf("totals of bom %d: %w", bomID, err)
	}
	sch, err := s.store.Schedule(ctx, bomID)
	if err != nil {
		return TreeTotals{}, fmt.Errorf("totals of bom %d: %w", bomID, err)
	}
	plan, err := PlanSchedule(in.Nodes, t, sch)
	if err != nil {
		return TreeTotals{}, fmt.Errorf("totals of bom %d: %w", bomID, err)
	}
	return TreeTotals{BOM: b, Nodes: nodes, Stock: stock, Totals: t, Planning: plan}, nil
}

func (s *Service) totalsInput(ctx context.Context, b BOM, nodes []Node, stock []PartStock) (TotalsInput, error) {
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
		attrs, err := p.BOMAttributes() // attribut NULL : erreur nommée, jamais zéro (AGENTS §3)
		if err != nil {
			return TotalsInput{}, fmt.Errorf("node %d: %w", n.ID, err)
		}
		calc[i].ProductionDays = attrs.ProductionDays // ADR 0007 règle 4 : production de la pièce (catalogue)
		calc[i].Part = &PartFacts{
			PartID: p.ID, OfferID: o.ID, LeadTimeDays: o.LeadTimeDays, CO2Kg: attrs.CO2KGPerUnit, MassKg: attrs.MassKG,
			LaborHours: p.LaborHours, // nil : heures absentes du catalogue → ETP indisponible, jamais zéro
		}
	}

	prices, rates, err := LoadPrices(ctx, s.pricing, b.PriceSnapshotID, offerIDs(nodes), currency)
	if err != nil {
		return TotalsInput{}, err
	}
	return TotalsInput{
		Nodes: calc, Currency: currency, Prices: prices, Rates: rates, PriceSnapshotID: b.PriceSnapshotID,
		Volume: b.Volume, AnnualHours: &b.AnnualHours, Stock: StockMap(stock),
	}, nil
}

// LoadPrices lit le prix de chaque offre de offerIDs et le taux de chaque devise d'offre vers currency :
// en direct en une seule lecture par lot (Pricing.OfferPrices / PairRates) si priceSnapshotID est nil, sinon
// dans ce snapshot de prix (déjà en mémoire chez l'adapter). Un prix ou un taux absent (pricing.ErrNoQuote /
// ErrNoRate) est rendu dans le résultat, pour devenir une erreur nommée sur le nœud ; toute autre erreur
// (snapshot de prix introuvable, offre inconnue de la source, panne) est renvoyée. Partagé par les totaux
// d'un brouillon et ceux d'un snapshot de BOM (domaine snapshot) : même lecture des prix, même calcul.
func LoadPrices(ctx context.Context, p pricing.Pricing, priceSnapshotID *int64, offerIDs []int64, currency money.Currency) (
	map[int64]PriceResult, map[money.Currency]RateResult, error,
) {
	ids := slices.Clone(offerIDs)
	slices.Sort(ids)
	ids = slices.Compact(ids)
	prices, err := loadOfferPrices(ctx, p, priceSnapshotID, ids)
	if err != nil {
		return nil, nil, err
	}
	var fromCurrencies []money.Currency
	for _, id := range ids {
		r := prices[id]
		switch {
		case errors.Is(r.Err, pricing.ErrNoQuote):
			prices[id] = PriceResult{Err: r.Err}
		case r.Err != nil:
			return nil, nil, r.Err
		default:
			if c := r.Quote.Price.Currency(); c != currency {
				fromCurrencies = append(fromCurrencies, c)
			}
		}
	}
	slices.Sort(fromCurrencies)
	fromCurrencies = slices.Compact(fromCurrencies)
	found, err := loadRates(ctx, p, priceSnapshotID, fromCurrencies, currency)
	if err != nil {
		return nil, nil, err
	}
	rates := make(map[money.Currency]RateResult, len(fromCurrencies))
	for _, from := range fromCurrencies {
		r := found[from]
		switch {
		case errors.Is(r.Err, pricing.ErrNoRate):
			rates[from] = RateResult{Err: r.Err}
		case r.Err != nil:
			return nil, nil, r.Err
		default:
			rates[from] = RateResult{Rate: r.Rate}
		}
	}
	return prices, rates, nil
}

// loadOfferPrices : une entrée par offre de ids. En direct, une seule lecture pour tout le lot.
func loadOfferPrices(ctx context.Context, p pricing.Pricing, snapID *int64, ids []int64) (map[int64]PriceResult, error) {
	if snapID == nil {
		return p.OfferPrices(ctx, ids)
	}
	out := make(map[int64]PriceResult, len(ids))
	for _, id := range ids {
		q, err := p.SnapshotOfferPrice(ctx, *snapID, id)
		out[id] = PriceResult{Quote: q, Err: err}
	}
	return out, nil
}

// loadRates : une entrée par devise d'origine. En direct, une seule lecture du cadre FX.
func loadRates(ctx context.Context, p pricing.Pricing, snapID *int64, from []money.Currency, to money.Currency) (map[money.Currency]RateResult, error) {
	out := make(map[money.Currency]RateResult, len(from))
	if snapID == nil {
		pairs := make([]pricing.FXPair, 0, len(from))
		for _, c := range from {
			pairs = append(pairs, pricing.FXPair{Base: c, Quote: to})
		}
		found, err := p.PairRates(ctx, pairs)
		if err != nil {
			return nil, err
		}
		for _, c := range from {
			out[c] = found[pricing.FXPair{Base: c, Quote: to}]
		}
		return out, nil
	}
	for _, c := range from {
		r, err := p.SnapshotRate(ctx, *snapID, c, to)
		out[c] = RateResult{Rate: r, Err: err}
	}
	return out, nil
}

// loadRate lit un seul taux (coût rendu, à la demande) : en direct ou dans le snapshot de prix.
func loadRate(ctx context.Context, p pricing.Pricing, snapID *int64, from, to money.Currency) (pricing.Rate, error) {
	found, err := loadRates(ctx, p, snapID, []money.Currency{from}, to)
	if err != nil {
		return pricing.Rate{}, err
	}
	r := found[from]
	return r.Rate, r.Err
}
