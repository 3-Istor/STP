# BOM Studio — règles du projet

Outil de conception et de pilotage de la BOM d'une voiture. Spec fonctionnelle : `docs/features.md`.
Les features s'implémentent une par une via `/spec-plan`.

## Environnement

- Tout se lance **dans WSL (Ubuntu)** : Go, make, Docker, Node y sont installés, pas côté Windows.
- `make run | test | lint | migrate | dev` (voir `Makefile`). Les outils Go (goose, golangci-lint, puis sqlc)
  sont déclarés en `tool` dans `go.mod` et lancés par `go tool <nom>` : rien à installer globalement.
- Front : `npm ci` (pas `npm install`, qui plante avec le npm 10 du système ; pour ajouter une dépendance :
  `npx npm@11 install <pkg>`).

## Architecture

- `cmd/server` : câblage uniquement (config par variables d'env, dépendances, routes, démarrage).
- `internal/httpapi` : routeur `net/http` standard (patterns `GET /api/v1/parts/{id}`), handlers fins.
- `internal/<domaine>` : `catalog`, `bom`, `supplier`, `pricing`, `snapshot`. Chaque domaine porte ses types,
  son service (règles métier) et son accès aux données.
- Base : PostgreSQL via pgx, requêtes typées générées par sqlc, migrations SQL goose dans `migrations/`.
  Jamais de modification d'une migration déjà commitée : on en ajoute une nouvelle.
- `web/` : React + TypeScript + Vite, proxy `/api` → `:8080` en dev. Three.js arrivera avec la vue 3D.
- Pas de dépendance ajoutée sans raison : stdlib d'abord (`net/http`, `log/slog`, `testing`).

## Règles métier non négociables

### Montants
- **Jamais de `float`** (Go, SQL ou TS) pour un montant, un taux de change ou un prix.
- Un montant est toujours un couple **(valeur décimale, devise ISO 4217)** : `NUMERIC` + `CHAR(3)` en base,
  type décimal en Go (`shopspring/decimal`), chaîne côté JSON/TS (`"92.00"`), jamais `number`.
- On conserve le montant et la devise d'origine ; la conversion se fait au calcul, avec un taux daté.
- Arrondi explicite aux unités mineures de la devise, uniquement en sortie (affichage, publication).
- Additionner deux devises différentes sans conversion est une erreur, pas un cas silencieux.

### BOM = arbre
- Nœuds avec `parent_id` (racine : `parent_id NULL`), une seule racine par BOM.
- **Aucun cycle** : tout déplacement vérifie que le nouveau parent n'est pas un descendant du nœud
  (CTE récursive, dans la même transaction que la mise à jour).
- Les totaux (coût, CO2, délai max…) sont calculés en Go en remontant l'arbre, pas stockés.

### Brouillon vs snapshot
- Un brouillon se modifie librement.
- Publier crée un **snapshot immuable** : structure, quantités, réglages **et** prix fournisseurs et taux de
  change utilisés, copiés au moment de la publication.
- Un snapshot publié **ne se modifie jamais** : refus côté service **et** côté base (trigger qui rejette
  `UPDATE`/`DELETE`). Pour changer, on crée un nouveau brouillon depuis le snapshot.

### IA
- Tout appel IA passe par une **interface Go** définie par le domaine qui l'utilise ;
  aucun SDK de fournisseur importé hors de l'implémentation de cette interface.
- Une proposition de l'IA ne s'applique qu'à un brouillon, après confirmation de l'utilisateur.

## Conventions de code

- Tests **table-driven** (`tests := []struct{...}` + `t.Run`). Tout calcul métier (monnaie, arbre, remises,
  budgets) a ses tests.
- Erreurs enveloppées avec contexte : `fmt.Errorf("move node %d: %w", id, err)`. Erreurs métier en
  sentinelles (`var ErrCycle = errors.New(...)`), testées avec `errors.Is`.
- **Pas de logique métier dans les handlers HTTP** : décoder → appeler un service → encoder.
  Les handlers traduisent les erreurs métier en codes HTTP.
- API JSON : préfixe `/api/v1`, champs en `snake_case`, erreurs `{"error": "message explicite"}`.

## UI

- Claire, peu d'informations à l'écran ; **détails à la demande** (panneau latéral, survol).
- Code couleur unique : vert = dans l'objectif, orange = proche, rouge = dépassé
  (variables CSS `--ok`, `--warn`, `--over`).
- Les actions courantes (ajouter, déplacer, supprimer une pièce) en un ou deux gestes.
- Montants affichés avec leur devise, formatés par `Intl.NumberFormat`, jamais recalculés en `number`.
