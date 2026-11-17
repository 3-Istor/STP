---
id: v2-lot-f
title: V2 lot F — intégration front (partie 1 : 3D dans Structurer, budgets, méthode de calcul, cascade)
source: manual
source_ref: docs/v2/plan-implementation.md (vague 3, lot F), maquettes docs/v2/maquettes/I4-1, I4-2, I4-3
created: 2026-11-17
status: in-progress
priority: P1
depends_on: [v2-lot-a, v2-lot-c, v2-lot-d]
branch: v2-lot-f
---

# V2 lot F — intégration front, partie 1

## Context

Les lots A (coût complet, budgets cibles, ventilation), C (shell en 5 étapes, Synthèse) et D (composant 3D) sont
fusionnés sur `v2-lot-f`. Le shell affiche encore des encarts « lot A / lot D » : `Vehicle3DSlot` (« Vue 3D à
venir »), colonne Budget « — », Synthèse sans budget ni méthode, réglages sans méthode, pas de cascade « Pourquoi
ce coût ? ». La partie 1 branche ces données ; la partie 2 (après fusion des lots B et E) branchera versions,
contrôles de publication, analyses et profils.

## Research Findings

### Relevant Files
- `web/src/bom/steps/Structurer.tsx`, `web/src/shell/Vehicle3DSlot.tsx` — bascule Arbre / Arbre + 3D / 3D, slot.
- `web/src/features/vehicle3d/index.tsx` (`Vehicle3D`, lazy) — props `bomId, nodes, selectedNodeId,
  onSelectNode, mode, colorBy, explode` ; `nodes[].budget_status` optionnel (absent → couleurs par système et
  avis ; `null` → gris « indisponible »).
- `web/src/bom/useBomData.ts` — chargement brouillon / snapshot, `run` relit l'arbre après chaque action.
- `web/src/shell/Summary.tsx`, `web/src/bom/BomTree.tsx`, `web/src/bom/BomWorkspace.tsx` (Settings, NodePanel).
- `internal/bom/budgets.go` — `EvaluateTargetBudgets` : état `ok | warn | over | unavailable` par
  `pricing.EvaluateBudget`, **seuil orange = coût ≥ 90 % du budget** (ADR 0018, défaut `warning = "90"`),
  rouge = coût > budget.
- `internal/httpapi/cost_method.go` — `GET/PUT /boms/{id}/cost-method`, `GET /boms/{id}/budgets`,
  `GET /boms/{id}/nodes/{nodeId}/cost-breakdown`, `GET /snapshots/{id}/nodes/{nodeId}/cost-breakdown`.
- `internal/httpapi/boms.go` (`tree`) — l'explication porte `cost_method` et `cost_as_of`.

### Existing Patterns
- Sélection de nœud dans l'URL (`?node=`, `replace`), vue Structurer dans `?tab=` (ADR 0034).
- Montants en chaînes, `formatMoney` / `formatDecimal` (Intl, half-even) ; aucun recalcul front.
- Donnée absente = texte explicite (« incomplet », « n/d »), jamais 0.

