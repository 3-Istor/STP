package sampledata

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

// GeneratorVersion identifie les règles de génération. Toute modification des données de référence ou
// des règles change l'empreinte du jeu : un dataset_version déjà chargé refuse alors le nouveau contenu.
const GeneratorVersion = "sampledata.v3" // v2 : coût complet (ADR 0031) ; v3 : matière des pièces (ADR 0041)

// Dataset est le jeu généré, prêt à charger. Toutes les valeurs décimales sont des chaînes canoniques
// (jamais de float) ; les liens passent par des clés logiques stables, pas par des ids de base.
type Dataset struct {
	Suppliers    []Supplier
	Parts        []Part
	Offers       []Offer
	Contracts    []Contract
	Alternatives []Alternative
	BOMs         []BOM
	// Coût complet (ADR 0031) : tables partagées (douane, transport, taux budget), chargées sans écraser une
	// ligne déjà saisie.
	CustomsDuties []CustomsDuty
	FreightRates  []FreightRate
	BudgetRates   []BudgetRate
}

// CustomsDuty : droit de douane fictif en % du prix d'achat, par (origine, destination).
type CustomsDuty struct {
	Origin, Destination, Percent string
}

// FreightRate : transport fictif par kg, par (origine, destination).
type FreightRate struct {
	Origin, Destination, AmountPerKg, Currency string
}

// BudgetRate : taux budget de l'année, 1 From = Rate To.
type BudgetRate struct {
	Year           int32
	From, To, Rate string
}

// Supplier est un fournisseur fictif.
type Supplier struct {
	Key, Name     string
	QualityScore  string // 0–5
	TrustScore    string // 0–5
	Country       string // ISO 3166-1 alpha-2, pays d'origine de ses offres
	Currency      string // ISO 4217, devise de ses offres
	baseLeadDays  int
	categories    []string
	minorQuantity bool // paliers bas : fournisseur de petites séries
}

// Part est une pièce du catalogue ; tous ses attributs sont renseignés (exigence sample-dataset).
type Part struct {
	Key, Name, Category, Zone string
	Material                  string // matière (ADR 0041)
	MassKG                    string
	CO2KGPerUnit              string
	RecycledPercent           string
	RecyclabilityPercent      string
	ManufacturingAmount       string
	ManufacturingCurrency     string
	ProductionDays            int
	LaborHours                string
	HourlyAmount              string
	HourlyCurrency            string
}

// Offer est une pièce vendue par un fournisseur, avec les bornes de cotation de la source de prix.
type Offer struct {
	Key, PartKey, SupplierKey string
	Currency                  string
	PriceMin, PriceMax        string
	LeadTimeDays              int
	OriginCountry             string
	QualityScore              string
	// PackagingAmount / ToolingAmount : dans la devise indiquée ; ToolingAmount vide = non renseigné (NULL).
	PackagingAmount, PackagingCurrency string
	ToolingAmount, ToolingCurrency     string
}

// Contract est un contrat-cadre d'un fournisseur sur toutes ses offres, avec ses paliers de remise.
type Contract struct {
	Key, SupplierKey string
	StartsOn, EndsOn string // AAAA-MM-JJ, bornes inclusives (ADR 0010)
	Conditions       string
	OfferKeys        []string
	Tiers            []Tier
}

// Tier est un palier : remise appliquée à toute la quantité dès MinQuantity (ADR 0010).
type Tier struct {
	MinQuantity, DiscountPercent string
}

// Alternative relie deux pièces interchangeables (relation symétrique).
type Alternative struct {
	PartKey, OtherKey string
}

// BOM est un brouillon : Nodes[0] est la racine, chaque parent précède ses enfants.
type BOM struct {
	Key, Name, Currency string
	Volume              int32
	Nodes               []Node
	// Réglages du coût complet (ADR 0031) et pilotage : destination, méthode, volume programme (0 : aucun),
	// budgets cibles par nœud, planning (dates AAAA-MM-JJ, vides : aucun).
	Destination       string
	CostBasis, FX     string
	ProgramVolume     int32
	Budgets           []Budget
	StartOn, TargetOn string
}

// Budget : budget cible par voiture d'un nœud (clé logique).
type Budget struct {
	NodeKey, Amount, Currency string
}

// Node est un sous-ensemble (Name) ou une pièce (PartKey + OfferKey) ; ParentKey vide : racine.
type Node struct {
	Key, ParentKey string
	Name           string
	PartKey        string
	OfferKey       string
	Quantity       string
	ProductionDays int32
}

// Fingerprint est l'empreinte SHA-256 du contenu (et de la version du générateur).
func (d Dataset) Fingerprint() string {
	b, err := json.Marshal(struct {
		Version string
		Data    Dataset
	}{GeneratorVersion, d})
	if err != nil { // types simples : impossible
		panic(err)
	}
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// NodeCount renvoie le nombre total de nœuds de toutes les BOM.
func (d Dataset) NodeCount() int {
	n := 0
	for _, b := range d.BOMs {
		n += len(b.Nodes)
	}
	return n
}
