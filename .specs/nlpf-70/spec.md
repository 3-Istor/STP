---
id: nlpf-70
title: Socle commun (les 3 ensemble)
source: jira
source_ref: https://3istors.atlassian.net/browse/NLPF-70
created: 2026-11-02
status: draft
priority: P1
depends_on: []
branch: nlpf-70
---

# Socle commun (les 3 ensemble)

## Context

Ticket fondateur : poser ce dont tous les domaines (`catalog`, `bom`, `supplier`, `pricing`, `snapshot`)
dépendent, pour que les trois membres de l'équipe avancent ensuite en parallèle. Il couvre deux décisions
d'architecture (où vit le snapshot de prix, canal temps réel), le type `Money`, le premier schéma de base
avec migrations + sqlc, et les interfaces `catalog` / `pricing` avec des stubs consommés par `bom`.
Critère transverse du ticket : CI verte, chaque point couvert par un test automatique, ADR pour toute
décision du §7 d'AGENTS.md.

## Research Findings

### Relevant Files
- `docs/features.md` — vocabulaire (offre, cotation, snapshot de prix / de BOM), live-price-feed (l. 124-161),
  bom-draft-and-snapshots (l. 167-191), multi-currency-costing (l. 195-213).
- `docs/adr/0001-pipeline-ci-devsecops.md` — seule ADR existante, modèle de format ; prochaine = `0002`.
- `internal/{bom,catalog,pricing,snapshot,supplier}/doc.go` — packages vides (doc seule).
- `internal/httpapi/router.go` — seulement `GET /healthz`.
- `cmd/server/main.go` — câblage, pas encore de base de données.
- `migrations/` — vide. `Makefile` : `migrate` lance goose s'il trouve des `.sql`.
- `go.mod` — `goose`, `golangci-lint`, `govulncheck` en `tool` ; `shopspring/decimal` et `pgx/v5` présents
  seulement en `// indirect` (tirés par les outils) ; **sqlc absent**.
- `.github/workflows/ci.yml` — jobs `go`, `web`, `security` ; **pas de service Postgres** ni de contrôle sqlc.
- `compose.yaml` — Postgres 18 (`bom/bom@bomstudio`).

### Existing Patterns
- Commentaires et docs en français, packages documentés par un `doc.go`.
- Erreurs sentinelles `var ErrX = errors.New(...)`, testées avec `errors.Is` (AGENTS §4).
- Tests table-driven, `log/slog`, stdlib d'abord.
- Outils Go déclarés en `tool` dans `go.mod`, lancés via `go tool <nom>`.

### Constraints
- **§11 AGENTS** : ajouter une dépendance demande un accord explicite. Ce ticket en ajoute trois
  (`shopspring/decimal` et `pgx/v5` en direct, `sqlc` en `tool`) → validation humaine avant T3/T4.
- **§6/§8** : l'agent rédige les ADR au statut `Proposé` ; seuls les humains (« les 3 ») les passent à
  `Accepté`. Le ticket ne peut pas être clos par l'agent seul.
- Le schéma des snapshots de prix **dépend de l'ADR 0002** → T4 ne démarre qu'après son acceptation.
- Montants : `NUMERIC` + `CHAR(3)` en base, `decimal.Decimal` en Go, chaîne en JSON ; jamais de float.
- Snapshots et cotations immuables : trigger qui rejette `UPDATE`/`DELETE`.
- Le domaine n'importe ni `net/http` ni pgx directement dans ses types métier (accès données isolé).

```
internal/
  money/        (nouveau, Q1) Money, ErrCurrencyMismatch, Round
  catalog/      Catalog (interface) + stub en mémoire
  pricing/      Pricing (interface : prix d'offre, taux) + stub en mémoire
  bom/          compile contre catalog.Catalog et pricing.Pricing
  db/           code généré par sqlc (emplacement à confirmer)
migrations/     00001_init.sql (+ triggers d'immutabilité)
sqlc.yaml
```

## Questions ouvertes (à trancher par l'humain avant exécution)

- ~~**Q1 — Emplacement de `Money`**~~ — **tranché le 2026-11-02 par Arthur.presle : `internal/money`**,
  package transverse sans dépendance vers les domaines. À mentionner dans AGENTS §2 lors de T3.
