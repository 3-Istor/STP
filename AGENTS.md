# AGENTS.md — carte du projet BOM Studio

Source unique des règles pour **toute l'équipe et tous les agents IA** (Claude, Codex, Copilot, Cursor,
Gemini…). `CLAUDE.md` et les autres fichiers d'outil ne font que pointer ici. Ce fichier est une **carte** :
le détail est dans les fichiers liés, à lire quand la tâche le demande (ADR 0040).

Outil de conception et de pilotage de la BOM d'une voiture. Spec : `docs/features.md`.
Principe : **l'IA produit, l'humain garantit.** La responsabilité ne se délègue pas au prompt.

## 1. Avant de commencer : le niveau de risque

Calculé sur le changement, jamais sur l'auteur ni sur la confiance de l'agent : `go run ./cmd/risk`
(`make risk`). Le plus élevé l'emporte ; on peut monter un niveau, jamais le descendre.

| Niveau | Quand (chemins) | Humain | Relecteurs IA (contexte neuf) |
|---|---|---|---|
| 0 Cosmétique | `docs/` hors ADR, `*.md`, CSS, captures | aucun en direct | — |
| 1 Routine | un seul domaine `internal/<d>/`, `web/src/`, tests ajoutés, fichier non classé | lit `.verify/verify-summary.md` | relecteur-code |
| 2 Sensible | `migrations/`, `internal/httpapi/` (contrat `/api/v1`), plusieurs domaines, `go.mod`, `web/package.json`, Docker, `internal/sampledata/`, `docs/adr/`, `docs/features.md` | valide le plan **avant** le code, puis le diff | + relecteur-architecture |
| 3 Critique | `internal/money/`, `internal/snapshot/`, calcul du coût (`internal/bom/cost*`, `totals*`, `bridge*`, `rootbudget*`), harnais (`.github/`, `.githooks/`, `.claude/`, `AGENTS.md`, `docs/agents/`, `cmd/risk/`, `internal/archtest/`, `scripts/`), test supprimé ou désactivé | 2 humains dont un CODEOWNER ; plan validé avant | + relecteur-securite, relecteur-metier |

La table vit dans `cmd/risk/rules.go` ; `TestRulesMatchAgentsDoc` échoue si ce fichier ne la suit plus.

## 2. Commandes

Tout se lance dans WSL (Ubuntu). Détail : `docs/agents/regles.md` §1.

| Commande | Rôle |
|---|---|
| `make hooks` | une fois : pre-commit = moulinette rapide, commit-msg = trailer IA |
| `make verify` | moulinette rapide (< 1 min) : gofmt, vet, architecture, contrat des routes, secrets, oxlint |
| `make verify LEVEL=pr` | = la CI en local (lint, tests -race, sqlc, tidy, govulncheck, build web) |
| `make verify LEVEL=nightly` | + fuzzing, mutation, secrets sur l'historique |
| `make run \| test \| lint \| migrate \| sqlc \| dev \| up` | voir `Makefile` |

Front : `npm ci` (jamais `npm install`). Une étape non lancée est `not_run`, jamais verte.

## 3. Carte du dépôt

- `cmd/server` câblage ; `cmd/priceapi` source de prix simulée ; `cmd/risk` niveau de risque.
- `internal/<domaine>` (`bom`, `catalog`, `supplier`, `pricing`, `snapshot`) : types, service, règles ;
  `*store` / `*db` = adapters PostgreSQL (sqlc). `internal/money` : `Money`, sans dépendance.
- `internal/httpapi` : handlers fins ; `testdata/routes.golden` fige le contrat des routes.
- `internal/archtest` : frontières vérifiées par test (domaine sans réseau, sans base, sans horloge, sans float).
- `migrations/` (goose), `web/` (React + TS + Vite), `docs/adr/` (décisions), `.specs/` (tickets).

## 4. Règles non négociables (détail et justification : `docs/agents/regles.md`)

