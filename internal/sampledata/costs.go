package sampledata

import (
	"fmt"
	"slices"
	"time"

	"github.com/shopspring/decimal"
)

// Coût complet (ADR 0031) : valeurs fictives d'ordre de grandeur, pas des tarifs douaniers ou de transport
// réels. Destinations : Multipla assemblée en Italie, Valhalla au Royaume-Uni.

// arden : le contrat-cadre d'Arden Composites a expiré au 2026-03-31 (démonstration : une remise qui ne
// s'applique plus une fois l'horloge simulée au-delà de cette date).
const ardenKey, ardenContractEnd = "sup.arden", "2026-03-31"

var origins = []string{"CN", "DE", "ES", "FR", "GB", "IT", "JP", "KR", "MX", "PL", "PT", "US"}

// customsPercent : droits par origine selon la destination (accords UE–RU, UE–Japon, UE–Corée, UE–Mexique : 0).
var customsPercent = map[string]map[string]string{
	"IT": {"CN": "4.5", "US": "4.5"},
	"GB": {"CN": "4.5", "US": "4.5"},
}

// freightPerKg : transport par kg vers la destination, dans sa devise.
var freightPerKg = map[string]struct {
	currency string
	perKg    map[string]string
}{
	"IT": {"EUR", map[string]string{"CN": "0.80", "DE": "0.14", "ES": "0.15", "FR": "0.12", "GB": "0.28", "IT": "0.06",
		"JP": "0.95", "KR": "0.88", "MX": "0.72", "PL": "0.16", "PT": "0.18", "US": "0.65"}},
	"GB": {"GBP", map[string]string{"CN": "0.68", "DE": "0.22", "ES": "0.24", "FR": "0.20", "GB": "0.05", "IT": "0.25",
		"JP": "0.80", "KR": "0.75", "MX": "0.62", "PL": "0.24", "PT": "0.26", "US": "0.55"}},
}

// budgetRates2026 : taux budget 2026 fixés par la trésorerie (fictifs), vers EUR (Multipla) et GBP (Valhalla).
var budgetRates2026 = []BudgetRate{
	{2026, "GBP", "EUR", "1.1500"}, {2026, "JPY", "EUR", "0.0062"}, {2026, "USD", "EUR", "0.9100"},
	{2026, "EUR", "GBP", "0.8700"}, {2026, "JPY", "GBP", "0.0054"}, {2026, "USD", "GBP", "0.7900"},
}

// toolingEUR : outillage (moules, outils d'emboutissage) par offre des pièces qui en ont, en EUR.
// Toutes les autres offres (Multipla, communes et Valhalla) déclarent 0 explicitement : le jeu ne laisse
// aucun outillage non renseigné, donc aucun coût complet en erreur (ErrNoTooling) à cause du jeu.
var toolingEUR = map[string]string{
	"mpl.porte-av": "84000", "mpl.porte-ar": "78000", "mpl.hayon": "92000", "mpl.capot": "65000",
	"mpl.pavillon": "110000", "mpl.cote-caisse": "240000", "mpl.plancher-av": "150000", "mpl.plancher-ar": "160000",
	"mpl.pare-choc-av": "120000", "mpl.pare-choc-ar": "110000", "mpl.planche": "180000",
	"mpl.bloc": "380000", "mpl.culasse": "290000", "mpl.carter-bv": "210000",
	// Valhalla : moules de composites et outillages de fonderie/forge, séries courtes (valeurs fictives).
	"vlh.monocoque": "1200000", "vlh.berceau-av": "180000", "vlh.berceau-ar": "220000", "vlh.crash-box": "90000",
	"vlh.arceau": "150000", "vlh.plancher-plat": "260000", "vlh.cloison-feu": "110000", "vlh.porte": "320000",
	"vlh.capot-av": "190000", "vlh.capot-moteur": "210000", "vlh.aile-av": "140000", "vlh.ponton": "150000",
	"vlh.bouclier-av": "200000", "vlh.diffuseur": "230000", "vlh.aileron": "280000", "vlh.bloc": "650000",
	"vlh.culasse": "480000", "vlh.vilebrequin": "160000", "vlh.collecteur-adm": "170000",
	"vlh.collecteur-ech": "140000", "vlh.turbo": "260000", "vlh.batterie-ht": "300000",
}

