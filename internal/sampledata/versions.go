package sampledata

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/shopspring/decimal"

	"bomstudio/internal/bom"
	"bomstudio/internal/money"
	"bomstudio/internal/snapshot"
)

// Versions de démonstration (vision V2 §3, maquettes I4-0, I4-6, I4-7) : la Multipla publiée en V1, un
// brouillon B créé depuis V1 avec quelques changements réalistes, et cinq pistes de réduction. Contrairement au
// jeu généré (Dataset, chargé en SQL), ces étapes passent par les services du domaine : la publication fige
// exactement ce que l'application figerait. Le Dataset et son empreinte ne changent pas ; chaque étape est
// enregistrée (seed_demo_steps) et n'est pas rejouée.

// DemoPlan : ce que les étapes de démonstration font, en clés logiques du jeu.
type DemoPlan struct {
	BOMKey                        string
	Note, ProposedBy, ValidatedBy string
	// Justifications : texte par code d'avertissement possible à la publication de V1.
	Justifications map[string]string
	OfferChanges   []DemoOfferChange
	Additions      []DemoAddition
	Ideas          []DemoIdea
}

// DemoOfferChange : dans le brouillon B, la pièce PartKey du sous-ensemble Assembly passe à l'offre OfferKey.
type DemoOfferChange struct {
	Assembly, PartKey, OfferKey string
}

// DemoAddition : dans le brouillon B, le sous-ensemble Assembly doit avoir Want nœuds de la pièce PartKey (ajoutés
// avec l'offre du nœud existant).
type DemoAddition struct {
	Assembly, PartKey string
	Want              int
}

// DemoIdea : piste de réduction de la Multipla (écart à son budget global) ; LinkDraft : chiffrée dans le brouillon
// B (une piste ne se lie jamais à sa propre BOM, son gain y serait déjà compté, v2-lot-g T6).
type DemoIdea struct {
	Title, Owner, Gain, Status string
	LinkDraft                  bool
}

var doors = []string{"Porte avant gauche", "Porte avant droite", "Porte arrière gauche", "Porte arrière droite"}

// Demo construit le plan depuis le jeu généré (même graine, même plan).
func Demo(ds Dataset) (DemoPlan, error) {
	p := DemoPlan{
		BOMKey: "bom.multipla", Note: "Jalon de lancement", ProposedBy: "Arthur P.", ValidatedBy: "Joe",
		Justifications: map[string]string{
			bom.CheckPricesFrozen: "Prix figés à la publication du jalon.",
			bom.CheckBudgets:      "Dépassements connus, pistes de réduction ouvertes.",
			bom.CheckTargetDate:   "Retard accepté pour le jalon de lancement.",
		},
	}
	var multipla *BOM
	for i := range ds.BOMs {
		if ds.BOMs[i].Key == p.BOMKey {
			multipla = &ds.BOMs[i]
		}
	}
	if multipla == nil {
		return DemoPlan{}, fmt.Errorf("demo: bom %s: %w", p.BOMKey, ErrInvalidSpec)
	}
	// Portes avant nues : l'offre utilisée est remplacée par l'autre offre déclarée de la même pièce (première par clé).
	const door = "mpl.porte-av"
	var current string
	for _, n := range multipla.Nodes {
		if n.PartKey == door {
			current = n.OfferKey
		}
	}
	var others []Offer
	for _, o := range ds.Offers {
		if o.PartKey == door && o.Key != current {
			others = append(others, o)
		}
	}
	if current == "" || len(others) == 0 {
		return DemoPlan{}, fmt.Errorf("demo: part %s needs a used offer and another one: %w", door, ErrInvalidSpec)
	}
	slices.SortFunc(others, func(a, b Offer) int { return compareStrings(a.Key, b.Key) })
	newOffer := others[0]
	supplierName := newOffer.SupplierKey
	for _, s := range ds.Suppliers {
		if s.Key == newOffer.SupplierKey {
			supplierName = s.Name
		}
	}
	for _, d := range doors[:2] {
		p.OfferChanges = append(p.OfferChanges, DemoOfferChange{Assembly: d, PartKey: door, OfferKey: newOffer.Key})
	}
	for _, d := range doors {
		p.Additions = append(p.Additions, DemoAddition{Assembly: d, PartKey: "mpl.haut-parleur", Want: 2})
	}
	p.Ideas = []DemoIdea{
		{Title: "Portes avant nues → " + supplierName, Owner: "Brian P.", Gain: "91", Status: "in_progress", LinkDraft: true},
		{Title: "Jante alu → tôle (base)", Owner: "Arthur P.", Gain: "286", Status: "to_validate"},
		{Title: "Renégocier Arden", Owner: "Joe", Gain: "102", Status: "in_progress"},
		{Title: "Garnitures → ES", Owner: "Brian P.", Gain: "68", Status: "idea"},
		{Title: "Calculateur : 2e source", Owner: "Arthur P.", Gain: "93", Status: "idea"},
	}
	return p, nil
}

