---
id: nlpf-70
status: review
branch: nlpf-70
current_task: null
started: 2026-11-02
updated: 2026-11-02
---

# State: Socle commun (les 3 ensemble)

## Progress

All tasks completed. Ready for verification.
- Tasks: 7/7 completed
- Branch: `nlpf-70`
- Commits: 15 sur la branche (dont suivi)
- Reste à faire côté humain : pousser la branche et ouvrir la PR (CI GitHub jamais exécutée sur ce
  workflow) ; confirmer l'acceptation des ADR 0002/0003 par les 3 membres (un seul nom inscrit).

## Task Log

### T1: Brouillon ADR 0002 — où vit le snapshot de prix
- **Status**: completed (brouillon) — acceptation humaine en attente
- **Time**: 2026-11-02
- **Commit**: 3939deb
- **Notes**: ADR au statut `Proposé` : option 1 (en-tête `price_snapshots` + liaisons vers les cotations
  immuables), portée par `pricing`. La partie « acceptée par les 3 » du Done when reste à faire par
  l'équipe ; elle bloque T4.

### T2: Brouillon ADR 0003 — canal temps réel
- **Status**: completed (brouillon) — acceptation humaine en attente
- **Time**: 2026-11-02
- **Commit**: 6926ff4
- **Notes**: ADR au statut `Proposé` : SSE (stdlib) portant un simple signal de tick ; l'UI relit les
  totaux en REST. Repli polling documenté. Acceptation par les 3 à faire.

### T3: Type `Money` et ses tests
- **Status**: completed
- **Time**: 2026-11-02
- **Commit**: 6146d5e
- **Notes**: `internal/money` (Money, Currency, ErrCurrencyMismatch, ErrUnknownCurrency, ErrInvalidAmount,
  Add, Mul(decimal), Round half-even, JSON en chaîne). Tests table-driven verts (`-race`), lint 0 issue.
  Ajout au test de `0.005 + 0.002 = 0.007` : le cas `0.005 + 0.005` de la spec ne peut pas détecter un
  arrondi prématuré (0.01 dans les deux cas). ADR 0004 au statut `Proposé`. AGENTS §2 mentionne
  `internal/money`. `shopspring/decimal` en direct.

### T4: Migration initiale + triggers d'immutabilité
- **Status**: completed
- **Time**: 2026-11-02
- **Commit**: 28ae721
- **Notes**: `migrations/00001_init.sql`. `make migrate`, `goose down` puis `up` passent sur Postgres 18 local.
  Vérifié à la main (transaction annulée) : UPDATE/DELETE/TRUNCATE refusés sur `price_quotes` et
  `price_snapshots` ; prix mini > maxi refusé ; deuxième racine refusée ; offre d'une autre pièce refusée ;
  supprimer la racine supprime le sous-arbre. Choix de schéma notés dans Decisions. Le test automatique du
  trigger est prévu en T7 (S1).

