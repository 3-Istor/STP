---
id: nlpf-71
status: in-progress
branch: nlpf-71
current_task: T3
started: 2026-11-10
updated: 2026-11-10
---

# State: Arbre BOM invisible : conflit CSS .workspace entre BOM et Fournisseurs

## Progress

Execution in progress on `nlpf-71` (T1, T2 done).

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

## Decisions

- T1 : la largeur de référence est celle de `.view` (moins le panneau s'il est à côté), pas celle de
  `.workspace` comme écrit dans la spec : sous le bug, c'est justement la grille `.workspace` qui se rétracte
  sur son contenu, la mesurer rendrait le test fragile.

## Blockers

None.

## Verification Report(s)
