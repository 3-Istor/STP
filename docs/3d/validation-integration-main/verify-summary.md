# Moulinette — niveau `pr` — 7a6d18b

| Étape | Statut | Durée | Note |
|---|---|---|---|
| gofmt | passed | 0 s | |
| go vet | passed | 4 s | |
| frontières d'architecture | passed | 0 s | |
| contrat des routes /api/v1 | passed | 1 s | |
| niveaux de risque + doc agents | passed | 0 s | |
| secrets (index) | not_run | | gitleaks absent : make tools |
| oxlint (web) | passed | 1 s | |
| go mod tidy à jour | passed | 0 s | |
| code sqlc à jour | passed | 0 s | |
| golangci-lint (dont gosec) | passed | 4 s | |
| tests Go -race (intégration incluse) | passed | 368 s | |
| govulncheck | passed | 3 s | |
| osv-scanner | not_run | | absent : make tools |
| actionlint | not_run | | absent : make tools |
| tests unitaires web | passed | 1 s | |
| build web (tsc + vite) | passed | 2 s | |
| npm audit (prod, high) | passed | 1 s | |

## Niveau de risque : 3 Critique

Comparé à `010d4d9e54d0990cbfe0f1368341ae386034107d`. Point de contrôle humain : **2 humains dont un CODEOWNER : plan validé avant le code, diff sensible relu ligne à ligne**.

Relecteurs IA en contexte neuf : relecteur-code, relecteur-architecture, relecteur-securite, relecteur-metier.

| Niveau | Fichier | Raison |
|---|---|---|
| 3 | `internal/bom/costfacts_test.go` | calcul du coût et de ses causes |
| 3 | `internal/snapshot/draft_budget_integration_test.go` | immuabilité des snapshots (AGENTS §3, ADR 0038) |
| 3 | `internal/snapshot/snapshot.go` | immuabilité des snapshots (AGENTS §3, ADR 0038) |
| 3 | `internal/snapshot/snapshotdb/models.go` | immuabilité des snapshots (AGENTS §3, ADR 0038) |
| 2 | `docs/adr/0041-methodes-d-anomalies-offres-historique-matiere.md` | décision d'architecture |
| 2 | `docs/adr/0042-budgets-hierarchiques-saisis-dans-structurer.md` | décision d'architecture |
| 2 | `docs/adr/0043-simulateur-de-prix-en-boucle-sur-2026.md` | décision d'architecture |
| 2 | `docs/adr/0044-budgets-multi-devises-au-taux-du-jour.md` | décision d'architecture |
| 2 | `docs/adr/0045-budget-global-dans-la-devise-de-la-bom.md` | décision d'architecture |
| 2 | `docs/adr/0046-cotation-en-base-fait-foi-au-rebouclage.md` | décision d'architecture |
| 2 | `docs/adr/0047-copie-de-brouillon-refuse-un-budget-fige-incoherent.md` | décision d'architecture |
| 2 | `docs/adr/0048-gel-de-prix-sur-tout-le-catalogue.md` | décision d'architecture |
| 2 | `docs/adr/0049-liaison-explicite-des-assets-aux-occurrences-bom.md` | décision d'architecture |
| 2 | `docs/adr/0050-simulation-reversible-des-motorisations.md` | décision d'architecture |
| 2 | `internal/httpapi/analysis.go` | contrat /api/v1 (routes, JSON) |
| 2 | `internal/httpapi/boms.go` | contrat /api/v1 (routes, JSON) |
| 2 | `internal/httpapi/boms_test.go` | contrat /api/v1 (routes, JSON) |
| 2 | `internal/httpapi/budgets_json_test.go` | contrat /api/v1 (routes, JSON) |
| 2 | `internal/httpapi/catalog.go` | contrat /api/v1 (routes, JSON) |
| 2 | `internal/httpapi/configurations.go` | contrat /api/v1 (routes, JSON) |
| 2 | `internal/httpapi/configurations_test.go` | contrat /api/v1 (routes, JSON) |
| 2 | `internal/httpapi/cost_method.go` | contrat /api/v1 (routes, JSON) |
| 2 | `internal/httpapi/errorcode.go` | contrat /api/v1 (routes, JSON) |
| 2 | `internal/httpapi/errorcode_test.go` | contrat /api/v1 (routes, JSON) |
| 2 | `internal/httpapi/logistics.go` | contrat /api/v1 (routes, JSON) |
| 2 | `internal/httpapi/pricing.go` | contrat /api/v1 (routes, JSON) |
| 2 | `internal/httpapi/pricing_stream.go` | contrat /api/v1 (routes, JSON) |
| 2 | `internal/httpapi/router.go` | contrat /api/v1 (routes, JSON) |
| 2 | `internal/httpapi/supplier.go` | contrat /api/v1 (routes, JSON) |
| 2 | `internal/httpapi/supplier_offers.go` | contrat /api/v1 (routes, JSON) |
| 2 | `internal/httpapi/testdata/routes.golden` | contrat /api/v1 (routes, JSON) |
| 2 | `internal/sampledata/costs.go` | jeu de données de démonstration |
| 2 | `internal/sampledata/dataset.go` | jeu de données de démonstration |
| 2 | `internal/sampledata/generate.go` | jeu de données de démonstration |
| 2 | `internal/sampledata/generate_test.go` | jeu de données de démonstration |
| 2 | `internal/sampledata/seedstore/store.go` | jeu de données de démonstration |
| 2 | `internal/sampledata/values.go` | jeu de données de démonstration |
| 2 | `migrations/00016_part_material.sql` | migration de schéma (jamais modifier une migration commitée) |
| 2 | `migrations/00017_pricing_year_cycle.sql` | migration de schéma (jamais modifier une migration commitée) |
| 1 | `internal/bom/analysis.go` | code ou fichier non classé (inconnu = routine) |
| … | 337 autres fichiers | |
