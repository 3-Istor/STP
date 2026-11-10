---
id: nlpf-71
title: Arbre BOM invisible : conflit CSS .workspace entre BOM et Fournisseurs
source: jira
source_ref: https://3istors.atlassian.net/browse/NLPF-71
created: 2026-11-10
status: in-progress
priority: P1
depends_on: []
branch: nlpf-71
---

# Arbre BOM invisible : conflit CSS .workspace entre BOM et Fournisseurs

## Context

Sur la stack Docker, à l'ouverture d'une BOM (ex. Fiat Multipla), la colonne de l'arbre fait 2 px :
l'arbre est invisible et non cliquable. La règle globale `.workspace` de l'écran Fournisseurs
(`margin: 0 auto`, `max-width`, `padding`, grille `260px 1fr`) s'applique aussi à `.bom-space .workspace`.
Dans le conteneur flex colonne de la BOM, `margin: auto` annule l'étirement : la grille se réduit à son
contenu et la colonne `1fr` (lignes en `position: absolute`, donc sans largeur intrinsèque) tombe à 0.
Régression d'intégration NLPF-67/68 × NLPF-69. Ticket Highest, label soutenance-2.

## Research Findings

### Relevant Files
- `web/src/index.css:34` — règle globale `.workspace` (écran Fournisseurs) ; `:162` et `:171` ses variantes
  responsive (`@media`), qui fuient aussi vers la BOM.
- `web/src/suppliers/SuppliersPage.tsx:93` — `<main className="workspace">` (seul consommateur Fournisseurs).
- `web/src/bom/bom.css:220` — `.bom-space .workspace` (grille `1fr 340px`, `flex: 1`) ; `:353` variante mobile.
- `web/src/bom/BomView.tsx:153`, `web/src/bom/SnapshotView.tsx:78` — `<div className="workspace">` côté BOM.
- `web/src/bom/BomTree.tsx` — arbre virtualisé, lignes `.tree-row` en `position: absolute`.
- `web/e2e/` — Playwright (`fixtures.ts`, `suppliers.spec.ts`, `pricing.live.spec.ts`) ; pas encore de
  spec e2e pour l'écran BOM.

### Existing Patterns
- Le CSS BOM est préfixé par `.bom-space` ; le CSS Fournisseurs est global dans `index.css`
  (`.supplier-sidebar`, `.supplier-list`…) : seul `.workspace` est un nom générique partagé.
- e2e : `@playwright/test`, fixtures dans `web/e2e/fixtures.ts`, lancé par `npm run test:e2e`.

### Constraints
- Pas de nouvelle dépendance (AGENTS §7, §11).
- Aucune règle métier touchée ; UI seulement (AGENTS §5).
- L'écran Fournisseurs ne doit pas changer visuellement.

## Requirements

### Must Have
- R1 : quand on ouvre une BOM (brouillon ou snapshot), l'arbre occupe toute la largeur disponible à gauche
  du panneau de détail.
- R2 : les styles d'un espace (BOM, Fournisseurs) ne s'appliquent pas à l'autre : plus de classe de mise en
  page générique partagée.
- R3 : un test e2e échoue si l'arbre a une largeur nulle.

### Should Have
- S1 : la variante mobile (`@media`) de chaque espace reste correcte.

### Won't Do
- Refonte du CSS (passage à CSS modules, renommage général des classes Fournisseurs).
- Suite e2e complète de l'écran BOM (ticket séparé « tests e2e de l'écran BOM ») : ici, seulement le test
  de non-régression de largeur.

## Tasks

- [x] T1: Test e2e de non-régression : l'arbre BOM a une largeur non nulle
  - **Files**: `web/e2e/bom-layout.spec.ts` (nouveau), `web/e2e/fixtures.ts` si un helper manque
  - **Approach**: ouvrir une BOM (créée via l'API dans le test, ou du seed si les fixtures le prévoient),
    attendre `.bom-space .tree`, vérifier `boundingBox().width` > moitié de la largeur de `.workspace`
    moins 340 px, et qu'une ligne `.tree-row` est cliquable (sélection → panneau de détail).
  - **Done when**: le test échoue sur `main` actuel (largeur ≈ 2 px) — échec constaté et noté dans state.md.

- [x] T2: Renommer la mise en page Fournisseurs `.workspace` → `.supplier-workspace`
  - **Files**: `web/src/index.css` (l. 34, 162, 171), `web/src/suppliers/SuppliersPage.tsx`
  - **Approach**: renommage pur de la classe et de ses variantes responsive ; `.workspace-tag` inchangé.
  - **Done when**: plus aucune règle globale `.workspace` dans `index.css` (`grep -n "^\s*\.workspace\b"`
    vide) ; `suppliers.spec.ts` passe.

- [ ] T3: Renommer la mise en page BOM `.workspace` → `.bom-workspace`
  - **Files**: `web/src/bom/bom.css` (l. 220, 353), `web/src/bom/BomView.tsx`, `web/src/bom/SnapshotView.tsx`
  - **Approach**: nom propre à l'espace, toujours préfixé `.bom-space` ; ajouter `min-width: 0` sur la
    grille pour que `1fr` ne dépende pas du contenu.
  - **Done when**: T1 passe ; vérification manuelle sur `make up` (Multipla) en desktop et mobile.

## Verification

### Automated Checks
- `cd web && npm run lint && npm run build`
- `cd web && npm run test:e2e` (T1 + `suppliers.spec.ts`)
- `make lint && make test` (pas de changement Go attendu, DoD AGENTS §15)

### Manual Checks
- `make up`, http://localhost:8000, ouvrir Fiat Multipla : arbre visible sur toute la largeur, sélection d'un
  nœud ouvre le panneau ; idem sur un snapshot publié.
- Écran Fournisseurs : rendu identique avant/après (sidebar 260 px, centrage, padding), desktop et mobile.

### Acceptance Criteria
- [ ] AC1 (R1) : arbre à pleine largeur à gauche du panneau de 340 px, brouillon et snapshot.
- [ ] AC2 (R2) : aucune classe de mise en page partagée entre `index.css` et `bom.css`.
- [ ] AC3 (R3) : `bom-layout.spec.ts` a échoué avant le correctif et passe après.