- **Jamais de `float`** pour un montant, un taux, un prix : `(décimal, ISO 4217)`, chaîne en JSON.
- Arrondi half-even uniquement en sortie ; additionner deux devises sans conversion = erreur nommée.
- BOM = arbre sans cycle (CTE dans la même transaction) ; totaux calculés en Go, pas stockés.
- Snapshot publié **immuable** (service **et** trigger) ; on repart d'un nouveau brouillon.
- Même entrée + même snapshot = même résultat ; tout résultat dit sur quoi il repose ;
  une donnée manquante est une erreur nommée, jamais un zéro.
- Domaine sans `net/http`, sans base, sans SDK d'IA, sans horloge ; prix externes via un adapter.
- Handlers : décoder → service → encoder. Tests table-driven qui peuvent échouer. Logs `log/slog`.

## 5. Ce que l'agent ne fait jamais sans accord explicite

- pousser, merger, ouvrir une PR, réécrire l'historique git ;
- ajouter une dépendance ;
- modifier une migration déjà commitée, une ADR acceptée, ou `docs/features.md` ;
- désactiver, supprimer ou affaiblir un test (ou un job de CI) pour faire passer la pipeline ;
- toucher aux données d'un snapshot publié (même en base de dev).

`.claude/settings.json` bloque déjà les plus dangereux (push forcé, `main`, `.env`).

## 6. Boucle de travail

1. `make risk` sur l'intention ; niveau ≥ 2 → plan court soumis à l'humain **avant** le code.
   Décision d'architecture (`docs/agents/travail-avec-agents.md` §7) → ADR `Proposé`, on attend.
2. Un test qui protège l'existant, puis le changement (petits pas, un commit = une intention).
3. `make verify` (puis `LEVEL=pr` avant de pousser) jusqu'au vert.
4. Relecture IA selon le niveau (skill `revue-par-niveau`) : constats `Important` corrigés,
   `Nit` au choix, `Pré-existant` en ticket. **3 tours au plus**, ensuite on revoit l'approche.
5. Livrable `docs/agents/travail-avec-agents.md` §9 : ce que fait le changement, un cas de panne,
   le test qui le couvre, ce qui n'est pas vérifié. L'approbation d'un agent ne vaut jamais celle d'un humain.

## 7. Skills et relecteurs

| Skill (`.claude/skills/`) | Quand |
|---|---|
| `ajouter-feature` | nouvelle tranche verticale depuis `docs/features.md` |
| `changer-contrat-api` | route ou JSON `/api/v1` ajouté ou modifié |
| `ajouter-migration` | changement de schéma PostgreSQL |
| `rediger-adr` | décision coûteuse à défaire |
| `revue-par-niveau` | avant de déclarer terminé : lance les bons relecteurs |
| `trier-constat` | un relecteur, la CI ou un scanner signale quelque chose |
| `audit-securite` | audit de sécurité complet, en phases, constats vérifiés |

Relecteurs (`.claude/agents/`, lecture seule, contexte neuf) : `relecteur-code`, `relecteur-architecture`,
`relecteur-securite`, `relecteur-metier`. Échelle de sévérité commune : `REVIEW.md`.

## 8. Quand un humain manque

L'agent ne tranche pas à sa place : il écrit la question dans `.specs/<ticket>/state.md` (section
« Questions ouvertes ») avec sa recommandation et le coût d'un non, et continue sur ce qui n'en dépend pas.

## 9. Pour aller plus loin

- `docs/agents/regles.md` (§1–5) : environnement, architecture, règles métier, conventions, UI.
- `docs/agents/travail-avec-agents.md` (§6–16) : décisions, ADR, livrable, petits pas, équipe multi-IA, CI.
- `docs/harnais.md` : l'usine logicielle (capteurs, niveaux, CI, IA) et ses sources.
- `docs/workflow.md`, `docs/adr/`. Les renvois « AGENTS §N » du code pointent vers `docs/agents/`.
