---
id: v2-lot-b
status: review
branch: v2-lot-b
current_task: null
started: 2026-11-17
updated: 2026-11-17
---

# State: V2 lot B — versions et publication

## Progress

All tasks completed. Ready for review (relecture humaine, ADR 0032 et 0033 à accepter).
- Tasks: 9/9 completed
- Branch: `v2-lot-b` (contient lots A et D) ; rien n'est poussé

## Task Log

### T1: Ticket et ADR 0032, 0033
- **Status**: completed
- **Time**: 2026-11-17
- **Notes**: base `stp_lot_b` migrée (version 12), tests de référence verts avant toute modification.

### T2: Migration 00013 et requêtes sqlc
- **Status**: completed
- **Commit**: 988fb96
- **Notes**: goose up/down/up passe ; sqlc régénère aussi les modèles des autres domaines (colonnes ajoutées).

### T3: Pont de coût pur
- **Status**: completed
- **Commit**: c8b47c0
- **Notes**: 15 cas table-driven (chaque cause isolée, combinées sur 3 devises, taux budget, méthode) ; mutation
  vérifiée : inverser `offer_change` et `fx` dans l'ordre fait échouer `TestComputeBridgeOrderOfferBeforeFX`.

### T4: Contrôles avant publication
- **Status**: completed
- **Commit**: c0f58d9

### T5: Publication tracée
- **Status**: completed
- **Commit**: d4209a9
- **Notes**: les tests de publication existants passent sans être affaiblis : seuls les avertissements sont
  justifiés (`anyWarning`) ; les erreurs nommées (ErrNoQuote, ErrNoRate, ErrIncompleteCost…) restent lisibles
  sous ErrPublicationBlocked.

### T6: Services comparaison, versions, pistes
- **Status**: completed
- **Commit**: 02d3dd5
- **Notes**: `NewDraft` copie désormais les budgets cibles de la BOM source (sinon un brouillon B n'a ni % du budget
  ni contrôle `budgets`).

### T7: API
- **Status**: completed
- **Commit**: 4ea719f

### T8: Données de démo
- **Status**: completed
- **Commit**: c477427
- **Notes**: seed sur base neuve 1 min 09 s (dont versions de démo ~25 s : NewDraft recrée ~30 000 nœuds un par un) ;
  second seed 0,7 s, aucun ajout. Les marches identiques des deux côtés ne sont plus rechiffrées : `compare`
  V1 → B mesuré 4,8 s avant, 2,8 s après (Multipla, 30 000 nœuds).

### T9: Vérification et livrable §9
- **Status**: completed
- **Notes**: voir rapport ci-dessous.

## Decisions

- D1 (seul, à valider) : ordre du pont `structure → offer_change → price → fx → discount → logistics → tooling →
  method` ; ajout des causes `logistics` et `method` (« ce que la méthode exige »). Causes à 0 omises.
- D2 (seul) : appariement des nœuds par chemin (nom de sous-ensemble / id de pièce, rang parmi les frères
  identiques) : aucune colonne ajoutée à `bom_nodes`. Remplacer une pièce par une autre (autre id) = retirée +
  ajoutée (`structure`), pas `offer_change`.
- D3 (seul) : un fait absent d'un côté est pris de l'autre ; l'outillage d'une nouvelle offre tombe dans
  `offer_change` (la maquette I4-6 le montrait à part, elle est « illustrative »).
- D4 (seul) : `prices_frozen` est un `warn` (publier en prix en direct crée un snapshot de prix, c'est un choix
  à justifier) ; justifications pour un contrôle non `warn` ignorées (non stockées).

- D5 (seul) : causes à 0 omises de la réponse ; montants exacts (non arrondis) comme le lot A, l'arrondi est à
  l'affichage ; `node_ids` = 20 plus gros écarts de la marche, `node_count` = total.
