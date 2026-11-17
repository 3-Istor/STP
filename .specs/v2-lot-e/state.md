---
id: v2-lot-e
status: review
branch: v2-lot-e
current_task: null
started: 2026-11-17
updated: 2026-11-17
---

# State: V2 lot E — analyses et profils de véhicule

## Progress

Toutes les tâches sont faites. Prêt pour revue (relecture humaine, ADR 0036 et 0037 à accepter).
- Tasks: 11/11 completed
- Branch: `v2-lot-e` (contient lots A et D) — rien n'est poussé
- Commits : T1 à T10 (+ deux commits `gofmt` séparés, pas de réécriture d'historique), T11 = ce commit

## Task Log

### T1: Ticket et ADR 0036, 0037
- **Status**: completed
- **Commit**: d2207c1
- **Notes**: ADR au statut Proposé.

### T2: Migration 00014 et requêtes sqlc
- **Status**: completed
- **Commit**: ca2e29e
- **Notes**: tables dédiées `bom_vehicle_profiles` et `bom_snapshot_vehicle_profiles` (immuable) ; goose up/down/up OK.

### T3: Profils et classement pondéré
- **Status**: completed
- **Commits**: 399fa8e, 791e2d0
- **Notes**: `strategyWeights` seul point de choix ; tests ADR 0028 inchangés et verts ; `RankWeighted`.

### T4: Gel du profil au snapshot
- **Status**: completed
- **Commit**: c8ac22b

### T5: Sensibilité au change
- **Status**: completed
- **Commit**: c3a997b

### T6: Pièces qui pèsent le plus
- **Status**: completed
- **Commit**: 13540a6

### T7: Sourcing
- **Status**: completed
- **Commits**: 63d3064, ed721eb

### T8: Anomalies €/kg
- **Status**: completed
- **Commit**: 1026e96
- **Notes**: `CostAnomalies` factorisée sur `anomaliesWith` (même résultat, tests ADR 0027 verts) ; les lectures
  de pièces d'une BOM aux prix figés sont désormais mises en cache par pièce.

### T9: API
- **Status**: completed
- **Commit**: 5b1505b
- **Notes**: une seule ligne ajoutée à `router.go` (`mountAnalysis`) ; JSON des suggestions enrichi (champs ajoutés).

### T10: Données de démo
- **Status**: completed
- **Commit**: ce32fc7

### T11: Vérification
- **Status**: completed
- **Notes**: voir rapport ci-dessous.

## Decisions

- E1 (seul, à valider) : une BOM **sans profil** par défaut (GET → `{"preset":null,"weights":null}`) : le
  classement reste exactement celui d'ADR 0028. Priorité stratégie de nœud > profil > aucun.
- E2 (seul) : identifiants de préréglage en anglais (`city_eco`, `family`, `premium`, `sport`, `custom`), libellés
  français dans `GET /vehicle-profiles` ; un préréglage impose ses poids (sinon 400, passer en `custom`).
- E3 (seul) : qualité qui pèse et offre sans note → candidate exclue (`ErrNoQualityScore`) ; critère de poids nul
  non évalué. Toutes les offres seedées ont une note.
- E4 (seul) : sensibilité = rechiffrage par le moteur, dernier taux **et** taux budget de la devise × (1 + choc) ;
  `share_percent` = exposition (rechiffrage au taux doublé − base) ÷ total. Elle ne compte que ce qui est
  converti depuis la devise : une pièce GBP livrée avec un transport en EUR n'expose que sa part GBP. D'où
  GBP 11,57 % en change contre 11,76 % « par devise d'offre » dans le sourcing (qui ventile le coût propre entier
  par devise de l'offre). Choc accepté : décimal non nul dans ]-100 ; 100], défaut 5.
- E5 (seul) : amplitude des cotations sur la fenêtre annuelle d'ADR 0011 finissant à la date de calcul du
  brouillon (cotation la plus récente utilisée, temps simulé), convertie au taux de la méthode ; par pièce, somme
  sur ses offres retenues ; non évaluables listées après les classées (`rank: null`).
- E6 (seul) : remise perdue d'un contrat expiré = évaluation du seul contrat à son dernier jour de validité, à la
  quantité commandée (quantité par voiture × volume, comme ADR 0031), appliquée au prix converti au taux de la
  méthode ; montant par voiture.
- E7 (seul) : effet taux budget → dernier taux = moteur en taux budget (devises sans taux budget au dernier
  taux dans les deux calculs), une devise basculée au dernier taux ; positif = la voiture coûte plus cher.
- E8 (seul) : mono-source = pièce de la BOM avec au plus une offre **active** au catalogue.
- E9 (seul) : anomalies €/kg par **pièce et offre** (nœuds regroupés), seuil par défaut 2 (maquette : 2,1× est
  signalé), 5 pièces évaluables minimum ; nouvelle route `GET /boms/{id}/analysis/anomalies`, l'ancienne
  `/anomalies` est inchangée.
- E10 (seul) : profils de démo hors `Dataset` (empreinte inchangée, pas de `demo-v5`) : posés au seed en
  `ON CONFLICT DO NOTHING` ; un profil changé par l'utilisateur est gardé et n'est pas une « modification » du jeu.
- E11 (seul) : montants exacts en JSON (comme la ventilation du lot A, arrondi à l'affichage) ; pourcentages
  arrondis à 2 décimales en sortie.
- E12 (seul) : analyses sur un brouillon seulement (pas de snapshot) ; profil figé lisible par
  `GET /snapshots/{id}/profile`.

## Blockers

Aucun.

## Verification Report(s)

### 2026-11-17 — T11 (exécuté localement)
| Check | Résultat |
|---|---|
| `go vet ./...` | PASS |
| `go tool golangci-lint run ./...` | PASS — `0 issues.` |
| `go test ./...` (`DATABASE_URL` = `stp_lot_e`) | PASS — 18 paquets `ok` |
| `go test -race -count=1 ./...` | 17/18 `ok` ; `internal/bom` : `TestTotals30kUnderOneSecond` à 1,017 s (> 1 s) sous charge (load average 7,5 : lots B et F en parallèle) ; relancé seul sous `-race` : `ok`. Code de `ComputeTotals` non modifié par ce lot. |
| `go mod tidy -diff` | PASS |
| `go tool sqlc generate` puis `git status` | PASS — aucun écart |
| `goose up` / `reset` / `up` (base neuve `stp_lot_e_reset`) | PASS — version 14 |
| Front | NON EXÉCUTÉ (aucun fichier de `web/` touché) |
| CI GitHub | NON EXÉCUTÉE (rien n'est poussé) |

**Contrôle manuel** : base `stp_lot_e_seed` (`server seed`, demo-v4, seed 42), API de prix :8095, serveur :8085,
arrêtés ensuite. Multipla (BOM 1, full + budget, budget 26 500 €). Valeurs au temps simulé 2026-04-08 :

- Sensibilité (+5 %) : USD 21,12 % → +278,67 € (100,64 % du budget) ; GBP 11,57 % → +152,74 € (100,17 %) ;
  JPY 11,32 % → +149,40 € (100,16 %). Part hors euro (exposition) : 44,0 % ; par devise d'offre : 44,96 %.
- Pièces influentes : 1 Armature de siège individuel 191,44 € ; 2 Bloc-cylindres fonte 1.6 16V 158,79 € ;
  3 Calculateur d'injection 152,71 € (fenêtre 2025-04-09 → 2026-04-09) ; aucune exclue.
- Sourcing : 26 fournisseurs, 5 premiers = 43,16 %, 0 pièce mono-source ; pays DE 19,15 %, IT 14,88 %, US 12,43 %,
  GB 11,76 %, JP 11,47 % ; contrat Arden Composites expiré le 2026-03-31, 13 pièces, remise perdue 9,7 % (palier
  ≥ 500) = 215,49 €/voiture ; taux budget vs dernier : GBP −0,09 % (−2,74 €), JPY −3,83 % (−114,53 €),
  USD +2,05 % (+114,51 €).
- Anomalies €/kg (seuil 2) : 15 anomalies sur 257 pièces (méthode médiane : 20) ; en tête Nappe chauffante de
  siège 178,7 €/kg vs 31,7 (Sièges), surcoût 133,00 €/voiture.
- Profils : `GET /boms/1/profile` → `family` 40/25/20/5/10 ; Valhalla sans profil (null) ; PUT somme 99 → 400.
  Suggestions du nœud 12934 classées par le profil familiale (`strategy: null`, `profile.preset: family`).
- Durées observées : fx-sensitivity 1,1–3,4 s, sourcing 7 s, price-drivers 1,1 s, anomalies 0,7 s (30 000 nœuds,
  machine chargée).

Extraits des réponses réelles (listes tronquées à un élément) :

```json
{"currency": "USD", "share_percent": "21.12", "impact": {"amount": "278.6749552196013075873929000000000", "currency": "EUR"}, "budget_percent_after": "100.64", "shock_percent": "5", "exposed": {"amount": "5573.4991043920261517478580000000000", "currency": "EUR"}, "cost_before": {"amount": "26391.7437196891949920623246666665000", "currency": "EUR"}, "cost_after": {"amount": "26670.4186749087962996497175666665000", "currency": "EUR"}, "budget": {"amount": "26500.00", "currency": "EUR"}, "budget_error": null, "rates": [{"from": "USD", "to": "EUR", "rate": "0.928696186400148", "source": "pricesim.v1/demo-v4", "quoted_at": "2026-04-08T00:00:00Z"}, {"from": "USD", "to": "EUR", "rate": "0.91", "source": "budget 2026", "quoted_at": "2026-01-01T00:00:00Z"}], "cost_method": {"basis": "full", "fx": "budget", "program_volume": 60000}, "price_snapshot_id": null}
{"part_id": 241, "name": "Armature de siège individuel", "amplitude": {"amount": "191.43738936090666", "currency": "EUR"}, "rank": 1, "error": null, "offers": [{"offer_id": 586, "quantity": "6", "min": {"amount": "69.91891674233511", "currency": "EUR"}, "max": {"amount": "101.82514830248622", "currency": "EUR"}, "quotes": 366, "amplitude": {"amount": "191.43738936090666", "currency": "EUR"}, "error": null}], "window_start": "2025-04-09T00:00:00Z", "window_end": "2026-04-09T00:00:00Z"}
{"currency": "EUR", "as_of": "2026-04-08T00:00:00Z", "total": {"amount": "26391.7437196891949920623246666665000", "currency": "EUR"}, "by_country": [{"key": "DE", "cost": {"amount": "5054.2936821225281793606666666666", "currency": "EUR"}, "share_percent": "19.15", "parts": 48}, {"key": "IT", "cost": {"amount": "3928.2268855176292491050000000000", "currency": "EUR"}, "share_percent": "14.88", "parts": 49}, {"key": "US", "cost": {"amount": "3279.80574075323144796008800000000", "currency": "EUR"}, "share_percent": "12.43", "parts": 29}], "by_currency": [{"key": "EUR", "cost": {"amount": "14526.5570489891283714569999999999", "currency": "EUR"}, "share_percent": "55.04", "parts": 148}, {"key": "USD", "cost": {"amount": "5735.58613939202615174785800000000", "currency": "EUR"}, "share_percent": "21.73", "parts": 58}], "supplier_count": 26, "top5_share_percent": "43.16", "suppliers": [{"supplier_id": 11, "name": "Arden Composites", "countries": ["GB"], "currencies": ["GBP"], "cost": {"amount": "2295.17120788324165816666666666660", "currency": "EUR"}, "share_percent": "8.70", "parts": 13, "contract": {"status": "expired", "contract_id": 11, "ends_on": "2026-03-31"}}], "mono_source_count": 0, "mono_source": [], "expired_contracts": [{"contract_id": 11, "supplier_id": 11, "supplier_name": "Arden Composites", "ends_on": "2026-03-31", "lost_amount": {"amount": "215.487682618007774175500", "currency": "EUR"}, "parts": [{"part_id": 63, "name": "Porte avant nue", "offer_id": 153, "ordered_quantity": "4000", "discount_percent": "9.7", "tier_min": "500", "amount": {"amount": "34.571042093445803916000", "currency": "EUR"}, "error": null}]}], "rates": [{"currency": "GBP", "budget_rate": {"from": "GBP", "to": "EUR", "rate": "1.15", "source": "budget 2026", "quoted_at": "2026-01-01T00:00:00Z"}, "latest_rate": {"from": "GBP", "to": "EUR", "rate": "1.148966679945708", "source": "pricesim.v1/demo-v4", "quoted_at": "2026-04-08T00:00:00Z"}, "gap_percent": "-0.09", "effect": {"amount": "-2.744826137382684211433980709824032000", "currency": "EUR"}, "error": null}], "cost_method": {"basis": "full", "fx": "budget", "program_volume": 60000}, "price_snapshot_id": null}
{"method": "per_kg", "threshold": "2", "currency": "EUR", "price_snapshot_id": null, "rates": [{"from": "GBP", "to": "EUR", "rate": "1.225590298456563", "source": "pricesim.v1/demo-v4", "quoted_at": "2026-04-09T00:00:00Z"}], "anomaly_count": 15, "parts": [{"part_id": 249, "name": "Nappe chauffante de siège", "category": "Sièges", "offer_id": 606, "node_ids": [27058, 27247, 27436, 27626, 27814, 28002], "quantity": "6", "status": "anomaly", "unit_cost": {"amount": "26.94720369518881423132896131688", "currency": "EUR"}, "per_kg": {"amount": "178.6949847161061951679639344621", "currency": "EUR"}, "median_per_kg": {"amount": "31.7035764439285299581978634464", "currency": "EUR"}, "reference_cost": {"amount": "4.78089932774442231769623780771712", "currency": "EUR"}, "ratio": "5.63", "excess": {"amount": "22.16630436744439191363272350916288", "currency": "EUR"}, "impact": {"amount": "132.99782620466635148179634105497728", "currency": "EUR"}, "peers": 13, "missing": 0, "reason": null}]}
```

Au temps simulé 2026-01-06 (avant l'expiration), même BOM : `expired_contracts: []`, Arden `active`.

Acceptance : AC1 `TestComputeFXSensitivity` (impact = exposition × choc, entrée non modifiée) ;
AC2 `TestRank`, `TestSuggestions` (ADR 0028) verts + `TestRankWeighted` ; AC3 `TestNewVehicleProfile`,
`TestAnalysisRoutes` (somme 99, poids décimal → 400), `TestStoreVehicleProfile` (CHECK en base) ;
AC4 `TestSnapshotFreezesVehicleProfile` ; AC5 `TestPerKgDetector`, `TestPartAnomalies`.