### Constraints
- AGENTS §3, §5 ; aucune dépendance ; pas de logique métier côté front (pourcentages et états budget viennent
  de l'API).
- `three` reste dans un chunk à part (import dynamique de `Vehicle3DView`).
- Encarts des lots B et E conservés.
- Budgets : `GET /boms/{id}/budgets` recalcule le coût du brouillon à part ; en prix en direct, l'horloge simulée
  avance entre deux requêtes → un % de budget lu à part peut ne pas correspondre au coût de l'arbre affiché.

## Requirements

### Must Have
- R1 Structurer « Arbre + 3D » et « 3D » : vrai composant `Vehicle3D` ; sélection arbre ↔ 3D par `?node=` ;
  contrôles Rendu (Simple / Réaliste), Colorer (Budget / Système), Éclatement ; focus au double-clic (I4-3).
- R2 Budgets : colonne Budget % de l'arbre, budget et % sous le total, « Hors budget », barres « % du budget » par
  sous-système dans la Synthèse, `budget_status` transmis à la 3D ; couleurs `--ok/--warn/--over` selon l'état
  calculé par le back. Pourcentage calculé par le back (champ ajouté), budgets lus avec l'arbre (même calcul).
- R3 Méthode : la Synthèse affiche `cost_method` et `cost_as_of` ; « Réglages de calcul » change `basis` et `fx`
  (et le volume programme) via `PUT /cost-method`, recalcul avec état « Recalcul en cours… » ; emplacement
  « Profil de véhicule » laissé au lot E.
- R4 « Pourquoi ce coût ? » dans le Détail (brouillon et snapshot) : lignes par nature avec libellé, source, date
  et montant, total ; une composante de la méthode sans ligne est affichée « absente », une erreur est affichée.
- R5 Encarts « lot A / lot D » retirés.

### Should Have
- S1 e2e « clic dans l'arbre → sélection 3D » (et 3D → arbre si réalisable avec WebGL logiciel).

### Won't Do
- Versions, contrôles de publication, comparaison (lot B) ; analyses, profils (lot E) : partie 2.
- Budgets d'un snapshot (non figés par le lot A) : affichés « non disponibles ».
- Héritage du statut budget d'un ancêtre par la 3D (un sous-ensemble sans budget propre reste gris).

## Tasks

- [x] T1: Ticket
  - **Files**: `.specs/v2-lot-f/spec.md`, `.specs/v2-lot-f/state.md`
  - **Approach**: lecture AGENTS, plan, maquettes, états des lots A/C/D, ADR 0031/0034/0035, `web/src`.
  - **Done when**: spec et état commités.
- [x] T2: Budgets dans l'arbre et pourcentage (back)
  - **Files**: `internal/bom/budgets.go`, `internal/bom/service.go`, `internal/httpapi/cost_method.go`,
    `internal/httpapi/boms.go`, `boms_json.go`, tests
  - **Approach**: `BudgetStatus.Percent` (coût ÷ budget × 100, arrondi half-even à 1 décimale, nil si
    indisponible ou budget nul) ; `Service.Totals` évalue les budgets sur les mêmes totaux ; `budgets` dans
    `GET /tree` (champ ajouté) et `percent` dans `GET /budgets`.
  - **Done when**: tests table-driven du pourcentage et test HTTP du champ verts.
- [x] T3: Budgets dans le front
  - **Files**: `web/src/api.ts`, `bom/useBomData.ts`, `bom/BomTree.tsx`, `shell/Summary.tsx`, CSS
  - **Done when**: colonne Budget %, budget sous le total, Hors budget, barres % du budget, couleurs par état.
- [x] T4: 3D dans Structurer
  - **Files**: `bom/steps/Structurer.tsx`, `shell/Vehicle3DSlot.tsx` (supprimé), `features/vehicle3d/*`, CSS
  - **Done when**: sélection synchronisée, contrôles et focus, chunk `three` séparé au build.
- [x] T5: Méthode de calcul
  - **Files**: `api.ts`, `shell/Summary.tsx`, `bom/BomWorkspace.tsx` (Settings)
  - **Done when**: méthode et date affichées ; changement → PUT puis relecture, état de recalcul.
- [x] T6: « Pourquoi ce coût ? »
  - **Files**: `bom/CostBreakdown.tsx`, `bom/NodePanel.tsx`, `bom/steps/Chiffrer.tsx`
  - **Done when**: cascade brouillon et snapshot, lignes absentes et erreurs explicites.
- [x] T7: Tests front
  - **Files**: `web/e2e/*.spec.ts`, tests unitaires
  - **Done when**: e2e existants à jour, nouveaux e2e (budgets, méthode, cascade, arbre → 3D) verts.
- [x] T8: Vérification réelle et captures
  - **Files**: `docs/v2/captures/lot-f/*`, `.specs/v2-lot-f/state.md`
  - **Done when**: appli lancée sur la base seedée, captures clair / sombre relues, contrôles du plan verts.

### Partie 2 (après fusion des lots B et E)

Contrats réels : `.specs/v2-lot-b/state.md` (versions, compare, publication-checks, ideas, publication tracée),
`.specs/v2-lot-e/state.md` (fx-sensitivity, price-drivers, sourcing, analysis/anomalies, profils), ADR 0032,
0033, 0036, 0037. Seed : V1 publiée (snapshot 1), brouillon A (BOM 1), brouillon B (BOM 3), 5 pistes.

- [ ] T9: Client API des lots B et E et spec de la partie 2
  - **Files**: `web/src/api.ts`, `.specs/v2-lot-f/*`
  - **Done when**: types et appels pour toutes les routes ci-dessus, `tsc` vert.
- [ ] T10: Toutes les BOM avec versions (`GET /boms/{id}/versions`) et aperçu
  - **Files**: `web/src/home/AllBoms.tsx`
  - **Done when**: versions sous chaque BOM (date à la seconde, coût, % budget, prix, proposé/validé par), aperçu.
- [ ] T11: Chiffrer : sensibilité (choc 5/10 %), pièces qui pèsent, anomalies €/kg par impact ; existant plus discret
  - **Files**: `web/src/bom/steps/Chiffrer.tsx`, `web/src/bom/analysis/*`
- [ ] T12: Sourcer › Vue d'ensemble (`analysis/sourcing`)
  - **Files**: `web/src/bom/steps/Sourcer.tsx`, `web/src/bom/analysis/*`
- [ ] T13: Profil de véhicule dans les réglages ; classement des alternatives selon le profil
  - **Files**: `web/src/bom/ProfileSettings.tsx`, `web/src/bom/StrategyPanel.tsx`
- [ ] T14: Arbitrer : comparaison, causes triées, pièces modifiées, pistes de réduction
  - **Files**: `web/src/bom/steps/Arbitrer.tsx`, `web/src/bom/Ideas.tsx`
- [ ] T15: Publier : contrôles, justifications, versions, note et noms, publication 409/422
  - **Files**: `web/src/bom/steps/Publier.tsx`
- [ ] T16: Badges des étapes alimentés par les données (ou retirés)
  - **Files**: `web/src/App.tsx`
- [ ] T17: Libellés de cascade arrondis à l'affichage ; fond de la 3D selon le thème
  - **Files**: `web/src/bom/CostBreakdown.tsx`, `web/src/features/vehicle3d/*`, `web/src/ui/tokens.css`
- [ ] T18: e2e (comparaison V1 → B et causes, publication refusée sur B, profil → classement), vérification
  réelle, captures clair/sombre, état
  - **Files**: `web/e2e/*`, `docs/v2/captures/lot-f/*`, `.specs/v2-lot-f/state.md`

Règles : aucun chiffre inventé (tout vient de l'API, montants exacts arrondis à l'affichage) ; encarts « lot B/E »
remplacés ; pas de logique métier côté front (écarts, sommes, classements : API).

## Verification

### Automated Checks
- `go vet ./...`, `go tool golangci-lint run ./...`, `go test ./...` (avec `DATABASE_URL`)
- `cd web && npm ci && npm run lint && npx tsc -b && npx vite build && npm run test:unit`
- Playwright `chromium-desktop` / `chromium-mobile` (Chrome système)

### Manual Checks
- Multipla seedée : Arbre + 3D Réaliste coloré par budget, clic arbre → cadre 3D, clic 3D → ligne, double-clic
  roue → focus ; Réglages : Complet → Achat → Complet ; cascade de « Porte avant nue ».

### Acceptance Criteria
- [x] AC1 (R1) : clic dans l'arbre → objet 3D sélectionné ; clic 3D → `?node=` et ligne de l'arbre.
- [x] AC2 (R2) : % et états identiques à l'API, aucun calcul front ; budget absent → « — ».
- [x] AC3 (R3) : le total change avec la méthode, la Synthèse montre la méthode et la date.
- [x] AC4 (R4) : cascade = lignes de l'API, total de l'API ; erreur et absence visibles, jamais 0.
- [x] AC5 : `three` absent du chunk d'entrée.
