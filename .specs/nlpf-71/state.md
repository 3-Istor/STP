---
id: nlpf-71
status: done
branch: nlpf-71
current_task: null
started: 2026-11-10
updated: 2026-11-10
---

# State: Arbre BOM invisible : conflit CSS .workspace entre BOM et Fournisseurs

## Progress

All tasks completed. Ready for verification.
- Tasks: 3/3 completed
- Branch: `nlpf-71`
- Commits: 7 (3 implementation, 4 tracking, dont ce correctif)

## Task Log

### T1: Test e2e de non-régression : l'arbre BOM a une largeur non nulle
- **Status**: completed
- **Time**: 2026-11-10
- **Commit**: b6f6218
- **Notes**: `web/e2e/bom-layout.spec.ts`, API simulée par `page.route` dans le spec (pas de helper ajouté à
  `fixtures.ts`). Ouvre la BOM (brouillon), mesure l'arbre, clique une ligne → panneau de détail ; puis ouvre
  le snapshot via le lien « snapshot #7 » et refait la même vérification. Échec constaté sur `main` :
  chromium-desktop `Expected: > 334, Received: 2`. chromium-mobile passe déjà (le bug ne se manifeste pas
  en une colonne). Firefox et WebKit non installés localement (`npx playwright install` non lancé) : non exécutés.

### T2: Renommer la mise en page Fournisseurs `.workspace` → `.supplier-workspace`
- **Status**: completed
- **Time**: 2026-11-10
- **Commit**: f3c0468
- **Notes**: `index.css` l. 34, 162, 171 et `SuppliersPage.tsx` renommés ; `.workspace-tag` inchangé.
  Le `grep -n "^\s*\.workspace\b"` du Done when renvoie encore `.workspace-tag` (`\b` matche avant `-`) :
  aucune règle `.workspace` seule ne subsiste. e2e chromium desktop + mobile : 22/22 (suppliers + bom-layout,
  qui passe déjà grâce à ce seul renommage).

### T3: Renommer la mise en page BOM `.workspace` → `.bom-workspace`
- **Status**: completed
- **Time**: 2026-11-10
- **Commit**: 054a8bb
- **Notes**: `bom.css` (règle + variante mobile), `BomView.tsx`, `SnapshotView.tsx`. `min-width: 0` ajouté
  sur la grille, et colonne de l'arbre en `minmax(0, 1fr)`. Vérifié : `npm run lint`, `npm run build`,
  e2e chromium desktop + mobile 22/22. Vérification « manuelle » faite par script Playwright contre la stack
  Docker en cours (Vite dev proxifié vers :8000, lectures seules) : Multipla brouillon, desktop 1440 px →
  arbre 812 px pour une vue de 1168 px, clic sur une ligne → panneau « Caisse en blanc » ; mobile 390 px →
  arbre 358/358 px. Snapshot publié réel non ouvert (couvert seulement par l'e2e simulé). La stack Docker
  elle-même n'a pas été reconstruite (`make up`) : elle sert encore l'ancien build.

## Decisions

- T1 : la largeur de référence est celle de `.view` (moins le panneau s'il est à côté), pas celle de
  `.workspace` comme écrit dans la spec : sous le bug, c'est justement la grille `.workspace` qui se rétracte
  sur son contenu, la mesurer rendrait le test fragile.
- T3 : en plus du `min-width: 0` prévu, `1fr` → `minmax(0, 1fr)` : c'est le minimum de la piste (et non
  celui de la grille) qui fait dépendre `1fr` du contenu ; les deux ensemble rendent l'intention explicite.

## Blockers

None.

## Verification Report(s)

### Exécution spec-execute (2026-11-10)
- `cd web && npm run lint && npm run build` : PASS
- `cd web && npm run test:e2e` : PASS sur chromium-desktop et chromium-mobile (22/22) ; firefox et webkit
  NON EXÉCUTÉS (navigateurs Playwright absents localement : `npx playwright install` à lancer).
- `make lint && make test` : PASS
- CI distante : non vérifiée (rien n'est poussé).

## Verification Report — 2026-11-10

### Overall: PASS

### Task Completion: 3/3

### Commit Verification
T1 `b6f6218`, T2 `f3c0468`, T3 `054a8bb` présents sur `nlpf-71` (+ commits de suivi).

### Automated Checks
| Check | Result | Notes |
|---|---|---|
| `cd web && npm run lint && npm run build` | PASS | oxlint propre, tsc + Vite OK |
| `cd web && npm run test:e2e` (chromium desktop + mobile) | PASS | 26/26 ; firefox/webkit non installés, non exécutés |
| `make lint && make test` | PASS | 0 issues ; tous paquets ok, `cmd/server` compris (résultat en cache) |

### Requirements Validation
| Requirement | Status | Evidence |
|---|---|---|
| R1 arbre pleine largeur | MET | `.bom-workspace` en `minmax(0, 1fr) 340px` + `min-width: 0` ; e2e `bom-layout` vert |
| R2 pas de classe partagée | MET | plus de `.workspace` global ; seul `.workspace-tag` (autre classe) reste |
| R3 e2e échoue si largeur nulle | MET | échec constaté sur `main` (`Received: 2`), vert maintenant |
| S1 variantes mobile | MET | variante `@media` renommée, e2e mobile vert |

### Acceptance Criteria
| Criterion | Result | Evidence |
|---|---|---|
| AC1 arbre pleine largeur, brouillon et snapshot | PASS | e2e (brouillon + snapshot simulé) ; script Playwright sur Multipla réelle : 812 px. Snapshot publié réel non ouvert |
| AC2 aucune classe partagée | PASS | plus de règle `.workspace` seule dans index.css ni bom.css |
| AC3 spec a échoué avant, passe après | PASS | noté dans state.md T1 |

### Code Quality (/simplify)
Non exécuté (revue manuelle du diff : renommages purs, rien relevé).

### Summary
Les 3 tâches sont faites et les vérifications automatiques passent. Réserves : Firefox/WebKit non exécutés, stack Docker non reconstruite, CI distante non vérifiée.
