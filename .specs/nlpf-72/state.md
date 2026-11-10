---
id: nlpf-72
status: in-progress
branch: nlpf-71
current_task: T3
started: 2026-11-10
updated: 2026-11-10
---

# State: Le sélecteur d'offres de la BOM affiche toujours « sans prix »

## Progress

Execution in progress on branch `nlpf-71` (choix utilisateur : même branche que NLPF-71).

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

## Decisions

- 2026-11-10 — Source du prix du sélecteur : option 2, même source que les totaux (direct ou snapshot de
  prix), via les endpoints existants ; aucun changement de contrat `/api/v1` (choix humain : Arthur Presle).
- 2026-11-10 — Exécution sur la branche `nlpf-71` et non `nlpf-72` (demande explicite de l'utilisateur).
- 2026-11-10 — T1 : `loadOfferPrices` renvoie `{ byOffer: Map<number, Money>, source: string | null }` au lieu d'une `Map` seule, pour que le survol (S1) dise la provenance réelle (direct, snapshot, repli `snapshot_fallback`).

## Blockers

None.

## Verification Report(s)
