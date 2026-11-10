---
id: nlpf-72
status: review
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