func compareStrings(a, b string) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

// DemoSteps : étapes déjà appliquées (seedstore les enregistre, insertion seule).
type DemoSteps interface {
	EntityIDs(ctx context.Context, kind string) (map[string]int64, error)
	Step(ctx context.Context, step string) (int64, bool, error)
	RecordStep(ctx context.Context, step string, id int64) error
}

// DemoBOMs : ce que les étapes demandent au domaine bom ; *bom.Service la satisfait.
type DemoBOMs interface {
	Tree(ctx context.Context, id int64) (bom.BOM, []bom.Node, error)
	PublicationChecks(ctx context.Context, bomID int64) ([]bom.PublicationCheck, error)
	SetNodeOffer(ctx context.Context, bomID, nodeID, offerID int64) (bom.Node, error)
	AddNode(ctx context.Context, bomID int64, n bom.NewNode) (bom.Node, error)
	IdeaList(ctx context.Context, bomID int64) ([]bom.Idea, error)
	CreateIdea(ctx context.Context, bomID int64, in bom.IdeaInput) (bom.Idea, error)
}

// DemoSnapshots : publication et brouillon depuis un snapshot ; *snapshot.Service la satisfait.
type DemoSnapshots interface {
	Publish(ctx context.Context, bomID int64, at time.Time, req snapshot.PublishRequest) (snapshot.Snapshot, error)
	NewDraft(ctx context.Context, id int64) (bom.BOM, error)
}

// Noms des étapes enregistrées.
const (
	stepV1    = "multipla.v1"
	stepDraft = "multipla.draft_b"
	stepIdeas = "multipla.ideas"
)

// DemoResult : ids créés ou retrouvés.
type DemoResult struct {
	SnapshotID, DraftID int64
	Created             bool
}

// ApplyDemo publie V1 (à l'instant at, temps simulé), crée le brouillon B, y applique les changements et les
// pistes. Chaque étape déjà enregistrée est sautée ; les changements de B et les pistes sont idempotents
// (un nœud déjà à la bonne offre, un haut-parleur déjà ajouté, une piste de même titre ne sont pas refaits).
func ApplyDemo(ctx context.Context, steps DemoSteps, boms DemoBOMs, snaps DemoSnapshots, p DemoPlan, at time.Time) (DemoResult, error) {
	var res DemoResult
	bomIDs, err := steps.EntityIDs(ctx, "bom")
	if err != nil {
		return res, err
	}
	parts, err := steps.EntityIDs(ctx, "part")
	if err != nil {
		return res, err
	}
	offers, err := steps.EntityIDs(ctx, "offer")
	if err != nil {
		return res, err
	}
	multipla, ok := bomIDs[p.BOMKey]
	if !ok {
		return res, fmt.Errorf("demo: bom %s not loaded: %w", p.BOMKey, ErrInvalidSpec)
	}

	v1, done, err := steps.Step(ctx, stepV1)
	if err != nil {
		return res, err
	}
	if !done {
		req, err := justify(ctx, boms, multipla, p)
		if err != nil {
			return res, err
		}
		s, err := snaps.Publish(ctx, multipla, at, req)
		if err != nil {
			return res, fmt.Errorf("demo: publish V1: %w", err)
		}
		if err := steps.RecordStep(ctx, stepV1, s.ID); err != nil {
			return res, err
		}
		v1, res.Created = s.ID, true
	}
	res.SnapshotID = v1

	draft, done, err := steps.Step(ctx, stepDraft)
	if err != nil {
		return res, err
	}
	if !done {
		b, err := snaps.NewDraft(ctx, v1)
		if err != nil {
			return res, fmt.Errorf("demo: draft B: %w", err)
		}
		if err := steps.RecordStep(ctx, stepDraft, b.ID); err != nil {
			return res, err
		}
		draft, res.Created = b.ID, true
	}
	res.DraftID = draft
	if err := editDraft(ctx, boms, draft, p, parts, offers); err != nil {
		return res, err
	}
	if _, done, err = steps.Step(ctx, stepIdeas); err != nil || done {
		return res, err
	}
	if err := addIdeas(ctx, boms, multipla, draft, p); err != nil {
		return res, err
	}
	return res, steps.RecordStep(ctx, stepIdeas, multipla)
}

