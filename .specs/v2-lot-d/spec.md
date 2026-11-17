---
id: v2-lot-d
title: V2 lot D — vue 3D par sous-ensemble (rendus Simple et Réaliste)
source: manual
source_ref: docs/v2/plan-implementation.md (lot D), docs/v2/vision-cible.md §6
created: 2026-11-17
status: in-progress
priority: P2
depends_on: []
branch: v2-lot-d
---

# V2 lot D — vue 3D par sous-ensemble

## Context

La vision V2 (`docs/v2/vision-cible.md` §6, validée le 17/11/2026) ajoute une vue 3D qui sert à **localiser** :
clic sur une pièce → nœud de BOM, sélection dans l'arbre → mise en évidence, coloration par statut budget,
éclaté, focus « roue avant gauche ». Deux rendus sont retenus : **Simple** (formes générées, défaut) et
**Réaliste** (modèle Khronos « Car Concept », CC-BY 4.0, en option). Les prototypes jetables validés sont dans
`docs/v2/prototype-3d/` (`car.html`, `car-realiste.html`). Maquettes : `docs/v2/maquettes/I4-2-Arbre3D.png`,
`I4-3-FocusRoue.png`. Le shell (lot C, en parallèle) intègre le composant ; ce lot livre le composant isolé et
une page de démo.

## Research Findings

### Relevant Files
- `docs/v2/prototype-3d/car.html` — rendu Simple : géométries par sous-ensemble, coordonnées (x = longueur,
  avant à −2 m ; z < 0 = gauche), vecteurs d'éclaté saisis à la main.
- `docs/v2/prototype-3d/car-realiste.html` — rendu Réaliste : table regex nœud glTF → sous-ensemble, logos
  masqués (`License Plate`, `InteriorSteeringEmblem`, texture `Tireside`), peinture neutre, éclaté calculé
  depuis les boîtes englobantes, application monde parent → enfant.
- `web/src/api.ts` — `TreeNode {id, parent_id, name, part_id, …, totals}` ; **aucun statut budget** par nœud
  aujourd'hui (ADR 0018 Proposé ; lot A/E l'apporteront peut-être).
- `internal/sampledata/multipla.go`, `valhalla.go` — noms réels des sous-ensembles : niveau 1 = systèmes
  (« Caisse en blanc », « Liaison au sol »…), niveau 2 = sous-ensembles (« Porte avant gauche », « Roue avant
  gauche », « Train avant gauche »…).
- `CarConcept.glb` (11,8 Mo) — 101 nœuds ; les pneus sont des maillages sans nom sous `WheelFrontL` etc.

### Existing Patterns
- React 19 + Vite 8, `tsc -b`, `oxlint` ; pas d'infrastructure de test unitaire front (seulement Playwright).
- Node 22 exécute le TypeScript « effaçable » nativement (`process.features.typescript = strip`) :
  `node --test` suffit pour la logique pure, sans dépendance.
- Variables CSS `--ok`, `--warn`, `--over` dans `web/src/index.css`.

### Constraints
- Une seule dépendance runtime ajoutée : `three` (+ `@types/three` en dev) — ADR 0035 (Proposé).
- `three` hors du bundle principal : `React.lazy` + import dynamique.
- Ne pas toucher au shell (`App.tsx`, `web/src/bom/*`).
- Le modèle Khronos n'est pas commité (`.gitignore`), il est téléchargé par `make models`.
- Aucun montant manipulé : la 3D ne lit qu'un statut budget déjà calculé (`budget_status`).

## Requirements

### Must Have
- `web/src/features/vehicle3d/` exporte `Vehicle3D` chargé à la demande, props
  `{bomId, nodes, selectedNodeId, onSelectNode, mode, colorBy, explode}`.
- Correspondance BOM ↔ 3D par **emplacement** (slot) partagé par les deux rendus : table nom de
  sous-ensemble → slot (`data/slots.json`), table slot → forme Simple (`data/simple-layout.json`), table
  nœud glTF → slot (`data/realistic-map.json`).
