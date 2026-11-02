# AGENTS.md — règles du projet BOM Studio

Source unique des règles pour **toute l'équipe et tous les agents IA** (Claude, Codex, Copilot, Cursor,
Gemini…). Les fichiers propres à un outil (`CLAUDE.md`, `GEMINI.md`…) ne font que pointer ici :
on ne duplique aucune règle ailleurs.

Outil de conception et de pilotage de la BOM d'une voiture. Spec fonctionnelle : `docs/features.md`.
Les features s'implémentent une par une.

Principe du cours : **l'IA produit, l'humain garantit.** La responsabilité ne se délègue pas au prompt.

---

## 1. Environnement

- Tout se lance **dans WSL (Ubuntu)** : Go, make, Docker, Node y sont installés, pas côté Windows.
- `make run | test | cover | sec | lint | migrate | dev` (voir `Makefile`). Les outils Go (goose, golangci-lint, govulncheck, puis sqlc)
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

---

# Travailler avec des agents IA

## 6. L'humain décide, l'agent propose

- L'agent **cadre, propose, implémente et vérifie** ; l'humain **choisit les compromis et assume**.
- Toute décision d'architecture (§7) est **soumise à l'humain avant d'écrire du code** :
  l'agent s'arrête, présente les options, et attend une réponse explicite.
- Spec ambiguë ou cas non couvert : l'agent **pose la question** plutôt que d'inventer une règle,
  en citant la ligne de `docs/features.md` concernée.
- L'agent ne tranche jamais un désaccord dans l'équipe : il expose les arguments de chaque côté.

## 7. Qu'est-ce qu'une décision d'architecture ?

Tout ce qui est **coûteux à défaire** :

- ajouter une dépendance (Go ou npm) ou un service (cache, file de messages…) ;
- créer, fusionner ou redécouper un domaine ou une frontière (domaine / adapters) ;
- changer le schéma d'une table partagée entre domaines, ou le modèle des snapshots / cotations ;
- changer un contrat d'API `/api/v1` de façon non rétrocompatible ;
- choisir une stratégie transverse : cache, synchronisation / hors-ligne, temps réel (SSE, WebSocket),
  pannes de l'API de prix, feature toggles, versionnement des facteurs carbone ;