### T5: sqlc — configuration, requêtes de base, code généré
- **Status**: completed
- **Time**: 2026-11-02
- **Commit**: 8609d2b
- **Notes**: `sqlc.yaml` (schéma = `migrations/`), sqlc v1.31.1 en `tool` (build wasm, pas de cgo), pgx/v5 en
  direct. Code généré par domaine : `internal/catalog/catalogdb`, `internal/pricing/pricingdb`. Overrides
  numeric → `decimal.Decimal` (NullDecimal si nullable), timestamptz → `time.Time`. `sqlc generate` puis
  `git diff --exit-code` propre ; `go build`, `go mod tidy -diff`, `make lint` (0 issue), `make test` OK.
  Aller-retour exact d'un `NUMERIC` à 29 chiffres via le code généré vérifié contre Postgres local
  (test jetable, non commité : le test d'intégration durable est en T7). Cible `make sqlc` ajoutée, citée
  dans AGENTS §1.

### T6: Interfaces `catalog` et `pricing` + stubs, `bom` les consomme
- **Status**: completed
- **Time**: 2026-11-02
- **Commit**: 458545f
- **Notes**: `catalog.Catalog` (Part, Offer ; ErrPartNotFound/ErrOfferNotFound), `pricing.Pricing`
  (OfferPrice → Quote{Price money.Money, Source, QuotedAt} ; Rate → Rate{From, To, decimal, Source, QuotedAt} ;
  ErrNoQuote/ErrNoRate). Stubs en mémoire : la cotation la plus récente gagne, à égalité la dernière de la
  liste. `bom.Service` + `PricedOffer` (offre + cotation) pour consommer réellement les interfaces.
  Tests table-driven ; `TestDomainHasNoForbiddenDependency` (`go list -deps`) refuse net/http et pgx dans
  le graphe de `bom`. Mutations vérifiées : import net/http dans catalog → test rouge ; stub qui renvoie une
  cotation vide au lieu d'ErrNoQuote → tests pricing et bom rouges.

### T7: CI — Postgres, migrations et contrôle sqlc
- **Status**: completed (local) — CI GitHub non vérifiée
- **Time**: 2026-11-02
- **Commit**: d1e45bb
- **Notes**: job `go` : service `postgres:18-alpine`, `DATABASE_URL` au niveau du job, étape sqlc
  (`generate` + `git diff --exit-code` + aucun fichier non suivi), étape goose `up` / `reset` / `up`.
  `internal/pricing/immutability_integration_test.go` : 10 cas (UPDATE/DELETE/TRUNCATE sur cotations,
  taux, snapshots et liens) attendent SQLSTATE 23001, dans une transaction annulée (aucune donnée
  laissée). Sauté si `DATABASE_URL` absente ou base injoignable en local ; **échoue** si `CI` est défini.
  Mutation vérifiée : trigger `price_quotes_immutable` retiré → sous-tests UPDATE/DELETE rouges.
  Étapes CI rejouées en local (tidy, sqlc, goose up/reset/up, lint 0 issue, test -race, govulncheck,
  npm audit) : vertes. Le Done when « CI verte sur la PR GitHub » demande un push + PR, que l'agent ne
  fait pas sans accord (AGENTS §11). `docs/workflow.md` §7 et AGENTS §16 mis à jour.

## Decisions

- 2026-11-02 — Q1 : `Money` vit dans `internal/money` (package transverse). Décideur : Arthur.presle.
- 2026-11-02 — Q2 : tables initiales = suppliers, parts, offers, price_quotes, fx_quotes, price_snapshots
  (+ liaisons selon ADR 0002), boms, bom_nodes. Décideur : Arthur.presle.
- 2026-11-02 — Q3 : arrondi half-even aux unités mineures ISO 4217, en sortie uniquement ; ADR 0004 à
  rédiger dans T3. Décideur : Arthur.presle.

- 2026-11-02 — Accord §11 : ajout de shopspring/decimal et pgx/v5 (directs) et sqlc (tool) approuvé.
  Décideur : Arthur.presle.
- 2026-11-02 — `.specs/CLAUDE.md` absent : exécution menée avec les règles d'`AGENTS.md` seules.
- 2026-11-02 — T1/T2 cochées une fois le brouillon livré ; la partie « acceptée par les 3 » de leur
  Done when reste suivie dans les critères d'acceptation et le blocker T4.
- 2026-11-02 — Devises : table fermée de devises prises en charge dans `internal/money` ; code inconnu =
  `ErrUnknownCurrency` (pas de 2 décimales par défaut). Consigné dans ADR 0004.
- 2026-11-02 — `Mul` prend un `decimal.Decimal` (quantités non entières possibles), pas un entier.
- 2026-11-02 — T4, schéma : `parts` réduit à nom/catégorie/zone (attributs masse, CO2, coûts… laissés au
  ticket part-catalog, par une nouvelle migration). Pas d'`archived_at` ni de note de qualité (idem).
- 2026-11-02 — T4 : `price_snapshot_fx` impose aussi un seul taux par paire et par snapshot (l'ADR 0002 ne
  le dit que pour les offres) ; même mécanisme de clé composite.
- 2026-11-02 — T4 : `price_quotes` unique sur `(offer_id, source, quoted_at)` (idem `fx_quotes` par paire)
  pour empêcher la duplication de l'historique au redémarrage du simulateur.
- 2026-11-02 — T4 : triggers aussi sur TRUNCATE. Limite connue : rien n'empêche encore d'INSÉRER un lien
  dans un snapshot de prix déjà existant ; c'est au service `pricing` de créer en-tête et liens dans une
  seule transaction.
- 2026-11-02 — T5 : code sqlc généré dans un sous-package par domaine (`internal/<domaine>/<domaine>db`)
  plutôt qu'un `internal/db` commun : AGENTS §2 dit que chaque domaine porte son accès aux données, et le
  package métier (`catalog`, `pricing`) n'importe pas pgx. Les modèles générés dupliquent les structs de
  toutes les tables dans chaque sous-package (comportement de sqlc) ; à revoir si gênant.
- 2026-11-02 — T6 : `pricing.Rate` ne renvoie que la paire exacte (pas d'inversion) ; l'inversion
  EUR → USD = 1 ÷ USD → EUR relève de multi-currency-costing.
- 2026-11-02 — T6 : ajout de `bom.Service.PricedOffer` (non demandé explicitement) pour que `bom` utilise
  les interfaces et porte le test « pas de cotation = erreur nommée ». Aucun calcul de total.
- 2026-11-02 — T7 : le `Makefile` exporte toujours `DATABASE_URL`, donc « skip si absente » ne suffisait
  pas pour `make test` sans base ; le test saute aussi si la base est injoignable, sauf en CI (`CI` défini)
  où il échoue, pour qu'une base absente ne passe jamais pour un succès.

## Blockers

### T4: Migration initiale + triggers d'immutabilité — levé le 2026-11-02
- ADR 0002 passée à `Accepté` (Décideurs : Arthur.presle). Note : la spec demandait « les 3 noms » ;
  un seul nom figure. Le critère d'acceptation R1/R2 « acceptée par les 3 » reste à confirmer par l'équipe.

## Verification Report(s)