- D6 (seul) : `NewDraft` copie les budgets cibles **actuels** de la BOM source (non figés au snapshot) ; le % du
  budget d'un snapshot dans `versions` utilise aussi le budget global actuel de sa BOM (budgets non figés : lot A).
- D7 (seul) : famille de versions = BOM d'origine + brouillons créés depuis ses snapshots (transitif) ; libellés
  `V1, V2…` (ordre de publication) et `Brouillon A, B…` (ordre de création, l'origine est A).
- D8 (seul) : pistes rattachées à une BOM ; ouvertes = idea, in_progress, to_validate ; gain dans la devise de
  la BOM (refus sinon) ; écart = coût − budget global, reste = écart − pistes ouvertes, signés (négatif = couvert).
- D9 (seul) : `GET /boms` non enrichi (S1) : `source_snapshot_id` existe déjà, `versions` donne le reste.
- D10 (seul) : seed des versions par les services (pas en SQL) pour figer exactement ce que l'API figerait ;
  étapes notées dans `seed_demo_steps` ; publication datée de l'heure réelle (comme l'API et `created_at`).
  Dataset et empreinte inchangés : une base `demo-v4` existante reçoit les versions sans conflit.
- D11 (seul) : portes avant nues → première autre offre de la même pièce (clé triée) : Fonderia Lombarda Brenta,
  la Multipla quittant Arden Composites (contrat expiré au 31/03). Haut-parleurs : un second par porte (4).
- D12 (seul) : un contrôle bloquant passe avant les avertissements (409 même si des justifications manquent).

## Blockers

Aucun.

## Verification Report(s)

