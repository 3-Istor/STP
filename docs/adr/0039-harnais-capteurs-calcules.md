# 0039 — Harnais : capteurs calculés, niveaux de risque et chaîne d'approvisionnement
Statut : Proposé — Date : 2026-11-17 — Décideurs : (à compléter par un humain)

## Contexte
Les agents écrivent l'essentiel du code (V2 : 7 lots en parallèle). La CI d'ADR 0001 vérifie lint, tests
et build, mais rien ne dit **combien** un changement doit être relu, les règles d'architecture d'AGENTS
(domaine sans réseau, sans base, sans horloge, sans float) ne sont vérifiées que par relecture, le contrat
`/api/v1` peut perdre une route sans qu'aucun test ne casse, et aucune analyse de secrets, de SBOM ni de
mutation n'existe. Demande de l'équipe : une usine logicielle sur le modèle d'`expense-io` et `sdi-harness`.

**À trancher par l'humain (recommandation : oui pour les quatre)** :
1. Ajouter 5 outils **hors `go.mod`**, épinglés dans `scripts/tools.env` (gitleaks, osv-scanner, actionlint,
   gremlins, cyclonedx-gomod). Coût d'un non : pas de détection de secrets ni de SBOM ; la mutation reste à faire.
2. Épingler les actions GitHub par SHA (Dependabot les tient à jour). Coût d'un non : une étiquette
   déplacée chez un tiers exécute du code dans notre CI.
3. Niveau de risque 0–3 calculé par `cmd/risk` et un point de contrôle humain par niveau (AGENTS.md §1).
4. Seuil de mutation à 80 % sur `internal/money` (mesuré : 83,9 %), cliquet qui ne descend pas.

## Options
1. **Ne rien faire** — zéro coût ; la relecture humaine reste le seul filet, uniforme quel que soit le risque.
2. **Plateforme externe** (SonarCloud, Snyk, CodeRabbit) — riche ; compte tiers, code envoyé dehors,
   règles métier du projet inconnues de l'outil.
3. **Capteurs calculés dans le dépôt** — tests Go (`internal/archtest`, `TestRoutesContract`,
   `TestRulesMatchAgentsDoc`), `cmd/risk`, `scripts/verify.sh fast|pr|nightly` identique en local et en CI,
   outils open source épinglés ; tout est lisible et modifiable par l'équipe, sans compte externe.

## Décision
Option 3 : les règles qui comptent sont propres au projet (montants, snapshots, frontières), un test Go les
vérifie mieux qu'un outil générique, et le même script tourne sur le poste et en CI.

## Conséquences
- Facile : savoir quoi relire (`make risk`), attraper une violation d'architecture en 0,2 s, prouver qu'aucun
  secret n'est dans l'historique, livrer un SBOM.
- Difficile : la table des niveaux doit suivre le code (un test le garantit pour AGENTS.md) ; 5 outils à monter
  de version (Dependabot ne voit pas `tools.env` : revue manuelle mensuelle).
- Retour arrière : supprimer les jobs `risk`/`supply-chain`, `nightly.yml` et `scripts/tools.env` ; les tests
  Go restent utiles seuls.
- Pivot : multi-source de prix ou carbone versionné = nouveaux domaines ajoutés à `domains` d'`archtest` et,
  si le calcul est critique, à la table de `cmd/risk`. Hors-ligne : sans effet.