// multiplaBudgets : budgets cibles par voiture (EUR) ; le nom désigne un sous-ensemble unique de l'arbre,
// la racine porte le budget global.
var multiplaBudgets = []struct{ name, amount string }{
	{"", "26500"},
	{"Caisse en blanc", "2850"}, {"Carrosserie et ouvrants", "3000"}, {"Groupe motopropulseur", "7300"},
	{"Liaison au sol", "5000"}, {"Électricité et électronique", "2950"}, {"Habitacle", "5400"},
	{"Porte avant gauche", "420"}, {"Porte avant droite", "420"}, {"Capot", "155"},
}

// Planning Multipla : départ à l'instant simulé initial ; la cible laisse 2 jours de marge sur la
// disponibilité calculée (58 jours de chemin critique avec le jeu demo, seed 42 : vérifié par test).
const multiplaStart, multiplaLeadDays, multiplaMarginDays = "2026-01-01", 58, 2

// costParams pose emballage et outillage sur les offres, et les tables partagées.
func costParams(seed string, d *Dataset) {
	for i := range d.Offers {
		o := &d.Offers[i]
		places := minorPlaces[o.Currency]
		mid := dec(o.PriceMin).Add(dec(o.PriceMax)).Div(dec("2"))
		pack := mid.Mul(between(seed, o.Key, "packaging", dec("0.008"), dec("0.020"), 4)).Round(places)
		o.PackagingAmount = decimal.Max(pack, decimal.New(1, -places)).StringFixed(places)
		o.PackagingCurrency = o.Currency
		switch {
		case toolingEUR[o.PartKey] != "":
			o.ToolingAmount, o.ToolingCurrency = toolingEUR[o.PartKey], "EUR"
		default:
			o.ToolingAmount, o.ToolingCurrency = "0", "EUR"
		}
	}
	for _, dest := range []string{"GB", "IT"} {
		for _, from := range origins {
			pct := customsPercent[dest][from]
			if pct == "" {
				pct = "0"
			}
			d.CustomsDuties = append(d.CustomsDuties, CustomsDuty{Origin: from, Destination: dest, Percent: pct})
			f := freightPerKg[dest]
			d.FreightRates = append(d.FreightRates, FreightRate{Origin: from, Destination: dest, AmountPerKg: f.perKg[from], Currency: f.currency})
		}
	}
	d.BudgetRates = slices.Clone(budgetRates2026)
}

// pilotMultipla pose sur la Multipla la méthode full + budget, le volume programme, les budgets et le planning.
func pilotMultipla(b *BOM) error {
	b.CostBasis, b.FX, b.ProgramVolume = "full", "budget", 60000
	for _, want := range multiplaBudgets {
		key := b.Nodes[0].Key
		if want.name != "" {
			var found []string
			for _, n := range b.Nodes {
				if n.Name == want.name && n.PartKey == "" {
					found = append(found, n.Key)
				}
			}
			if len(found) != 1 {
				return fmt.Errorf("budget %q: %d nodes with this name, want 1: %w", want.name, len(found), ErrInvalidSpec)
			}
			key = found[0]
		}
		b.Budgets = append(b.Budgets, Budget{NodeKey: key, Amount: want.amount, Currency: "EUR"})
	}
	start, err := dateOnly(multiplaStart)
	if err != nil {
		return err
	}
	b.StartOn = multiplaStart
	b.TargetOn = start.AddDate(0, 0, multiplaLeadDays+multiplaMarginDays).Format("2006-01-02")
	return nil
}

func dateOnly(s string) (time.Time, error) {
	t, err := time.Parse(time.DateOnly, s)
	if err != nil {
		return t, fmt.Errorf("date %q: %w", s, ErrInvalidSpec)
	}
	return t, nil
}
