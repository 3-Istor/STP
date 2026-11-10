package sampledata

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/shopspring/decimal"
)

// ErrInvalidSpec : les données de référence sont incohérentes (clé inconnue ou dupliquée, catégorie
// sans assez de fournisseurs…). C'est une erreur de programmation détectée par les tests.
var ErrInvalidSpec = errors.New("invalid sample dataset spec")

// Dates des contrats-cadres : couvrent l'historique simulé (SIM_AS_OF − 1 an) et la suite.
const contractStart, contractEnd = "2025-01-01", "2027-12-31"

type bomSpec struct {
	key, name, currency string
	volume              int32
	tree                []item
}

var bomSpecs = []bomSpec{
	{"bom.multipla", "Fiat Multipla 1.6 16V", "EUR", 2000, multiplaTree},
	{"bom.valhalla", "Aston Martin Valhalla", "GBP", 150, valhallaTree},
}

// Generate construit le jeu complet pour une graine. Même graine = même Dataset, octet pour octet.
func Generate(seed string) (Dataset, error) {
	var d Dataset
	suppliers := make(map[string]Supplier, len(supplierSpecs))
	for _, s := range supplierSpecs {
		if _, dup := suppliers[s.key]; dup {
			return d, fmt.Errorf("supplier %s: duplicate: %w", s.key, ErrInvalidSpec)
		}
		sup := Supplier{
			Key: s.key, Name: s.name, Country: s.country, Currency: s.currency,
			QualityScore: between(seed, s.key, "quality", dec("3.0"), dec("4.9"), 1).StringFixed(1),
			TrustScore:   between(seed, s.key, "trust", dec("2.8"), dec("4.9"), 1).StringFixed(1),
			baseLeadDays: s.leadDays, categories: s.categories, minorQuantity: s.smallSeries,
		}
		suppliers[s.key] = sup
		d.Suppliers = append(d.Suppliers, sup)
	}

	offersByPart := map[string][]string{}
	for _, sections := range [][]section{commonParts, multiplaParts, valhallaParts} {
		for _, sec := range sections {
			for _, p := range sec.parts {
				if _, dup := offersByPart[p.key]; dup {
					return d, fmt.Errorf("part %s: duplicate: %w", p.key, ErrInvalidSpec)
				}
				d.Parts = append(d.Parts, makePart(seed, sec, p))
				offers, err := makeOffers(seed, sec.category, p, d.Suppliers)
				if err != nil {
					return d, err
				}
				d.Offers = append(d.Offers, offers...)
				for _, o := range offers {
					offersByPart[p.key] = append(offersByPart[p.key], o.Key)
				}
			}
		}
	}

	for _, s := range d.Suppliers {
		c := Contract{
			Key: "ctr." + strings.TrimPrefix(s.Key, "sup."), SupplierKey: s.Key, StartsOn: contractStart, EndsOn: contractEnd,
			Conditions: "Contrat-cadre fictif " + contractStart[:4] + "–" + contractEnd[:4] +
				" : paiement à 60 jours fin de mois, livraison franco usine, révision annuelle des bornes de prix.",
			Tiers: makeTiers(seed, s),
		}
		for _, o := range d.Offers {
			if o.SupplierKey == s.Key {
				c.OfferKeys = append(c.OfferKeys, o.Key)
			}
		}
		if len(c.OfferKeys) > 0 {
			d.Contracts = append(d.Contracts, c)
		}
	}

	for _, pairs := range [][][2]string{multiplaAlternatives, valhallaAlternatives} {
		for _, p := range pairs {
			for _, k := range p {
				if _, ok := offersByPart[k]; !ok {
					return d, fmt.Errorf("alternative %s: unknown part: %w", k, ErrInvalidSpec)
				}
			}
			d.Alternatives = append(d.Alternatives, Alternative{PartKey: p[0], OtherKey: p[1]})
		}
	}

	for _, b := range bomSpecs {
		bom, err := makeBOM(seed, b, offersByPart)
		if err != nil {
			return d, err
		}
		d.BOMs = append(d.BOMs, bom)
	}
	return d, nil
}

func makePart(seed string, sec section, p partSpec) Part {
	m := materials[p.mat]
	mass := dec(p.mass).Mul(between(seed, p.key, "mass", dec("0.97"), dec("1.03"), 4)).Round(4)
	if mass.LessThanOrEqual(decimal.Zero) {
		mass = decimal.New(1, -4)
	}
	co2 := mass.Mul(dec(m.co2PerKg)).Mul(between(seed, p.key, "co2", dec("0.9"), dec("1.1"), 3)).Round(4)
	labor := mass.Mul(dec(m.laborPerKg)).Mul(between(seed, p.key, "labor", dec("0.8"), dec("1.2"), 3)).Round(2)
	if labor.LessThan(dec("0.01")) {
		labor = dec("0.01")
	}
	currency := "EUR"
	if strings.HasPrefix(p.key, "vlh.") {
		currency = "GBP"
	}
	ref := dec(p.price).Mul(dec(eurTo[currency]))
	return Part{
		Key: p.key, Name: p.name, Category: sec.category, Zone: sec.zone,
		MassKG:                mass.String(),
		CO2KGPerUnit:          co2.String(),
		RecycledPercent:       strconv.Itoa(intBetween(seed, p.key, "recycled", m.recycled[0], m.recycled[1])),
		RecyclabilityPercent:  strconv.Itoa(intBetween(seed, p.key, "recyclability", m.recyclability[0], m.recyclability[1])),
		ManufacturingAmount:   ref.Mul(between(seed, p.key, "manufacturing", dec("0.25"), dec("0.40"), 3)).Round(2).StringFixed(2),
		ManufacturingCurrency: currency,
		ProductionDays:        p.days,
		LaborHours:            labor.String(),
		HourlyAmount:          dec(eurTo[currency]).Mul(between(seed, p.key, "hourly", dec("38"), dec("54"), 2)).Round(2).StringFixed(2),
		HourlyCurrency:        currency,
	}
}