// justify : la publication de V1 reprend les contrôles du moment ; un avertissement sans texte prévu ou un
// contrôle bloquant arrête le seed (rien n'est publié en silence).
func justify(ctx context.Context, boms DemoBOMs, bomID int64, p DemoPlan) (snapshot.PublishRequest, error) {
	checks, err := boms.PublicationChecks(ctx, bomID)
	if err != nil {
		return snapshot.PublishRequest{}, fmt.Errorf("demo: checks of V1: %w", err)
	}
	req := snapshot.PublishRequest{Note: p.Note, ProposedBy: p.ProposedBy, ValidatedBy: p.ValidatedBy, Justifications: map[string]string{}}
	var errs []error
	for _, c := range checks {
		switch c.Level {
		case bom.CheckBlock:
			errs = append(errs, fmt.Errorf("check %s blocks V1: %s", c.Code, c.Detail))
		case bom.CheckWarn:
			text, ok := p.Justifications[c.Code]
			if !ok {
				errs = append(errs, fmt.Errorf("check %s warns (%s) and the demo has no justification for it", c.Code, c.Detail))
			}
			req.Justifications[c.Code] = text
		}
	}
	if len(errs) > 0 {
		return req, fmt.Errorf("demo: %w", errors.Join(errs...))
	}
	return req, nil
}

func editDraft(ctx context.Context, boms DemoBOMs, draft int64, p DemoPlan, parts, offers map[string]int64) error {
	_, nodes, err := boms.Tree(ctx, draft)
	if err != nil {
		return fmt.Errorf("demo: draft B: %w", err)
	}
	assembly := map[string]int64{}
	for _, n := range nodes {
		if !n.IsPart() {
			if _, dup := assembly[n.Name]; dup {
				assembly[n.Name] = -1 // nom ambigu
				continue
			}
			assembly[n.Name] = n.ID
		}
	}
	children := func(asm string, partKey string) (int64, []bom.Node, error) {
		id := assembly[asm]
		if id <= 0 {
			return 0, nil, fmt.Errorf("demo: assembly %q missing or ambiguous in draft B: %w", asm, ErrInvalidSpec)
		}
		var out []bom.Node
		for _, n := range nodes {
			if n.ParentID != nil && *n.ParentID == id && n.PartID != nil && *n.PartID == parts[partKey] {
				out = append(out, n)
			}
		}
		if len(out) == 0 {
			return 0, nil, fmt.Errorf("demo: no %s under %q: %w", partKey, asm, ErrInvalidSpec)
		}
		return id, out, nil
	}
	for _, c := range p.OfferChanges {
		_, ns, err := children(c.Assembly, c.PartKey)
		if err != nil {
			return err
		}
		for _, n := range ns {
			if *n.OfferID == offers[c.OfferKey] {
				continue
			}
			if _, err := boms.SetNodeOffer(ctx, draft, n.ID, offers[c.OfferKey]); err != nil {
				return fmt.Errorf("demo: %s in %q: %w", c.PartKey, c.Assembly, err)
			}
		}
	}
	for _, a := range p.Additions {
		parent, ns, err := children(a.Assembly, a.PartKey)
		if err != nil {
			return err
		}
		for k := len(ns); k < a.Want; k++ {
			if _, err := boms.AddNode(ctx, draft, bom.NewNode{ParentID: parent, PartID: ns[0].PartID, OfferID: ns[0].OfferID,
				Quantity: decimal.NewFromInt(1)}); err != nil {
				return fmt.Errorf("demo: add %s in %q: %w", a.PartKey, a.Assembly, err)
			}
		}
	}
	return nil
}

func addIdeas(ctx context.Context, boms DemoBOMs, bomID, draft int64, p DemoPlan) error {
	existing, err := boms.IdeaList(ctx, bomID)
	if err != nil {
		return err
	}
	for _, i := range p.Ideas {
		if slices.ContainsFunc(existing, func(e bom.Idea) bool { return e.Title == i.Title }) {
			continue
		}
		gain, err := money.Parse(i.Gain, "EUR")
		if err != nil {
			return fmt.Errorf("demo: idea %q: %w", i.Title, err)
		}
		in := bom.IdeaInput{Title: i.Title, Owner: i.Owner, Gain: gain, Status: bom.IdeaStatus(i.Status)}
		if i.LinkDraft {
			in.DraftBOMID = &draft
		}
		if _, err := boms.CreateIdea(ctx, bomID, in); err != nil {
			return fmt.Errorf("demo: idea %q: %w", i.Title, err)
		}
	}
	return nil
}