- toute règle de calcul absente de la spec (arrondi, ordre d'application des remises…).

Ne l'est pas : un nom de variable, un refactor local, un test en plus, un composant UI isolé.

## 8. ADR (Architecture Decision Records)

- Emplacement : `docs/adr/NNNN-titre-en-kebab.md` (numérotation continue, `0001`, `0002`…).
- L'agent **rédige le brouillon** au statut `Proposé` ; **seul un humain** le passe à `Accepté`
  et inscrit son nom dans `Décideurs`.
- Une ADR acceptée **ne se réécrit pas** : on en crée une nouvelle qui la remplace (`Remplacée par 00XX`).
- Une page max. Format :

```markdown
# NNNN — Titre de la décision
Statut : Proposé | Accepté | Remplacé par NNNN — Date : AAAA-MM-JJ — Décideurs : <noms>

## Contexte
Le besoin, la contrainte ou le risque (lien vers la feature / l'invariant).

## Options
1. Option A — avantages / inconvénients
2. Option B — avantages / inconvénients
(au moins deux, dont « ne rien faire » si pertinent)

## Décision
L'option retenue, en une phrase, et pourquoi elle gagne **dans ce contexte**.

## Conséquences
Ce que ça rend facile, ce que ça rend difficile, comment on reviendrait en arrière.
Impact si le pivot arrive (hors-ligne, multi-source de prix, carbone versionné, multi-usine).
```

- Le code qui implémente une ADR la référence (message de commit ou commentaire : `voir ADR 0003`).

## 9. Pas de code orphelin

*Si l'équipe ne peut pas l'expliquer, ce code ne lui appartient pas.* À chaque livraison, l'agent fournit :

1. **ce que fait le changement** et la responsabilité de chaque fichier touché ;
2. **un cas de panne prévisible** (entrée, état ou dépendance qui le ferait échouer) ;
3. **le test qui le met en défaut** (ou qui prouve qu'il résiste à ce cas) ;
4. **ce qui n'est pas fait ou pas vérifié**, dit honnêtement.

Du code simple qu'un humain modifie sans nouveau prompt vaut mieux que du code « malin ».

## 10. Petits pas vérifiables (Test & Learn)

- Hypothèse → prototype → test → mesure → décision. Une erreur doit être **rapide, observable, réversible**.
- Livrer des **tranches verticales** (de la BOM au résultat expliqué) plutôt qu'une couche complète sans usage.
- Un commit = une intention. Pas de commit géant mélangeant refactor, feature et formatage.
- Avant de modifier un comportement existant : **un test qui protège l'existant d'abord**, puis le changement.

## 11. Ce que l'agent ne fait jamais sans accord explicite

- pousser, merger, ouvrir une PR, réécrire l'historique git ;
- ajouter une dépendance ;
- modifier une migration déjà commitée, une ADR acceptée, ou `docs/features.md` ;
- désactiver, supprimer ou affaiblir un test (ou un job de CI) pour faire passer la pipeline ;
- toucher aux données d'un snapshot publié (même en base de dev).

## 12. Préparer le pivot sans l'anticiper

- Le pivot (hors-ligne strict, multiples sources de prix, carbone versionné, multi-usine / multi-devise)
  sera imposé après la soutenance 1. On **ne code pas** ces cas « au cas où », mais on **ne ferme pas
  la porte** : frontières nettes, pas de dépendance à l'heure ou au réseau dans le domaine, données versionnées.
- Au moment du pivot : **cartographier l'impact avant de coder** (fichiers, ADR concernées, tests à ajouter),
  proposer à l'humain, migrer progressivement.

## 13. Équipe multi-IA

- **Mêmes règles quel que soit l'outil** : ce fichier fait foi. Un fichier propre à un outil contient
  seulement un renvoi vers `AGENTS.md` (ex. `CLAUDE.md` = `@AGENTS.md`).
  Codex, Copilot et Cursor lisent `AGENTS.md` directement ; pour Gemini CLI, créer `GEMINI.md` avec `@AGENTS.md`.
- Préférences personnelles (modèle, style, raccourcis) : fichiers locaux non commités
  (`CLAUDE.local.md`, réglages de l'éditeur…), jamais dans les règles partagées.
- **Traçabilité** : un commit écrit avec l'aide d'une IA le dit dans un trailer
  (`Co-Authored-By: <outil / modèle> <adresse>`). L'humain qui commite reste l'auteur et le responsable.
- Une IA peut relire le code d'une autre (ou d'un humain) ; la relecture finale avant merge est **humaine**.
- Un agent ne reprend pas en cours la branche d'un autre sans que l'humain propriétaire soit d'accord.

## 14. Ownership partagé

- L'agent écrit pour **toute l'équipe**, pas pour la personne qui l'utilise : noms explicites,
  ADR et commits qui racontent le pourquoi.
- Le dépôt doit raconter la même histoire que la soutenance : chaque choix important se retrouve
  dans une ADR, chaque règle métier dans un test.

## 15. Définition de « terminé »

Une tâche n'est terminée que si :

- `make lint` et `make test` passent en local, et la CI est verte ;
- le livrable du §9 est fourni ;
- une ADR existe si une décision du §7 a été prise.

Sinon l'agent le dit, avec la sortie de la commande qui échoue.

## 16. Pipeline CI

`.github/workflows/ci.yml`, sur chaque push vers `main` et chaque PR (voir ADR 0001, `docs/workflow.md`) :

- **go** : `go mod tidy -diff`, `go vet`, `golangci-lint` (dont `gosec`), `go test -race` + couverture (non bloquante) ;
- **web** : `npm ci`, `oxlint`, build (`tsc` + Vite), `npm audit` ;
- **security** : `govulncheck`.

Une PR ne se merge pas si la CI est rouge. On ajoute une étape quand le besoin existe
(ex. service Postgres + `goose up` dès le premier test qui touche la base).
