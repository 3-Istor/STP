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
	// CostMethod : méthode de calcul figée (ADR 0031) ; nil pour un snapshot publié avant (purchase + latest).
	CostMethod *bom.CostMethod
	// CostAsOf : date de calcul figée (contrats, année du taux budget) ; nil avec la méthode par défaut.
	CostAsOf *time.Time
	// Profile : profil de véhicule figé (ADR 0036) ; nil si la BOM n'en avait pas (ou publié avant).
	Profile     *bom.VehicleProfile
	PublishedAt time.Time
	// Trace de la décision (ADR 0033) : nil pour un snapshot publié avant, ou champ non saisi.
	Note        *string
	ProposedBy  *string
	ValidatedBy *string
}

// FrozenCheck : contrôle avant publication tel qu'évalué au moment de publier (ADR 0033) ; Justification
// porte le texte saisi pour un avertissement.
type FrozenCheck struct {
	Code, Level, Title, Detail string
	Justification              *string
}

// BudgetRate : taux budget figé, 1 From = Rate To pour l'année Year.
type BudgetRate struct {
	Year     int32
	From, To money.Currency
	Rate     decimal.Decimal
}

// FrozenOffer : faits de coût d'une offre copiés à la publication (remise retenue comprise).
type FrozenOffer struct {
	OfferID int64
	bom.OfferCost
}

// FrozenCost : ce que la publication copie pour rejouer le coût de la méthode sans lire les tables vivantes.
type FrozenCost struct {
	BudgetRates []BudgetRate
	Offers      []FrozenOffer
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
	// Create insère l'en-tête, tous les nœuds, le stock et les faits de coût figés dans une même transaction ;
	// s.ID est ignoré.
	Create(ctx context.Context, s Snapshot, nodes []Node, stock []bom.PartStock, cost FrozenCost, checks []FrozenCheck) (Snapshot, error)
	// Checks renvoie les contrôles figés à la publication (ADR 0033), vides pour un snapshot publié avant.
	Checks(ctx context.Context, id int64) ([]FrozenCheck, error)
	// List renvoie l'en-tête de tous les snapshots, par id (versions d'une famille de BOM).
	List(ctx context.Context) ([]Snapshot, error)
	// CostFacts renvoie les faits de coût figés (ADR 0031), vides pour un snapshot publié avant.
	CostFacts(ctx context.Context, id int64) (FrozenCost, error)
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
	// suppliers : noms des fournisseurs pour les explications de la comparaison ; nil → « fournisseur <id> ».
	suppliers bom.SupplierNames
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
	Checks   []FrozenCheck
	// Input : entrée du calcul (pont de coût, ADR 0032).
	Input bom.TotalsInput
}