- Simple : un objet par sous-ensemble de niveau 2 (ou de niveau 1 sans sous-ensemble enfant) ; nom inconnu →
  objet générique placé dans la zone de son système, marqué « position estimée ».
- Réaliste : `/models/CarConcept.glb` s'il est présent ; logos masqués, peinture neutre, attribution
  affichée, badge « SIMILAIRE ». Fichier absent ou illisible → repli Simple + message.
- OrbitControls ; clic → `onSelectNode(id BOM)` ; sélection externe (n'importe quel nœud, remonté à son
  sous-ensemble ou système) → surbrillance + cadrage ; coloration budget (`budget_status`) sinon système,
  avec avis ; éclaté calculé depuis les boîtes englobantes ; focus par double-clic (reste transparent,
  éclatement dans l'axe, avec les slots associés : roue + train) ; bouton recentrer.
- Rendu à la demande, libération des ressources au démontage, instancing (sièges, roues).
- `make models` ; cible ignorée par git ; compression meshopt si `gltfpack` est installé, sinon tel quel.

### Should Have
- Infobulle au survol (nom du sous-ensemble).
- Page de démo `web/vehicle3d-demo.html` (BOM réelle via l'API ou fichier de nœuds exporté).

### Won't Do
- Rendu « Technique » (`car-technique.html`) — seulement s'il reste du temps (vision §6).
- Calcul du statut budget côté front (règle ADR 0018, appartient au back).
- Post-traitement GTAO du prototype (trop coûteux en WebGL logiciel, gain faible).
- Intégration dans le shell (lot C / vague 3).

## Tasks

- [ ] **T1 — Ticket et ADR**. Files: `.specs/v2-lot-d/*`, `docs/adr/0035-vue-3d-par-sous-ensemble.md`.
  Approach: spec, état, ADR Proposé. Done when: fichiers commités.
- [ ] **T2 — Dépendance `three`**. Files: `web/package.json`, `web/package-lock.json`.
  Approach: `npx npm@11 install three` et `-D @types/three`. Done when: `npm ci` passe.
- [ ] **T3 — Logique pure et tables**. Files: `features/vehicle3d/{mapping,explode,colors}.ts`, `data/*.json`,
  tests `*.test.ts`, script `test:unit`. Approach: fonctions sans `three`, testées par `node --test`
  (dont la couverture des noms réels Multipla). Done when: tests verts et capables d'échouer.
- [ ] **T4 — Moteur 3D et rendu Simple**. Files: `viewer.ts`, `simpleModel.ts`, `Vehicle3DView.tsx`,
  `index.ts`, `vehicle3d.css`. Done when: `tsc -b` et lint passent.
- [ ] **T5 — Rendu Réaliste et `make models`**. Files: `realisticModel.ts`, `Makefile`, `.gitignore`.
  Done when: modèle chargé, repli vérifié sans fichier.
- [ ] **T6 — Page de démo et captures**. Files: `web/vehicle3d-demo.html`, `features/vehicle3d/demo/*`,
  `docs/v2/captures/lot-d/*.png`. Done when: captures regardées et commitées.
- [ ] **T7 — Vérification et livrable §9**. Files: `state.md`. Done when: lint, tsc, build (chunk séparé),
  `go test ./...` verts.

## Verification

### Automated Checks
- `cd web && npm ci && npm run lint && npx tsc -b && npx vite build && npm run test:unit`
- `go test ./...` (avec `DATABASE_URL`).

### Manual Checks
- Captures Playwright (Chrome, WebGL logiciel) : Simple assemblé, éclaté, focus, sélection ; Réaliste
  assemblé, éclaté, focus roue ; repli sans modèle.

### Acceptance Criteria
- `three` n'apparaît pas dans le chunk d'entrée.
- Clic sur un objet → callback avec l'id du nœud BOM ; sélection d'une pièce profonde → son sous-ensemble.
- Aucun sous-ensemble Multipla de niveau 2 en « position estimée ».
