# BOM Studio — features mises en place

État au 2026-11-16, établi d'après `docs/features.md`, les ADR, les routes de `internal/httpapi` et l'historique git.
Spec de référence : `docs/features.md`. Décisions : `docs/adr/`.

## Légende
- ✅ en place sur `main`
- 🟡 en place sur la branche courante `feat/price-snapshot-choice`, pas encore mergée
- 🚧 commencée sur une branche (non mergée)
- ⏳ non commencée

## P1

| Feature | État | Ce qui existe | ADR |
|---|---|---|---|
| part-catalog | ✅ | Pièces, offres (devise, bornes min/max, délai, qualité, pays), alternatives symétriques, recherche/filtres, archivage des pièces/offres utilisées. API `/api/v1/parts`, `/offers` | 0009 |
| bom-management | ✅ | Créer, lister, renommer, supprimer une BOM (refus si snapshot publié), devise de référence et volume. API `/api/v1/boms` | 0008 |
| bom-tree-editor | ✅ | Ajout/suppression/déplacement de nœuds (anti-cycle), choix de l'offre, totaux coût/CO2/délai remontés dans l'arbre, quantités multipliées. UI : `BomTree`, `AddNodeDialog`, `NodePanel` | 0007, 0020 |
| live-price-feed | ✅ | Interface prix/taux, simulateur déterministe (graine, historique d'un an, temps accéléré), cotations immuables, snapshots de prix, flux SSE `/api/v1/quotes/stream` | 0002, 0003, 0005, 0014, 0015, 0016 |
| bom-draft-and-snapshots | ✅ / 🟡 | Brouillon, publication en snapshot immuable, nouveau brouillon depuis un snapshot, figer/défiger les prix. 🟡 Choix de la source des prix parmi les snapshots existants (nommés à la seconde), indicateur de calcul en cours après changement de stratégie | 0006 |
| multi-currency-costing | ✅ | Conversion par taux daté direct ou inverse, sans pivot, taux et date indiqués, erreur nommée si taux absent. `POST /api/v1/convert` | 0004, 0017 |
| suppliers-and-contracts | ✅ | Page fournisseurs, contrats à paliers de remise, validité, prix en direct, min/max/variation 12 mois, sparkline, historique converti au taux du jour, mise à jour sans rechargement | 0010, 0011, 0030 (Proposé) |
| target-costing | ✅ | Budget global et par sous-système, code couleur vert/orange/rouge, volume déduit d'un budget de production | 0018 |
| sample-dataset | ✅ | `make seed` idempotent : Fiat Multipla et Aston Martin Valhalla, catalogue, contrats, offres multi-devises | 0012, 0021 |

## P2 (mergées via la PR #17, `feat/p2`)

| Feature | État | Ce qui existe | ADR |
|---|---|---|---|
| stock-and-reuse | ✅ | Stock propre à chaque BOM, couverture totale/partielle. `StockPanel`, `/boms/{id}/stock` | 0022 |
| lead-time-planning | ✅ | Date de disponibilité, date cible, chemin critique. `PlanningPanel`, `/boms/{id}/schedule` | 0023 |
| workforce-fte | ✅ | ETP par sous-système avec base horaire paramétrable. `Workforce` | 0024 |
| total-landed-cost | ✅ | Achat + douane + transport + emballage, tables paramétrables, comparaison de deux offres. `LandedPanel`, `/customs-duties`, `/freight-rates` | 0025 |
| make-or-buy | ✅ | Coût interne vs Tier 1, volume d'équilibre. `MakeOrBuyPanel` | 0026 |
| cost-anomalies | ✅ | Écart à la médiane de catégorie (3× par défaut, catégorie ≥ 5 pièces), surbrillance avec raison. `Anomalies` | 0027 |
| node-strategies | ✅ | Stratégies coût/délai/CO2/équilibré par nœud, suggestions triées en conséquence. `StrategyPanel` | 0028 |

## Perf et qualité transverses
- Chargement de BOM : lecture des prix et taux par lot (PR #22), dernière cotation en `LATERAL`.
- UI : même identité visuelle BOM / fournisseurs, fenêtres de confirmation de l'app, devise toujours en menu déroulant, liste de fournisseurs défilante et filtrable, ajout d'une pièce avec recherche et choix d'offre.
- CI DevSecOps (ADR 0001) : lint, tests avec Postgres, build, `npm audit`, `govulncheck`. Tests e2e navigateur reproductibles (ADR 0013).

## Pas encore sur main
- 🚧 **exploded-3d-view** (P4) : branches `feat/exploded-3d-view-models` et `feat/exploded-3d-view-viewer`. Scène 3D générée depuis la BOM, vue BOM / vue 3D synchronisées, éclaté progressif, modèles GLB persistés avec génération asynchrone. ADR 0031 et 0032 (0032 à valider par un humain).