- ~~**Q2 — Périmètre des tables**~~ — **tranché le 2026-11-02 par Arthur.presle** : `suppliers`, `parts`,
  `offers`, `price_quotes`, `fx_quotes`, `price_snapshots` (+ tables de liaison selon ADR 0002), `boms`,
  `bom_nodes`. Les contrats/remises, budgets et snapshots de BOM restent pour leurs tickets.
- ~~**Q3 — Arrondi**~~ — **tranché le 2026-11-02 par Arthur.presle : half-even (arrondi bancaire)**, aux
  unités mineures ISO 4217 (EUR/USD 2, JPY 0…). Règle de calcul absente de la spec → consignée dans
  l'ADR 0004 (T3).

## Requirements

### Must Have
- R1 : ADR 0002 « où vit le snapshot de prix » rédigée (statut `Proposé`), soumise aux 3.
- R2 : ADR 0003 « canal temps réel (SSE ou polling) » rédigée (statut `Proposé`), soumise aux 3.
- R3 : type `Money` (décimal + devise ISO 4217) ; `Add` de deux devises différentes renvoie
  `ErrCurrencyMismatch` ; aucun arrondi dans les opérations, arrondi explicite aux unités mineures
  uniquement via une fonction de sortie.
- R4 : migration goose initiale ; `make migrate` crée les tables sur une base vide ; trigger d'immutabilité
  sur cotations et snapshots de prix.
- R5 : sqlc configuré (`sqlc.yaml`, `tool` dans `go.mod`) ; la génération ne produit aucune différence avec
  le code commité, vérifié en CI.
- R6 : interfaces `catalog` et `pricing` définies dans leur domaine, avec un stub en mémoire chacune ;
  `bom` compile en les utilisant.
- R7 : CI verte sur GitHub, avec un service Postgres qui joue `goose up` et la vérification sqlc.

### Should Have
- S1 : test d'intégration qui prouve que le trigger refuse un `UPDATE` sur une cotation (base réelle en CI).
- S2 : sérialisation JSON de `Money` en chaîne (`"92.00"`) + devise, jamais `number`.

### Won't Do
- Implémenter le simulateur de prix (live-price-feed) ou le streaming temps réel lui-même : seule la
  décision (ADR) est dans ce ticket.
- Handlers HTTP / endpoints `/api/v1` métier.
- Conversion de devises (multi-currency-costing) au-delà de ce que `Money` exige.
- Calcul des totaux de BOM, déplacements de nœuds, détection de cycles.
- Tables contrats/remises, budgets, snapshots de BOM.
- UI.

## Tasks

- [x] T1 : Brouillon ADR 0002 — où vit le snapshot de prix
  - **Files**: `docs/adr/0002-stockage-snapshot-de-prix.md`
  - **Approach**: options comparées (ex. table `price_snapshots` + liaison vers les cotations existantes
    immuables ; copie des valeurs dans le snapshot ; snapshot porté par le domaine `snapshot` vs `pricing`),
    impact pivot multi-source de prix. Statut `Proposé`.
  - **Done when**: fichier présent au format §8 ; l'humain l'a passé à `Accepté` avec les 3 noms (bloquant pour T4).

- [x] T2 : Brouillon ADR 0003 — canal temps réel
  - **Files**: `docs/adr/0003-canal-temps-reel.md`
  - **Approach**: options SSE / polling / WebSocket / ne rien faire, au regard de la cadence (~6,6 s par
    jour simulé), du proxy Vite, du pivot hors-ligne. Statut `Proposé`.
  - **Done when**: fichier présent au format §8 ; acceptée par les 3.

- [x] T3 : Type `Money` et ses tests
  - **Files**: `internal/money/money.go`, `internal/money/money_test.go`, `go.mod`, `go.sum`,
    `docs/adr/0004-arrondi-montants.md`, `AGENTS.md` (§2 : mention de `internal/money`)
  - **Approach**: `type Money struct{ amount decimal.Decimal; currency Currency }`, constructeur validant le
    code ISO, `Add` → `ErrCurrencyMismatch`, `Mul` par quantité, `Round()` half-even aux unités mineures
    ISO 4217 (`RoundBank`), seule fonction qui arrondit ; JSON en chaîne. Dépendance `shopspring/decimal`
    passée en direct (accord §11). ADR 0004 (statut `Proposé`) documente le choix half-even.
  - **Done when**: tests table-driven verts, dont EUR+USD → `errors.Is(err, ErrCurrencyMismatch)`,
    `0.005 + 0.005` non arrondi avant `Round`, `0.125 EUR → 0.12`, `0.135 EUR → 0.14`, `100.5 JPY → 100`.

