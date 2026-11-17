---
id: v2-lot-d
status: review
branch: v2-lot-d
current_task: null
started: 2026-11-17
updated: 2026-11-17
---

# State: V2 lot D — vue 3D par sous-ensemble

## Progress

All tasks completed. Ready for verification (relecture par un agent à contexte vierge, puis humaine).
- Tasks: 7/7 completed
- Branch: `v2-lot-d`
- Commits: 8 (T1, T2, T3, T4, T5, T6 ×2, T7)

## Task Log

### T1: Ticket et ADR
- **Status**: completed
- **Time**: 2026-11-17
- **Notes**: spec, état, ADR 0035 au statut Proposé (décideur à compléter par un humain).

### T2: Dépendance `three`
- **Status**: completed
- **Time**: 2026-11-17
- **Notes**: `three@^0.187.0`, `@types/three@^0.187.0` (dev), via `npx npm@11 install`.

### T3: Logique pure et tables
- **Status**: completed
- **Time**: 2026-11-17
- **Notes**: `mapping.ts`, `explode.ts`, `colors.ts` + 3 fichiers de tests (16 tests). Pas d'infrastructure de
  test unitaire front : `node --test` exécute le TypeScript effaçable nativement (Node 22.18+, CI en Node 24),
  aucune dépendance ; `tsconfig.unit.json` vérifie les tests avec les types Node ; étape CI ajoutée.
  Mutation manuelle (coefficient latéral, remontée au sous-ensemble) → 2 tests rouges, puis rétabli.

### T4: Moteur 3D et rendu Simple
- **Status**: completed
- **Time**: 2026-11-17
- **Notes**: repère corrigé par rapport au prototype (gauche = +z quand l'avant est vers -x).

### T5: Rendu Réaliste et `make models`
- **Status**: completed
- **Time**: 2026-11-17
- **Notes**: téléchargement réussi (11,8 Mo). `gltfpack` absent de la machine → copie sans compression
  (branche gltfpack non exécutée). Orientation vérifiée : `WheelFrontL` côté +z = gauche.

### T6: Page de démo et captures
- **Status**: completed
- **Time**: 2026-11-17
- **Notes**: 11 captures dans `docs/v2/captures/lot-d/`. Correction après relecture des captures : la
  surbrillance émissive rendait rose une pièce rouge → cadre de sélection.

### T7: Vérification et livrable §9
- **Status**: completed
- **Time**: 2026-11-17
- **Notes**: voir rapport ci-dessous.

## Decisions

- **Slot (emplacement) comme pivot** entre BOM et 3D : une table nom → slot partagée par les deux rendus
  (ADR 0035, vision §6 « les trois partagent la même correspondance »).
- **Statut budget** : lu sur un champ optionnel `budget_status` des nœuds ; l'API actuelle ne le fournit pas
  → coloration par système avec un avis visible. Aucun calcul budget côté front (ADR 0018 appartient au back).
  La démo propose des statuts **fictifs**, étiquetés, pour voir la coloration.
- **« ok » adouci** (70 % vers le gris carrosserie) ; `warn`/`over` = variables CSS exactes ; absent = gris
  « indisponible », jamais vert.
- **Sélection** = cadre bleu (pas de teinte) ; un clic dans la 3D ne recadre pas la caméra, une sélection
  externe (arbre) recadre.
- **Focus** : la roue s'éclate avec son train (`focusWith`), ordre intérieur → extérieur par rang `axial`
  déclaré dans les tables (ressort, frein, jante, pneu).
- **Pas de GTAO** (post-traitement du prototype) : coût élevé, gain faible.
- **Page de démo hors build de production** (seul `index.html` est construit) ; accès `__viewer` réservé au dev.

## Blockers

Aucun.

## Verification Reports

### 2026-11-17 — T7
- `cd web && npm ci` : PASS (0 vulnérabilité).
- `npm run lint` (oxlint) : PASS, 0 avertissement.
- `npx tsc -b` (app, node, unit) : PASS.
- `npm run test:unit` : PASS, 16/16.
- `npx vite build` : PASS ; build de production inchangé (la 3D n'est pas encore importée par le shell).
  Build de contrôle avec la page de démo comme entrée supplémentaire (config temporaire, non commitée) :
  `three` uniquement dans `Vehicle3DView-*.js` (677 kB, 172 kB gzip), `GLTFLoader-*.js` 46 kB,
  `meshopt_decoder-*.js` 26 kB ; entrée `app-*.js` (94 kB) et `demo-*.js` sans `three`.
  Vite avertira (> 500 kB) quand le shell importera le composant : chunk chargé à la demande, à arbitrer.
- `npm audit --omit=dev --audit-level=high` : PASS.
- `go vet ./...`, `go test ./...` (base `stp_lot_d`) : PASS (aucun code Go modifié).
- `make models` : PASS (réseau autorisé, gltfpack absent → non compressé) ; 2e lancement → « déjà présent ».
- Playwright (Chrome, SwiftShader) sur l'API réelle (seed demo-v3, ports 8083/8093/5183) : clic réel sur la
  porte → `onSelectNode(8437)` ; double-clic réel sur la roue (Réaliste) → focus `wheel_fl` ; sélection
  depuis la liste → cadre + recadrage ; repli sans .glb → Simple + message ; 0 image rendue au repos
  (compteur stable 1,5 s) ; 3 allers-retours Simple ↔ Réaliste : géométries 69 ↔ 108, textures 5 ↔ 17,
  stables (pas de fuite).
- Non exécuté : tests e2e Playwright existants (`npm run test:e2e`, shell non modifié) ; `golangci-lint`
  (aucun code Go modifié) ; démontage du composant (vérifié par revue de code de `Viewer.dispose`, pas par test).
