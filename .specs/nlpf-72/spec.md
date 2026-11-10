---
id: nlpf-72
title: Le sélecteur d'offres de la BOM affiche toujours « sans prix »
source: jira
source_ref: https://3istors.atlassian.net/browse/NLPF-72
created: 2026-11-10
status: in-progress
priority: P2
depends_on: []
branch: nlpf-72
---

# Le sélecteur d'offres de la BOM affiche toujours « sans prix »

## Context

Dans l'arbre BOM et dans le panneau d'ajout de pièce, chaque offre s'affiche « <fournisseur> · sans prix ·
N j » alors que la cotation existe. `fromCatalog` (`web/src/catalog.ts`) met `price: null` en dur, car
`GET /api/v1/parts` ne porte pas de prix. L'écran annonce une donnée manquante qui existe, ce qui contredit
AGENTS §3 (« une donnée manquante n'est jamais cachée ») dans l'autre sens.

## Research Findings

### Relevant Files
- `web/src/catalog.ts` — type `Offer.price: Money | null` existe déjà ; `fromCatalog` force `null` (l. 55).
- `web/src/bom/BomTree.tsx:105`, `web/src/bom/NodePanel.tsx:234` — affichage `formatMoney(o.price)` ou
  « sans prix » : rien à changer si `price` est renseigné.
- `internal/httpapi/pricing.go:65-91` — **endpoints existants** `GET /api/v1/quotes`,
  `/quotes/offers/{id}`, `/quotes/suppliers/{id}` (option `as_of`) → `pricing.PricePage`
  `{context, quotes: [{offer_id, quote, error}], coverage_complete}`.
- `internal/httpapi/pricing_snapshots.go:33` — `GET /api/v1/price-snapshots/{id}` (prix figés).
- `internal/bom/totals.go:64,100` — `PriceSnapshotID` (nil = en direct) : la BOM sait déjà d'où viennent
  ses prix ; `totals.quote` en dépend.
- `web/src/money.ts` — `Money` en chaînes + `formatMoney` (`Intl.NumberFormat`).

### Existing Patterns
- Le front compose plusieurs lectures en parallèle (`loadCatalog` : parts + suppliers) et dégrade proprement
  si une lecture secondaire échoue.
- Les montants restent des chaînes jusqu'à `formatMoney` ; jamais de `number`.

### Constraints
- AGENTS §3 : même entrée + même snapshot = même résultat ; le prix affiché doit dire sa source.
- AGENTS §7 : un nouveau champ dans `/api/v1` est un changement de contrat à faire valider. **Les endpoints
  existants suffisent** : aucun changement de contrat n'est prévu par cette spec.

### Décision à trancher par un humain (ticket « À trancher »)
D'où vient le prix affiché dans le sélecteur :
1. **Toujours la dernière cotation en direct** (`GET /api/v1/quotes`) — simple, mais pour une BOM figée sur
   un snapshot de prix, le sélecteur et le coût du nœud peuvent diverger.
2. **Même source que les totaux de la BOM** (recommandé) — en direct si `price_snapshot_id` est nul, sinon
   prix du snapshot (`GET /api/v1/price-snapshots/{id}`). Cohérent avec §3, deux lectures existantes.
3. Ajouter `latest_price` à `GET /api/v1/parts` — une seule requête, mais changement de contrat et couplage
   catalogue → pricing côté API.

**Retenu : option 2** (décision humaine du 2026-11-10, voir state.md).

## Requirements

### Must Have
- R1 : quand une offre a une cotation, le sélecteur affiche son prix dans sa devise d'origine.
- R2 : si une offre n'a réellement aucune cotation (ou une erreur de cotation), « sans prix » reste affiché.
- R3 : le montant reste une chaîne formatée par `Intl.NumberFormat`, jamais un `number`.
- R4 : le prix du sélecteur vient de la même source que les totaux de la BOM (direct ou snapshot de prix),
  selon la décision humaine ci-dessus.

### Should Have
- S1 : le survol de l'option (attribut `title`) indique la source : « en direct au <date> » ou
  « snapshot de prix #N ».

### Won't Do
- Nouveau champ ou nouvel endpoint `/api/v1` (option 3) sans ADR acceptée.
- Mise à jour temps réel du sélecteur via `/api/v1/quotes/stream`.
- Conversion du prix de l'offre dans la devise de la BOM (le ticket demande la devise d'origine).

## Tasks

- [x] T1: Lecture des prix par offre, source directe ou snapshot
  - **Files**: `web/src/catalog.ts` (ou `web/src/offerPrices.ts` nouveau)
  - **Approach**: `loadOfferPrices(priceSnapshotID: number | null): Promise<Map<number, Money>>` —
    `GET /api/v1/quotes` ou `GET /api/v1/price-snapshots/{id}` ; seules les entrées avec `quote` non nul
    entrent dans la map (une entrée en erreur reste absente → « sans prix »).
  - **Done when**: fonction exportée, typée sans `number` pour les montants ; `npm run build` passe.

- [x] T2: Brancher les prix dans les sélecteurs de l'arbre et du panneau
  - **Files**: `web/src/bom/BomView.tsx`, `web/src/bom/BomTree.tsx`, `web/src/bom/NodePanel.tsx`
  - **Approach**: `BomView` charge la map avec le `price_snapshot_id` de la BOM (rechargée après
    figer/défiger) et la passe aux deux sélecteurs ; `o.price` vient de la map. Échec de lecture → la map
    reste vide et « sans prix » s'affiche (pas d'invention de prix).
  - **Done when**: sur Multipla seedée, les offres cotées affichent un prix ; le `title` donne la source (S1).

- [ ] T3: Test e2e du sélecteur
  - **Files**: `web/e2e/bom-offer-price.spec.ts` (nouveau)
  - **Approach**: intercepter `/api/v1/quotes` (`page.route`) avec une offre cotée et une offre sans
    cotation ; vérifier le texte des deux `<option>` (prix formaté + devise / « sans prix »).
  - **Done when**: le test échoue avant T2 et passe après.

## Verification

### Automated Checks
- `cd web && npm run lint && npm run build && npm run test:e2e`
- `make lint && make test`

### Manual Checks
- `make up` + seed, ouvrir Multipla : le sélecteur d'un nœud montre « Fournisseur · 92,00 € · 12 j ».
- Figer les prix, attendre un tick de cotation : le sélecteur garde le prix figé ; défiger : prix en direct.

### Acceptance Criteria
- [ ] AC1 (R1) : une offre cotée affiche son prix avec sa devise d'origine.
- [ ] AC2 (R2) : une offre sans cotation affiche « sans prix ».
- [ ] AC3 (R3) : aucun montant converti en `number` dans le diff (`grep -n "Number(\|parseFloat" ` sur les
  fichiers touchés ne concerne pas un montant).
- [ ] AC4 (R4) : BOM figée → prix du snapshot ; BOM en direct → dernière cotation.
