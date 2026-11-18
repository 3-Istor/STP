# Vérification locale du visualiseur — 2026-11-17

Extrait du résumé du harnais existant, avant les dernières corrections ciblées frontend.
Ces corrections ont ensuite passé les tests unitaires, le lint, le build et les parcours navigateur.
Aucun contrôle Go n'a été rendu vert par une relecture IA ; le backend est inchangé.

Attention au libellé PostgreSQL : `pg_isready` absent fait afficher « sans base » et « not_run »
par le script. `DATABASE_URL` pointait pourtant vers la base temporaire migrée et les tests Go l'ont
utilisée. Une exécution explicite `go test -count=1 -v -run '^TestStoreVehicleProfile$'
./internal/bom/bomstore` confirme ses quatre sous-cas PASS sans skip.
Le `make test` complet sur cette base a aussi passé, avant le passage `-race`.

Le risque de notre diff est **1 Routine par rapport à 010d4d9**. Le résumé brut du harnais compare
par défaut à origin/main et inclut les 101 commits hérités de V2 (niveau 3, notamment son harnais).
Ce niveau hérité n'est pas une modification du harnais par la refonte 3D.

# Moulinette — niveau `pr` — 48a34a4

| Étape | Statut | Durée | Note |
|---|---|---|---|
| gofmt | passed | 0 s | |
| go vet | passed | 0 s | |
| frontières d'architecture | passed | 1 s | |
| contrat des routes /api/v1 | passed | 0 s | |
| niveaux de risque + doc agents | passed | 0 s | |
| secrets (index) | not_run | | gitleaks absent : make tools |
| oxlint (web) | passed | 0 s | |
| go mod tidy à jour | passed | 15 s | |
| code sqlc à jour | passed | 29 s | |
| golangci-lint (dont gosec) | passed | 2 s | |
| tests Go -race (sans base) | passed | 168 s | |
| tests d'intégration PostgreSQL | not_run | | DATABASE_URL injoignable : la CI les lance |
| govulncheck | passed | 4 s | |
| osv-scanner | not_run | | absent : make tools |
| actionlint | not_run | | absent : make tools |
| tests unitaires web | passed | 1 s | |
| build web (tsc + vite) | passed | 1 s | |
| npm audit (prod, high) | passed | 0 s | |