- [x] T4 : Migration initiale + triggers d'immutabilité
  - **Files**: `migrations/00001_init.sql`
  - **Approach**: tables de Q2 avec `NUMERIC`/`CHAR(3)`, `bom_nodes.parent_id` + index unique « une racine
    par BOM », triggers `BEFORE UPDATE OR DELETE` sur `price_quotes`, `fx_quotes`, `price_snapshots`
    selon ADR 0002.
  - **Done when**: `make migrate` sur base vide passe ; `goose down` puis `up` passe.

- [x] T5 : sqlc — configuration, requêtes de base, code généré
  - **Files**: `sqlc.yaml`, `internal/<domaine>/queries.sql` (ou `queries/`), code généré, `go.mod`
    (`tool github.com/sqlc-dev/sqlc/cmd/sqlc`, `pgx/v5` en direct), `Makefile` (cible `sqlc`)
  - **Approach**: sqlc lit le schéma depuis `migrations/`, override `numeric` → `decimal.Decimal` ;
    quelques requêtes minimales (insert/get pièce, insert cotation) pour que le code généré soit utilisé.
  - **Done when**: `go tool sqlc generate && git diff --exit-code` propre ; `go build ./...` passe.

- [x] T6 : Interfaces `catalog` et `pricing` + stubs, `bom` les consomme
  - **Files**: `internal/catalog/catalog.go`, `internal/catalog/stub.go`, `internal/pricing/pricing.go`,
    `internal/pricing/stub.go`, `internal/bom/service.go`, tests associés
  - **Approach**: `catalog.Catalog` (Part, Offer par id) ; `pricing.Pricing` (prix d'une offre, taux d'une
    paire, renvoyant `money.Money` + source + horodatage, erreurs `ErrNoQuote`/`ErrNoRate`) ; stubs en mémoire
    déterministes ; `bom.Service` reçoit les deux interfaces par constructeur.
  - **Done when**: `go build ./...` passe ; test qui vérifie qu'un stub sans cotation renvoie `ErrNoQuote`
    (pas de prix à zéro) ; `bom` n'importe ni pgx ni `net/http`.

- [ ] T7 : CI — Postgres, migrations et contrôle sqlc
  - **Files**: `.github/workflows/ci.yml`, `docs/workflow.md` si besoin
  - **Approach**: service `postgres:18-alpine` dans le job `go`, étape `goose up`, étape
    `sqlc generate` + `git diff --exit-code` ; test d'intégration du trigger (S1) derrière `DATABASE_URL`
    (skip propre si absente en local).
  - **Done when**: CI verte sur la PR GitHub ; un `UPDATE` sur une cotation fait échouer le test d'intégration
    si le trigger est retiré.

## Verification

### Automated Checks
- `make lint`
- `make test`
- `make migrate` (base vide via `docker compose`)
- `go tool sqlc generate && git diff --exit-code`
- CI GitHub Actions verte

### Manual Checks
- Les 3 membres relisent et acceptent ADR 0002 et 0003 (statut + `Décideurs`).
- `psql` : tenter un `UPDATE price_quotes ...` → refusé.

### Acceptance Criteria
- [ ] ADR « où vit le snapshot de prix » acceptée par les 3 (R1)
- [ ] ADR « canal temps réel (SSE ou polling) » acceptée par les 3 (R2)
- [ ] `Money` refuse EUR + USD avec `ErrCurrencyMismatch` et n'arrondit qu'en sortie (R3)
- [ ] `make migrate` crée les tables, et sqlc génère le code sans différence (R4, R5)
- [ ] Interfaces `catalog` et `pricing` avec stubs ; `bom` compile en les utilisant (R6)
- [ ] La CI tourne et passe au vert sur GitHub (R7)
