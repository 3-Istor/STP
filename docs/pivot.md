# Le pivot : backend en Go, réseau autorisé

Ce document dit ce que le pivot imposé après la soutenance 1 a changé, ce que l'équipe a décidé
et ce que cela a coûté. Chaque affirmation renvoie à une preuve du dépôt. Chiffres relevés sur
`main` le 2026-11-10.

## 1. Ce qui a été imposé

| | Soutenance 1 | Après le pivot |
|---|---|---|
| Langage du backend | Python | **Go ou Rust**, au choix de l'équipe |
| Réseau | l'application devait fonctionner **hors ligne** | l'application **peut utiliser internet** |

Deux contraintes bougent en même temps : le langage est restreint, le hors-ligne est levé.

## 2. Ce que l'équipe a décidé

**Go, et une réécriture complète.** Aucun fichier du système Python n'a été repris : ce dépôt démarre
sur un squelette Go (commit `18a4751`). Ce qui a été gardé, ce sont les idées et la spec
(`docs/features.md`), pas le code.

**Pourquoi Go plutôt que Rust.** Le code est en grande partie produit avec des agents IA
(127 commits sur 135 portent un trailer `Co-Authored-By`). Le critère retenu est donc la relecture :
du code Go généré se lit, s'audite et se corrige à la main plus facilement que du code Rust généré.
C'est la règle du projet : « du code simple qu'un humain modifie sans nouveau prompt vaut mieux que
du code malin » (`AGENTS.md` §9).

**Ce à quoi on renonce.** Rust aurait vérifié davantage de choses à la compilation. En Go, ces
garanties sont reportées sur l'outillage, rejoué à chaque PR (`.github/workflows/ci.yml`, ADR 0001) :
`go vet`, `golangci-lint` avec `gosec`, `govulncheck`, `go test -race`, requêtes SQL typées par sqlc.

## 3. Cartographie de l'impact

| Sujet | Effet du pivot | Preuve |
|---|---|---|
| Backend | Réécrit en Go, stdlib d'abord (`net/http`, `log/slog`, `testing`) ; 2 dépendances directes d'exécution (`pgx`, `shopspring/decimal`) | `go.mod`, `AGENTS.md` §2 |
| Stockage | Le hors-ligne étant levé, les données vivent dans un PostgreSQL serveur, plus sur le poste | `compose.yaml`, `migrations/` |
| Prix | L'API de prix est un processus séparé, appelé par HTTP ; en cas de panne, repli daté sur le dernier snapshot valide | ADR 0005, ADR 0015 |
| Temps réel | L'interface est prévenue des nouvelles cotations par SSE | ADR 0003 |
| Synchronisation | Pas de journal de synchronisation ni de résolution de conflits : sans objet une fois le hors-ligne levé | aucune ligne de code à ce sujet |
| Règles métier | Inchangées par le pivot : montants décimaux avec devise, snapshots immuables, résultat explicable et reproductible | `AGENTS.md` §3, `internal/money`, ADR 0004, 0006, 0016 |
| Frontend | React + TypeScript + Vite, séparé du backend | `web/` |

Le réseau est autorisé, mais le calcul n'en dépend pas : `internal/<domaine>` n'importe ni
`net/http` ni client d'API (`AGENTS.md` §2), et un snapshot publié se relit sans l'API de prix
(ADR 0002, 0006). Couper le conteneur `priceapi` ne casse donc ni la lecture ni le recalcul d'un
snapshot.

## 4. Comment la migration a été menée

Réécriture par tranches verticales, une PR par tranche, CI verte avant chaque merge
(`docs/workflow.md`) :

1. Règles du projet et pipeline CI avant tout code métier : `AGENTS.md`, ADR 0001 (PR 1).
2. Socle commun : `money`, snapshots de prix, canal temps réel (PR 7, ADR 0002 à 0004).
3. Toute l'application en un seul `docker compose` (PR 8, ADR 0005).
4. BOM : arbre éditable, totaux expliqués, snapshots immuables (PR 9, ADR 0006 à 0008).
5. Catalogue, fournisseurs, flux de prix et devises (PR 10, ADR 0009 à 0019).
6. Raccordement de la BOM au vrai catalogue et aux vrais prix, jeu de démonstration
   (PR 13, ADR 0020 et 0021).

Les étapes 4 et 5 ont été développées en parallèle par deux personnes et se contredisaient à la
fusion (migration et ADR en double, types incompatibles). L'ADR 0020 décrit le conflit et sa
résolution.

## 5. Coût réel

| Mesure | Valeur |
|---|---|
| Période | du 2026-10-19 au 2026-11-10, 9 jours avec au moins un commit |
| Commits sur `main` | 135, par 2 auteurs |
| Go écrit à la main, hors tests | environ 11 300 lignes |
| Tests Go | environ 9 600 lignes |
| Frontend TypeScript | environ 1 600 lignes |
| Migrations SQL | 6 |
| ADR | 21 |

Ce que la réécriture a coûté en plus du code : le système de la soutenance 1 n'est plus exécutable
dans ce dépôt, et toutes ses fonctionnalités n'ont pas été refaites (voir §6).

## 6. Ce qui n'est pas fait

- Les features de `docs/features.md` au-delà du catalogue, de la BOM, des prix et des snapshots ne
  sont pas implémentées (stock, délais, vue 3D, assistant IA…).
- Budgets cibles : évaluateurs testés, mais ni persistance ni écran (ADR 0018).
- L'ADR 0020 est encore au statut `Proposé`.
- Il n'existe pas d'ADR pour le choix de Go lui-même : la décision est consignée ici.

## 7. À compléter par l'équipe

Ces points ne se déduisent pas du dépôt :

- la date d'annonce du pivot et l'énoncé exact de l'enseignant ;
- la taille du système Python abandonné (lignes, fonctionnalités), pour chiffrer ce qui a été jeté ;
- ce qui a été appris du système de la soutenance 1 et réutilisé comme idée (modèle de BOM,
  snapshots, explication des résultats).
