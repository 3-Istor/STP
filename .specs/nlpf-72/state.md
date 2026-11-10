---
id: nlpf-72
status: done
branch: nlpf-71
current_task: null
started: 2026-11-10
updated: 2026-11-10
---

# State: Le sélecteur d'offres de la BOM affiche toujours « sans prix »

## Progress

All tasks completed. Ready for verification.
- Tasks: 3/3 completed
- Branch: `nlpf-71`
- Commits: 7

## Task Log

### T1: Lecture des prix par offre, source directe ou snapshot
- **Status**: completed
- **Time**: 2026-11-10
- **Commit**: faaa927
- **Notes**: Nouveau `web/src/offerPrices.ts` : `loadOfferPrices(priceSnapshotID)` lit `GET /api/v1/quotes` ou `GET /api/v1/price-snapshots/{id}` ; seules les entrées cotées entrent dans la map. Renvoie `{ byOffer, source }` (et non une simple `Map`) pour porter la provenance (S1). `withOfferPrices` fusionne prix et provenance dans le catalogue ; `Offer` gagne `price_source`.

### T2: Brancher les prix dans les sélecteurs de l'arbre et du panneau
- **Status**: completed
- **Time**: 2026-11-10
- **Commit**: 40498a1
- **Notes**: `BomView` charge les prix pour `bom.price_snapshot_id` (relu après figer/défiger), les garde associés à ce `price_snapshot_id` (jamais affichés pour une autre source) et passe un catalogue enrichi (`withOfferPrices`) à `BomTree` et `NodePanel`. Les `<option>` ont `title` = provenance (S1). Échec de lecture → « sans prix ». Vérification manuelle sur Multipla seedée non faite (stack non lancée) ; couverte par l'e2e T3 sur données simulées.

### T3: Test e2e du sélecteur
- **Status**: completed
- **Time**: 2026-11-10
- **Commit**: c78d5fc
- **Notes**: `web/e2e/bom-offer-price.spec.ts` : BOM en direct (offre EUR cotée, offre USD cotée, offre en erreur → « sans prix », `title` « en direct au … ») et BOM figée (prix du snapshot #5, offres non couvertes → « sans prix », `/api/v1/quotes` non lu). Vérifié : échoue sur chromium-desktop sans les changements T2, passe avec. Lancé en local sur chromium uniquement (Firefox/WebKit non installés sur le poste ; la CI les installe).

## Decisions

- 2026-11-10 — Source du prix du sélecteur : option 2, même source que les totaux (direct ou snapshot de
  prix), via les endpoints existants ; aucun changement de contrat `/api/v1` (choix humain : Arthur Presle).
- 2026-11-10 — Exécution sur la branche `nlpf-71` et non `nlpf-72` (demande explicite de l'utilisateur).
- 2026-11-10 — T1 : `loadOfferPrices` renvoie `{ byOffer: Map<number, Money>, source: string | null }` au lieu d'une `Map` seule, pour que le survol (S1) dise la provenance réelle (direct, snapshot, repli `snapshot_fallback`).

## Blockers

None.

## Verification Report(s)

## Verification Report — 2026-11-10

### Overall: PASS

### Task Completion: 3/3

### Commit Verification
T1 `faaa927`, T2 `40498a1`, T3 `c78d5fc` présents (sur `nlpf-71` et non `nlpf-72`, décision tracée).

### Automated Checks
| Check | Result | Notes |
|---|---|---|
| `cd web && npm run lint && npm run build` | PASS | oxlint propre, tsc + Vite OK |
| `cd web && npm run test:e2e` (chromium desktop + mobile) | PASS | 26/26 ; firefox/webkit non installés, non exécutés |
| `make lint && make test` | PASS | 0 issues ; tous paquets ok, `cmd/server` compris (résultat en cache) |

### Requirements Validation
| Requirement | Status | Evidence |
|---|---|---|
| R1 prix en devise d'origine | MET | `loadOfferPrices` + `withOfferPrices` ; e2e offres EUR et USD |
| R2 « sans prix » si pas de cotation / erreur | MET | seules les cotations non nulles entrent dans la map ; e2e |
| R3 montants en chaînes | MET | aucun `Number(`/`parseFloat` dans le diff de `offerPrices.ts`, `catalog.ts`, `BomView.tsx` |
| R4 même source que les totaux | MET | `BomView` charge selon `bom.price_snapshot_id`, prix liés à leur source ; e2e BOM figée sans lecture de `/quotes` |
| S1 `title` = source | MET | e2e vérifie « en direct au … » |

### Acceptance Criteria
| Criterion | Result | Evidence |
|---|---|---|
| AC1 offre cotée avec sa devise | PASS | e2e `bom-offer-price` |
| AC2 offre sans cotation | PASS | e2e |
| AC3 pas de `number` pour un montant | PASS | grep vide sur le diff |
| AC4 figée = snapshot, direct = cotation | PASS | deux scénarios e2e ; vérification manuelle sur Multipla seedée non faite |

### Code Quality (/simplify)
Non exécuté (revue manuelle : la garde `snapshotID` évite d'afficher les prix d'une autre source).

### Summary
Conforme à l'option 2 choisie, sans changement de contrat d'API. Réserves : test manuel sur la stack réelle non fait, Firefox/WebKit non exécutés, CI non vérifiée.