### 2026-11-17 — T9 (exécuté localement)
| Check | Résultat |
|---|---|
| `go vet ./...` | PASS |
| `go tool golangci-lint run ./...` | PASS — `0 issues.` |
| `go test -race -count=1 ./...` (`DATABASE_URL` = `stp_lot_b`) | PASS — 18 paquets `ok`, aucun échec (cmd/server 283 s, seedstore 119 s) |
| `go mod tidy -diff` | PASS |
| `go tool sqlc generate` puis `git status` | PASS — aucun écart |
| `goose up` / `reset` / `up` (base neuve `stp_lot_b_reset`) | PASS — version 13 |
| `cd web && npm ci && npm run lint && npx tsc -b && npx vite build` | PASS (web non modifié) |
| CI GitHub | NON EXÉCUTÉE (rien n'est poussé) |

Contrôle manuel : base neuve `stp_lot_b_seed`, seed (`SIM_DATASET_VERSION=demo-v4`, `SIM_SEED=42`,
`SIM_AS_OF=2026-01-01T00:00:00Z`, `SIM_YEAR_DURATION=40m`, `SIM_FX_PAIRS` du Makefile/compose), seed relancé
(`created=false`, mêmes comptes : 3 BOM, 74 861 nœuds, 1 snapshot, 5 pistes, 3 étapes), API de prix :8094,
serveur :8084 (`PRICE_API_URL=http://127.0.0.1:8094`), puis serveurs arrêtés. V1 = snapshot 1, B = BOM 3.

Juste après le seed (temps simulé ~2026-01-10) : causes `structure` (+73,32 €, 4 haut-parleurs), `offer_change`
(+19,41 €, portes avant nues Arden → Fonderia), `price` ; Σ causes = écart (vérifié). Après 12 min (temps simulé
2026-04-17, contrat Arden expiré le 31/03) :

`GET /api/v1/compare?from=snapshot:1&to=bom:3` (extrait : 5 premiers `node_ids`, 3 premières pièces) :
```json
{
 "from": {
  "ref": "snapshot:1",
  "label": "V1",
  "totals": {
   "cost": {
    "amount": "25764.893287492528427302582666666500",
    "currency": "EUR"
   },
   "co2_kg": "4449.69043",
   "mass_kg": "1148.54614",
   "lead_time_days": 58
  }
 },
 "to": {
  "ref": "bom:3",
  "label": "Brouillon B",
  "totals": {
   "cost": {
    "amount": "26096.8396072962966585434331666665000",
    "currency": "EUR"
   },
   "co2_kg": "4502.80283",
   "mass_kg": "1150.20094",
   "lead_time_days": 58
  }
 },
 "delta": {
  "cost": {
   "amount": "331.9463198037682312408505000000000",
   "currency": "EUR"
  },
  "co2_kg": "53.1124",
  "mass_kg": "1.6548",
  "lead_time_days": 0
 },
 "causes": [
  {
   "kind": "structure",
   "amount": {
    "amount": "73.319177347553120200000000000000",
    "currency": "EUR"
   },
   "explanation": "4 pièces ajoutées (Haut-parleur 13 cm ×4).",
   "node_ids": [
    74858,
    74859,
    74860,
    74861
   ],
   "from_node_ids": [],
   "node_count": 4
  },
  {
   "kind": "offer_change",
   "amount": {
    "amount": "28.299008152732578179000000000000",
    "currency": "EUR"
   },
   "explanation": "Porte avant nue : Arden Composites → Fonderia Lombarda Brenta (2 nœuds) (outillage et faits de la nouvelle offre compris).",
   "node_ids": [
    53318,
    53840
   ],
   "from_node_ids": [],
   "node_count": 2
  },
  {
   "kind": "price",
   "amount": {
    "amount": "60.2931013229593961688505000000000",
    "currency": "EUR"
   },
   "explanation": "Nouvelles cotations sur 250 offres, surtout Bloc-cylindres fonte 1.6 16V.",
   "node_ids": [
    57385,
    57386,
    60439,
    71197,
    57809,
    "…"
   ],
   "from_node_ids": [],
   "node_count": 13638
  },
  {
   "kind": "discount",
   "amount": {
    "amount": "170.0350329805231366930000000000000",
    "currency": "EUR"
   },
   "explanation": "Contrat Arden Composites expiré : la remise (jusqu'à 9,7 %) ne s'applique plus sur 12 offres.",
   "node_ids": [
    54362,
    54884,
    55406,
    44887,
    55853,
    "…"
   ],
   "from_node_ids": [],
   "node_count": 39
  }
 ],
 "changed_parts": [
  {
   "change": "offer",
   "path": "Carrosserie et ouvrants > Porte avant gauche > Porte avant nue",
   "node_id": 53318,
   "from_node_id": 8438,
   "part_id": 63,
   "before": "Arden Composites · 1 u.",
   "after": "Fonderia Lombarda Brenta · 1 u.",
   "delta": {
    "amount": "14.14950407636628908950000000000",
    "currency": "EUR"
   }
  },
  {
   "change": "offer",
   "path": "Carrosserie et ouvrants > Porte avant droite > Porte avant nue",
   "node_id": 53840,
   "from_node_id": 8960,
   "part_id": 63,
   "before": "Arden Composites · 1 u.",
   "after": "Fonderia Lombarda Brenta · 1 u.",
   "delta": {
    "amount": "14.14950407636628908950000000000",
    "currency": "EUR"
   }
  },
  {
   "change": "added",
   "path": "Carrosserie et ouvrants > Porte avant gauche > Haut-parleur 13 cm",
   "node_id": 74858,
   "from_node_id": null,
   "part_id": 210,
   "before": "—",
   "after": "Elbtal Elektrik Systeme · 1 u.",
   "delta": {
    "amount": "18.3527468704319670700000000000",
    "currency": "EUR"
   }
  }
 ]
}
// changed_parts : 6 au total
```
Σ causes = 73.319177347553120200 + 28.299008152732578179 + 60.2931013229593961688505 + 170.0350329805231366930
= 331.9463198037682312408505 = `delta.cost` (vérifié en Decimal Python).

`GET /api/v1/boms/3/publication-checks` :
```json
[
    {
        "code": "offers_active",
        "level": "ok",
        "title": "Toutes les pièces ont une offre active",
        "detail": "29981 nœuds",
        "node_ids": []
    },
    {
        "code": "fx_complete",
        "level": "ok",
        "title": "Aucune conversion de devise manquante",
        "detail": "4 devise(s)",
        "node_ids": []
    },
    {
        "code": "prices_frozen",
        "level": "warn",
        "title": "Prix figés dans un snapshot",
        "detail": "prix en direct : un snapshot de prix sera créé à la publication",
        "node_ids": []
    },
    {
        "code": "method_complete",
        "level": "ok",
        "title": "Composantes de la méthode présentes",
        "detail": "méthode full · budget",
        "node_ids": []
    },
    {
        "code": "target_date",
        "level": "ok",
        "title": "Date cible tenue",
        "detail": "58 j pour 60 j",
        "node_ids": []
    },
    {
        "code": "budgets",
        "level": "warn",
        "title": "6 sous-système(s) au-dessus du budget",
        "detail": "Carrosserie et ouvrants 118,8 %, Porte avant gauche 119,2 %, Porte avant droite 119,2 %, Capot 106,2 %, Groupe motopropulseur 101,8 %, Habitacle 100,4 % : justification requise",
        "node_ids": [
            53316,
            53317,
            53839,
            55852,
            57382,
            71194
        ]
    },
    {
        "code": "expired_contracts",
        "level": "block",
        "title": "Contrat expiré encore utilisé",
        "detail": "Arden Composites, 31/03",
        "node_ids": [
            44887,
            48899,
            50105,
            51310,
            53322,
            53323,
            53844,
            53845,
            54362,
            54366,
            54367,
            54884,
            54888,
            54889,
            55406,
            55408,
            55410,
            55411,
            55853,
            55855,
            55856,
            71931,
            71936,
            71938,
            72120,
            72125,
            72127,
            72309,
            72314,
            72316,
            72499,
            72504,
            72506,
            72687,
            72692,
            72694,
            72875,
            72880,
            72882
        ]
    }
]
```
`POST /api/v1/boms/3/publish` avec note, proposé par et justifications → **409** `expired_contracts (Arden
Composites, 31/03): publication blocked by a check` ; aucun snapshot créé (toujours 1).

Acceptance : AC1 `TestComputeBridgeCausesAddUpToDelta`, `TestCompareSnapshotToDraft`, `checkDemoVersions` (seed) ;
AC2 `TestPublishChecksAndTrace`, `TestPublishedTraceIsImmutable`, `TestVersionsAndPublicationRoutes` ;
AC3 `TestSummarizeIdeas`, `TestReductionIdeas` ; AC4 `TestVersionsOfFamily` ; AC5 `TestRunSeedTwiceWithSameParametersAddsNothing`.

Non fait / non vérifié :
- Le front actuel publie sans corps : il reçoit maintenant 422 tant qu'un avertissement existe (ex. prix en
  direct) ; le lot F doit envoyer note, noms et justifications. Les tests e2e Playwright n'ont pas été lancés.
- `GET /boms` non enrichi (S1). Pas de pont CO₂ décomposé (écart CO₂ et masse seulement).
- `compare` ≈ 2,8 s et `versions` ≈ 1 s sur la Multipla (30 000 nœuds) : acceptable pour un écran d'analyse,
  pas optimisé davantage. `NewDraft` d'une BOM de 30 000 nœuds ≈ 25 s (nœud par nœud, existant).
- Seed des versions non transactionnel avec ses marqueurs : un arrêt entre la publication et l'écriture de
  l'étape ferait publier une seconde V1 au seed suivant (fenêtre de quelques ms ; les autres étapes sont idempotentes).
- Budgets non figés au snapshot (lot A) : le % du budget d'un snapshot suit le budget actuel de sa BOM.