// Publish fige le brouillon bomID (R10) : structure, quantités, offres, attributs des pièces (CO2, masse,
// production) et délai de livraison des offres sont copiés ; les prix et taux sont ceux du snapshot de prix
// du brouillon s'il est figé, sinon un snapshot de prix est créé à l'instant at (fourni par l'appelant,
// pas d'horloge dans le domaine). S'il manque un prix ou un taux, rien n'est publié : l'erreur liste tout
// ce qui manque (pricing.ErrNoQuote / ErrNoRate), pour ne pas graver un trou dans un snapshot immuable.
//
// Avant tout, les contrôles avant publication sont recalculés (ADR 0033) : un contrôle block refuse
// (ErrPublicationBlocked), un warn sans justification refuse (ErrJustificationRequired). Note, noms saisis,
// contrôles et justifications sont copiés dans le snapshot.
//
// Le brouillon est lu une seule fois (bom.Store.Draft, lecture cohérente) : contrôles, nœuds figés, stock,
// planning, réglages, budgets et faits de coût viennent tous de cet état, même si un PATCH arrive pendant la
// publication (v2-lot-g T1, ADR 0038). En prix en direct, le même état est rechiffré avec le snapshot de prix
// créé, sans relire le brouillon.
func (s *Service) Publish(ctx context.Context, bomID int64, at time.Time, req PublishRequest) (Snapshot, error) {
	trace, err := req.normalize()
	if err != nil {
		return Snapshot{}, fmt.Errorf("publish bom %d: %w", bomID, err)
	}
	dc, err := s.boms.DraftCost(ctx, bomID)
	if err != nil {
		return Snapshot{}, fmt.Errorf("publish bom %d: %w", bomID, err)
	}
	checks, err := s.boms.PublicationChecksOf(ctx, dc)
	if err != nil {
		return Snapshot{}, fmt.Errorf("publish bom %d: %w", bomID, err)
	}
	frozenChecks, err := gate(checks, trace.Justifications)
	if err != nil {
		return Snapshot{}, fmt.Errorf("publish bom %d: %w", bomID, err)
	}
	b := dc.BOM
	nodes, err := s.freezeNodes(ctx, dc.Nodes)
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
	} else {
		if priceSnapID, err = s.pricing.FreezePrices(ctx, bomID, offers, []money.Currency{currency}, at); err != nil {
			return Snapshot{}, fmt.Errorf("publish bom %d: %w", bomID, err)
		}
		// Même état du brouillon, chiffré avec les prix figés : ce qui est publié est ce qui est copié.
		if dc, err = s.boms.CostOfDraft(ctx, dc.Draft(), &priceSnapID); err != nil {
			return Snapshot{}, fmt.Errorf("publish bom %d: %w", bomID, err)
		}
	}

	header := Snapshot{
		BOMID: b.ID, PriceSnapshotID: priceSnapID, Name: b.Name, Currency: b.Currency, Volume: b.Volume,
		AnnualHours: &b.AnnualHours, Schedule: dc.Schedule, DestinationCountry: b.DestinationCountry, PublishedAt: at,
		Note: trace.Note, ProposedBy: trace.ProposedBy, ValidatedBy: trace.ValidatedBy,
	}
	frozen, err := freezeCost(dc, &header)
	if err != nil {
		return Snapshot{}, fmt.Errorf("publish bom %d: %w", bomID, err)
	}
	if header.Profile, err = s.boms.VehicleProfile(ctx, bomID); err != nil && !errors.Is(err, bom.ErrProfilesUnavailable) {
		return Snapshot{}, fmt.Errorf("publish bom %d: %w", bomID, err)
	}
	out, err := s.store.Create(ctx, header, nodes, dc.Stock, frozen, frozenChecks)
	if err != nil {
		return Snapshot{}, fmt.Errorf("publish bom %d: %w", bomID, err)
	}
	return out, nil
}

// ErrIncompleteCost : la méthode de la BOM ne donne pas de coût complet (composante manquante) ; rien n'est
// publié, pour ne pas graver un trou dans un snapshot immuable.
var ErrIncompleteCost = errors.New("cost under the bom's method is incomplete")

