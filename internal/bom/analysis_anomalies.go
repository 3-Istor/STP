package bom

import (
	"cmp"
	"context"
	"fmt"
	"slices"

	"github.com/shopspring/decimal"

	"bomstudio/internal/money"
	"bomstudio/internal/pricing"
)

// AnomalyMethod : méthode de détection des anomalies de coût (ADR 0027, ADR 0037).
type AnomalyMethod string

const (
	// AnomalyPerKg : prix au kg comparé à la médiane €/kg de la famille (ADR 0037), défaut des analyses.
	AnomalyPerKg AnomalyMethod = "per_kg"
	// AnomalyMedian : prix unitaire comparé à la médiane de la catégorie (ADR 0027).
	AnomalyMedian AnomalyMethod = "median"
	// AnomalyOffers : coût de l'offre retenue comparé à la médiane des offres de la même pièce (ADR 0039).
	AnomalyOffers AnomalyMethod = "offers"
	// AnomalyHistory : prix coté comparé à la médiane de ses cotations des 12 derniers mois (ADR 0039).
	AnomalyHistory AnomalyMethod = "history"
	// AnomalyMaterial : prix au kg comparé à la médiane €/kg de la famille catégorie × matière (ADR 0039).
	AnomalyMaterial AnomalyMethod = "material"
)

// anomalyDetectorFor : le seul endroit où une méthode d'anomalie devient un détecteur (AGENTS §4), avec son
// seuil par défaut. median garde le détecteur injecté (WithAnomalyDetector, remplaçable par une IA).
func (s *Service) anomalyDetectorFor(m AnomalyMethod) (pricing.AnomalyDetector, decimal.Decimal, error) {
	switch m {
	case AnomalyPerKg:
		return pricing.PerKgDetector{MinPeers: 5}, pricing.DefaultPerKgThreshold, nil
	case AnomalyMedian:
		return s.anomalies, pricing.DefaultAnomalyThreshold, nil
	case AnomalyOffers:
		return pricing.OfferSpreadDetector{MinOffers: 2}, pricing.DefaultOfferSpreadThreshold, nil
	case AnomalyHistory:
		return pricing.HistoryDetector{MinQuotes: 5}, pricing.DefaultHistoryThreshold, nil
	case AnomalyMaterial:
		return pricing.PerKgDetector{MinPeers: 5, ByFamily: true}, pricing.DefaultPerKgThreshold, nil
	}
	return nil, decimal.Decimal{}, fmt.Errorf("anomaly method %q (want per_kg, median, offers, history or material): %w", m, ErrInvalidInput)
}

// PartAnomaly : verdict d'une pièce de la BOM (à une offre retenue) et surcoût estimé par voiture.
type PartAnomaly struct {
	pricing.Anomaly
	Name     string
	OfferID  int64
	NodeIDs  []int64
	Quantity decimal.Decimal // Σ quantités réelles des nœuds, par voiture
	// Impact : Excess × Quantity (surcoût estimé par voiture, négatif sous la référence) ; nil si non évaluable.
	Impact *money.Money
}

// PartAnomalies : anomalies par pièce, classées : anomalies, puis normales, par surcoût décroissant (égalité :
// id de pièce) ; enfin les non évaluables, par id de pièce, avec leur raison.
type PartAnomalies struct {
	Method          AnomalyMethod
	Threshold       decimal.Decimal
	Currency        money.Currency
	PriceSnapshotID *int64
	Rates           []pricing.Rate
	Parts           []PartAnomaly
}

// PartAnomalies juge chaque pièce du brouillon selon la méthode ; threshold nil → seuil par défaut de la méthode.
func (s *Service) PartAnomalies(ctx context.Context, bomID int64, method AnomalyMethod, threshold *decimal.Decimal) (PartAnomalies, error) {
	detector, th, err := s.anomalyDetectorFor(method)
	if err != nil {
		return PartAnomalies{}, fmt.Errorf("anomalies of bom %d: %w", bomID, err)
	}
	if threshold != nil {
		th = *threshold
	}
	res, judged, nodes, err := s.anomaliesWith(ctx, bomID, detector, th)
	if err != nil {
		return PartAnomalies{}, err
	}
	qty, err := realQuantities(nodes)
	if err != nil {
		return PartAnomalies{}, fmt.Errorf("anomalies of bom %d: %w", bomID, err)
	}
	out := GroupPartAnomalies(res, judged, qty)
	out.Method = method
	return out, nil
}

// GroupPartAnomalies regroupe les verdicts par (pièce, offre) — mêmes entrées, même verdict — et chiffre le
// surcoût sur la quantité réelle totale de la pièce.
func GroupPartAnomalies(res CostAnomalies, judged []anomalyNode, qty map[int64]decimal.Decimal) PartAnomalies {
	out := PartAnomalies{Threshold: res.Threshold, Currency: res.Currency, PriceSnapshotID: res.PriceSnapshotID, Rates: res.Rates}
	byNode := make(map[int64]pricing.Anomaly, len(res.Nodes))
	for _, n := range res.Nodes {
		byNode[n.NodeID] = n.Anomaly
	}
	type key struct{ part, offer int64 }
	index := map[key]int{}
	for _, j := range judged {
		k := key{*j.node.PartID, *j.node.OfferID}
		i, ok := index[k]
		if !ok {
			i = len(out.Parts)
			index[k] = i
			out.Parts = append(out.Parts, PartAnomaly{Anomaly: byNode[j.node.ID], Name: j.part.Name, OfferID: k.offer})
		}
		p := &out.Parts[i]
		p.NodeIDs = append(p.NodeIDs, j.node.ID)
		p.Quantity = p.Quantity.Add(qty[j.node.ID])
	}
	for i := range out.Parts {
		if p := &out.Parts[i]; p.Excess != nil {
			impact := p.Excess.Mul(p.Quantity)
			p.Impact = &impact
		}
	}
	rank := map[pricing.AnomalyStatus]int{pricing.AnomalyDetected: 0, pricing.AnomalyNormal: 1, pricing.AnomalyNotEvaluable: 2}
	slices.SortFunc(out.Parts, func(a, b PartAnomaly) int {
		if c := cmp.Compare(rank[a.Status], rank[b.Status]); c != 0 {
			return c
		}
		if a.Impact != nil && b.Impact != nil {
			if c := b.Impact.Amount().Cmp(a.Impact.Amount()); c != 0 {
				return c
			}
		}
		return cmp.Or(cmp.Compare(a.PartID, b.PartID), cmp.Compare(a.OfferID, b.OfferID))
	})
	return out
}
