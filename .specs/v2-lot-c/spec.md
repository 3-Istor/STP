---
id: v2-lot-c
title: V2 lot C — shell front en 5 étapes (navigation, synthèse, thème)
source: manual
source_ref: docs/v2/plan-implementation.md (lot C), docs/v2/vision-cible.md §4, maquettes docs/v2/maquettes/I4-*
created: 2026-11-17
status: review
priority: P1
depends_on: []
branch: v2-lot-c
---

# V2 lot C — shell front en 5 étapes

## Context

La V1 sépare deux espaces (BOM / Fournisseurs) qui n'ont ni la même mise en page ni le même style.
La V2 (vision validée le 17/11/2026) organise le travail en 5 étapes à la Lightroom — Structurer, Chiffrer,
Sourcer, Arbitrer, Publier — avec la même sélection partout, une colonne « Synthèse » commune et une barre
d'état. Ce lot pose le shell front ; les contenus nouveaux arrivent par les lots A, B, D et E.
Règle du lot : **aucune fonction existante ne disparaît, elle change de place**.

## Research Findings

### Relevant Files
- `web/src/App.tsx` — deux espaces par ancre (`#boms`, `#fournisseurs`).
- `web/src/bom/BomPage.tsx`, `BomList.tsx`, `BomView.tsx`, `SnapshotView.tsx` — liste, brouillon, snapshot.
- `web/src/bom/BomTree.tsx` (+ `flatten.ts`) — arbre virtualisé maison (NLPF-69), sélecteur d'offre en ligne.
- `web/src/bom/NodePanel.tsx` — détail du nœud + stratégie, make-or-buy, coût rendu, stock, édition.
- `web/src/bom/PriceSource.tsx` — choix du snapshot de prix (PR #23), « Figer maintenant ».
- `web/src/bom/{PlanningPanel,Workforce,Anomalies,Totals}.tsx` — planning, ETP, anomalies, totaux.
- `web/src/suppliers/*` — fournisseurs, offres (cartes), contrats, historique (dialogue), création.
- `web/src/index.css`, `web/src/bom/bom.css` — deux feuilles, couleurs en dur, pas de thème sombre.
- `web/e2e/*.spec.ts` — 6 suites Playwright sur API simulée (`page.route`).
- `docs/v2/maquettes/I4-*.dc.html` — CSS de référence (barre sombre, `.col` 360 px, `.status`, `.sys`, `.snap`).

### Existing Patterns
- Montants en chaînes, `formatMoney` (Intl, half-even) ; graphe de prix en BigInt (`suppliers/graph.ts`).
- Fenêtres de l'app (`useAppDialog`), jamais `window.confirm`.
- Chaque action relit l'arbre (`run` → `api.tree`), aucun recalcul front.

### Constraints
- Aucune dépendance npm, aucun changement d'API backend (manques notés dans state.md).
- Pas de budget, pas de liste des snapshots publiés, pas de méthode de coût dans l'API actuelle :
  rien n'est inventé, des encarts « lot A/B/E » le disent.
- ADR 0034 réservée (navigation 5 étapes, routeur maison, thème), statut Proposé.

## Requirements

### Must Have
- R1 Barre du haut sombre : logo, fil d'Ariane « Toutes les BOM / nom · brouillon|snapshot », 5 étapes.
- R2 Écran « Toutes les BOM » : tableau des BOM, snapshots connus en dessous (dates à la seconde) ; ouvrir,
  créer, renommer, supprimer, nouveau brouillon depuis un snapshot.
- R3 Structurer : arbre virtualisé en grand (Qté, Offre retenue, Coût/véh., Budget %, CO₂, Délai), ajouter
  avec choix de l'offre, déplacer, supprimer, changer d'offre ; bascule Arbre / Arbre + 3D / 3D avec
  `Vehicle3DSlot` (`bomId`, `nodes`, `selectedNodeId`, `onSelectNode`) affichant « Vue 3D à venir ».
- R4 Chiffrer / Arbitrer / Publier : pages minimales regroupant les fonctions existantes ; encarts pour le reste.
- R5 Sourcer : « Vue d'ensemble » minimale ; « Fournisseurs, offres et contrats » en tables denses avec toutes
  les fonctions actuelles et l'historique des prix lisible avec choix de période.
- R6 Colonne « Synthèse » identique sur toutes les étapes d'une BOM : état, total, budget, disponibilité, CO₂,
  masse, hors budget, barres par sous-système, prix utilisés sélectionnables (dont « en direct »), méthode si
  l'API la donne ; dessous onglets Détail / Réglages de calcul (placeholder lot E).
- R7 Barre d'état : bouton thème clair/sombre (localStorage protégé, `prefers-color-scheme` par défaut),
  nombre de nœuds, état de l'API de prix, devise et volume.
- R8 Une seule feuille de tokens (`web/src/ui/tokens.css`) : `--ok --warn --over`, thème sombre par variables.
- R9 Routeur maison (hash) avec URL profondes `#/boms/1/structurer?node=8437` ; sélection dans l'URL,
  partagée entre étapes ; testé.

### Should Have
- S1 Police IBM Plex Sans si installée, repli système, chiffres tabulaires.
- S2 Anciennes ancres `#boms`, `#fournisseurs` redirigées.

### Won't Do
- Contenus des lots A/B/D/E (coût complet, comparaison, contrôles, 3D, analyses, profils).
- Annuler / rétablir, choix des colonnes (absents de l'existant).
- Nouvel endpoint (liste des snapshots publiés, budgets) : noté pour les lots B/E.

## Tasks

- [x] T1: Feuille de tokens, thème clair/sombre
  - **Files**: `web/src/ui/tokens.css`, `web/src/ui/theme.ts`, `web/src/index.css`, `web/src/bom/bom.css`, `web/src/main.tsx`
  - **Approach**: variables sur `:root` / `[data-theme=dark]` ; les deux feuilles existantes ne gardent que des `var()`.
  - **Done when**: aucune couleur en dur hors tokens.css ; le thème suit la préférence système puis le choix enregistré.
- [x] T2: Routeur maison
  - **Files**: `web/src/router.ts`, `web/e2e/router.spec.ts`
  - **Approach**: `parseRoute(hash)` / `formatRoute(route)` purs + `useRoute()` sur `hashchange`.
  - **Done when**: tests aller-retour et anciennes ancres verts.
- [x] T3: Shell, Toutes les BOM, Structurer, Synthèse, barre d'état, étapes Chiffrer/Arbitrer/Publier
  - **Files**: `web/src/App.tsx`, `web/src/shell/*`, `web/src/bom/*`
  - **Approach**: `useBomData` (brouillon ou snapshot) partagé par les étapes ; `NodePanel` découpé en détail
    (colonne de droite) et panneaux d'étape (coût rendu, make-or-buy → Chiffrer ; stratégie → Arbitrer).
  - **Done when**: chaque fonction existante a une place (carte dans state.md).
- [x] T4: Sourcer refondu
  - **Files**: `web/src/suppliers/*`
  - **Approach**: tables denses façon I4-5b ; carte d'historique avec période (1 mois, 3 mois, 6 mois, tout) ;
    le dialogue d'historique détaillé reste accessible.
  - **Done when**: création fournisseur, offre, contrat, modification de contrat, historique utilisables.
- [x] T5: e2e mis à jour sans affaiblir les vérifications ; nouvelles vérifications du shell
- [x] T6: ADR 0034 (Proposé)
- [x] T7: Vérification réelle, captures clair/sombre dans `docs/v2/captures/lot-c/`, state.md, livrable §9

## Verification

### Automated Checks
- `cd web && npm ci && npm run lint && npx tsc -b && npx vite build`
- `cd web && npx playwright test` (chromium, firefox, webkit en CI)
- `go vet ./... && go test ./...` (avec `DATABASE_URL`)

### Manual Checks
- Serveur réel (seed demo-v3) : parcours des 5 étapes, deep link `#/boms/1/structurer?node=<id>`, bascule de
  thème rechargée, captures clair et sombre de chaque écran.

### Acceptance Criteria
- [x] AC1 (R1, R6, R7) : shell identique sur les 5 étapes, captures à l'appui.
- [x] AC2 (R2–R5) : carte « fonction existante → nouvelle place » complète, aucune fonction perdue.
- [x] AC3 (R8, R9) : tokens uniques, thème persistant, routeur testé.