// freezeCost copie, depuis le calcul dc fait avec le snapshot de prix publié, méthode, date de calcul, taux
// budget de l'année et faits de chaque offre (remise retenue comprise). Méthode par défaut : rien d'autre à figer.
func freezeCost(dc bom.DraftCost, h *Snapshot) (FrozenCost, error) {
	ci := dc.Input.Cost
	m := dc.BOM.CostMethod.OrDefault()
	h.CostMethod = &m
	if ci == nil || m == bom.DefaultCostMethod() || (m.Basis == bom.BasisPurchase && m.FX == bom.FXLatest) {
		return FrozenCost{}, nil
	}
	if root, ok := dc.Totals.Node(bom.RootID(dc.Nodes)); ok && root.CostErr != nil {
		errs := []error{root.CostErr}
		for _, id := range root.MissingCost { // la cause de chaque pièce sans coût, nommée
			if n, ok := dc.Totals.Node(id); ok && n.CostErr != nil {
				errs = append(errs, n.CostErr)
			}
		}
		return FrozenCost{}, fmt.Errorf("%w: %w", ErrIncompleteCost, errors.Join(errs...))
	}
	at := ci.AsOf
	h.CostAsOf = &at
	var out FrozenCost
	from := make([]money.Currency, 0, len(ci.BudgetRates))
	for c := range ci.BudgetRates {
		from = append(from, c)
	}
	slices.Sort(from)
	for _, c := range from {
		out.BudgetRates = append(out.BudgetRates, BudgetRate{
			Year: int32(at.Year()), From: c, To: money.Currency(dc.BOM.Currency), Rate: ci.BudgetRates[c], // #nosec G115 -- année < 10000
		})
	}
	ids := make([]int64, 0, len(ci.Offers))
	for id := range ci.Offers {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	for _, id := range ids {
		oc := ci.Offers[id]
		if d, ok := dc.Totals.Discounts[id]; ok {
			oc.Discount = &d
		}
		out.Offers = append(out.Offers, FrozenOffer{OfferID: id, OfferCost: oc})
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
	ci, err := s.frozenCostInput(ctx, h)
	if err != nil {
		return Published{}, fmt.Errorf("get snapshot %d: %w", id, err)
	}
	if err := bom.AddFactRates(ctx, s.pricing, &h.PriceSnapshotID, ci, currency, rates); err != nil {
		return Published{}, fmt.Errorf("get snapshot %d: %w", id, err)
	}
	in := bom.TotalsInput{
		Nodes: calc, Currency: currency, Prices: prices, Rates: rates, PriceSnapshotID: &h.PriceSnapshotID,
		Volume: h.Volume, AnnualHours: h.AnnualHours, Stock: bom.StockMap(stock),
		LaborNotFrozen: h.AnnualHours == nil, // publié avant ADR 0024 : heures non figées
		Cost:           ci,
	}
	t, err := bom.ComputeTotals(in)
	if err != nil {
		return Published{}, fmt.Errorf("get snapshot %d: %w", id, err)
	}
	plan, err := bom.PlanSchedule(calc, t, h.Schedule)
	if err != nil {
		return Published{}, fmt.Errorf("get snapshot %d: %w", id, err)
	}
	checks, err := s.store.Checks(ctx, id)
	if err != nil {
		return Published{}, fmt.Errorf("get snapshot %d: %w", id, err)
	}
	return Published{Snapshot: h, Nodes: nodes, Stock: stock, Totals: t, Planning: plan, Checks: checks, Input: in}, nil
}

// frozenCostInput reconstruit l'entrée de la méthode depuis les seules copies du snapshot (ADR 0031) : ni
// contrat, ni table, ni taux budget vivant n'est relu. Snapshot antérieur : méthode par défaut.
func (s *Service) frozenCostInput(ctx context.Context, h Snapshot) (*bom.CostInput, error) {
	if h.CostMethod == nil {
		return nil, nil //nolint:nilnil // snapshot antérieur à ADR 0031 : calcul historique
	}
	ci := &bom.CostInput{Method: *h.CostMethod, Frozen: true, FactsSource: fmt.Sprintf("snapshot %d", h.ID)}
	if h.CostAsOf != nil {
		ci.AsOf = *h.CostAsOf
	}
	frozen, err := s.store.CostFacts(ctx, h.ID)
	if err != nil {
		return nil, err
	}
	if len(frozen.BudgetRates) > 0 {
		ci.BudgetRates = make(map[money.Currency]decimal.Decimal, len(frozen.BudgetRates))
		for _, r := range frozen.BudgetRates {
			ci.BudgetRates[r.From] = r.Rate
		}
	}
	if len(frozen.Offers) > 0 {
		ci.Offers = make(map[int64]bom.OfferCost, len(frozen.Offers))
		for _, o := range frozen.Offers {
			ci.Offers[o.OfferID] = o.OfferCost
		}
	}
	return ci, nil
}

// CostBreakdown : ventilation d'un nœud du snapshot selon sa méthode figée.
func (s *Service) CostBreakdown(ctx context.Context, id, nodeKey int64) (bom.Breakdown, error) {
	p, err := s.Get(ctx, id)
	if err != nil {
		return bom.Breakdown{}, err
	}
	found, hasChildren := false, false
	for _, n := range p.Nodes {
		found = found || n.Key == nodeKey
		hasChildren = hasChildren || (n.ParentKey != nil && *n.ParentKey == nodeKey)
	}
	if !found {
		return bom.Breakdown{}, fmt.Errorf("cost breakdown of node %d in snapshot %d: %w", nodeKey, id, bom.ErrNodeNotFound)
	}
	out, err := bom.CostBreakdown(p.Totals, nodeKey, hasChildren)
	if err != nil {
		return bom.Breakdown{}, fmt.Errorf("cost breakdown of node %d in snapshot %d: %w", nodeKey, id, err)
	}
	return out, nil
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
	if h.CostMethod != nil && *h.CostMethod != bom.DefaultCostMethod() { // le brouillon garde la méthode figée
		withMethod, err := s.boms.SetCostMethod(ctx, b.ID, *h.CostMethod)
		if err != nil {
			err = fmt.Errorf("copy cost method: %w", err)
			if delErr := s.boms.DeleteBOM(ctx, b.ID); delErr != nil {
				err = errors.Join(err, delErr)
			}
			return bom.BOM{}, fmt.Errorf("new draft from snapshot %d: %w", id, err)
		}
		b = withMethod
	}
	if p := h.Profile; p != nil { // le brouillon garde le profil figé (ADR 0036)
		w := p.Weights
		if _, err := s.boms.SetVehicleProfile(ctx, b.ID, string(p.Preset), &w); err != nil {
			err = fmt.Errorf("copy vehicle profile: %w", err)
			if delErr := s.boms.DeleteBOM(ctx, b.ID); delErr != nil {
				err = errors.Join(err, delErr)
			}
			return bom.BOM{}, fmt.Errorf("new draft from snapshot %d: %w", id, err)
		}
	}
	if err := s.copyDraft(ctx, b.ID, h.BOMID, nodes, stock, h.Schedule); err != nil {
		if delErr := s.boms.DeleteBOM(ctx, b.ID); delErr != nil {
			err = errors.Join(err, fmt.Errorf("remove partial draft %d: %w", b.ID, delErr))
		}
		return bom.BOM{}, fmt.Errorf("new draft from snapshot %d: %w", id, err)
	}
	return b, nil
}

// copyDraft recopie l'arbre, le stock (ADR 0022) et le planning (ADR 0023) figés dans le brouillon bomID, puis
// les budgets cibles de la BOM source (non figés au snapshot : ceux d'aujourd'hui) des nœuds présents dans le
// snapshot, pour que le brouillon se compare à la même cible.
func (s *Service) copyDraft(ctx context.Context, bomID, sourceBOM int64, nodes []Node, stock []bom.PartStock, sch *bom.Schedule) error {
	newIDs, err := s.copyTree(ctx, bomID, nodes)
	if err != nil {
		return err
	}
	budgets, err := s.boms.TargetBudgetList(ctx, sourceBOM)
	if err != nil {
		return err
	}
	for _, b := range budgets {
		id, ok := newIDs[b.NodeID]
		if !ok {
			continue
		}
		b.NodeID = id
		if err := s.boms.SetTargetBudget(ctx, bomID, b); err != nil {
			return fmt.Errorf("copy budget: %w", err)
		}
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
// parmi les nœuds prêts (ordre déterministe des nouveaux ids). Rend la correspondance clé figée → nouvel id.
func (s *Service) copyTree(ctx context.Context, bomID int64, nodes []Node) (map[int64]int64, error) {
	_, draft, err := s.boms.Tree(ctx, bomID)
	if err != nil {
		return nil, err
	}
	if len(draft) != 1 {
		return nil, fmt.Errorf("new bom %d has %d nodes, want only its root", bomID, len(draft))
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
		return nil, errors.New("snapshot has no root")
	}
	newIDs := map[int64]int64{root.Key: draftRoot.ID}
	if !root.Quantity.Equal(draftRoot.Quantity) {
		if _, err := s.boms.SetNodeQuantity(ctx, bomID, draftRoot.ID, root.Quantity); err != nil {
			return nil, fmt.Errorf("root: %w", err)
		}
	}
	if root.Part == nil && root.ProductionDays != draftRoot.ProductionDays {
		if _, err := s.boms.SetNodeProductionDays(ctx, bomID, draftRoot.ID, root.ProductionDays); err != nil {
			return nil, fmt.Errorf("root: %w", err)
		}
	}
	if root.Strategy != nil {
		if _, err := s.boms.SetNodeStrategy(ctx, bomID, draftRoot.ID, root.Strategy); err != nil {
			return nil, fmt.Errorf("root: %w", err)
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
			return nil, fmt.Errorf("copy node %d: %w", c.Key, err)
		}
		newIDs[c.Key] = added.ID
		if c.Strategy != nil {
			if _, err := s.boms.SetNodeStrategy(ctx, bomID, added.ID, c.Strategy); err != nil {
				return nil, fmt.Errorf("copy strategy of node %d: %w", c.Key, err)
			}
		}
		push(c.Key, added.ID)
	}
	return newIDs, nil
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