// makeOffers choisit 2 ou 3 fournisseurs de la catégorie (classement par hachage) et fixe leurs bornes.
func makeOffers(seed, category string, p partSpec, suppliers []Supplier) ([]Offer, error) {
	var eligible []Supplier
	for _, s := range suppliers {
		if slices.Contains(s.categories, category) {
			eligible = append(eligible, s)
		}
	}
	if len(eligible) < 2 {
		return nil, fmt.Errorf("part %s: category %q has %d suppliers, want at least 2: %w", p.key, category, len(eligible), ErrInvalidSpec)
	}
	slices.SortFunc(eligible, func(a, b Supplier) int {
		return fraction(seed, p.key+"/"+a.Key, "rank").Cmp(fraction(seed, p.key+"/"+b.Key, "rank"))
	})
	n := min(intBetween(seed, p.key, "offers", 2, 3), len(eligible))
	out := make([]Offer, n)
	for i, s := range eligible[:n] {
		key := "off." + p.key + "." + strings.TrimPrefix(s.Key, "sup.")
		places := minorPlaces[s.Currency]
		unit := decimal.New(1, -places)
		base := dec(p.price).Mul(dec(eurTo[s.Currency])).Mul(between(seed, key, "level", dec("0.90"), dec("1.15"), 3))
		lo := decimal.Max(base.Mul(between(seed, key, "min", dec("0.82"), dec("0.92"), 3)).Round(places), unit)
		hi := decimal.Max(base.Mul(between(seed, key, "max", dec("1.08"), dec("1.25"), 3)).Round(places), lo)
		quality := dec(s.QualityScore).Add(between(seed, key, "quality", dec("-0.5"), dec("0.5"), 1))
		quality = decimal.Min(decimal.Max(quality, decimal.Zero), decimal.NewFromInt(5))
		out[i] = Offer{
			Key: key, PartKey: p.key, SupplierKey: s.Key, Currency: s.Currency,
			PriceMin: lo.StringFixed(places), PriceMax: hi.StringFixed(places),
			LeadTimeDays:  s.baseLeadDays + intBetween(seed, key, "lead", 0, 12),
			OriginCountry: s.Country,
			QualityScore:  quality.StringFixed(1),
		}
	}
	return out, nil
}

// makeTiers : trois paliers croissants ; seuils bas pour un fournisseur de petites séries.
func makeTiers(seed string, s Supplier) []Tier {
	thresholds := []string{"500", "5000", "50000"}
	if s.minorQuantity {
		thresholds = []string{"20", "100", "500"}
	}
	ranges := [][2]string{{"1", "3"}, {"3.5", "6"}, {"6.5", "10"}}
	out := make([]Tier, len(thresholds))
	for i, t := range thresholds {
		out[i] = Tier{MinQuantity: t,
			DiscountPercent: between(seed, s.Key, "tier"+t, dec(ranges[i][0]), dec(ranges[i][1]), 1).StringFixed(1)}
	}
	return out
}

func makeBOM(seed string, b bomSpec, offersByPart map[string][]string) (BOM, error) {
	out := BOM{Key: b.key, Name: b.name, Currency: b.currency, Volume: b.volume}
	root := b.key + "/0"
	out.Nodes = append(out.Nodes, Node{Key: root, Name: b.name, Quantity: "1", ProductionDays: 3})
	var walk func(parent string, depth int, items []item) error
	walk = func(parent string, depth int, items []item) error {
		i := 0
		var visit func(items []item) error
		visit = func(items []item) error {
			for _, it := range items {
				if it.name == "" && it.part == "" { // group : enfants insérés dans le parent
					if err := visit(it.children); err != nil {
						return err
					}
					continue
				}
				i++
				key := parent + "." + strconv.Itoa(i)
				if it.part != "" {
					offers := offersByPart[it.part]
					if len(offers) == 0 {
						return fmt.Errorf("bom %s node %s: unknown part %s: %w", b.key, key, it.part, ErrInvalidSpec)
					}
					offer := offers[intBetween(seed, b.key+"/"+it.part, "offer", 0, len(offers)-1)]
					out.Nodes = append(out.Nodes, Node{Key: key, ParentKey: parent, PartKey: it.part, OfferKey: offer, Quantity: it.qty})
					continue
				}
				days := intBetween(seed, key, "assembly", 0, 2)
				if depth == 1 {
					days = intBetween(seed, key, "assembly", 1, 4)
				}
				out.Nodes = append(out.Nodes, Node{Key: key, ParentKey: parent, Name: it.name, Quantity: "1", ProductionDays: int32(days)}) // #nosec G115 -- 0..4
				if err := walk(key, depth+1, it.children); err != nil {
					return err
				}
			}
			return nil
		}
		return visit(items)
	}
	if err := walk(root, 1, b.tree); err != nil {
		return BOM{}, err
	}
	return out, nil
}
