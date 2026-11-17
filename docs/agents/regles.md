# Règles du projet — produit, métier, code (AGENTS §1 à §5)

Texte d'origine d'`AGENTS.md`, déplacé tel quel quand `AGENTS.md` est devenu une carte (ADR 0040).
Les renvois « AGENTS §N » du code et des ADR pointent ici. Modifier ce fichier = niveau de risque 3.

## 1. Environnement

- Tout se lance **dans WSL (Ubuntu)** : Go, make, Docker, Node y sont installés, pas côté Windows.
- `make run | test | cover | sec | lint | migrate | sqlc | dev` (voir `Makefile`). Les outils Go (goose, golangci-lint, govulncheck, puis sqlc)
  sont déclarés en `tool` dans `go.mod` et lancés par `go tool <nom>` : rien à installer globalement.
- Front : `npm ci` (pas `npm install`, qui plante avec le npm 10 du système ; pour ajouter une dépendance :
  `npx npm@11 install <pkg>`).
- CI : `.github/workflows/ci.yml` rejoue lint, tests et build à chaque push et PR (voir §16).

## 2. Architecture

- `cmd/server` : câblage uniquement (config par variables d'env, dépendances, routes, démarrage).
- `internal/httpapi` : routeur `net/http` standard (patterns `GET /api/v1/parts/{id}`), handlers fins.
- `internal/<domaine>` : `catalog`, `bom`, `supplier`, `pricing`, `snapshot`. Chaque domaine porte ses types,
  son service (règles métier) et son accès aux données.
- `internal/money` : type transverse `Money` (montant décimal + devise ISO 4217, arrondi half-even en sortie,
  voir ADR 0004). Utilisé par tous les domaines, il ne dépend d'aucun d'eux.
- Base : PostgreSQL via pgx, requêtes typées générées par sqlc, migrations SQL goose dans `migrations/`.
  Jamais de modification d'une migration déjà commitée : on en ajoute une nouvelle.
- `web/` : React + TypeScript + Vite, proxy `/api` → `:8080` en dev. Three.js arrivera avec la vue 3D.
- Pas de dépendance ajoutée sans raison : stdlib d'abord (`net/http`, `log/slog`, `testing`).
- **Le domaine ne connaît ni écran, ni réseau, ni fournisseur de prix** : `internal/<domaine>` ne dépend
  pas de `net/http`, du front, ni d'un client d'API externe. L'API de prix (simulée ou réelle) passe par
  un **adapter** qui valide et normalise (devise, bornes, horodatage).
- Face à l'API de prix : timeout → dernier snapshot valide ; prix aberrant → rejet ou quarantaine,
  journalisé ; recalcul → on sait quelle version a été utilisée.

## 3. Règles métier non négociables

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

### Résultats explicables et reproductibles
- **Même entrée + même snapshot = même résultat.** Aucun calcul ne dépend de l'heure courante,
  d'un ordre de map Go ou d'un aléa non semé.
- Tout résultat (coût, CO2, délai) peut dire **sur quoi il repose** : version de BOM, snapshot de prix
  ou « en direct », taux utilisés et leur date, devise, règles appliquées (remises…).
- Une donnée manquante n'est jamais cachée : pas de prix à zéro, pas de conversion ignorée,
  une erreur nommée à la place.

### IA dans le produit
- Tout appel IA passe par une **interface Go** définie par le domaine qui l'utilise ;
  aucun SDK de fournisseur importé hors de l'implémentation de cette interface.
- Une proposition de l'IA ne s'applique qu'à un brouillon, après confirmation de l'utilisateur.

## 4. Conventions de code

- Tests **table-driven** (`tests := []struct{...}` + `t.Run`). Tout calcul métier (monnaie, arbre, remises,
  budgets) a ses tests. **Pas de couverture décorative** : un test qui ne peut pas échouer n'est pas un test.
- Erreurs enveloppées avec contexte : `fmt.Errorf("move node %d: %w", id, err)`. Erreurs métier en
  sentinelles (`var ErrCycle = errors.New(...)`), testées avec `errors.Is`.
- **Pas de logique métier dans les handlers HTTP** : décoder → appeler un service → encoder.
  Les handlers traduisent les erreurs métier en codes HTTP.
- API JSON : préfixe `/api/v1`, champs en `snake_case`, erreurs `{"error": "message explicite"}`.
- Logs via `log/slog` (structurés, avec contexte) ; jamais de `fmt.Println` / `console.log` laissé dans le code.
- Une nouvelle variante de calcul (`prix.v2`, `carbone.v2`) est une **stratégie** choisie à un seul endroit,
  pas des `if` éparpillés. Tout feature toggle a un propriétaire, une raison et une **date de suppression**.

## 5. UI

- Claire, peu d'informations à l'écran ; **détails à la demande** (panneau latéral, survol).
- Code couleur unique : vert = dans l'objectif, orange = proche, rouge = dépassé
  (variables CSS `--ok`, `--warn`, `--over`).
- Les actions courantes (ajouter, déplacer, supprimer une pièce) en un ou deux gestes.
- Montants affichés avec leur devise, formatés par `Intl.NumberFormat`, jamais recalculés en `number`.
